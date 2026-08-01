package tasktree

import (
	"testing"

	"github.com/filipemolina/chore-completer/src/apptypes"
)

// rows builds a flat list of n root tasks with ids "1".."n".
func rows(n int) []apptypes.Row {
	out := make([]apptypes.Row, n)
	for i := range out {
		out[i] = apptypes.Row{Task: apptypes.Task{ID: string(rune('1' + i))}}
	}
	return out
}

// The cursor-preservation rule (docs/DESIGN.md §7): the selection is matched
// by task id across a refresh, so a row that keeps its id keeps the cursor
// even when its index moves.
func TestSelectionSurvivesByIdAcrossRefresh(t *testing.T) {
	m := Model{}
	m.applyRows(rows(3))
	m.selectedID = "2"

	// The selected task moved from index 1 to index 0.
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "2"}},
		{Task: apptypes.Task{ID: "1"}},
		{Task: apptypes.Task{ID: "3"}},
	})

	if m.selectedID != "2" {
		t.Errorf("selection = %q, want %q (must follow the id, not the index)", m.selectedID, "2")
	}
}

// When the selected id is gone, the selection falls back to the nearest
// surviving row: the old index clamped into the new list.
func TestSelectionFallsBackToNearestSurvivingRow(t *testing.T) {
	m := Model{}
	m.applyRows(rows(4))
	m.selectedID = "3" // index 2

	// Task 3 was deleted; the list is now two rows.
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "1"}},
		{Task: apptypes.Task{ID: "2"}},
	})

	if m.selectedID != "2" {
		t.Errorf("selection = %q, want %q (old index 2 clamped to the last row)", m.selectedID, "2")
	}
}

// A refresh that removes rows before the selection must not shift the
// cursor past the new end of the list.
func TestSelectionClampsToNewListEnd(t *testing.T) {
	m := Model{}
	m.applyRows(rows(4))
	m.selectedID = "4" // index 3, last row

	m.applyRows(rows(2)) // ids "1", "2"

	if m.selectedID != "2" {
		t.Errorf("selection = %q, want %q (clamped to the new last row)", m.selectedID, "2")
	}
}

func TestEmptyRefreshClearsSelection(t *testing.T) {
	m := Model{}
	m.applyRows(rows(3))
	m.selectedID = "2"

	m.applyRows(nil)

	if m.selectedID != "" || len(m.rows) != 0 {
		t.Errorf("empty refresh should clear rows and selection, got %d rows, selection %q", len(m.rows), m.selectedID)
	}
}

// A first load with no prior selection picks the first row.
func TestFirstLoadSelectsFirstRow(t *testing.T) {
	m := Model{}
	m.applyRows(rows(3))

	if m.selectedID != "1" {
		t.Errorf("selection = %q, want %q (first row)", m.selectedID, "1")
	}
}
