package model

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/components/aboutmodal"
	"github.com/filipemolina/farol/src/components/confirmmodal"
	"github.com/filipemolina/farol/src/components/helpoverlay"
	"github.com/filipemolina/farol/src/components/importexportmodal"
	"github.com/filipemolina/farol/src/components/listnamemodal"
	"github.com/filipemolina/farol/src/components/searchpicker"
	"github.com/filipemolina/farol/src/components/themepickermodal"
	"github.com/filipemolina/farol/src/config"
	"github.com/filipemolina/farol/src/constants"
	"github.com/filipemolina/farol/src/keys"
	"github.com/filipemolina/farol/src/store"
)

// Update handles every message. The shape mirrors stack-stitcher's: ctrl+c
// quits ahead of every other claim on the keyboard; a modal owns all key
// input while it is open; then AppModel's own key handling, the layout
// messages, and finally every message is forwarded to the zones (each
// component ignores what it does not answer to).
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var finalCmds []tea.Cmd

	// ctrl+c quits from anywhere, ahead of every other claim on the
	// keyboard: a modal, a text input, anything phase 5 adds. It is the one
	// key nothing gets to swallow, and the only binding that leaves the app.
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && key.Matches(keyMsg, keys.Global.ForceQuit) {
		return m, tea.Quit
	}

	// While a modal is open, it owns all input exclusively — the zones
	// and the global keys are frozen until it closes. Only the messages the
	// modal can act on are routed to it: keypresses (its buttons, inputs and
	// esc/enter) and terminal pastes (a focused textarea inside the modal).
	// Everything else — CloseModalMsg above all, but also refresh, poll and
	// layout messages — must fall through to AppModel's own handling;
	// forwarding them to the modal too would deadlock closing (the modals
	// only answer to KeyPressMsg/PasteMsg, so CloseModalMsg would be
	// swallowed and no modal could ever close).
	if m.activeModal != nil {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.PasteMsg:
			var modalCmd tea.Cmd
			m.activeModal, modalCmd = m.activeModal.Update(msg)
			return m, modalCmd
		}
	}

	// While the Details side panel is visible it owns every keypress except
	// ctrl+c (returned above): its own bindings (ctrl+s, tab, ←/→, esc, and
	// the discard prompt) act, and no global key or tree navigation does. It
	// is a body surface rather than a modal, so it sits just after the modal
	// check and before AppModel's own esc/tab/global switch. Only keypresses
	// are captured here; refresh, poll, and layout messages still fall through
	// to the normal handlers and the component fan-out (docs/DESIGN.md §5).
	if m.detailsPanelVisible {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			var detailsCmd tea.Cmd
			m.components.DetailsPanel, detailsCmd = m.components.DetailsPanel.Update(msg)
			return m, detailsCmd
		}
	}

	// While the Archive page is visible it owns every keypress except ctrl+c
	// (returned above), the same way the Details side panel does just above —
	// its own esc closes it, and no global key or tree navigation acts. Unlike
	// Details it is a full-body page, not a modal, but the keyboard-ownership
	// rule is identical, so it sits right after the Details check. Only
	// keypresses are captured here; refresh, poll, and layout messages still
	// fall through to the normal handlers and the component fan-out.
	if m.archivePageVisible {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			var archiveCmd tea.Cmd
			m.components.ArchivePage, archiveCmd = m.components.ArchivePage.Update(msg)
			return m, archiveCmd
		}
	}

	// keyboardOwned reports whether a focused child component has claimed the
	// keyboard for itself (add input with text, tree typing a /-filter, list
	// typing a filter). While true, global keys that would open overlays or
	// toggle panels are suppressed so typing is not interrupted. tab/shift+tab
	// focus navigation is suppressed only while the inline create input is
	// live (creating a task focuses only the text input); while a /-filter is
	// being typed, tab still cycles the panels.
	keyboardOwned := func() bool {
		switch m.focusedZone {
		case constants.COMPONENT_TASK_TREE:
			if tasks, ok := m.components.TaskPanel.(interface{ OwnsKeyboard() bool }); ok {
				return tasks.OwnsKeyboard()
			}
		case constants.COMPONENT_LISTS_PANEL:
			if lists, ok := m.components.ListsPanel.(interface{ OwnsKeyboard() bool }); ok {
				return lists.OwnsKeyboard()
			}
		}
		return false
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keys.Global.Back):
			// Esc ladder (docs/DESIGN.md §5, "esc is the most overloaded key"):
			// a modal closes itself first — it intercepts all keypresses at the
			// top of Update, so by the time we reach here no modal is open.
			// Next, a focused child that declared KeepsEsc (tree with applied
			// filter or inline create; lists panel with active filter) claims
			// esc for itself. Do not consume it here — let the component's own
			// Update handle it. When no child claims it, the Lists panel (if
			// that is what's focused) closes as esc's cancel of the transient
			// picker, the same "commit or cancel closes it" contract enter's
			// Lists.Select case commits. Anything else is a no-op.
			claimed := false
			switch m.focusedZone {
			case constants.COMPONENT_TASK_TREE:
				if tasks, ok := m.components.TaskPanel.(interface{ KeepsEsc() bool }); ok && tasks.KeepsEsc() {
					claimed = true
				}
			case constants.COMPONENT_LISTS_PANEL:
				if lists, ok := m.components.ListsPanel.(interface{ KeepsEsc() bool }); ok && lists.KeepsEsc() {
					claimed = true
				}
			}
			if !claimed {
				if m.focusedZone == constants.COMPONENT_LISTS_PANEL && m.listsPanelVisible {
					finalCmds = append(finalCmds, m.closeListsPanel())
					break
				}
				return m, nil
			}

		// q is the ordinary way out; ctrl+c above is the unconditional one.
		// Because q is a printable character it has to yield to everything
		// that could be typing one, and by this point most of that has
		// already returned: a modal and the Details panel both intercept
		// every keypress above. keyboardOwned() covers the rest — the inline
		// create row and a /-filter being typed in either panel — and when it
		// is true the case falls through to the component fan-out below, so
		// the input still receives a literal "q" (docs/DESIGN.md §5).
		case key.Matches(msg, keys.Global.Quit):
			if !keyboardOwned() {
				switch m.focusedZone {
				case constants.COMPONENT_TASK_TREE, constants.COMPONENT_LISTS_PANEL:
					return m, tea.Quit
				}
			}

		case key.Matches(msg, keys.Global.Help):
			// ? is a printable character: while an input owns the keyboard it
			// must land in the input, not open the overlay.
			if !keyboardOwned() {
				finalCmds = append(finalCmds, cmds.OpenHelpModal())
			}

		case key.Matches(msg, keys.Global.Theme):
			if !keyboardOwned() {
				finalCmds = append(finalCmds, cmds.OpenThemePicker())
			}

		case key.Matches(msg, keys.Global.About):
			if !keyboardOwned() {
				finalCmds = append(finalCmds, cmds.OpenAboutModal())
			}

		case key.Matches(msg, keys.Global.ArchivePage):
			if !keyboardOwned() {
				finalCmds = append(finalCmds, cmds.OpenArchivePage())
			}

		// / enters a local filter: the task tree's fuzzy filter when the tree
		// is focused, the lists panel's filter when the lists panel is.
		// F opens the cross-list picker. Both are global keys — they work
		// whenever no modal owns the keyboard, and the filter's target
		// follows focus (docs/DESIGN.md §5).
		case key.Matches(msg, keys.Global.Filter):
			if !keyboardOwned() {
				if m.focusedZone == constants.COMPONENT_LISTS_PANEL {
					finalCmds = append(finalCmds, cmds.ActivateListFilter())
				} else {
					finalCmds = append(finalCmds, cmds.ActivateFilter())
				}
			}

		case key.Matches(msg, keys.Global.Picker):
			if !keyboardOwned() {
				finalCmds = append(finalCmds, cmds.OpenSearchPicker())
			}

		// tab/shift+tab are focus keys and cycle the panels — except while the
		// inline create input is live: creating a task focuses only the text
		// input, so tab must not carry the cursor (and the half-typed title)
		// off to another panel mid-entry. The /-filter keeps the old behavior:
		// tab still cycles while filtering. AppModel routes tab before the
		// tree's allowlist runs, so the input never sees it as a character; a
		// suppressed tab falls through to the tree, whose allowlist swallows it.
		case key.Matches(msg, keys.Global.NextPanel):
			if !m.createInputLive() {
				finalCmds = append(finalCmds, m.ChangeFocus(1))
			}

		case key.Matches(msg, keys.Global.PrevPanel):
			if !m.createInputLive() {
				finalCmds = append(finalCmds, m.ChangeFocus(-1))
			}

		case key.Matches(msg, keys.Global.ToggleListsPanel):
			if !keyboardOwned() {
				m.listsPanelVisible = !m.listsPanelVisible
				m.bodyLayout = m.calculateBodyLayout()
				finalCmds = append(finalCmds, m.broadcastBodyLayout(), m.footerContextCmd())
				// A panel leaving the layout cannot keep the focus: fall back
				// to the task tree. Opening Lists makes it the active surface.
				if m.listsPanelVisible {
					finalCmds = append(finalCmds, m.ChangeFocus(1))
				} else if m.focusedZone == constants.COMPONENT_LISTS_PANEL {
					finalCmds = append(finalCmds, m.ChangeFocus(1))
				}
			}

		case key.Matches(msg, keys.Global.CopyID):
			if !keyboardOwned() {
				var idToCopy string
				if m.focusedZone == constants.COMPONENT_LISTS_PANEL {
					idToCopy = m.highlightedListID()
				} else {
					if tasks, ok := m.components.TaskPanel.(interface{ SelectedID() string }); ok {
						idToCopy = tasks.SelectedID()
					}
				}
				if idToCopy != "" {
					finalCmds = append(finalCmds, tea.SetClipboard(idToCopy))
				}
			}

		case key.Matches(msg, keys.Global.Sort):
			if !keyboardOwned() {
				finalCmds = append(finalCmds, cmds.CycleSortMode())
			}
		// the Tasks panel to it (docs/DESIGN.md §5), so there is nothing to
		// write — just close the transient picker and hand focus to Tasks.
		// Guarded on !keyboardOwned() so a filter being typed keeps enter for
		// itself (list.KeyMap's AcceptWhileFiltering) rather than this case
		// stealing it — the raw keypress still reaches the list unconsumed via
		// the component fan-out below.
		case m.listsPanelVisible && m.focusedZone == constants.COMPONENT_LISTS_PANEL && !keyboardOwned() && key.Matches(msg, keys.Lists.Select):
			finalCmds = append(finalCmds, m.closeListsPanel())

		// List CRUD keys: only active when lists panel is visible and focused,
		// and never while the panel's /-filter is being typed — n, R, d, e and
		// i are all printable, so while the filter input owns the keyboard they
		// are query characters, not commands (the same guard Lists.Select above
		// uses to keep enter for the filter). The suppressed keypress falls
		// through to the component fan-out, where the list's own filter input
		// consumes it. Both rename and delete act on the panel's highlighted
		// list, not on the list open in the tasks panel: the two diverge
		// whenever the active list changes without the panel cursor moving (the
		// global picker jumping to another list, or a delete ahead of the
		// cursor).
		case m.listsPanelVisible && m.focusedZone == constants.COMPONENT_LISTS_PANEL && !keyboardOwned() && key.Matches(msg, keys.Lists.New):
			m.activeModal = listnamemodal.New(listnamemodal.ModeNew, "", m.store)

		case m.listsPanelVisible && m.focusedZone == constants.COMPONENT_LISTS_PANEL && !keyboardOwned() && key.Matches(msg, keys.Lists.Rename):
			if target := m.highlightedListID(); target != "" {
				m.activeModal = listnamemodal.New(listnamemodal.ModeRename, target, m.store)
			}

		case m.listsPanelVisible && m.focusedZone == constants.COMPONENT_LISTS_PANEL && !keyboardOwned() && key.Matches(msg, keys.Lists.Delete):
			if target := m.highlightedListID(); target != "" {
				// Name the list and its task count so d (bound to both panels,
				// with no undo anywhere) cannot wipe a list the user mistook for
				// a task. Fall back to the generic string on any store error.
				body := "Are you sure? This will delete every task in the list."
				if l, err := m.store.GetList(target); err == nil {
					total := 0
					if summaries, err := m.store.ListLists(); err == nil {
						for _, s := range summaries {
							if s.List.ID == target {
								total = s.PendingCount + s.CompleteCount
								break
							}
						}
					}
					body = fmt.Sprintf("Delete %q and its %d tasks? This cannot be undone.", l.Name, total)
				}
				m.activeModal = confirmmodal.New("Delete list", body, func() tea.Msg {
					if err := m.store.DeleteList(target); err != nil {
						// Report through the same channel a failed refresh uses
						// (the RefreshListsMsg handler records it in lastError)
						// instead of swallowing it like the old nil return did.
						return cmds.RefreshListsMsg{Err: err}
					}
					return cmds.RefreshLists(m.store)()
				})
			}

		// Export / import open the matching modal. They act on (or add to) the
		// lists panel, so they share the focused-Lists-panel guard the other
		// list CRUD keys use (docs/DESIGN.md §9, export/import).
		case m.listsPanelVisible && m.focusedZone == constants.COMPONENT_LISTS_PANEL && !keyboardOwned() && key.Matches(msg, keys.Lists.Export):
			m.activeModal = importexportmodal.NewExport(m.store, m.highlightedListIDptr(), m.terminalWidth)

		case m.listsPanelVisible && m.focusedZone == constants.COMPONENT_LISTS_PANEL && !keyboardOwned() && key.Matches(msg, keys.Lists.Import):
			m.activeModal = importexportmodal.NewImport(m.store, m.terminalWidth)

		}

	// This is executed once when the app loads and after every resize.
	// Components never see it — only the layout message below.
	case tea.WindowSizeMsg:
		m.terminalWidth = msg.Width
		m.terminalHeight = msg.Height
		// One-time startup policy: seed the Lists preference from terminal
		// width before the first layout — open at AUTO_SHOW_LISTS_MIN_WIDTH or
		// wider, hidden below it. Never re-applied, so a later resize can't
		// reverse a user's L toggle (docs/DESIGN.md §5). Focus stays on Tasks —
		// showing Lists by width is not an instruction to focus it.
		if !m.layoutInitialized {
			m.layoutInitialized = true
			m.listsPanelVisible = msg.Width >= constants.AUTO_SHOW_LISTS_MIN_WIDTH
		}
		m.bodyLayout = m.calculateBodyLayout()
		// If a resize just took a rendered Lists panel out from under the focus
		// (too narrow, or width returned but focus never left), pull focus back
		// to Tasks. The stored preference is untouched, so Lists reappears on a
		// later resize if it is still on.
		if m.focusedZone == constants.COMPONENT_LISTS_PANEL && !m.listsPanelRendered() {
			m.focusedZone = constants.COMPONENT_TASK_TREE
			finalCmds = append(finalCmds, cmds.SetFocus(constants.COMPONENT_TASK_TREE))
		}
		finalCmds = append(finalCmds, m.broadcastBodyLayout(), m.footerContextCmd())
		// Keep the open Details modal sized to the resized terminal (it is
		// layered over the body, so it is sized from the terminal directly
		// rather than from the body split).
		if m.detailsPanelVisible {
			mw, mh := m.detailsModalSize()
			finalCmds = append(finalCmds, cmds.SetDetailsLayout(mw, mh))
		}

	// The poll tick re-issues itself here, which is what makes the poll
	// recurring for the life of the app (docs/DESIGN.md §7).
	case cmds.PollTickMsg:
		finalCmds = append(finalCmds, cmds.PollTick(config.PollInterval(m.cfg)))
		finalCmds = append(finalCmds, cmds.RefreshLists(m.store))
		if m.activeListID != "" {
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID, m.sortMode))
		}
		// Keep the open Details panel current with external CLI writes; a
		// response only replaces a clean editor (docs/DESIGN.md §5).
		if m.detailsPanelVisible && m.detailsTaskID != "" {
			finalCmds = append(finalCmds, cmds.RefreshDetails(m.store, m.detailsTaskID))
		}
		// Keep the open Archive page current with external CLI archive/
		// unarchive writes, the same live-refresh contract every other open
		// surface gets (docs/DESIGN.md §7).
		if m.archivePageVisible {
			finalCmds = append(finalCmds, cmds.RefreshArchivedLists(m.store))
		}

	case cmds.AnimTickMsg:
		m.animFrame = (m.animFrame + 1) % 8
		finalCmds = append(finalCmds, cmds.SetAnimFrame(m.animFrame))
		if m.animActive {
			finalCmds = append(finalCmds, cmds.AnimTick(cmds.AnimInterval))
		}

	case cmds.RefreshListsMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			break
		}
		// A clean (error-free) refresh clears any prior error, so a failed
		// export/import does not leave its message stale on screen after a
		// subsequent success (docs/DESIGN.md §9, lastError lifecycle).
		m.lastError = ""
		m.lists = msg.Lists
		wasActive := m.animActive
		m.animActive = len(msg.Activities) > 0
		if m.animActive && !wasActive {
			finalCmds = append(finalCmds, cmds.AnimTick(cmds.AnimInterval))
		}
		if m.activeListID == "" {
			if len(msg.Lists) > 0 {
				// Startup: reopen the list the user last had active, when it
				// still exists, else fall back to the first list
				// (docs/DESIGN.md §7). GetSetting returns "" for a fresh
				// store, so a first run is the fallback too.
				m.activeListID = msg.Lists[0].List.ID
				if saved, err := m.store.GetSetting(store.KeyLastListID); err == nil && saved != "" {
					for _, l := range msg.Lists {
						if l.List.ID == saved {
							m.activeListID = saved
							break
						}
					}
				}
				finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID, m.sortMode))
				// Align the lists panel's highlight with the active list we
				// just reopened (docs/DESIGN.md §7). The panel cannot know the
				// saved last-active list on its own, so it would otherwise
				// highlight index 0 and disagree with the Tasks panel.
				finalCmds = append(finalCmds, cmds.SelectListInPanel(m.activeListID))
				m.persistActiveList()
			} else if m.store != nil {
				if id, err := m.store.CreateList(constants.DEFAULT_LIST_NAME, ""); err == nil {
					m.activeListID = id
					m.persistActiveList()
					finalCmds = append(finalCmds, cmds.RefreshLists(m.store))
				}
			}
		} else {
			// If the active list was deleted (e.g. by a test or the user),
			// fall back to the first remaining list, or create a new one.
			found := false
			for _, l := range msg.Lists {
				if l.List.ID == m.activeListID {
					found = true
					break
				}
			}
			if !found {
				if len(msg.Lists) > 0 {
					m.activeListID = msg.Lists[0].List.ID
					finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID, m.sortMode))
					m.persistActiveList()
				} else if m.store != nil {
					if id, err := m.store.CreateList(constants.DEFAULT_LIST_NAME, ""); err == nil {
						m.activeListID = id
						m.persistActiveList()
						finalCmds = append(finalCmds, cmds.RefreshLists(m.store))
					}
				}
			}
		}
		finalCmds = append(finalCmds, m.footerContextCmd())

	case cmds.RefreshTasksMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			break
		}
		m.lastError = ""
		wasActive := m.animActive
		m.animActive = len(msg.Activities) > 0
		if m.animActive && !wasActive {
			finalCmds = append(finalCmds, cmds.AnimTick(cmds.AnimInterval))
		}
		if draftCmd := m.applyCreateDraft(msg.Rows); draftCmd != nil {
			finalCmds = append(finalCmds, draftCmd)
		}
		// Restore collapsed state for this list (if any).
		if msg.ListID != "" && m.store != nil {
			if collapsed, err := m.store.GetCollapsedTasks(msg.ListID); err == nil {
				if tasks, ok := m.components.TaskPanel.(interface{ SetCollapsed(map[string]bool) }); ok {
					tasks.SetCollapsed(collapsed)
				}
			}
		}
		finalCmds = append(finalCmds, m.footerContextCmd())

	case cmds.OpenHelpModalMsg:
		m.activeModal = helpoverlay.New(m.helpContext(), m.terminalWidth, m.terminalHeight)

	case cmds.OpenThemePickerMsg:
		m.activeModal = themepickermodal.New(m.terminalHeight)

	case cmds.OpenAboutModalMsg:
		m.activeModal = aboutmodal.New(m.terminalWidth)

	case cmds.OpenSearchPickerMsg:
		m.activeModal = searchpicker.New(m.store, m.terminalHeight)

	case cmds.OpenExportModalMsg:
		m.activeModal = importexportmodal.NewExport(m.store, m.highlightedListIDptr(), m.terminalWidth)

	case cmds.OpenImportModalMsg:
		m.activeModal = importexportmodal.NewImport(m.store, m.terminalWidth)

	// The global picker jumped to a task, possibly in another list: switch
	// the active list to the result's list (when different) and move the tree
	// selection to the task. SelectTask is sent unconditionally — if the task
	// is already in rows it selects immediately, and if the list just changed
	// the task tree's pending-select lands it once those rows arrive.
	case cmds.JumpToTaskMsg:
		if m.activeListID != msg.ListID {
			m.activeListID = msg.ListID
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID, m.sortMode))
			m.persistActiveList()
		}
		finalCmds = append(finalCmds, cmds.SelectTask(msg.TaskID))

	case cmds.OpenDetailsMsg:
		// Enter on a selected task opens the Task details modal: a centered
		// surface layered over the page (not a body surface — it no longer
		// competes with Lists for the row, docs/DESIGN.md §5). It takes focus so
		// its keys own the keyboard while it is up, is sized to most of the
		// screen, and requests its task's current notes/progress and comments.
		if msg.TaskID != "" {
			m.detailsTaskID = msg.TaskID
			m.detailsPanelVisible = true
			m.focusedZone = constants.COMPONENT_DETAILS_PANEL
			mw, mh := m.detailsModalSize()
			finalCmds = append(finalCmds,
				cmds.SetDetailsLayout(mw, mh),
				cmds.SetFocus(constants.COMPONENT_DETAILS_PANEL),
				m.footerContextCmd(),
				cmds.RefreshDetails(m.store, msg.TaskID),
			)
		}

	case cmds.CloseDetailsSideMsg:
		// The Details modal asked to close (clean Esc, discarded edit, or a
		// completed save). Only AppModel changes visibility and focus: hide the
		// modal, return focus to the task tree, then run the save's refresh
		// follow-up if there is one.
		m.detailsPanelVisible = false
		m.detailsTaskID = ""
		m.focusedZone = constants.COMPONENT_TASK_TREE
		finalCmds = append(finalCmds,
			cmds.SetFocus(constants.COMPONENT_TASK_TREE),
			m.footerContextCmd(),
		)
		if msg.Follow != nil {
			finalCmds = append(finalCmds, msg.Follow)
		}

	case cmds.OpenArchivePageMsg:
		// The archive-page toggle key emits this. Entering follows the same
		// shape as OpenDetailsMsg: take the keyboard, move focus onto the new
		// surface, refresh the footer for it, and load the archived set —
		// AppModel drives when this query runs (open, and every poll tick
		// below while the page stays open), the same way it drives
		// RefreshLists/RefreshTasks; the component only renders what arrives.
		m.archivePageVisible = true
		m.focusedZone = constants.COMPONENT_ARCHIVE_PAGE
		finalCmds = append(finalCmds,
			cmds.SetFocus(constants.COMPONENT_ARCHIVE_PAGE),
			m.footerContextCmd(),
			cmds.RefreshArchivedLists(m.store),
		)

	case cmds.CloseArchivePageMsg:
		// The Archive page asked to close (esc). Only AppModel changes
		// visibility and focus, mirroring CloseDetailsSideMsg: hide the page,
		// return focus to the task tree, then run any follow-up command.
		m.archivePageVisible = false
		m.focusedZone = constants.COMPONENT_TASK_TREE
		finalCmds = append(finalCmds,
			cmds.SetFocus(constants.COMPONENT_TASK_TREE),
			m.footerContextCmd(),
		)
		if msg.Follow != nil {
			finalCmds = append(finalCmds, msg.Follow)
		}

	case cmds.CloseModalMsg:
		m.activeModal = nil
		if msg.Follow != nil {
			finalCmds = append(finalCmds, msg.Follow)
		}

	case cmds.ThemeAppliedMsg:
		// The whole UI already repainted live while the picker was open; a
		// persist failure is the only thing left to report.
		m.lastError = ""
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
		}

	case cmds.CycleSortModeMsg:
		// Cycle to the next sort mode and refresh the task tree.
		m.sortMode = m.sortMode.Next()
		if m.activeListID != "" {
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID, m.sortMode))
		}

	case cmds.CreateTaskFromInputMsg:
		// The task tree's inline input submitted a draft. Don't create yet:
		// resolve the insertion against the freshest rows on the next
		// RefreshTasksMsg, so a poll or delete during typing can't anchor the
		// new task to a stale selection.
		m.createDraft = &msg
		if m.activeListID != "" {
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID, m.sortMode))
		}

	case cmds.DeleteTaskMsg:
		// The tree's d binding emitted this (it owns the keypress); route it
		// through the same confirm modal pattern as list delete (docs/DESIGN.md
		// §9: destructive ops need confirmation in the TUI).
		if msg.TaskID != "" {
			taskID := msg.TaskID
			// Name the task and count its descendants from the rows the
			// tasks panel already holds (depth-first preorder), so the dialog
			// says what is about to be destroyed. Fall back to the generic
			// string on any store error.
			body := "Are you sure?"
			if title, err := m.store.GetTask(taskID); err == nil {
				count := 0
				if tasks, ok := m.components.TaskPanel.(interface{ Rows() []apptypes.Row }); ok {
					count = descendantCount(tasks.Rows(), taskID)
				}
				if count == 0 {
					body = fmt.Sprintf("Delete %q? This cannot be undone.", title.Title)
				} else {
					sub := "subtasks"
					if count == 1 {
						sub = "subtask"
					}
					body = fmt.Sprintf("Delete %q and its %d %s? This cannot be undone.", title.Title, count, sub)
				}
			}
			m.activeModal = confirmmodal.New("Delete task", body, func() tea.Msg {
				if err := m.store.DeleteTask(taskID); err != nil {
					return nil
				}
				return cmds.RefreshTasks(m.store, m.activeListID, m.sortMode)()
			})
		}

	case cmds.DeleteCommentMsg:
		// The Details modal's d binding emitted this (handleCommentsKey owns
		// it); route it through the same confirm modal pattern as task and
		// list delete. The dialog quotes the comment's own text — the same
		// "name what you're about to destroy" fix the Bugs list's delete-dialog
		// task applied everywhere else — so d never wipes the wrong comment.
		if msg.CommentID != "" {
			commentID, taskID := msg.CommentID, msg.TaskID
			body := fmt.Sprintf("Delete this comment? This cannot be undone.\n\n%q", msg.Note)
			m.activeModal = confirmmodal.New("Delete comment", body, func() tea.Msg {
				if err := m.store.DeleteComment(commentID); err != nil {
					return nil
				}
				return cmds.RefreshDetails(m.store, taskID)()
			})
		}

	case cmds.DeleteAttachmentMsg:
		// The Details modal's d binding emitted this (handleAttachmentsKey owns
		// it); route it through the same confirm modal pattern as task and
		// comment delete. The dialog quotes the attachment's path — the same
		// "name what you're about to destroy" pattern — so d never wipes the
		// wrong attachment.
		if msg.AttachmentID != "" {
			attachmentID, taskID := msg.AttachmentID, msg.TaskID
			body := fmt.Sprintf("Delete this attachment? This cannot be undone.\n\n%q", msg.Path)
			m.activeModal = confirmmodal.New("Delete attachment", body, func() tea.Msg {
				if err := m.store.DeleteAttachment(attachmentID); err != nil {
					return nil
				}
				return cmds.RefreshDetails(m.store, taskID)()
			})
		}

	case cmds.ToggleTaskMsg:
		if err := m.store.Toggle(msg.TaskID); err != nil {
			m.lastError = err.Error()
			break
		}
		if m.activeListID != "" {
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID, m.sortMode))
		}

	case cmds.CollapsedStateChangedMsg:
		// Persist the collapsed state for the current list.
		if msg.ListID != "" && m.store != nil {
			if err := m.store.SetCollapsedTasks(msg.ListID, msg.Collapsed); err != nil {
				m.lastError = err.Error()
			}
		}

	case cmds.UnassignTaskMsg:
		// The tree's u binding. The release is forced: this is the human's
		// escape hatch for an abandoned grab, and a stale assignment is by
		// definition held by an agent that is not the person at the keyboard.
		// force makes the holder tag irrelevant, which is why "" is passed for
		// it rather than the TUI inventing an agent identity of its own
		// (decision 2).
		if msg.TaskID != "" {
			if err := m.store.UnassignTask(msg.TaskID, "", true); err != nil {
				m.lastError = err.Error()
				break
			}
			if m.activeListID != "" {
				finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID, m.sortMode))
			}
		}

	case cmds.ReleaseListMsg:
		// The tree's U binding. Unlike u this can free work several agents are
		// holding at once, so it goes through the same confirm modal every
		// other bulk or destructive TUI action uses (docs/DESIGN.md §9), and
		// the dialog counts what is about to be released — read from the rows
		// the tasks panel already holds, not a second store query.
		if msg.ListID != "" {
			listID := msg.ListID
			held := 0
			if tasks, ok := m.components.TaskPanel.(interface{ Rows() []apptypes.Row }); ok {
				held = assignedCount(tasks.Rows())
			}
			body := "Release every assignment in this list? Nothing else about the tasks changes."
			if held > 0 {
				unit := "assignments"
				if held == 1 {
					unit = "assignment"
				}
				body = fmt.Sprintf("Release %d %s in this list? Nothing else about the tasks changes.", held, unit)
			}
			m.activeModal = confirmmodal.New("Release list", body, func() tea.Msg {
				if _, err := m.store.UnassignList(listID); err != nil {
					return nil
				}
				return cmds.RefreshTasks(m.store, listID, m.sortMode)()
			})
		}

	case cmds.MoveTaskMsg:
		if err := m.store.MoveTask(msg.TaskID, msg.AfterID); err != nil {
			m.lastError = err.Error()
			break
		}
		if m.activeListID != "" {
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID, m.sortMode))
		}

	case cmds.ReparentTaskMsg:
		if err := m.store.Reparent(msg.TaskID, msg.ParentID); err != nil {
			m.lastError = err.Error()
			break
		}
		if m.activeListID != "" {
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID, m.sortMode))
		}

	case cmds.SelectListMsg:
		// Lists panel selected a different list; switch to it immediately
		// rather than waiting for the next poll tick.
		if m.activeListID != msg.ListID {
			m.activeListID = msg.ListID
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID, m.sortMode))
			m.persistActiveList()
		}

	case cmds.ListCreatedMsg:
		// listnamemodal's ModeNew follow: land on the new list and close the
		// transient picker, in that order (docs/DESIGN.md §5) — closing first
		// would leave Tasks showing the previous list while the brand-new one
		// silently became active underneath it. activeListID is set here,
		// synchronously, before RefreshLists is even scheduled, so whichever
		// order the batched refresh commands actually complete in, the lists
		// refresh's "is the active list still present" check already agrees
		// with this one.
		if msg.ID != "" {
			m.activeListID = msg.ID
			m.persistActiveList()
			finalCmds = append(finalCmds,
				cmds.RefreshLists(m.store),
				cmds.RefreshTasks(m.store, msg.ID, m.sortMode),
				m.closeListsPanel(),
			)
		}
	}

	// Forward the message to every component. TaskPanel forwards to the tree
	// and input controls after deriving their shared Tasks-surface state.
	var menuCmd, barCmd, listsCmd, tasksCmd, detailsCmd, archiveCmd tea.Cmd
	m.components.MainMenu, menuCmd = m.components.MainMenu.Update(msg)
	m.components.KeybindingBar, barCmd = m.components.KeybindingBar.Update(msg)
	m.components.ListsPanel, listsCmd = m.components.ListsPanel.Update(msg)
	m.components.TaskPanel, tasksCmd = m.components.TaskPanel.Update(msg)
	m.components.DetailsPanel, detailsCmd = m.components.DetailsPanel.Update(msg)
	m.components.ArchivePage, archiveCmd = m.components.ArchivePage.Update(msg)
	finalCmds = append(finalCmds, menuCmd, barCmd, listsCmd, tasksCmd, detailsCmd, archiveCmd)

	return m, tea.Batch(finalCmds...)
}

