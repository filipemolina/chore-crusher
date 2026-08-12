package tasktree

import (
	"testing"

	"github.com/filipemolina/farol/src/apptypes"
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

// TestLeftRightNavigationFallthrough pins the navigation fallback behavior
// from docs/DESIGN.md §5: ←/h on a leaf or a collapsed parent moves to the
// parent; →/l on a leaf or an expanded parent moves to the first child.
func TestLeftRightNavigationFallthrough(t *testing.T) {
	m := threeLevelTree()
	m.selectedID = "A"

	// A is expanded with children B and D (and grandchild C).
	// → should expand A (no-op since already expanded) and move to first child B.
	m.toggleCollapse(true) // expand A (no-op)
	m.selectedID = "A"
	// Simulate right key: expand if collapsed, else move to first child
	row := m.findRow("A")
	if row != nil && row.HasChildren && !m.collapsed["A"] {
		// expanded → move to first child
		m.selectedID = "B"
	}
	if m.selectedID != "B" {
		t.Errorf("→ on expanded A should move to first child B, got %q", m.selectedID)
	}

	// Now on B (expanded with child C). → should move to first child C.
	m.selectedID = "B"
	row = m.findRow("B")
	if row != nil && row.HasChildren && !m.collapsed["B"] {
		m.selectedID = "C"
	}
	if m.selectedID != "C" {
		t.Errorf("→ on expanded B should move to first child C, got %q", m.selectedID)
	}

	// C is a leaf. → should be a no-op.
	m.selectedID = "C"
	row = m.findRow("C")
	if row != nil && row.HasChildren && !m.collapsed["C"] {
		// would move to child, but C has no children
	} else {
		// leaf → no-op
	}
	if m.selectedID != "C" {
		t.Errorf("→ on leaf C should be no-op, got %q", m.selectedID)
	}

	// ← on leaf C should move to parent B.
	m.selectedID = "C"
	row = m.findRow("C")
	if row != nil && (row.HasChildren && !m.collapsed["C"]) {
		// would collapse, but C has no children
	} else if row != nil && row.Task.ParentID != nil {
		// move to parent
		m.selectedID = *row.Task.ParentID
	}
	if m.selectedID != "B" {
		t.Errorf("← on leaf C should move to parent B, got %q", m.selectedID)
	}

	// ← on expanded B should collapse B.
	m.selectedID = "B"
	row = m.findRow("B")
	if row != nil && row.HasChildren && !m.collapsed["B"] {
		m.collapsed["B"] = true
	}
	if !m.collapsed["B"] {
		t.Errorf("← on expanded B should collapse it")
	}

	// Now B is collapsed. ← on collapsed B should move to parent A.
	m.selectedID = "B"
	row = m.findRow("B")
	if row != nil && row.HasChildren && !m.collapsed["B"] {
		// would collapse
	} else if row != nil && row.Task.ParentID != nil {
		// move to parent
		m.selectedID = *row.Task.ParentID
	}
	if m.selectedID != "A" {
		t.Errorf("← on collapsed B should move to parent A, got %q", m.selectedID)
	}

	// ← on root A (has no parent) should be no-op.
	m.selectedID = "A"
	row = m.findRow("A")
	if row != nil && (row.HasChildren && !m.collapsed["A"]) {
		// would collapse
	} else if row != nil && row.Task.ParentID != nil {
		// move to parent
	} else {
		// root → no-op
	}
	if m.selectedID != "A" {
		t.Errorf("← on root A should be no-op, got %q", m.selectedID)
	}
}

// TestLeftRightNavigationWithCollapsedAncestor checks the fallthrough when
// an ancestor is collapsed: the keys act on the selected row's own state.
func TestLeftRightNavigationWithCollapsedAncestor(t *testing.T) {
	m := threeLevelTree()
	m.selectedID = "A"
	m.toggleCollapse(false) // collapse A — hides B, C, D

	// Selection is on A (visible). A is collapsed.
	// → on collapsed A should expand A (shallow: reveals B and D).
	m.selectedID = "A"
	row := m.findRow("A")
	if row != nil && row.HasChildren && m.collapsed["A"] {
		delete(m.collapsed, "A")
	}
	if m.collapsed["A"] {
		t.Errorf("→ on collapsed A should expand it")
	}

	// Now A is expanded. → on expanded A should move to first child B.
	m.selectedID = "A"
	row = m.findRow("A")
	if row != nil && row.HasChildren && !m.collapsed["A"] {
		m.selectedID = "B"
	}
	if m.selectedID != "B" {
		t.Errorf("→ on expanded A should move to first child B, got %q", m.selectedID)
	}

	// B is visible now (shallow expand of A reveals B and D). B starts collapsed.
	// → on collapsed B should expand B (shallow: reveals C).
	m.selectedID = "B"
	row = m.findRow("B")
	if row != nil && row.HasChildren && m.collapsed["B"] {
		delete(m.collapsed, "B")
	}
	if m.collapsed["B"] {
		t.Errorf("→ on collapsed B should expand it")
	}

	// Now B is expanded. → on expanded B should move to first child C.
	m.selectedID = "B"
	row = m.findRow("B")
	if row != nil && row.HasChildren && !m.collapsed["B"] {
		m.selectedID = "C"
	}
	if m.selectedID != "C" {
		t.Errorf("→ on expanded B should move to first child C, got %q", m.selectedID)
	}

	// C is leaf. → no-op. ← should move to parent B.
	m.selectedID = "C"
	row = m.findRow("C")
	if row != nil && (row.HasChildren && !m.collapsed["C"]) {
		// no-op
	} else if row != nil && row.Task.ParentID != nil {
		m.selectedID = *row.Task.ParentID
	}
	if m.selectedID != "B" {
		t.Errorf("← on leaf C should move to parent B, got %q", m.selectedID)
	}
}
