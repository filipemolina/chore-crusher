package store

import (
	"errors"
	"strings"
	"testing"
)

// setPriorityDirect sets a task's priority straight in the database. The
// public SetPriority API lands in step 4 of the assignment plan; these
// step-3 tests only need the column set.
func setPriorityDirect(t *testing.T, s *Store, taskID string, p Priority) {
	t.Helper()
	if _, err := s.db.Exec(`UPDATE Task SET priority = ? WHERE id = ?`, p, taskID); err != nil {
		t.Fatalf("set priority on %s: %v", taskID, err)
	}
}

func TestAssignTaskConflictAndForce(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	if err := s.AssignTask(id, "alpha", false); err != nil {
		t.Fatalf("AssignTask(alpha): %v", err)
	}
	got, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Assignee != "alpha" || got.AssignedAt == nil {
		t.Fatalf("after assign: assignee=%q assigned_at=%v, want alpha with a timestamp", got.Assignee, got.AssignedAt)
	}

	err = s.AssignTask(id, "beta", false)
	if !errors.Is(err, ErrAssigned) {
		t.Fatalf("second AssignTask error = %v, want ErrAssigned", err)
	}
	if !strings.Contains(err.Error(), "alpha") {
		t.Fatalf("conflict error %q does not name the holder alpha", err)
	}

	// The holder re-assigning to itself is a no-op refresh, not a conflict.
	if err := s.AssignTask(id, "alpha", false); err != nil {
		t.Fatalf("self re-assign: %v", err)
	}

	if err := s.AssignTask(id, "beta", true); err != nil {
		t.Fatalf("force AssignTask: %v", err)
	}
	got, err = s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask after force: %v", err)
	}
	if got.Assignee != "beta" {
		t.Fatalf("after force: assignee=%q, want beta", got.Assignee)
	}
}

func TestAssignTaskSubtreeReservation(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	root, child, grand := threeLevelTree(t, s, lid)
	if err := s.AssignTask(root, "alpha", false); err != nil {
		t.Fatalf("AssignTask(root, alpha): %v", err)
	}

	// A descendant of alpha's task is refused for beta — ancestor held.
	for _, id := range []string{child, grand} {
		err := s.AssignTask(id, "beta", false)
		if !errors.Is(err, ErrAssigned) {
			t.Fatalf("AssignTask(%s, beta) error = %v, want ErrAssigned (ancestor held)", id, err)
		}
	}
	// The holder itself is not blocked by its own reservation.
	if err := s.AssignTask(child, "alpha", false); err != nil {
		t.Fatalf("AssignTask(child, alpha): %v", err)
	}

	// A separate tree: the ancestor is refused when a descendant is held.
	root2 := mustTask(t, s, lid, "root2", nil)
	child2 := mustTask(t, s, lid, "child2", &root2)
	if err := s.AssignTask(child2, "alpha", false); err != nil {
		t.Fatalf("AssignTask(child2, alpha): %v", err)
	}
	if err := s.UnassignTask(child, "alpha", false); err != nil {
		t.Fatalf("UnassignTask(child): %v", err)
	}
	err := s.AssignTask(root2, "beta", false)
	if !errors.Is(err, ErrAssigned) {
		t.Fatalf("AssignTask(root2, beta) error = %v, want ErrAssigned (descendant held)", err)
	}
	// force does not override the subtree invariant either.
	if err := s.AssignTask(root2, "beta", true); !errors.Is(err, ErrAssigned) {
		t.Fatalf("force AssignTask(root2, beta) error = %v, want ErrAssigned", err)
	}
}

// TestSubtreeConflictIsDistinguishable pins the difference the MCP layer has
// to render (plan §4): a conflict on the task itself is fixable with
// force=true, a conflict via the subtree is NOT — force overrides a holder,
// never decision 4's invariant. Both wrap ErrAssigned; only the subtree case
// wraps ErrSubtreeAssigned, and step 9 must key its hint off that. Without
// the distinction the error tells an agent to "pass force=true to take it"
// in the one case where force cannot.
func TestSubtreeConflictIsDistinguishable(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	root, child, _ := threeLevelTree(t, s, lid)

	if err := s.AssignTask(child, "alpha", false); err != nil {
		t.Fatalf("AssignTask(child, alpha): %v", err)
	}

	// Direct conflict on the row: force is the documented way out.
	direct := s.AssignTask(child, "beta", false)
	if !errors.Is(direct, ErrAssigned) {
		t.Fatalf("direct conflict = %v, want ErrAssigned", direct)
	}
	if errors.Is(direct, ErrSubtreeAssigned) {
		t.Fatalf("direct conflict must not report as a subtree conflict: %v", direct)
	}
	if err := s.AssignTask(child, "beta", true); err != nil {
		t.Fatalf("force on a direct conflict must succeed, got %v", err)
	}

	// Subtree conflict: force is not a way out, so it must be marked.
	viaSubtree := s.AssignTask(root, "gamma", false)
	if !errors.Is(viaSubtree, ErrSubtreeAssigned) {
		t.Fatalf("subtree conflict = %v, want ErrSubtreeAssigned", viaSubtree)
	}
	if err := s.AssignTask(root, "gamma", true); !errors.Is(err, ErrSubtreeAssigned) {
		t.Fatalf("force on a subtree conflict = %v, want it still refused", err)
	}

	// The escape hatch the hint must point at: release the blocker, then grab.
	if err := s.UnassignTask(child, "gamma", true); err != nil {
		t.Fatalf("force UnassignTask(child): %v", err)
	}
	if err := s.AssignTask(root, "gamma", false); err != nil {
		t.Fatalf("AssignTask(root) after releasing the blocker: %v", err)
	}
}

