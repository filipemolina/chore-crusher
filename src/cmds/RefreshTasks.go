package cmds

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/store"
)

// RefreshTasksMsg carries one list's tasks as flattened tree rows, converted
// to apptypes at the boundary. Rows is nil when the query failed; Err holds
// the failure. Activities carries the live agent-claim set so the TUI can
// render spinners on claimed rows. The poll loop's RefreshTasks
// routes this to the task tree.
type RefreshTasksMsg struct {
	ListID     string
	ListName   string
	Rows       []apptypes.Row
	Activities []apptypes.AgentActivity
	Err        error
}

// RefreshTasks queries one list's tasks and flattens them into renderable
// rows (parents before children, depth-annotated — apptypes.Flatten is the
// one ordering both the CLI and the TUI render from, docs/DESIGN.md §10).
// sortMode controls how tasks are ordered before flattening.
func RefreshTasks(s *store.Store, listID string, sortMode apptypes.SortMode) tea.Cmd {
	return func() tea.Msg {
		tasks, err := s.ListTasks(listID)
		if err != nil {
			return RefreshTasksMsg{ListID: listID, Err: err}
		}
		work, err := s.ListWork()
		if err != nil {
			return RefreshTasksMsg{ListID: listID, Err: err}
		}
		// Carry the list's display name so the Tasks panel can show it in its
		// header (docs/DESIGN.md §12). A lookup failure is non-fatal — the rows
		// are what matter — so the name simply stays empty.
		listName := ""
		if l, err := s.GetList(listID); err == nil {
			listName = l.Name
		}
		// Convert to apptypes first, then sort if needed
		appTasks := apptypes.FromStoreTasks(tasks)
		if sortMode != apptypes.SortManual {
			appTasks = apptypes.SortTasks(appTasks, sortMode)
		}
		rows := apptypes.Flatten(appTasks)
		// One batch query for which tasks in this list have comments, so the
		// tasktree can draw the comments glyph on every row without an N+1
		// per-row lookup.
		commented, err := s.TaskIDsWithComments(listID)
		if err != nil {
			return RefreshTasksMsg{ListID: listID, Err: err}
		}
		for i := range rows {
			rows[i].HasComments = commented[rows[i].Task.ID]
		}
		return RefreshTasksMsg{
			ListID:     listID,
			ListName:   listName,
			Rows:       rows,
			Activities: apptypes.FromStoreActivities(work),
		}
	}
}
