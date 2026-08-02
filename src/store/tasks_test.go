package store

import (
	"strings"
	"testing"
)

func TestCreateTaskValidations(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	parent := mustTask(t, s, lid, "parent", nil)

	if _, err := s.CreateTask(lid, "   ", nil, ""); err == nil {
		t.Fatal("CreateTask accepted a blank title")
	}
	if _, err := s.CreateTask("no-such-list", "x", nil, ""); err == nil {
		t.Fatal("CreateTask accepted a nonexistent list")
	}
	if _, err := s.CreateTask(lid, "x", strptr("no-such-parent"), ""); err == nil {
		t.Fatal("CreateTask accepted a nonexistent parent")
	}

	// Cross-list parent: a task belongs to exactly one list, and a parent in
	// another list would orphan the task from every tree reader scoped by
	// list_id (docs/DESIGN.md §2).
	other := mustList(t, s, "other list")
	if _, err := s.CreateTask(other, "x", &parent, ""); err == nil {
		t.Fatal("CreateTask accepted a parent from a different list")
	}
}

func TestCreateTaskAssignsParentAndPosition(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	a := mustTask(t, s, lid, "a", nil)
	b := mustTask(t, s, lid, "b", &a)
	c := mustTask(t, s, lid, "c", nil)

	bt, err := s.GetTask(b)
	if err != nil {
		t.Fatalf("GetTask(b): %v", err)
	}
	if bt.ParentID == nil || *bt.ParentID != a {
		t.Fatalf("b's parent = %v, want %s", bt.ParentID, a)
	}

	tasks, err := s.ListTasks(lid)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	// Creation order: a, b, c — b is a's child but appended after a.
	if len(tasks) != 3 {
		t.Fatalf("ListTasks returned %d rows, want 3", len(tasks))
	}
	if tasks[0].ID != a || tasks[1].ID != b || tasks[2].ID != c {
		t.Fatalf("ListTasks order = %s, %s, %s; want a, b, c", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetTask("no-such-id"); err == nil {
		t.Fatal("GetTask on a missing id did not error")
	}
}

func TestRenameTask(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "old title", nil)

	if err := s.RenameTask(id, "new title"); err != nil {
		t.Fatalf("RenameTask: %v", err)
	}
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Title != "new title" {
		t.Fatalf("renamed task has title %q, want %q", task.Title, "new title")
	}
	if err := s.RenameTask("no-such-id", "x"); err == nil {
		t.Fatal("RenameTask on a missing id did not error")
	}
}

func TestSetNotes(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	if err := s.SetNotes(id, "first notes"); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	// SetNotes replaces, not appends.
	if err := s.SetNotes(id, "replacement"); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Notes != "replacement" {
		t.Fatalf("notes = %q, want %q", task.Notes, "replacement")
	}
}

// TestDeleteTaskCascadesToDescendants is the required coverage for the
// "removes every descendant of it and leaves sibling subtrees untouched" rule.
func TestDeleteTaskCascadesToDescendants(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	root, child, grand := threeLevelTree(t, s, lid)
	sibRoot := mustTask(t, s, lid, "sibling root", nil)
	sibChild := mustTask(t, s, lid, "sibling child", &sibRoot)

	if err := s.DeleteTask(child); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	if _, err := s.GetTask(child); err == nil {
		t.Fatal("deleted task still readable")
	}
	if _, err := s.GetTask(grand); err == nil {
		t.Fatal("descendant of deleted task still readable")
	}
	// The sibling subtree must be untouched, including its own child.
	if _, err := s.GetTask(sibRoot); err != nil {
		t.Fatalf("sibling root lost: %v", err)
	}
	if _, err := s.GetTask(sibChild); err != nil {
		t.Fatalf("sibling child lost: %v", err)
	}
	// The deleted subtree's parent keeps its own identity.
	if _, err := s.GetTask(root); err != nil {
		t.Fatalf("root of deleted subtree lost: %v", err)
	}

	// The task that was deleted is gone from ListTasks too.
	tasks, err := s.ListTasks(lid)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 3 { // root, sibling root, sibling child
		t.Fatalf("ListTasks returned %d rows, want 3", len(tasks))
	}
}

func TestDeleteTaskRequiresExisting(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteTask("no-such-id")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("DeleteTask on missing id error = %v, want a not-found error", err)
	}
}

