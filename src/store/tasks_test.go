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