func TestUnassignTask(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	// Releasing an unassigned task is a silent no-op.
	if err := s.UnassignTask(id, "alpha", false); err != nil {
		t.Fatalf("UnassignTask on unassigned task: %v", err)
	}

	if err := s.AssignTask(id, "alpha", false); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}

	// A different agent cannot release without force.
	err := s.UnassignTask(id, "beta", false)
	if !errors.Is(err, ErrAssigned) {
		t.Fatalf("UnassignTask(beta) error = %v, want ErrAssigned", err)
	}

	// The holder can.
	if err := s.UnassignTask(id, "alpha", false); err != nil {
		t.Fatalf("UnassignTask(alpha): %v", err)
	}
	got, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Assignee != "" || got.AssignedAt != nil {
		t.Fatalf("after unassign: assignee=%q assigned_at=%v, want cleared", got.Assignee, got.AssignedAt)
	}
}

func TestUnassignListCount(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	a := mustTask(t, s, lid, "a", nil)
	b := mustTask(t, s, lid, "b", nil)
	mustTask(t, s, lid, "c", nil) // stays unassigned

	if err := s.AssignTask(a, "alpha", false); err != nil {
		t.Fatalf("AssignTask(a): %v", err)
	}
	if err := s.AssignTask(b, "beta", false); err != nil {
		t.Fatalf("AssignTask(b): %v", err)
	}

	n, err := s.UnassignList(lid)
	if err != nil {
		t.Fatalf("UnassignList: %v", err)
	}
	if n != 2 {
		t.Fatalf("UnassignList freed %d tasks, want 2", n)
	}
	for _, id := range []string{a, b} {
		got, err := s.GetTask(id)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", id, err)
		}
		if got.Assignee != "" || got.AssignedAt != nil {
			t.Fatalf("task %s after UnassignList: assignee=%q assigned_at=%v, want cleared", id, got.Assignee, got.AssignedAt)
		}
	}

	// A second release has nothing left to free.
	if n, err := s.UnassignList(lid); err != nil || n != 0 {
		t.Fatalf("second UnassignList = %d, %v; want 0, nil", n, err)
	}
}

func TestCompleteClearsAssignment(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	root, child, grand := threeLevelTree(t, s, lid)

	if err := s.AssignTask(root, "alpha", false); err != nil {
		t.Fatalf("AssignTask(root): %v", err)
	}
	if err := s.AssignTask(child, "alpha", false); err != nil {
		t.Fatalf("AssignTask(child): %v", err)
	}
	if err := s.AssignTask(grand, "alpha", false); err != nil {
		t.Fatalf("AssignTask(grand): %v", err)
	}

	if err := s.Complete(root); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	for _, id := range []string{root, child, grand} {
		got, err := s.GetTask(id)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", id, err)
		}
		if got.Status != StatusComplete {
			t.Fatalf("task %s status = %q, want complete", id, got.Status)
		}
		if got.Assignee != "" || got.AssignedAt != nil {
			t.Fatalf("task %s still assigned to %q after Complete", id, got.Assignee)
		}
	}
}

