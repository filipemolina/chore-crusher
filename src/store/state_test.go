package store

import (
	"strings"
	"testing"
)

// statusOf is a small helper asserting a task can be read.
func mustGet(t *testing.T, s *Store, id string) Task {
	t.Helper()
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask(%s): %v", id, err)
	}
	return task
}

// TestCompleteCascadesToAllDescendants — "completing a task with pending
// children cascades to every descendant, at every depth (three levels deep,
// not just parent→child)".
func TestCompleteCascadesToAllDescendants(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	root, child, grand := threeLevelTree(t, s, lid)

	if err := s.Complete(root); err != nil {
		t.Fatalf("Complete(root): %v", err)
	}

	for _, id := range []string{root, child, grand} {
		task := mustGet(t, s, id)
		if task.Status != StatusComplete {
			t.Fatalf("task %s status = %s, want complete", id, task.Status)
		}
		if task.ProgressKind != ProgressNone {
			t.Fatalf("task %s progress_kind = %s, want none after completion", id, task.ProgressKind)
		}
		if task.CompletedAt == nil {
			t.Fatalf("task %s has no completed_at after completion", id)
		}
	}
}

// TestCompletePromotesSubtasksParentsChain — "completing a task whose parent
// is in subtasks mode with no other pending siblings promotes the parent to
// complete too, and that promotion walks up again if the grandparent is also
// subtasks-mode and now fully complete."
func TestCompletePromotesSubtasksParentsChain(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	grand := mustTask(t, s, lid, "grandparent", nil)
	parent := mustTask(t, s, lid, "parent", &grand)
	c1 := mustTask(t, s, lid, "child 1", &parent)
	c2 := mustTask(t, s, lid, "child 2", &parent)

	if err := s.SetProgress(parent, ProgressSubtasks, nil); err != nil {
		t.Fatalf("SetProgress(parent, subtasks): %v", err)
	}
	if err := s.SetProgress(grand, ProgressSubtasks, nil); err != nil {
		t.Fatalf("SetProgress(grand, subtasks): %v", err)
	}

	if err := s.Complete(c1); err != nil {
		t.Fatalf("Complete(c1): %v", err)
	}
	if got := mustGet(t, s, parent).Status; got != StatusInProgress {
		t.Fatalf("parent status after one child complete = %s, want in_progress", got)
	}

	if err := s.Complete(c2); err != nil {
		t.Fatalf("Complete(c2): %v", err)
	}
	if got := mustGet(t, s, parent).Status; got != StatusComplete {
		t.Fatalf("parent status after all children complete = %s, want complete", got)
	}
	if got := mustGet(t, s, grand).Status; got != StatusComplete {
		t.Fatalf("grandparent status after child promoted = %s, want complete (walk must continue past one level)", got)
	}
}

// TestCompleteDoesNotPromoteSubtasksParentWithPendingSiblings.
func TestCompleteDoesNotPromoteSubtasksParentWithPendingSiblings(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	parent := mustTask(t, s, lid, "parent", nil)
	c1 := mustTask(t, s, lid, "child 1", &parent)
	mustTask(t, s, lid, "child 2", &parent)

	if err := s.SetProgress(parent, ProgressSubtasks, nil); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}
	if err := s.Complete(c1); err != nil {
		t.Fatalf("Complete(c1): %v", err)
	}

	if got := mustGet(t, s, parent).Status; got != StatusInProgress {
		t.Fatalf("parent with a pending sibling promoted to %s, want in_progress", got)
	}
}

// TestSetProgressSubtasksZeroChildrenIsSimple — "setting progress_kind =
// subtasks on a task with zero children succeeds, and DerivedProgress reports
// displayAsSimple = true for it."
// TestToggleFlipsCompleteAndPending — "toggle = whichever applies": a
// pending task toggles to complete with the cascade, a complete task toggles
// back to pending without cascading (reopen is lossy, docs/DESIGN.md §3).
func TestToggleFlipsCompleteAndPending(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	root := mustTask(t, s, lid, "root", nil)
	child := mustTask(t, s, lid, "child", &root)

	if err := s.Toggle(root); err != nil {
		t.Fatalf("Toggle(pending): %v", err)
	}
	if got := mustGet(t, s, root).Status; got != StatusComplete {
		t.Errorf("root after first toggle: got %q, want complete", got)
	}
	if got := mustGet(t, s, child).Status; got != StatusComplete {
		t.Errorf("child after cascade: got %q, want complete", got)
	}

	if err := s.Toggle(root); err != nil {
		t.Fatalf("Toggle(complete): %v", err)
	}
	if got := mustGet(t, s, root).Status; got != StatusPending {
		t.Errorf("root after second toggle: got %q, want pending", got)
	}
	if got := mustGet(t, s, child).Status; got != StatusComplete {
		t.Errorf("child after reopen must stay complete, got %q", got)
	}
}

