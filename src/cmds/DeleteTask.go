package cmds

import tea "charm.land/bubbletea/v2"

// DeleteTaskMsg requests that the store delete the given task.
type DeleteTaskMsg struct {
	TaskID string
}

// DeleteTask returns a command that asks AppModel to delete the given task.
func DeleteTask(taskID string) tea.Cmd {
	return func() tea.Msg {
		return DeleteTaskMsg{TaskID: taskID}
	}
}
