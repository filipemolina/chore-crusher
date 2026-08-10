package cmds

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/store"
)

// RefreshListsMsg carries the store's lists, converted to apptypes at the
// boundary. Lists is nil when the query failed; Err holds the failure.
// Activities carries the live agent-claim set so the TUI can render
// spinners on claimed rows. ClaimedLists is the set of list ids with
// any live task claim, so the lists panel can show a spinner on a list an
// agent is working inside. The poll loop's RefreshLists routes this
// to the lists panel.
type RefreshListsMsg struct {
	Lists        []apptypes.ListSummary
	Activities   []apptypes.AgentActivity
	ClaimedLists map[string]bool
	Err          error
}

// RefreshLists queries the store and converts the rows at the boundary.
// The cmd imports store while components never do (docs/DESIGN.md §10:
// apptypes is the shape components pass around; the store is SQL).
func RefreshLists(s *store.Store) tea.Cmd {
	return func() tea.Msg {
		lists, err := s.ListLists()
		if err != nil {
			return RefreshListsMsg{Err: err}
		}
		work, err := s.ListWork()
		if err != nil {
			return RefreshListsMsg{Err: err}
		}
		claimedLists, err := s.ClaimedTaskListIDs()
		if err != nil {
			return RefreshListsMsg{Err: err}
		}
		return RefreshListsMsg{
			Lists:        apptypes.FromStoreLists(lists),
			Activities:   apptypes.FromStoreActivities(work),
			ClaimedLists: claimedLists,
		}
	}
}