func TestSetProgressSubtasksZeroChildrenIsSimple(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	if err := s.SetProgress(id, ProgressSubtasks, nil); err != nil {
		t.Fatalf("SetProgress(subtasks, no children): %v", err)
	}
	kind, pct, simple, err := s.DerivedProgress(id)
	if err != nil {
		t.Fatalf("DerivedProgress: %v", err)
	}
	if kind != ProgressSubtasks {
		t.Fatalf("DerivedProgress kind = %s, want subtasks", kind)
	}
	if pct != 0 {
		t.Fatalf("DerivedProgress pct = %d, want 0", pct)
	}
	if !simple {
		t.Fatal("DerivedProgress displayAsSimple = false, want true (zero children falls back to simple)")
	}
}

// TestPercentage100DoesNotAutoComplete.
func TestPercentage100DoesNotAutoComplete(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	if err := s.SetProgress(id, ProgressPercentage, intptr(100)); err != nil {
		t.Fatalf("SetProgress(percentage, 100): %v", err)
	}
	got := mustGet(t, s, id)
	if got.Status != StatusInProgress {
		t.Fatalf("status after percentage 100 = %s, want in_progress (a claim, not a verified fact)", got.Status)
	}

	// ...and DerivedProgress still reports the honest 100, not complete.
	kind, pct, simple, err := s.DerivedProgress(id)
	if err != nil {
		t.Fatalf("DerivedProgress: %v", err)
	}
	if kind != ProgressPercentage || pct != 100 || simple {
		t.Fatalf("DerivedProgress = %s/%d/simple=%v, want percentage/100/false", kind, pct, simple)
	}
}

// TestReopenReopensOnlyThatTask — "Reopen on a complete task with complete
// children reopens only that task; the children remain complete."
func TestReopenReopensOnlyThatTask(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	root, child, grand := threeLevelTree(t, s, lid)

	if err := s.Complete(root); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if err := s.Reopen(root); err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	rt := mustGet(t, s, root)
	if rt.Status != StatusPending {
		t.Fatalf("reopened task status = %s, want pending", rt.Status)
	}
	if rt.CompletedAt != nil {
		t.Fatalf("reopened task still has completed_at set")
	}
	if rt.ProgressKind != ProgressNone || rt.ProgressPct != nil {
		t.Fatalf("reopened task kept progress state: kind=%s pct=%v", rt.ProgressKind, rt.ProgressPct)
	}
	for _, id := range []string{child, grand} {
		if got := mustGet(t, s, id).Status; got != StatusComplete {
			t.Fatalf("child %s status = %s after reopening its parent, want complete (reopen does not cascade)", id, got)
		}
	}
}

// TestSetProgressOnCompleteTaskErrors.
func TestSetProgressOnCompleteTaskErrors(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	if err := s.Complete(id); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	err := s.SetProgress(id, ProgressSimple, nil)
	if err == nil {
		t.Fatal("SetProgress on a complete task did not error")
	}
	if !strings.Contains(err.Error(), "complete") {
		t.Fatalf("SetProgress error %q does not say the task is complete", err)
	}
}

// TestSetProgressValidation — percent 0..100 and only with percentage kind.
func TestSetProgressValidation(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	if err := s.SetProgress(id, ProgressPercentage, nil); err == nil {
		t.Fatal("percentage without a percent did not error")
	}
	if err := s.SetProgress(id, ProgressPercentage, intptr(-1)); err == nil {
		t.Fatal("percent -1 did not error")
	}
	if err := s.SetProgress(id, ProgressPercentage, intptr(101)); err == nil {
		t.Fatal("percent 101 did not error")
	}
	if err := s.SetProgress(id, ProgressSimple, intptr(50)); err == nil {
		t.Fatal("percent alongside simple did not error")
	}
	if err := s.SetProgress(id, ProgressSubtasks, intptr(50)); err == nil {
		t.Fatal("percent alongside subtasks did not error")
	}
	if err := s.SetProgress(id, ProgressKind("bogus"), nil); err == nil {
		t.Fatal("unknown progress kind did not error")
	}
}