// persistActiveList records the active list id in the Setting table so the
// next launch reopens the same list (docs/DESIGN.md §7). It is called only
// where activeListID actually changes, never from the poll path: every poll
// must stay a read (docs/DESIGN.md §7/§8), and a 1s-tick UPSERT would turn
// the TUI into a writer on every tick.
func (m *AppModel) persistActiveList() {
	if m.store == nil || m.activeListID == "" {
		return
	}
	if err := m.store.SetSetting(store.KeyLastListID, m.activeListID); err != nil {
		m.lastError = err.Error()
	}
}

// calculateBodyLayout returns the exact box each body zone must render
// into. Width: ListsWidth + BODY_GUTTER_WIDTH + MainWidth == the terminal
// width when the sidebar is visible. Height is the remaining rows after the
// header and footer; TaskPanel owns its internal one-row input footer.
//
// The lists panel gets LEFT_PANEL_WIDTH of the row (after the gutter is
// taken out) and the main panel gets whatever is left, so rounding can
// never make the two panels overflow or leave a ragged column. Both panels
// are held at MIN_PANEL_WIDTH where the terminal allows it; below that the
// row is split evenly. A terminal too narrow for any sidebar at all
// (guttered < MIN_PANEL_WIDTH) gives the whole row to the main panel — the
// lists panel yields rather than rendering at a degenerate width, and L
// still brings it back when the terminal grows.
// terminalTooSmall reports whether the terminal is below the minimum size the
// app supports (constants.MIN_TERMINAL_WIDTH x MIN_TERMINAL_HEIGHT). It is the
// single predicate for that decision — View renders the "Terminal too small"
// line from it rather than each surface deciding for itself (docs/DESIGN.md
// §12). It answers false until the first WindowSizeMsg has arrived, so the
// pre-layout frame (width and height still 0) is not mistaken for a tiny
// terminal.
func (m AppModel) terminalTooSmall() bool {
	if !m.layoutInitialized {
		return false
	}
	return m.terminalWidth < constants.MIN_TERMINAL_WIDTH ||
		m.terminalHeight < constants.MIN_TERMINAL_HEIGHT
}