func TestCreateTaskAfterInsertsBetweenSiblings(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	// Create initial siblings a, b, c
	a := mustTask(t, s, lid, "a", nil)
	b := mustTask(t, s, lid, "b", nil)
	c := mustTask(t, s, lid, "c", nil)

	// Insert x between a and b
	x, err := s.CreateTaskAfter(lid, "x", nil, "", a)
	if err != nil {
		t.Fatalf("CreateTaskAfter: %v", err)
	}

	tasks, err := s.ListTasks(lid)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	// Order should be: a, x, b, c
	if len(tasks) != 4 {
		t.Fatalf("ListTasks returned %d rows, want 4", len(tasks))
	}
	if tasks[0].ID != a || tasks[1].ID != x || tasks[2].ID != b || tasks[3].ID != c {
		t.Fatalf("ListTasks order = %s, %s, %s, %s; want a, x, b, c",
			tasks[0].ID, tasks[1].ID, tasks[2].ID, tasks[3].ID)
	}

	// Verify positions are correct
	for i, task := range tasks {
		if task.Position != i {
			t.Errorf("task %s position = %d, want %d", task.ID, task.Position, i)
		}
	}
}

func TestReparentMovesWithinTree(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	root := mustTask(t, s, lid, "root", nil)
	child := mustTask(t, s, lid, "child", &root)
	other := mustTask(t, s, lid, "other", nil)

	if err := s.Reparent(other, &child); err != nil {
		t.Fatalf("Reparent(other under child): %v", err)
	}
	task, err := s.GetTask(other)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.ParentID == nil || *task.ParentID != child {
		t.Fatalf("other's parent = %v, want %s", task.ParentID, child)
	}
	if task.ListID != lid {
		t.Fatalf("reparent changed the task's list to %s", task.ListID)
	}
}

// TestReparentCollapsesPositionsAndAppends checks that Reparent closes the
// gap in the old sibling run and appends to the end of the new parent's
// children — no position holes, and reordering comes only from later moves.
func TestReparentClosesGapsAndAppends(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	a := mustTask(t, s, lid, "a", nil)
	b := mustTask(t, s, lid, "b", nil)
	c := mustTask(t, s, lid, "c", nil)

	// Move the middle root b under a.
	if err := s.Reparent(b, &a); err != nil {
		t.Fatalf("Reparent: %v", err)
	}

	tasks, err := s.ListTasks(lid)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	pos := map[string]int{}
	for _, task := range tasks {
		pos[task.ID] = task.Position
	}
	// b left its root slot; the roots a and c are renumbered 0, 1.
	if pos[a] != 0 || pos[c] != 1 {
		t.Errorf("root positions = a:%d c:%d; want 0 and 1 after the gap closes", pos[a], pos[c])
	}
	bb, err := s.GetTask(b)
	if err != nil {
		t.Fatalf("GetTask(b): %v", err)
	}
	if bb.ParentID == nil || *bb.ParentID != a {
		t.Errorf("b's parent = %v after move, want a", bb.ParentID)
	}

	// Appending: a second reparent under a lands after b.
	d := mustTask(t, s, lid, "d", nil)
	if err := s.Reparent(d, &a); err != nil {
		t.Fatalf("Reparent(d under a): %v", err)
	}
	dd, _ := s.GetTask(d)
	bb2, _ := s.GetTask(b)
	if bb2.Position >= dd.Position {
		t.Errorf("b (pos %d) should precede d (pos %d) under a", bb2.Position, dd.Position)
	}
}

