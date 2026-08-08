package cmds

import tea "charm.land/bubbletea/v2"

// SetSelectionMsg tells the add-input component which task is currently
// selected in the tree, so it can compute valid level-offset transitions
// transitions.
type SetSelectionMsg struct {
	TaskID string
	Depth  int
}

// SetSelection returns a command that broadcasts the current selection.
func SetSelection(taskID string, depth int) tea.Cmd {
	return func() tea.Msg {
		return SetSelectionMsg{TaskID: taskID, Depth: depth}
	}
}
