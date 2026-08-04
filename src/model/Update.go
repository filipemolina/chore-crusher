package model

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/confirmmodal"
	"github.com/filipemolina/chore-crusher/src/components/detailsmodal"
	"github.com/filipemolina/chore-crusher/src/components/helpoverlay"
	"github.com/filipemolina/chore-crusher/src/components/listnamemodal"
	"github.com/filipemolina/chore-crusher/src/components/searchpicker"
	"github.com/filipemolina/chore-crusher/src/components/themepickermodal"
	"github.com/filipemolina/chore-crusher/src/config"
	"github.com/filipemolina/chore-crusher/src/constants"
	"github.com/filipemolina/chore-crusher/src/keys"
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

	// While a modal is open, it owns all key input exclusively — the zones
	// and the global keys are frozen until it closes.
	if m.activeModal != nil {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			var modalCmd tea.Cmd
			m.activeModal, modalCmd = m.activeModal.Update(msg)
			return m, modalCmd
		}
	}

	// keyboardOwned reports whether a focused child component has claimed the
	// keyboard for itself (add input with text, tree typing a /-filter, list
	// typing a filter). While true, global keys that would open overlays or
	// toggle panels are suppressed so typing is not interrupted. tab/shift+tab
	// focus navigation is never suppressed — those are focus keys, not
	// characters, so they cannot interrupt typing.
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
			// Esc ladder (docs/DESIGN.md §5): a modal closes itself first —
			// it intercepts all keypresses at the top of Update, so by
			// the time we reach here no modal is open. Next, a focused child
			// that declared KeepsEsc (tree with applied filter or inline
			// create; lists panel with active filter) claims esc for itself.
			// Do not consume it here — let the component's own Update handle
			// it. When no child claims it, esc is a no-op.
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
				return m, nil
			}

		case key.Matches(msg, keys.Global.Help):
			finalCmds = append(finalCmds, cmds.OpenHelpModal())

		case key.Matches(msg, keys.Global.Theme):
			if !keyboardOwned() {
				finalCmds = append(finalCmds, cmds.OpenThemePicker())
			}

		// / enters the task tree's local filter; F opens the cross-list picker.
		// Both are global keys — they work whenever no modal owns the
		// keyboard, focused zone notwithstanding (docs/DESIGN.md §5).
		case key.Matches(msg, keys.Global.Filter):
			if !keyboardOwned() {
				finalCmds = append(finalCmds, cmds.ActivateFilter())
			}

		case key.Matches(msg, keys.Global.Picker):
			if !keyboardOwned() {
				finalCmds = append(finalCmds, cmds.OpenSearchPicker())
			}

		// tab/shift+tab are focus keys: they cycle the panels even while the
		// tree's create or filter input owns the keyboard, so focus is never
		// stuck inside an input. AppModel routes them before the tree's
		// allowlist runs, so the input never sees them as characters.
		case key.Matches(msg, keys.Global.NextPanel):
			finalCmds = append(finalCmds, m.ChangeFocus(1))

		case key.Matches(msg, keys.Global.PrevPanel):
			finalCmds = append(finalCmds, m.ChangeFocus(-1))

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

		// List CRUD keys: only active when lists panel is visible and focused
		case m.listsPanelVisible && m.focusedZone == constants.COMPONENT_LISTS_PANEL && key.Matches(msg, keys.Lists.New):
			m.activeModal = listnamemodal.New(listnamemodal.ModeNew, "", m.store)

		case m.listsPanelVisible && m.focusedZone == constants.COMPONENT_LISTS_PANEL && key.Matches(msg, keys.Lists.Rename):
			if m.activeListID != "" {
				m.activeModal = listnamemodal.New(listnamemodal.ModeRename, m.activeListID, m.store)
			}

		case m.listsPanelVisible && m.focusedZone == constants.COMPONENT_LISTS_PANEL && key.Matches(msg, keys.Lists.Delete):
			if m.activeListID != "" {
				m.activeModal = confirmmodal.New("Delete list", "Are you sure? This will delete every task in the list.", func() tea.Msg {
					if err := m.store.DeleteList(m.activeListID); err != nil {
						return nil
					}
					return cmds.RefreshLists(m.store)()
				})
			}
		}

	// This is executed once when the app loads and after every resize.
	// Components never see it — only the layout message below.
	case tea.WindowSizeMsg:
		m.terminalWidth = msg.Width
		m.terminalHeight = msg.Height
		m.bodyLayout = m.calculateBodyLayout()
		finalCmds = append(finalCmds, m.broadcastBodyLayout(), m.footerContextCmd())

	// The poll tick re-issues itself here, which is what makes the poll
	// recurring for the life of the app (docs/DESIGN.md §7).
	case cmds.PollTickMsg:
		finalCmds = append(finalCmds, cmds.PollTick(config.PollInterval(m.cfg)))
		finalCmds = append(finalCmds, cmds.RefreshLists(m.store))
		if m.activeListID != "" {
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID))
		}

	case cmds.RefreshListsMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			break
		}
		m.lists = msg.Lists
		if m.activeListID == "" {
			if len(msg.Lists) > 0 {
				m.activeListID = msg.Lists[0].List.ID
				finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID))
			} else if m.store != nil {
				if id, err := m.store.CreateList("New List"); err == nil {
					m.activeListID = id
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
					finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID))
				} else if m.store != nil {
					if id, err := m.store.CreateList("New List"); err == nil {
						m.activeListID = id
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
		if draftCmd := m.applyCreateDraft(msg.Rows); draftCmd != nil {
			finalCmds = append(finalCmds, draftCmd)
		}
		finalCmds = append(finalCmds, m.footerContextCmd())

	case cmds.OpenHelpModalMsg:
		m.activeModal = helpoverlay.New(m.helpContext(), m.terminalWidth)

	case cmds.OpenThemePickerMsg:
		m.activeModal = themepickermodal.New(m.terminalHeight)

	case cmds.OpenSearchPickerMsg:
		m.activeModal = searchpicker.New(m.store, m.terminalHeight)

	// The global picker jumped to a task, possibly in another list: switch
	// the active list to the result's list (when different) and move the tree
	// selection to the task. SelectTask is sent unconditionally — if the task
	// is already in rows it selects immediately, and if the list just changed
	// the task tree's pending-select lands it once those rows arrive.
	case cmds.JumpToTaskMsg:
		if m.activeListID != msg.ListID {
			m.activeListID = msg.ListID
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID))
		}
		finalCmds = append(finalCmds, cmds.SelectTask(msg.TaskID))

	case cmds.OpenDetailsMsg:
		m.activeModal = detailsmodal.New(msg.TaskID, m.activeListID, m.store)

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

	case cmds.CreateTaskFromInputMsg:
		// The task tree's inline input submitted a draft. Don't create yet:
		// resolve the insertion against the freshest rows on the next
		// RefreshTasksMsg, so a poll or delete during typing can't anchor the
		// new task to a stale selection.
		m.createDraft = &msg
		if m.activeListID != "" {
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID))
		}

	case cmds.DeleteTaskMsg:
		// The tree's d binding emitted this (it owns the keypress); route it
		// through the same confirm modal pattern as list delete (docs/DESIGN.md
		// §9: destructive ops need confirmation in the TUI).
		if msg.TaskID != "" {
			taskID := msg.TaskID
			m.activeModal = confirmmodal.New("Delete task", "Are you sure?", func() tea.Msg {
				if err := m.store.DeleteTask(taskID); err != nil {
					return nil
				}
				return cmds.RefreshTasks(m.store, m.activeListID)()
			})
		}

	case cmds.ToggleTaskMsg:
		if err := m.store.Toggle(msg.TaskID); err != nil {
			m.lastError = err.Error()
			break
		}
		if m.activeListID != "" {
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID))
		}

	case cmds.MoveTaskMsg:
		if err := m.store.MoveTask(msg.TaskID, msg.AfterID); err != nil {
			m.lastError = err.Error()
			break
		}
		if m.activeListID != "" {
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID))
		}

	case cmds.ReparentTaskMsg:
		if err := m.store.Reparent(msg.TaskID, msg.ParentID); err != nil {
			m.lastError = err.Error()
			break
		}
		if m.activeListID != "" {
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID))
		}

	case cmds.SelectListMsg:
		// Lists panel selected a different list; switch to it immediately
		// rather than waiting for the next poll tick.
		if m.activeListID != msg.ListID {
			m.activeListID = msg.ListID
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID))
		}
	}

	// Forward the message to every component. TaskPanel forwards to the tree
	// and input controls after deriving their shared Tasks-surface state.
	var menuCmd, barCmd, listsCmd, tasksCmd tea.Cmd
	m.components.MainMenu, menuCmd = m.components.MainMenu.Update(msg)
	m.components.KeybindingBar, barCmd = m.components.KeybindingBar.Update(msg)
	m.components.ListsPanel, listsCmd = m.components.ListsPanel.Update(msg)
	m.components.TaskPanel, tasksCmd = m.components.TaskPanel.Update(msg)
	finalCmds = append(finalCmds, menuCmd, barCmd, listsCmd, tasksCmd)

	return m, tea.Batch(finalCmds...)
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
func (m AppModel) calculateBodyLayout() cmds.SetBodyLayoutMsg {
	height := max(0, m.terminalHeight-constants.HEADER_HEIGHT-constants.FOOTER_HEIGHT)
	available := max(0, m.terminalWidth)

	if !m.listsPanelVisible {
		return cmds.SetBodyLayoutMsg{
			Height:        height,
			ListsWidth:    0,
			MainWidth:     available,
			TerminalWidth: available,
		}
	}

	guttered := available - constants.BODY_GUTTER_WIDTH
	var listsWidth, mainWidth int
	switch {
	case guttered < constants.MIN_PANEL_WIDTH:
		listsWidth, mainWidth = 0, available
	case guttered < 2*constants.MIN_PANEL_WIDTH:
		listsWidth = guttered / 2
		mainWidth = guttered - listsWidth
	default:
		listsWidth = int(float32(guttered) * constants.LEFT_PANEL_WIDTH)
		listsWidth = max(listsWidth, constants.MIN_PANEL_WIDTH)
		listsWidth = min(listsWidth, guttered-constants.MIN_PANEL_WIDTH)
		mainWidth = guttered - listsWidth
	}

	return cmds.SetBodyLayoutMsg{
		Height:        height,
		ListsWidth:    listsWidth,
		MainWidth:     mainWidth,
		TerminalWidth: available,
	}
}

// focusableZones is the computed focus cycle (docs/DESIGN.md §5, step 4 of
// the phase-3 plan): the task tree always, the lists panel only while it is
// visible. Inline creation lives inside the tree, so there is no separate add
// input zone to cycle to — a static slice could not express the lists panel
// entering and leaving the cycle at runtime.
func (m AppModel) focusableZones() []int {
	zones := []int{constants.COMPONENT_TASK_TREE}
	if m.listsPanelVisible {
		zones = append(zones, constants.COMPONENT_LISTS_PANEL)
	}
	return zones
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
		cmds.RefreshTasks(m.store, m.activeListID),
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