func TestReparentToRoot(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	_, child, grand := threeLevelTree(t, s, lid)

	// Move the grandchild to the list root (nil parent).
	if err := s.Reparent(grand, nil); err != nil {
		t.Fatalf("Reparent(grand, nil): %v", err)
	}
	task, err := s.GetTask(grand)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.ParentID != nil {
		t.Fatalf("grand's parent = %v after moving to root, want nil", task.ParentID)
	}
	if _, err := s.GetTask(child); err != nil {
		t.Fatalf("child should survive, got %v", err)
	}
}

func TestReparentCycleRejected(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	root, child, grand := threeLevelTree(t, s, lid)

	// Moving root under its own descendant grand would loop forever.
	if err := s.Reparent(root, &grand); err == nil {
		t.Fatal("Reparent root under its own descendant succeeded; must reject a cycle")
	}
	// Moving child under its own descendant grand is the same cycle.
	if err := s.Reparent(child, &grand); err == nil {
		t.Fatal("Reparent child under its own descendant succeeded; must reject a cycle")
	}
	if err := s.Reparent(root, &root); err == nil {
		t.Fatal("Reparent task under itself succeeded")
	}

	// A rejected move must not have mutated anything.
	tree, err := s.GetTask(root)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if tree.ParentID != nil || tree.Position != 0 {
		t.Fatalf("rejected move mutated root: parent %v position %d", tree.ParentID, tree.Position)
	}
}

func TestReparentCrossListRejected(t *testing.T) {
	s := newTestStore(t)
	l1 := mustList(t, s, "one")
	l2 := mustList(t, s, "two")
	task := mustTask(t, s, l1, "task", nil)
	parent := mustTask(t, s, l2, "parent", nil)

	if err := s.Reparent(task, &parent); err == nil || !strings.Contains(err.Error(), "different list") {
		t.Fatalf("Reparent across lists error = %v, want a different-list error", err)
	}
}

// TestReparentCompleteParentRuledOut pins docs/DESIGN.md §3's hard invariant:
// a complete ancestor may not gain a pending descendant, and reparent — a
// deliberate restructure, not an add — must not create that state silently.
func TestReparentCompleteParentRejected(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	done := mustTask(t, s, lid, "done", nil)
	if err := s.Complete(done); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	task := mustTask(t, s, lid, "still working", nil)

	err := s.Reparent(task, &done)
	if err == nil || !strings.Contains(err.Error(), "complete it first") {
		t.Fatalf("moving a pending task under a complete parent error = %v, want a complete-first error", err)
	}
	// A complete subtree may move under a complete parent (both sides complete).
	doneSub := mustTask(t, s, lid, "done subtree", nil)
	if err := s.Complete(doneSub); err != nil {
		t.Fatalf("Complete(doneSub): %v", err)
	}
	if err := s.Reparent(doneSub, &done); err != nil {
		t.Fatalf("moving a complete subtree under a complete parent should be allowed: %v", err)
	}
}

func TestReparentNoOpSameParent(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	root := mustTask(t, s, lid, "root", nil)
	child := mustTask(t, s, lid, "child", &root)

	if err := s.Reparent(child, &root); err != nil {
		t.Fatalf("reparenting to the current parent: %v", err)
	}
	task, err := s.GetTask(child)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.ParentID == nil || *task.ParentID != root {
		t.Fatalf("no-op reparent changed the parent to %v", task.ParentID)
	}
}

func TestCreateTaskAfterWithNoAfterIDAppends(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	a := mustTask(t, s, lid, "a", nil)
	b := mustTask(t, s, lid, "b", nil)

	// Call CreateTaskAfter with empty afterID — should append like CreateTask
	c, err := s.CreateTaskAfter(lid, "c", nil, "", "")
	if err != nil {
		t.Fatalf("CreateTaskAfter with empty afterID: %v", err)
	}

	tasks, err := s.ListTasks(lid)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	if len(tasks) != 3 {
		t.Fatalf("ListTasks returned %d rows, want 3", len(tasks))
	}
	if tasks[0].ID != a || tasks[1].ID != b || tasks[2].ID != c {
		t.Fatalf("ListTasks order = %s, %s, %s; want a, b, c",
			tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}
