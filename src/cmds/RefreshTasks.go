package cmds

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/store"
)

// RefreshTasksMsg carries one list's tasks as flattened tree rows, converted
// to apptypes at the boundary. Rows is nil when the query failed; Err holds
// the failure. Activities carries the live agent-claim set so the TUI can
// render spinners on claimed rows (docs/plan/mcp-server-enhancement.md §3.5).
// The poll loop's RefreshTasks routes this to the task tree.
type RefreshTasksMsg struct {
	ListID     string
	Rows       []apptypes.Row
	Activities []apptypes.AgentActivity
	Err        error
}

// RefreshTasks queries one list's tasks and flattens them into renderable
// rows (parents before children, depth-annotated — apptypes.Flatten is the
// one ordering both the CLI and the TUI render from, docs/DESIGN.md §10).
func RefreshTasks(s *store.Store, listID string) tea.Cmd {
	return func() tea.Msg {
		tasks, err := s.ListTasks(listID)
		if err != nil {
			return RefreshTasksMsg{ListID: listID, Err: err}
		}
		work, err := s.ListWork()
		if err != nil {
			return RefreshTasksMsg{ListID: listID, Err: err}
		}
		return RefreshTasksMsg{
			ListID:     listID,
			Rows:       apptypes.Flatten(apptypes.FromStoreTasks(tasks)),
			Activities: apptypes.FromStoreActivities(work),
		}
	}
}