func (m AppModel) calculateBodyLayout() cmds.SetBodyLayoutMsg {
	height := max(0, m.terminalHeight-constants.HEADER_HEIGHT-constants.FOOTER_HEIGHT)
	available := max(0, m.terminalWidth)

	// Details is a modal now, layered over the body — it no longer takes a
	// column here (docs/DESIGN.md §5). The only body side surface is Lists; with
	// it hidden, Tasks fills the row.
	if !m.listsPanelVisible {
		return cmds.SetBodyLayoutMsg{
			Height:        height,
			MainWidth:     available,
			TerminalWidth: available,
		}
	}

	guttered := available - constants.BODY_GUTTER_WIDTH
	var sideWidth, mainWidth int
	switch {
	case guttered < constants.MIN_PANEL_WIDTH:
		// Too narrow to seat Lists next to Tasks: it yields the row to Tasks and
		// returns on a later resize.
		sideWidth, mainWidth = 0, available
	case guttered < 2*constants.MIN_PANEL_WIDTH:
		sideWidth = guttered / 2
		mainWidth = guttered - sideWidth
	default:
		sideWidth = int(float32(guttered) * constants.LEFT_PANEL_WIDTH)
		sideWidth = max(sideWidth, constants.MIN_PANEL_WIDTH)
		sideWidth = min(sideWidth, guttered-constants.MIN_PANEL_WIDTH)
		mainWidth = guttered - sideWidth
	}

	return cmds.SetBodyLayoutMsg{
		Height:        height,
		ListsWidth:    sideWidth,
		MainWidth:     mainWidth,
		TerminalWidth: available,
	}
}

