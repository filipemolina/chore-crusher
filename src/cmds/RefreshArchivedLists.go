package cmds

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/store"
)

// RefreshArchivedListsMsg carries every archived list, converted to apptypes
// at the boundary. Lists is nil when the query failed; Err holds the
// failure. The Archive page filters this set client-side (mirroring the
// Lists panel's own name filter), so this always loads the full archived set
// rather than taking a query string.
type RefreshArchivedListsMsg struct {
	Lists []apptypes.ListSummary
	Err   error
}

// RefreshArchivedLists queries every archived list (newest-archived first,
// store.ListArchivedLists's own order) and converts the rows at the
// boundary.
func RefreshArchivedLists(s *store.Store) tea.Cmd {
	return func() tea.Msg {
		lists, err := s.ListArchivedLists("")
		if err != nil {
			return RefreshArchivedListsMsg{Err: err}
		}
		return RefreshArchivedListsMsg{Lists: apptypes.FromStoreLists(lists)}
	}
}
