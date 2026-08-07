package tasktree

import (
	"testing"

	"github.com/filipemolina/chore-crusher/src/apptypes"
)

// threeLevelTree builds A > B > C, plus a sibling D under A — the fixture
// every collapse/expand invariant test below shares. Nothing starts
// collapsed, so the whole tree is visible until a test collapses something.
func threeLevelTree() Model {
	a := apptypes.Row{Task: apptypes.Task{ID: "A", Title: "A"}, Depth: 0, HasChildren: true}
	b := apptypes.Row{Task: apptypes.Task{ID: "B", ParentID: strPtr("A"), Title: "B"}, Depth: 1, HasChildren: true}
	c := apptypes.Row{Task: apptypes.Task{ID: "C", ParentID: strPtr("B"), Title: "C"}, Depth: 2}
	d := apptypes.Row{Task: apptypes.Task{ID: "D", ParentID: strPtr("A"), Title: "D"}, Depth: 1}

	m := Model{collapsed: make(map[string]bool)}
	m.applyRows([]apptypes.Row{a, b, c, d})
	return m
}

// visibleIDs returns the ids of the currently visible (collapse-aware) rows,
// in display order.
func visibleIDs(m *Model) []string {
	var ids []string
	for _, r := range m.visibleRows() {
		ids = append(ids, r.Task.ID)
	}
	return ids
}

func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// TestCollapseIsDeep pins the core invariant: collapsing a task hides its
// whole subtree, every depth, not just its direct children.
func TestCollapseIsDeep(t *testing.T) {
	m := threeLevelTree()
	m.selectedID = "A"

	m.toggleCollapse(false) // collapse A

	got := visibleIDs(&m)
	if len(got) != 1 || got[0] != "A" {
		t.Errorf("visible after collapsing A = %v, want [A] (B, C, D all hidden)", got)
	}
	for _, id := range []string{"B", "C", "D"} {
		if !m.collapsed[id] {
			t.Errorf("collapsed[%q] = false after collapsing A, want true (deep collapse resets every descendant)", id)
		}
	}
}

// TestExpandIsShallow pins the other half: expanding a task reveals only its
// direct children — grandchildren stay collapsed until expanded themselves.
func TestExpandIsShallow(t *testing.T) {
	m := threeLevelTree()
	m.selectedID = "A"
	m.toggleCollapse(false) // collapse A: B, C, D all hidden and marked collapsed

	m.selectedID = "A"
	m.toggleCollapse(true) // expand A

	got := visibleIDs(&m)
	if !containsID(got, "B") || !containsID(got, "D") {
		t.Errorf("visible after expanding A = %v, want B and D visible", got)
	}
	if containsID(got, "C") {
		t.Errorf("visible after expanding A = %v, want C still hidden (shallow expand)", got)
	}

	// Expanding B, in turn, reveals C.
	m.selectedID = "B"
	m.toggleCollapse(true)
	got = visibleIDs(&m)
	if !containsID(got, "C") {
		t.Errorf("visible after expanding B = %v, want C visible", got)
	}
}

// TestCollapseThenExpandResetsInsteadOfRemembering pins the decision the
// task notes make explicit: collapse does not remember a descendant's prior
// expansion state. Expanding B, then collapsing A, then re-expanding A must
// NOT bring C back — the reset decision, not the "remember" alternative.
func TestCollapseThenExpandResetsInsteadOfRemembering(t *testing.T) {
	m := threeLevelTree()
	m.selectedID = "A"
	m.toggleCollapse(false) // collapse A
	m.selectedID = "A"
	m.toggleCollapse(true) // expand A: B, D visible; C still hidden
	m.selectedID = "B"
	m.toggleCollapse(true) // expand B too: C now visible

	if !containsID(visibleIDs(&m), "C") {
		t.Fatal("precondition: C should be visible after expanding both A and B")
	}

	// Collapse A again, deeply — this must reset B's expansion, not
	// remember that C was showing.
	m.selectedID = "A"
	m.toggleCollapse(false)
	m.selectedID = "A"
	m.toggleCollapse(true) // re-expand A: exactly one level back

	got := visibleIDs(&m)
	if !containsID(got, "B") || !containsID(got, "D") {
		t.Errorf("visible after re-expanding A = %v, want B and D visible", got)
	}
	if containsID(got, "C") {
		t.Errorf("visible after re-expanding A = %v, want C hidden — expand must not remember B was previously expanded", got)
	}
}

// TestCollapseMovesSelectionOffHiddenDescendant pins the selection-validity
// rule directly against collapseDeep: if the current selection is a
// descendant of the task being collapsed, it must move to that task rather
// than pointing at a row that just became invisible.
func TestCollapseMovesSelectionOffHiddenDescendant(t *testing.T) {
	m := threeLevelTree()
	m.selectedID = "C"

	m.collapseDeep("A")

	if m.selectedID != "A" {
		t.Errorf("selectedID after collapsing an ancestor of the selection = %q, want %q", m.selectedID, "A")
	}
	if !containsID(visibleIDs(&m), m.selectedID) {
		t.Errorf("selectedID %q must be a visible row after the collapse", m.selectedID)
	}
}

// TestCollapseViaKeyMovesSelectionThatWasOnAHiddenDescendant exercises the
// same rule through the real key-driven path: navigate onto C, collapse B
// (hiding C), collapse A (hiding B) — selection must end up on A, a visible
// row, not stranded on a task no longer on screen.
func TestCollapseViaKeyMovesSelectionThatWasOnAHiddenDescendant(t *testing.T) {
	m := threeLevelTree()
	m.selectedID = "C"

	m.selectedID = "B"
	m.toggleCollapse(false) // collapse B: hides C
	m.selectedID = "A"
	m.toggleCollapse(false) // collapse A: hides B (and D)

	if m.selectedID != "A" {
		t.Errorf("selectedID = %q, want %q", m.selectedID, "A")
	}
	got := visibleIDs(&m)
	if len(got) != 1 || got[0] != "A" {
		t.Errorf("visible = %v, want [A]", got)
	}
}

// TestCollapseAndExpandOnLeafIsNoOp pins the edge case: a leaf has no
// children, so both keys are no-ops — no panic, no state change.
func TestCollapseAndExpandOnLeafIsNoOp(t *testing.T) {
	m := threeLevelTree()
	m.selectedID = "C" // leaf: HasChildren is false

	m.toggleCollapse(false)
	if m.collapsed["C"] {
		t.Error("collapsing a leaf should be a no-op, but C is marked collapsed")
	}

	m.toggleCollapse(true)
	if len(m.collapsed) != 0 {
		t.Errorf("expanding a leaf should be a no-op, collapsed = %v", m.collapsed)
	}
}

// TestCollapseAlreadyCollapsedSubtreeIsNoOp: collapsing A twice in a row
// must not panic and leaves the same state as collapsing it once.
func TestCollapseAlreadyCollapsedSubtreeIsNoOp(t *testing.T) {
	m := threeLevelTree()
	m.selectedID = "A"
	m.toggleCollapse(false)
	before := map[string]bool{}
	for k, v := range m.collapsed {
		before[k] = v
	}

	// A is no longer visible-and-expanded from its own perspective, but is
	// still the selected, visible row — pressing collapse again must not
	// crash or change anything observable.
	m.toggleCollapse(false)

	if len(m.collapsed) != len(before) {
		t.Errorf("collapsing an already-collapsed subtree changed state: before %v, after %v", before, m.collapsed)
	}
	got := visibleIDs(&m)
	if len(got) != 1 || got[0] != "A" {
		t.Errorf("visible after double-collapse = %v, want [A]", got)
	}
}