// detailsModalSize is the Task details modal's outer box: about 90% of the
// terminal on each axis ("most of the screen", with a thin margin so the dimmed
// body still shows around the edge). AppModel computes it on open and on resize
// and hands it to the component via SetDetailsLayout — the modal is sized from
// the terminal directly because it is layered over the body, not seated in it.
func (m AppModel) detailsModalSize() (int, int) {
	return m.terminalWidth * 9 / 10, m.terminalHeight * 9 / 10
}

// focusableZones is the computed focus cycle (docs/DESIGN.md §5, step 4 of
// the phase-3 plan): the task tree always, the lists panel only while it is
// visible. Inline creation lives inside the tree, so there is no separate add
// input zone to cycle to — a static slice could not express the lists panel
// entering and leaving the cycle at runtime.
func (m AppModel) focusableZones() []int {
	zones := []int{constants.COMPONENT_TASK_TREE}
	if m.listsPanelRendered() {
		zones = append(zones, constants.COMPONENT_LISTS_PANEL)
	}
	return zones
}

// highlightedListID returns the id of the list currently highlighted in the
// lists panel, or "" when the panel has no selection. The lists-panel CRUD
// keys act on this, not on the open list: the two differ whenever the active
// list changes without the panel cursor moving.
func (m AppModel) highlightedListID() string {
	lists, ok := m.components.ListsPanel.(interface{ SelectedListID() string })
	if !ok {
		return ""
	}
	return lists.SelectedListID()
}

