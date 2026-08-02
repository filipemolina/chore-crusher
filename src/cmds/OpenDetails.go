package cmds

import tea "charm.land/bubbletea/v2"

// OpenDetailsMsg signals that the details screen for a task should open.
type OpenDetailsMsg struct{ TaskID string }

// OpenDetails creates a command that opens the details screen for a task.
func OpenDetails(taskID string) tea.Cmd {
	return func() tea.Msg { return OpenDetailsMsg{TaskID: taskID} }
}
