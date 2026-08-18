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
	// Now p2 is complete because it is in subtasks mode and all children are complete.
	// To test setting progress to subtasks after children are complete, we must
	// reopen p2 first (which sets it to pending and progress_kind to none).
	if err := s.Reopen(p2); err != nil {
		t.Fatalf("Reopen(p2): %v", err)
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

// TestAutoSwitchParentToSubtasks pins the "Auto percentage on parent tasks"
// rule (backlog #7): a task that gains its first subtask switches to subtasks
// mode but does NOT start — a pending parent stays pending with a
// subtasks-derived percentage (docs/DESIGN.md §3: creating a subtask is
// planning, not starting). An explicit kind is never overridden, a complete
// parent is never touched, and a later sibling leaves an already-switched
// parent alone. Both add paths exercise it — CreateTask and CreateTaskAfter —
// so the CLI and the TUI cannot diverge.
func TestAutoSwitchParentToSubtasks(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "l")

	// A pending parent with no kind yet: the first child auto-switches it to
	// subtasks mode but leaves it pending — work has not begun on any child.
	parent := mustTask(t, s, lid, "parent", nil)
	mustTask(t, s, lid, "child", &parent)
	if p := mustGet(t, s, parent); p.ProgressKind != ProgressSubtasks || p.Status != StatusPending {
		t.Fatalf("parent after first child = kind %q status %q, want subtasks/pending",
			p.ProgressKind, p.Status)
	}
	kind, pct, simple, err := s.DerivedProgress(parent)
	if err != nil {
		t.Fatalf("DerivedProgress: %v", err)
	}
	if kind != ProgressSubtasks || pct != 0 || simple {
		t.Fatalf("DerivedProgress = %s/%d/simple=%v, want subtasks/0/false", kind, pct, simple)
	}

	// A second child leaves an already-switched parent alone.
	mustTask(t, s, lid, "child 2", &parent)
	if p := mustGet(t, s, parent); p.ProgressKind != ProgressSubtasks {
		t.Errorf("second child changed kind to %q, want subtasks", p.ProgressKind)
	}

	// An explicit kind is never overridden by a new child.
	explicit := mustTask(t, s, lid, "explicit", nil)
	if err := s.SetProgress(explicit, ProgressPercentage, intptr(40)); err != nil {
		t.Fatalf("SetProgress(explicit, percentage 40): %v", err)
	}
	mustTask(t, s, lid, "explicit child", &explicit)
	if e := mustGet(t, s, explicit); e.ProgressKind != ProgressPercentage || e.Status != StatusInProgress {
		t.Errorf("explicit-kind parent overridden to %q/%q after a child", e.ProgressKind, e.Status)
	}

	// A complete parent is reopened when a child is added, because a complete
	// task with a pending child is a forbidden state (docs/DESIGN.md §3).
	// The auto-switch to subtasks mode then applies to the reopened parent,
	// which stays pending until a child starts.
	done := mustTask(t, s, lid, "done", nil)
	if err := s.Complete(done); err != nil {
		t.Fatalf("Complete(done): %v", err)
	}
	mustTask(t, s, lid, "done child", &done)
	if d := mustGet(t, s, done); d.ProgressKind != ProgressSubtasks || d.Status != StatusPending {
		t.Errorf("complete parent should be reopened and switched to subtasks (pending), got %q/%q", d.ProgressKind, d.Status)
	}

	// CreateTaskAfter — the TUI's inline-create path — fires the same
	// auto-switch, so the CLI and the TUI cannot diverge on whether a new
	// subtask starts its parent. The append branch (afterID "") and the
	// positioned branch (after a sibling) both route through it.
	afterParent := mustTask(t, s, lid, "after parent", nil)
	if _, err := s.CreateTaskAfter(lid, "after child", &afterParent, "", ""); err != nil {
		t.Fatalf("CreateTaskAfter append: %v", err)
	}
	if a := mustGet(t, s, afterParent); a.ProgressKind != ProgressSubtasks || a.Status != StatusPending {
		t.Errorf("CreateTaskAfter parent = kind %q status %q, want subtasks/pending",
			a.ProgressKind, a.Status)
	}
	firstChild := mustTask(t, s, lid, "after child 2", &afterParent)
	if _, err := s.CreateTaskAfter(lid, "after child 3", &afterParent, "", firstChild); err != nil {
		t.Fatalf("CreateTaskAfter positioned: %v", err)
	}
	if a := mustGet(t, s, afterParent); a.ProgressKind != ProgressSubtasks {
		t.Errorf("positioned CreateTaskAfter changed kind to %q, want subtasks", a.ProgressKind)
	}
}