// highlightedListIDptr returns the highlighted list id as a *string, for the
// export modal which treats nil as "whole store". It returns nil (not a
// pointer to "") when nothing is highlighted.
func (m AppModel) highlightedListIDptr() *string {
	if id := m.highlightedListID(); id != "" {
		return &id
	}
	return nil
}

// ChangeFocus moves focus delta steps through the computed cycle (tab +1,
// shift+tab -1) and returns the SetFocusMsg that tells the zones which one
// is focused. A request for a zone that is not currently focusable is
// ignored by the zones themselves (they compare against their own id).
func (m *AppModel) ChangeFocus(delta int) tea.Cmd {
	zones := m.focusableZones()
	cur := slices.Index(zones, m.focusedZone)
	if cur < 0 {
		cur = 0
	}
	next := (cur + delta + len(zones)) % len(zones)
	m.focusedZone = zones[next]
	return cmds.SetFocus(m.focusedZone)
}

// closeListsPanel hides the Lists panel and returns focus to the task tree —
// the shared "commit and close" path every exit from the transient picker
// reuses (enter selecting a list, esc cancelling, landing in a newly created
// list), so the three don't each grow a slightly different idea of "close"
// (docs/DESIGN.md §5). It never writes to the store: the highlighted list is
// already the active one by the time any of these fire.
func (m *AppModel) closeListsPanel() tea.Cmd {
	m.listsPanelVisible = false
	m.bodyLayout = m.calculateBodyLayout()
	focusCmd := m.ChangeFocus(1)
	return tea.Batch(m.broadcastBodyLayout(), m.footerContextCmd(), focusCmd)
}

