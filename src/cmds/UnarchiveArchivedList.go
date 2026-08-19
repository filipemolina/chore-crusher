package cmds

import tea "charm.land/bubbletea/v2"

// UnarchiveArchivedListMsg requests that AppModel confirm and then restore
// an archived list to normal discovery. ListName travels with the id so the
// confirm dialog can name what it is about to restore, the same reasoning
// DeleteArchivedListMsg carries ListName and TaskCount for (docs/DESIGN.md
// §9).
type UnarchiveArchivedListMsg struct {
	ListID   string
	ListName string
}

// UnarchiveArchivedList returns a command that asks AppModel to confirm and
// restore the given archived list to normal discovery.
func UnarchiveArchivedList(listID, listName string) tea.Cmd {
	return func() tea.Msg {
		return UnarchiveArchivedListMsg{ListID: listID, ListName: listName}
	}
}
