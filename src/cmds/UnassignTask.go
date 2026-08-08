package cmds

import tea "charm.land/bubbletea/v2"

// UnassignTaskMsg requests that the store release one task's durable
// assignment. The task tree sends it on u; AppModel calls
// store.UnassignTask and refreshes the rows, the same request/response
// split ToggleTaskMsg uses so the tree never touches the store
// (docs/DESIGN.md §5, §9).
//
// The release is unconditional — it is how a human frees a task whose agent
// died, so it must work on an assignment the TUI does not own
// (decision 2).
type UnassignTaskMsg struct {
	TaskID string
}

// UnassignTask returns a command that asks AppModel to release the given
// task's assignment.
func UnassignTask(taskID string) tea.Cmd {
	return func() tea.Msg {
		return UnassignTaskMsg{TaskID: taskID}
	}
}