// broadcastBodyLayout returns a command that sends the current body layout
// to all chrome and body components.
func (m AppModel) broadcastBodyLayout() tea.Cmd {
	l := m.bodyLayout
	return cmds.SetBodyLayout(l.Height, l.ListsWidth, l.MainWidth, l.TerminalWidth)
}

// applyCreateDraft resolves a pending inline creation against the just-refreshed
// rows, writes the task through the store, and returns the commands that move
// the tree's selection onto the new task and re-fetch the rows (now including
// it). It is a no-op when no creation is pending or when there is no active
// list. The draft is always cleared, so a second resolution pass can't create
// the task twice.
func (m *AppModel) applyCreateDraft(rows []apptypes.Row) tea.Cmd {
	if m.createDraft == nil || m.activeListID == "" {
		return nil
	}
	draft := *m.createDraft
	m.createDraft = nil

	parentID, afterID, depth := resolveCreateLocation(rows, draft)
	title := strings.TrimSpace(draft.Title)
	if title == "" {
		return nil
	}

	newID, err := m.store.CreateTaskAfter(m.activeListID, title, parentID, "", afterID)
	if err != nil {
		m.lastError = err.Error()
		return nil
	}
	return tea.Batch(
		cmds.CreateTaskConfirmed(newID, depth),
		cmds.RefreshTasks(m.store, m.activeListID, m.sortMode),
	)
}

