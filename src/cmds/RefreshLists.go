package cmds

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/store"
)

// RefreshListsMsg carries the store's lists, converted to apptypes at the
// boundary. Lists is nil when the query failed; Err holds the failure.
// The poll loop's RefreshLists routes this to the lists panel.
type RefreshListsMsg struct {
	Lists []apptypes.ListSummary
	Err   error
}

// RefreshLists queries the store and converts the rows at the boundary.
// The cmd imports store while components never do — same split as
// stack-stitcher's commands importing the data layer (docs/DESIGN.md §10:
// apptypes is the shape components pass around; the store is SQL).
func RefreshLists(s *store.Store) tea.Cmd {
	return func() tea.Msg {
		lists, err := s.ListLists()
		if err != nil {
			return RefreshListsMsg{Err: err}
		}
		return RefreshListsMsg{Lists: apptypes.FromStoreLists(lists)}
	}
}
