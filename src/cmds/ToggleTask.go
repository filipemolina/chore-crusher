package cmds

import tea "charm.land/bubbletea/v2"

// ToggleTaskMsg requests that the store flip the given task between complete
// and pending. AppModel handles this so the task tree stays decoupled from
// the store: the tree sends the request on its space key, and AppModel calls
// store.Toggle and refreshes the rows (docs/DESIGN.md §5, §9).
type ToggleTaskMsg struct {
	TaskID string
}

// ToggleTask returns a command that asks AppModel to toggle the given task.
func ToggleTask(taskID string) tea.Cmd {
	return func() tea.Msg {
		return ToggleTaskMsg{TaskID: taskID}
	}
}