// TestParentStatusDerivedFromChildren pins the derived parent-status rule
// (docs/DESIGN.md §3): a subtasks-mode parent is in_progress when any direct
// child is in_progress or complete, and pending when every direct child is
// pending again. Creating a child does not start the parent; starting or
// completing a child does; reopening the last in-progress child returns the
// parent to pending.
func TestParentStatusDerivedFromChildren(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	// Creating a child under a pending/none parent switches it to subtasks
	// mode but leaves it pending — planning, not starting.
	parent := mustTask(t, s, lid, "parent", nil)
	c1 := mustTask(t, s, lid, "child 1", &parent)
	if p := mustGet(t, s, parent); p.ProgressKind != ProgressSubtasks || p.Status != StatusPending {
		t.Fatalf("parent after first child = kind %q status %q, want subtasks/pending", p.ProgressKind, p.Status)
	}

	// A child going in_progress starts the parent.
	if err := s.SetProgress(c1, ProgressSimple, nil); err != nil {
		t.Fatalf("SetProgress(c1, simple): %v", err)
	}
	if got := mustGet(t, s, parent).Status; got != StatusInProgress {
		t.Fatalf("parent after child in_progress = %s, want in_progress", got)
	}

	// A second child completing keeps the parent in_progress.
	c2 := mustTask(t, s, lid, "child 2", &parent)
	if err := s.Complete(c2); err != nil {
		t.Fatalf("Complete(c2): %v", err)
	}
	if got := mustGet(t, s, parent).Status; got != StatusInProgress {
		t.Fatalf("parent after child complete = %s, want in_progress", got)
	}

	// Reopening the last in-progress child (c1) leaves c2 complete, so the
	// parent stays in_progress.
	if err := s.Reopen(c1); err != nil {
		t.Fatalf("Reopen(c1): %v", err)
	}
	if got := mustGet(t, s, parent).Status; got != StatusInProgress {
		t.Fatalf("parent after reopening one of two children = %s, want in_progress (c2 still complete)", got)
	}

	// Reopening the remaining complete child returns the parent to pending.
	if err := s.Reopen(c2); err != nil {
		t.Fatalf("Reopen(c2): %v", err)
	}
	if got := mustGet(t, s, parent).Status; got != StatusPending {
		t.Fatalf("parent after all children pending = %s, want pending", got)
	}
}

// TestParentStatusAllChildrenCompletePromotes — completing every child of a
// subtasks-mode parent promotes it to complete (existing auto-completion),
// not merely to in_progress.
func TestParentStatusAllChildrenCompletePromotes(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	parent := mustTask(t, s, lid, "parent", nil)
	c1 := mustTask(t, s, lid, "child 1", &parent)
	c2 := mustTask(t, s, lid, "child 2", &parent)

	if err := s.Complete(c1); err != nil {
		t.Fatalf("Complete(c1): %v", err)
	}
	if got := mustGet(t, s, parent).Status; got != StatusInProgress {
		t.Fatalf("parent after one child complete = %s, want in_progress", got)
	}
	if err := s.Complete(c2); err != nil {
		t.Fatalf("Complete(c2): %v", err)
	}
	if got := mustGet(t, s, parent).Status; got != StatusComplete {
		t.Fatalf("parent after all children complete = %s, want complete", got)
	}
}

// TestParentStatusExplicitKindNotOverridden — a parent with an explicit
// progress kind is not started by its children's status changes.
func TestParentStatusExplicitKindNotOverridden(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	parent := mustTask(t, s, lid, "parent", nil)
	if err := s.SetProgress(parent, ProgressPercentage, intptr(40)); err != nil {
		t.Fatalf("SetProgress(parent, percentage 40): %v", err)
	}
	child := mustTask(t, s, lid, "child", &parent)
	if err := s.Complete(child); err != nil {
		t.Fatalf("Complete(child): %v", err)
	}
	p := mustGet(t, s, parent)
	if p.ProgressKind != ProgressPercentage || p.Status != StatusInProgress {
		t.Fatalf("explicit-kind parent changed to %q/%q after child complete, want percentage/in_progress", p.ProgressKind, p.Status)
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
