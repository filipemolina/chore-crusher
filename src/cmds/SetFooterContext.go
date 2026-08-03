package cmds

import tea "charm.land/bubbletea/v2"

// SetFooterContextMsg is the screen state the keybinding bar needs to decide
// which keys are live. It mirrors keys.Context without importing keys, so
// cmds stays a leaf package.
type SetFooterContextMsg struct {
	Focused           int
	ListsPanelVisible bool
	TaskTreeEmpty     bool
	HasActiveList     bool
	Creating          bool
	Filtering         bool
	HasModal          bool
}

// SetFooterContext builds the message from the same facts the help overlay
// uses, keeping the footer and the overlay in lockstep.
func SetFooterContext(focused int, listsPanelVisible, taskTreeEmpty, hasActiveList, creating, filtering, hasModal bool) tea.Cmd {
	return func() tea.Msg {
		return SetFooterContextMsg{
			Focused:           focused,
			ListsPanelVisible: listsPanelVisible,
			TaskTreeEmpty:     taskTreeEmpty,
			HasActiveList:     hasActiveList,
			Creating:          creating,
			Filtering:         filtering,
			HasModal:          hasModal,
		}
	}
}
