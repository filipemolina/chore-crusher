package cmds

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/store"
)

// RefreshArchivedListPreviewMsg carries one archived list's tasks, flattened
// into renderable rows the same way RefreshTasksMsg does — apptypes.Flatten
// is the single ordering both front ends render from (docs/DESIGN.md §10).
// ListID identifies which list this response is for, so a stale response
// racing a newer selection can be told apart and dropped.
type RefreshArchivedListPreviewMsg struct {
	ListID string
	Rows   []apptypes.Row
	Err    error
}

// RefreshArchivedListPreview loads one archived list's tasks for the
// Archive page's read-only preview column. It skips the mentions/comments
// enrichment RefreshTasks does for the live tree — the preview is plain
// title text, not an editable surface.
func RefreshArchivedListPreview(s *store.Store, listID string) tea.Cmd {
	return func() tea.Msg {
		tasks, err := s.ListTasks(listID)
		if err != nil {
			return RefreshArchivedListPreviewMsg{ListID: listID, Err: err}
		}
		rows := apptypes.Flatten(apptypes.FromStoreTasks(tasks))
		return RefreshArchivedListPreviewMsg{ListID: listID, Rows: rows}
	}
}
