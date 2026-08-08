package cmds

import tea "charm.land/bubbletea/v2"

// ReleaseListMsg requests that every assignment in one list be released. The
// task tree sends it on U; AppModel confirms first (it can free work several
// agents hold at once) and then calls store.UnassignList.
//
// This is the escape hatch the assignment model is built around: assignment
// has no TTL and no sweeper, so releasing a whole board is the human's way
// out of a pile of abandoned grabs (docs/DESIGN.md §3, decision 2).
type ReleaseListMsg struct {
	ListID string
}

// ReleaseList returns a command that asks AppModel to release every
// assignment in the given list.
func ReleaseList(listID string) tea.Cmd {
	return func() tea.Msg {
		return ReleaseListMsg{ListID: listID}
	}
}
