package model

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-completer/src/cmds"
	"github.com/filipemolina/chore-completer/src/components/confirmmodal"
	"github.com/filipemolina/chore-completer/src/components/detailsmodal"
	"github.com/filipemolina/chore-completer/src/components/helpoverlay"
	"github.com/filipemolina/chore-completer/src/components/listnamemodal"
	"github.com/filipemolina/chore-completer/src/components/themepickermodal"
	"github.com/filipemolina/chore-completer/src/config"
	"github.com/filipemolina/chore-completer/src/constants"
	"github.com/filipemolina/chore-completer/src/keys"
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
	// key nothing gets to swallow, which is why it is a binding of its own
	// and not part of Global.Quit.
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

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keys.Global.Quit):
			return m, tea.Quit

		case key.Matches(msg, keys.Global.Help):
			finalCmds = append(finalCmds, cmds.OpenHelpModal())

		case key.Matches(msg, keys.Global.Theme):
			finalCmds = append(finalCmds, cmds.OpenThemePicker())

		case key.Matches(msg, keys.Global.NextPanel):
			finalCmds = append(finalCmds, m.ChangeFocus(1))

		case key.Matches(msg, keys.Global.PrevPanel):
			finalCmds = append(finalCmds, m.ChangeFocus(-1))

		case key.Matches(msg, keys.Global.ToggleListsPanel):
			m.listsPanelVisible = !m.listsPanelVisible
			m.bodyLayout = m.calculateBodyLayout()
			finalCmds = append(finalCmds, m.broadcastBodyLayout())
			// A panel leaving the layout cannot keep the focus: fall back
			// to the task tree. A panel entering it is not focused either —
			// focus stays where it is until tab moves it.
			if !m.listsPanelVisible && m.focusedZone == constants.COMPONENT_LISTS_PANEL {
				finalCmds = append(finalCmds, m.ChangeFocus(1))
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
				m.activeModal = confirmmodal.New("Delete list", "Are you sure? This will delete every task in the list.", m.activeListID, m.store)
			}
		}

	// This is executed once when the app loads and after every resize.
	// Components never see it — only the layout message below.
	case tea.WindowSizeMsg:
		m.terminalWidth = msg.Width
		m.terminalHeight = msg.Height
		m.bodyLayout = m.calculateBodyLayout()
		finalCmds = append(finalCmds, m.broadcastBodyLayout())

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
		// First load (or a store emptied since): adopt the first list so
		// the task tree has something to poll.
		if m.activeListID == "" && len(msg.Lists) > 0 {
			m.activeListID = msg.Lists[0].List.ID
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID))
		}

	case cmds.RefreshTasksMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
		}

	case cmds.OpenHelpModalMsg:
		m.activeModal = helpoverlay.New(m.helpContext(), m.terminalWidth)

	case cmds.OpenThemePickerMsg:
		m.activeModal = themepickermodal.New(m.terminalHeight)

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

	case cmds.CreateTaskMsg:
		// Add-input created a task; refresh immediately and move selection to it.
		if m.activeListID != "" {
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID))
			finalCmds = append(finalCmds, cmds.SetSelection(msg.NewID, msg.Depth))
		}

	case cmds.SelectListMsg:
		// Lists panel selected a different list; switch to it immediately
		// rather than waiting for the next poll tick.
		if m.activeListID != msg.ListID {
			m.activeListID = msg.ListID
			finalCmds = append(finalCmds, cmds.RefreshTasks(m.store, m.activeListID))
		}
	}

	// Forward the message to every zone. Each component answers to a
	// subset (SetBodyLayoutMsg and SetFocusMsg go to all three;
	// RefreshListsMsg to the lists panel; RefreshTasksMsg to the task tree)
	// and ignores the rest.
	var listsCmd, treeCmd, inputCmd tea.Cmd
	m.components.ListsPanel, listsCmd = m.components.ListsPanel.Update(msg)
	m.components.TaskTree, treeCmd = m.components.TaskTree.Update(msg)
	m.components.AddInput, inputCmd = m.components.AddInput.Update(msg)
	finalCmds = append(finalCmds, listsCmd, treeCmd, inputCmd)

	return m, tea.Batch(finalCmds...)
}

// calculateBodyLayout returns the exact box each body zone must render
// into: ListsWidth + BODY_GUTTER_WIDTH + MainWidth == the terminal width,
// and TreeHeight + InputHeight == Height (the add input pinned to the
// bottom at ADD_INPUT_HEIGHT, docs/DESIGN.md §5).
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
	height := max(0, m.terminalHeight)
	available := max(0, m.terminalWidth)

	inputHeight := constants.ADD_INPUT_HEIGHT
	treeHeight := max(0, height-inputHeight)

	if !m.listsPanelVisible {
		return cmds.SetBodyLayoutMsg{
			Height:      height,
			ListsWidth:  0,
			MainWidth:   available,
			TreeHeight:  treeHeight,
			InputHeight: inputHeight,
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
		Height:      height,
		ListsWidth:  listsWidth,
		MainWidth:   mainWidth,
		TreeHeight:  treeHeight,
		InputHeight: inputHeight,
	}
}

// focusableZones is the computed focus cycle (docs/DESIGN.md §5, step 4 of
// the phase-3 plan): the task tree always, the lists panel only while it is
// visible, the add input always. A static slice could not express the lists
// panel entering and leaving the cycle at runtime.
func (m AppModel) focusableZones() []int {
	zones := []int{constants.COMPONENT_TASK_TREE}
	if m.listsPanelVisible {
		zones = append(zones, constants.COMPONENT_LISTS_PANEL)
	}
	return append(zones, constants.COMPONENT_ADD_INPUT)
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
// to the zones.
func (m AppModel) broadcastBodyLayout() tea.Cmd {
	l := m.bodyLayout
	return cmds.SetBodyLayout(l.Height, l.ListsWidth, l.MainWidth, l.TreeHeight, l.InputHeight)
}
