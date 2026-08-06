package cmds

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/store"
)

// RefreshDetailsMsg carries one task's current details for the Details side
// panel, converted to apptypes at the boundary. Task is the fresh row;
// DerivedPct and DisplayAsSimple are the subtasks-mode display values
// (store.DerivedProgress); Comments is the task's comment thread, oldest first.
// Err holds a read failure; the other fields are zero when Err is set. The
// panel replaces its displayed fields from this only while its editor is clean
// — a dirty draft keeps its edits (docs/DESIGN.md §5, the poll-refresh-while-clean
// rule).
type RefreshDetailsMsg struct {
	TaskID          string
	Task            apptypes.Task
	DerivedPct      int
	DisplayAsSimple bool
	Comments        []apptypes.Comment
	Err             error
}

// RefreshDetails reads one task's details, derived progress, and comment
// thread. It never writes, and it converts the store rows with apptypes
// functions so no store row type crosses into a component (the same apptypes
// boundary the other refresh commands keep, docs/DESIGN.md §10).
func RefreshDetails(s *store.Store, taskID string) tea.Cmd {
	return func() tea.Msg {
		t, err := s.GetTask(taskID)
		if err != nil {
			return RefreshDetailsMsg{TaskID: taskID, Err: err}
		}
		_, derivedPct, displayAsSimple, err := s.DerivedProgress(taskID)
		if err != nil {
			return RefreshDetailsMsg{TaskID: taskID, Err: err}
		}
		comments, err := s.ListComments(taskID)
		if err != nil {
			return RefreshDetailsMsg{TaskID: taskID, Err: err}
		}
		return RefreshDetailsMsg{
			TaskID:          taskID,
			Task:            apptypes.FromStore(t),
			DerivedPct:      derivedPct,
			DisplayAsSimple: displayAsSimple,
			Comments:        apptypes.FromStoreComments(comments),
		}
	}
}