// TestSetProgressStartsPendingTask — setting any progress implies it has
// started (docs/DESIGN.md §3).
func TestSetProgressStartsPendingTask(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	if err := s.SetProgress(id, ProgressSimple, nil); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}
	if got := mustGet(t, s, id).Status; got != StatusInProgress {
		t.Fatalf("status after SetProgress = %s, want in_progress", got)
	}
}

// TestDerivedProgressSubtasksPercent — the derived percentage over direct
// children only, with rounding.
func TestDerivedProgressSubtasksPercent(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	parent := mustTask(t, s, lid, "parent", nil)
	c1 := mustTask(t, s, lid, "child 1", &parent)
	c2 := mustTask(t, s, lid, "child 2", &parent)
	c3 := mustTask(t, s, lid, "child 3", &parent)

	if err := s.SetProgress(parent, ProgressSubtasks, nil); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}
	if err := s.Complete(c1); err != nil {
		t.Fatalf("Complete(c1): %v", err)
	}

	kind, pct, simple, err := s.DerivedProgress(parent)
	if err != nil {
		t.Fatalf("DerivedProgress: %v", err)
	}
	if kind != ProgressSubtasks || simple {
		t.Fatalf("DerivedProgress = %s/simple=%v, want subtasks/false", kind, simple)
	}
	if pct != 33 { // round(100/3)
		t.Fatalf("derived pct = %d, want 33", pct)
	}

	// A second complete child derives 67 (round(200/3)).
	if err := s.Complete(c2); err != nil {
		t.Fatalf("Complete(c2): %v", err)
	}
	_, pct, _, err = s.DerivedProgress(parent)
	if err != nil {
		t.Fatalf("DerivedProgress: %v", err)
	}
	if pct != 67 {
		t.Fatalf("derived pct after 2/3 complete = %d, want 67", pct)
	}

	// The last child completion auto-promotes the parent (subtasks at 100% is
	// a verified fact), so the derived 100 is no longer observable on this
	// task — it is complete. The 100% read is reachable instead when the mode
	// is set after the children are already done (Reopen + SetProgress),
	// since the auto-promotion path never fired.
	if err := s.Complete(c3); err != nil {
		t.Fatalf("Complete(c3): %v", err)
	}
	if got := mustGet(t, s, parent).Status; got != StatusComplete {
		t.Fatalf("parent after all children complete = %s, want complete", got)
	}

	p2 := mustTask(t, s, lid, "parent 2", nil)
	d1 := mustTask(t, s, lid, "child of p2", &p2)
	d2 := mustTask(t, s, lid, "other child of p2", &p2)
	if err := s.Complete(d1); err != nil {
		t.Fatalf("Complete(d1): %v", err)
	}
	if err := s.Complete(d2); err != nil {
		t.Fatalf("Complete(d2): %v", err)
	}
	if err := s.SetProgress(p2, ProgressSubtasks, nil); err != nil {
		t.Fatalf("SetProgress(p2, subtasks): %v", err)
	}
	kind, pct, _, err = s.DerivedProgress(p2)
	if err != nil {
		t.Fatalf("DerivedProgress(p2): %v", err)
	}
	if kind != ProgressSubtasks || pct != 100 {
		t.Fatalf("DerivedProgress(p2) = %s/%d, want subtasks/100", kind, pct)
	}
}

// TestDerivedProgressSimpleMode — a simple-mode task reports no percentage.
func TestDerivedProgressSimpleMode(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	if err := s.SetProgress(id, ProgressSimple, nil); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}
	kind, pct, simple, err := s.DerivedProgress(id)
	if err != nil {
		t.Fatalf("DerivedProgress: %v", err)
	}
	if kind != ProgressSimple || pct != 0 || !simple {
		t.Fatalf("DerivedProgress = %s/%d/simple=%v, want simple/0/true", kind, pct, simple)
	}
}

// TestCompleteIsIdempotentAndReopenCompletesTheCycle — a complete/reopen pair
// on the same task round-trips without error.
func TestCompleteIsIdempotentAndReopenCompletesTheCycle(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	if err := s.Complete(id); err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	if err := s.Complete(id); err != nil {
		t.Fatalf("second Complete (already complete): %v", err)
	}
	if err := s.Reopen(id); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if err := s.Reopen(id); err != nil {
		t.Fatalf("second Reopen (already pending): %v", err)
	}
}