// resolveCreateLocation maps a create draft onto a concrete store insertion
// point, using the freshest rows. The create-before anchor is the selected
// task; LevelOffset selects the relationship, mirroring addinput's
// levelOffset semantics (docs/DESIGN.md §12): +1 inserts as its first child,
// 0 inserts as its next sibling, -1 inserts as a sibling of its parent.
func resolveCreateLocation(rows []apptypes.Row, draft cmds.CreateTaskFromInputMsg) (parentID *string, afterID string, depth int) {
	ref := findRowByID(rows, draft.BeforeID)
	switch draft.LevelOffset {
	case 1: // first child of the anchor
		if ref != nil {
			return &ref.Task.ID, "", ref.Depth + 1
		}
		return nil, "", 0
	case -1: // sibling of the anchor's parent (insert after the parent)
		if ref != nil && ref.Task.ParentID != nil {
			if parent := findRowByID(rows, *ref.Task.ParentID); parent != nil {
				return parent.Task.ParentID, *ref.Task.ParentID, ref.Depth - 1
			}
		}
		return nil, "", 0
	default: // 0: next sibling of the anchor
		if ref != nil {
			return ref.Task.ParentID, ref.Task.ID, ref.Depth
		}
		return nil, "", 0
	}
}

// findRowByID returns the row carrying the given task id, or nil if the id is
// not present in rows (e.g. the anchor was deleted while typing).
func findRowByID(rows []apptypes.Row, id string) *apptypes.Row {
	for i := range rows {
		if rows[i].Task.ID == id {
			return &rows[i]
		}
	}
	return nil
}

// assignedCount counts the rows carrying a durable assignee, for the
// release-list confirmation. It reads the rows already on screen rather than
// re-querying, so the number the dialog quotes is the number the user can see.
func assignedCount(rows []apptypes.Row) int {
	n := 0
	for i := range rows {
		if rows[i].Task.Assignee != "" {
			n++
		}
	}
	return n
}

// descendantCount returns the number of descendants of the row whose id is
// target, given rows in depth-first preorder. It finds the target's index,
// notes its depth, then walks forward counting every row until it reaches
// one whose depth is <= the target's — that run is the subtree.
func descendantCount(rows []apptypes.Row, target string) int {
	idx := -1
	depth := -1
	for i := range rows {
		if rows[i].Task.ID == target {
			idx = i
			depth = rows[i].Depth
			break
		}
	}
	if idx < 0 {
		return 0
	}
	count := 0
	for i := idx + 1; i < len(rows); i++ {
		if rows[i].Depth <= depth {
			break
		}
		count++
	}
	return count
}