// TestCompleteClearsAssignmentOnPromotedAncestor covers the third path into
// setComplete: not the task, not the cascade, but a subtasks-mode ancestor
// promoted because its last child finished. A promoted ancestor is complete,
// and a complete task has no owner (decision 5) — otherwise finishing the
// last subtask silently leaves the parent in the abandoned state the TUI's
// stale tier is meant to flag (assigned, complete, nobody working).
func TestCompleteClearsAssignmentOnPromotedAncestor(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	parent := mustTask(t, s, lid, "parent", nil)
	only := mustTask(t, s, lid, "only child", &parent)
	if err := s.SetProgress(parent, ProgressSubtasks, nil); err != nil {
		t.Fatalf("SetProgress(parent, subtasks): %v", err)
	}
	if err := s.AssignTask(parent, "alpha", false); err != nil {
		t.Fatalf("AssignTask(parent): %v", err)
	}

	// Completing the only child promotes the parent — a different code path
	// from Complete(parent), which would clear it via the cascade.
	if err := s.Complete(only); err != nil {
		t.Fatalf("Complete(only): %v", err)
	}

	got, err := s.GetTask(parent)
	if err != nil {
		t.Fatalf("GetTask(parent): %v", err)
	}
	if got.Status != StatusComplete {
		t.Fatalf("parent status = %q, want complete (promotion did not fire)", got.Status)
	}
	if got.Assignee != "" || got.AssignedAt != nil {
		t.Fatalf("promoted ancestor still assigned to %q (assigned_at=%v)", got.Assignee, got.AssignedAt)
	}
}

func TestAssignmentSurvivesReleaseAllClaims(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	if err := s.AssignTask(id, "alpha", false); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	// A live presence claim next to the assignment, so ReleaseAllClaims has
	// something real to clear — the assertion is about the assignment
	// surviving an actual purge, not a no-op one.
	if _, err := s.ClaimWork("task", id, "alpha", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}

	n, err := s.ReleaseAllClaims()
	if err != nil {
		t.Fatalf("ReleaseAllClaims: %v", err)
	}
	if n != 1 {
		t.Fatalf("ReleaseAllClaims cleared %d claims, want 1", n)
	}

	got, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Assignee != "alpha" {
		t.Fatalf("assignment lost after ReleaseAllClaims: assignee=%q, want alpha", got.Assignee)
	}
}

func TestNextAssignablePriorityAndPreorder(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	// Preorder a, b, c: a and c are both "none", so preorder is the
	// tie-break inside the lowest rank.
	a := mustTask(t, s, lid, "a", nil)
	b := mustTask(t, s, lid, "b", nil)
	c := mustTask(t, s, lid, "c", nil)
	setPriorityDirect(t, s, b, PriorityHigh)

	got, err := s.NextAssignable(lid, "alpha")
	if err != nil {
		t.Fatalf("NextAssignable: %v", err)
	}
	if got.ID != b {
		t.Fatalf("first grab = %s, want %s (high beats none)", got.ID, b)
	}
	if got.Assignee != "alpha" {
		t.Fatalf("grabbed task assignee = %q, want alpha", got.Assignee)
	}

	got, err = s.NextAssignable(lid, "alpha")
	if err != nil {
		t.Fatalf("second NextAssignable: %v", err)
	}
	if got.ID != a {
		t.Fatalf("second grab = %s, want %s (preorder tie-break)", got.ID, a)
	}

	got, err = s.NextAssignable(lid, "alpha")
	if err != nil {
		t.Fatalf("third NextAssignable: %v", err)
	}
	if got.ID != c {
		t.Fatalf("third grab = %s, want %s", got.ID, c)
	}

	if _, err := s.NextAssignable(lid, "alpha"); !errors.Is(err, ErrNoAssignable) {
		t.Fatalf("exhausted NextAssignable error = %v, want ErrNoAssignable", err)
	}
}

func TestNextAssignableSkipsAssignedAndComplete(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	a := mustTask(t, s, lid, "a", nil) // will be assigned to another agent
	b := mustTask(t, s, lid, "b", nil) // complete
	c := mustTask(t, s, lid, "c", nil) // eligible
	root := mustTask(t, s, lid, "root", nil)
	mustTask(t, s, lid, "child", &root) // ancestor held by another agent

	if err := s.AssignTask(a, "other", false); err != nil {
		t.Fatalf("AssignTask(a): %v", err)
	}
	if err := s.AssignTask(root, "other", false); err != nil {
		t.Fatalf("AssignTask(root): %v", err)
	}
	if err := s.Complete(b); err != nil {
		t.Fatalf("Complete(b): %v", err)
	}

	got, err := s.NextAssignable(lid, "alpha")
	if err != nil {
		t.Fatalf("NextAssignable: %v", err)
	}
	if got.ID != c {
		t.Fatalf("grab = %s, want %s (a held, b complete, child's ancestor held)", got.ID, c)
	}

	// c now held too; only child remains, blocked by its ancestor.
	if _, err := s.NextAssignable(lid, "alpha"); !errors.Is(err, ErrNoAssignable) {
		t.Fatalf("error = %v, want ErrNoAssignable", err)
	}
}
