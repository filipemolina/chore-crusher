package cmds

import tea "charm.land/bubbletea/v2"

// DeleteArchivedListMsg requests that AppModel confirm and then permanently
// delete an archived list. ListName and TaskCount travel with the id so the
// confirm dialog can name what it is about to destroy (docs/DESIGN.md §9)
// without AppModel re-deriving them from a query scoped for the wrong set —
// store.ListLists excludes archived lists, so it cannot answer "how many
// tasks does this archived list have" the way the Lists panel's own delete
// dialog does for an active one.
type DeleteArchivedListMsg struct {
	ListID    string
	ListName  string
	TaskCount int
}

// DeleteArchivedList returns a command that asks AppModel to confirm and
// permanently delete the given archived list.
func DeleteArchivedList(listID, listName string, taskCount int) tea.Cmd {
	return func() tea.Msg {
		return DeleteArchivedListMsg{ListID: listID, ListName: listName, TaskCount: taskCount}
	}
}
