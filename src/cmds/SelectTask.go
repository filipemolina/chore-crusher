package cmds

import tea "charm.land/bubbletea/v2"

// SelectTaskMsg asks the task tree to move its selection to the task with
// the given id. When the task is not in the tree's current rows (the picker
// jumped to a task in a list that has not loaded yet) the tree remembers the
// request and honours it on the next refresh that contains the id — the
// "jump and select" behaviour, reused here for the global picker.
type SelectTaskMsg struct {
	TaskID string
}

// SelectTask asks the tree to select a task by id.
func SelectTask(taskID string) tea.Cmd {
	return func() tea.Msg { return SelectTaskMsg{TaskID: taskID} }
}
