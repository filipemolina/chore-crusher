package cmds

import tea "charm.land/bubbletea/v2"

// MoveListMsg requests that the store reposition ListID to be the immediate
// successor of AfterID (another list, or "" for the front of the ordering).
// The lists panel resolves a move gesture into a concrete after-id from its
// own items; AppModel executes it through store.MoveList and refreshes
// (docs/DESIGN.md §5).
type MoveListMsg struct {
	ListID  string
	AfterID string
}

// MoveList returns a command that asks AppModel to reposition a list.
func MoveList(listID, afterID string) tea.Cmd {
	return func() tea.Msg {
		return MoveListMsg{ListID: listID, AfterID: afterID}
	}
}
