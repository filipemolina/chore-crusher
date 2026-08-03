package cmds

import tea "charm.land/bubbletea/v2"

// MoveTaskMsg requests that the store reposition TaskID to be the immediate
// successor of AfterID (a sibling, or "" for the front of the task's current
// parent run). The task tree resolves a move gesture — move up/down, outdent
// — into a concrete after-id from its own rows; AppModel executes it through
// store.MoveTask and refreshes (docs/DESIGN.md §5).
type MoveTaskMsg struct {
	TaskID  string
	AfterID string
}

// MoveTask returns a command that asks AppModel to reposition a task.
func MoveTask(taskID, afterID string) tea.Cmd {
	return func() tea.Msg {
		return MoveTaskMsg{TaskID: taskID, AfterID: afterID}
	}
}
