package cmds

import tea "charm.land/bubbletea/v2"

// JumpToTaskMsg is the global picker's "enter on a result" hand-off: switch
// the active list to the result's list (when different), refresh it, and
// move the tree's selection to the matched task. The picker sends it as the
// follow-up behind CloseModal so the modal is gone before the jump runs.
type JumpToTaskMsg struct {
	TaskID string
	ListID string
}

// JumpToTask carries a picker result to AppModel: switch list (if needed)
// and select the task.
func JumpToTask(taskID, listID string) tea.Cmd {
	return func() tea.Msg { return JumpToTaskMsg{TaskID: taskID, ListID: listID} }
}