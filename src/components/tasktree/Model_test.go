package tasktree

import (
	"testing"

	"github.com/filipemolina/chore-crusher/src/apptypes"
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

// A 3-level tree whose only title match is a leaf. The /-filter must keep the
// leaf's whole ancestor chain visible even though none of them match, so the
// leaf never floats with no visible parent (docs/plans/phase-8-search.md step 1).
func TestFilterKeepsAncestorsOfMatchedLeaf(t *testing.T) {
	root := apptypes.Row{Task: apptypes.Task{ID: "root", Title: "Project"}, Depth: 0, HasChildren: true}
	sub := apptypes.Row{Task: apptypes.Task{ID: "sub", ParentID: strPtr("root"), Title: "Milestone"}, Depth: 1, HasChildren: true}
	leaf := apptypes.Row{Task: apptypes.Task{ID: "leaf", ParentID: strPtr("sub"), Title: "Ship the zorb"}, Depth: 2}
	other := apptypes.Row{Task: apptypes.Task{ID: "other", Title: "Unrelated task"}, Depth: 0}

	m := Model{}
	m.applyRows([]apptypes.Row{root, sub, leaf, other})
	m.filterTyping = true
	m.filterQuery = "zorb"

	got := m.displayedRows()
	wantIDs := []string{"root", "sub", "leaf"}
	if len(got) != len(wantIDs) {
		t.Fatalf("filtered rows = %d, want %d", len(got), len(wantIDs))
	}
	for i, want := range wantIDs {
		if got[i].Task.ID != want {
			t.Errorf("filtered[%d].ID = %q, want %q", i, got[i].Task.ID, want)
		}
	}
}

// An unrelated root task with no match — and no matched descendant — drops out
// of the filtered view entirely.
func TestFilterDropsUnrelatedRoots(t *testing.T) {
	root := apptypes.Row{Task: apptypes.Task{ID: "root", Title: "Project"}, Depth: 0, HasChildren: true}
	sub := apptypes.Row{Task: apptypes.Task{ID: "sub", ParentID: strPtr("root"), Title: "Milestone"}, Depth: 1}
	m := Model{}
	m.applyRows([]apptypes.Row{root, sub})
	m.filterTyping = true
	m.filterQuery = "nowhere-nothing"

	got := m.displayedRows()
	if len(got) != 0 {
		t.Errorf("filtered rows = %d, want 0", len(got))
	}
}

// An empty query shows everything: typing / and nothing yet is a no-op filter.
func TestEmptyQueryDoesNotFilter(t *testing.T) {
	root := apptypes.Row{Task: apptypes.Task{ID: "a", Title: "Alpha"}}
	sub := apptypes.Row{Task: apptypes.Task{ID: "b", Title: "Beta"}}
	m := Model{}
	m.applyRows([]apptypes.Row{root, sub})
	m.filterQuery = ""

	got := m.displayedRows()
	if len(got) != 2 {
		t.Errorf("empty query filtered to %d rows, want 2", len(got))
	}
}

// A directly-matched row is distinguishable from an ancestor-only row: only
// real matches land in the matched set used to dim ancestors.
func TestMatchedVisibleSeparatesMatchesFromAncestors(t *testing.T) {
	root := apptypes.Row{Task: apptypes.Task{ID: "root", Title: "Project"}, HasChildren: true}
	leaf := apptypes.Row{Task: apptypes.Task{ID: "leaf", ParentID: strPtr("root"), Title: "Zorble"}}

	_, matched := matchVisible([]apptypes.Row{root, leaf}, "zorble")
	if !matched["leaf"] {
		t.Errorf("leaf should be a direct match")
	}
	if matched["root"] {
		t.Errorf("root should not count as a direct match, only an ancestor")
	}
}

func strPtr(s string) *string { return &s }

// A first load with no prior selection picks the first row.
func TestFirstLoadSelectsFirstRow(t *testing.T) {
	m := Model{}
	m.applyRows(rows(3))

	if m.selectedID != "1" {
		t.Errorf("selection = %q, want %q (first row)", m.selectedID, "1")
	}
}
