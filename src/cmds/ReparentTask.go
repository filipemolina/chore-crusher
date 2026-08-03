package cmds

import tea "charm.land/bubbletea/v2"

// ReparentTaskMsg requests that the store move TaskID under ParentID (nil =
// the list root). The tree's indent key uses it: the selected task becomes
// the last child of its previous sibling (docs/DESIGN.md §5).
type ReparentTaskMsg struct {
	TaskID   string
	ParentID *string
}

// ReparentTask returns a command that asks AppModel to re-parent a task.
func ReparentTask(taskID string, parentID *string) tea.Cmd {
	return func() tea.Msg {
		return ReparentTaskMsg{TaskID: taskID, ParentID: parentID}
	}
}
