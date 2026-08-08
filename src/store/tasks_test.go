package store

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/filipemolina/chore-crusher/src/store/migrations"
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

// listOrder returns the preorder ids of every task in listID.
func listOrder(t *testing.T, s *Store, listID string) []string {
	t.Helper()
	tasks, err := s.ListTasks(listID)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	ids := make([]string, 0, len(tasks))
	for _, tk := range tasks {
		ids = append(ids, tk.ID)
	}
	return ids
}

// TestMoveTaskReordersWithinRun: moving a task after a later sibling shifts
// it down within the same parent run.
func TestMoveTaskReordersWithinRun(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	a := mustTask(t, s, lid, "a", nil)
	b := mustTask(t, s, lid, "b", nil)
	c := mustTask(t, s, lid, "c", nil)
	d := mustTask(t, s, lid, "d", nil)

	if err := s.MoveTask(b, d); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	want := []string{a, c, d, b}
	if got := listOrder(t, s, lid); !slices.Equal(got, want) {
		t.Errorf("order after move = %v, want %v", got, want)
	}
}

// TestMoveTaskToFront: an empty afterID moves the task to the front of its
// current parent run.
func TestMoveTaskToFront(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	a := mustTask(t, s, lid, "a", nil)
	b := mustTask(t, s, lid, "b", nil)
	c := mustTask(t, s, lid, "c", nil)

	if err := s.MoveTask(c, ""); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	want := []string{c, a, b}
	if got := listOrder(t, s, lid); !slices.Equal(got, want) {
		t.Errorf("order after move-to-front = %v, want %v", got, want)
	}
}

// TestMoveTaskOutdentsAfterParent: moving a task after its own parent makes
// it the parent's next sibling — the outdent gesture (docs/DESIGN.md §5).
func TestMoveTaskOutdentsAfterParent(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	root, child, grand := threeLevelTree(t, s, lid)

	if err := s.MoveTask(child, root); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	want := []string{root, child, grand}
	if got := listOrder(t, s, lid); !slices.Equal(got, want) {
		t.Errorf("order after outdent = %v, want %v", got, want)
	}

	tk, err := s.GetTask(child)
	if err != nil {
		t.Fatalf("GetTask(child): %v", err)
	}
	if tk.ParentID != nil {
		t.Errorf("outdented child still has parent %v, want root level", *tk.ParentID)
	}
}

// TestMoveTaskValidation pins the same validity rules Reparent enforces: the
// target must be in the same list, not the task itself, and not a descendant.
func TestMoveTaskValidation(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	other := mustList(t, s, "other")
	a := mustTask(t, s, lid, "a", nil)
	cross := mustTask(t, s, other, "cross", nil)

	if err := s.MoveTask(a, cross); err == nil {
		t.Error("MoveTask across lists did not error")
	}
	if err := s.MoveTask(a, a); err == nil {
		t.Error("MoveTask after itself did not error")
	}

	root, _, grand := threeLevelTree(t, s, lid)
	if err := s.MoveTask(root, grand); err == nil {
		t.Error("MoveTask after its own descendant did not error")
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

// TestTasksChangedSince pins the contract: a task created before `since` is
// absent from the result; one mutated after `since` (here via SetNotes) is
// present. Deletions are not representable — see TasksChangedSince's own
// docstring.
func TestTasksChangedSince(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "L")
	a := mustTask(t, s, lid, "a", nil)
	_ = mustTask(t, s, lid, "b", nil)

	// Everything was created "now"; nothing has changed since a future time.
	got, err := s.TasksChangedSince(lid, time.Now().Unix()+10)
	if err != nil {
		t.Fatalf("TasksChangedSince future: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no changes since the future, got %d", len(got))
	}

	// Cutoff now; sleep past updated_at's 1-second resolution, then change
	// exactly task a.
	cutoff := time.Now().Unix()
	time.Sleep(1100 * time.Millisecond)
	if err := s.SetNotes(a, "changed"); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}
	got, err = s.TasksChangedSince(lid, cutoff)
	if err != nil {
		t.Fatalf("TasksChangedSince cutoff: %v", err)
	}
	if len(got) != 1 || got[0].ID != a {
		t.Fatalf("expected only task a changed since cutoff, got %d rows: %+v", len(got), got)
	}
}

// TestTasksChangedSinceIncludesNewComment verifies that AddComment (Commit 1)
// bumping updated_at is observable through the store method:
// a task that only received a new comment since `since` appears in the result.
func TestTasksChangedSinceIncludesNewComment(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "L")
	a := mustTask(t, s, lid, "a", nil)
	cutoff := time.Now().Unix()
	time.Sleep(1100 * time.Millisecond)
	if _, err := s.AddComment(a, "pi", "note"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	got, err := s.TasksChangedSince(lid, cutoff)
	if err != nil {
		t.Fatalf("TasksChangedSince: %v", err)
	}
	if len(got) != 1 || got[0].ID != a {
		t.Fatalf("a new comment should make the task 'changed'; got %d rows: %+v", len(got), got)
	}
}

// openStoreAtMigration builds a database whose schema is exactly the first
// upto migrations recorded — a pre-0006 file that prod Open can no longer
// produce, because Open always migrates to head. The historical state is
// built with the store's own runner (applyOneMigration) over a raw
// connection, so a later migration genuinely lands on a file that existed
// before it, not on one rebuilt from the current schema.
func openStoreAtMigration(t *testing.T, path string, upto int) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_journal_mode=WAL&_foreign_keys=1&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	// Mirror Open's single-connection pool. This helper bypasses Open, and a
	// Store whose pool differs from every other Store in the process is not
	// the thing under test (store.go: SetMaxOpenConns(1) serializes writes
	// within the process).
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	t.Cleanup(func() { s.Close() })

	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("migrations FS: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.SortFunc(names, func(a, b string) int { return versionOf(a) - versionOf(b) })
	for _, name := range names {
		v := versionOf(name)
		if v <= 0 || v > upto {
			continue
		}
		contents, err := migrations.FS.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := s.applyOneMigration(v, string(contents)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return s
}

// TestMigration0006PreservesExistingTasks: 0006 adds assignee, assigned_at
// and priority. A database created before 0006 (here: at 0005, holding real
// tasks) must be upgraded in place with every pre-existing task landing on
// the new defaults — assignee ”, assigned_at NULL, priority 'none' — and
// both read paths must return the new fields.
func TestMigration0006PreservesExistingTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	s5 := openStoreAtMigration(t, path, 5)
	lid := mustList(t, s5, "list")
	root := mustTask(t, s5, lid, "root", nil)
	_ = mustTask(t, s5, lid, "child", &root)
	s5.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open after 0006: %v", err)
	}
	defer s.Close()

	got, err := s.ListTasks(lid)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListTasks returned %d rows, want the 2 pre-existing tasks", len(got))
	}
	for _, tk := range got {
		if tk.Assignee != "" || tk.AssignedAt != nil || tk.Priority != PriorityNone {
			t.Errorf("migrated task %q is not unassigned/none: %+v", tk.ID, tk)
		}
	}

	one, err := s.GetTask(root)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if one.Assignee != "" || one.AssignedAt != nil || one.Priority != PriorityNone {
		t.Errorf("GetTask did not return the migrated defaults: %+v", one)
	}

	// A task created after the migration starts from the same defaults.
	fresh := mustTask(t, s, lid, "fresh", nil)
	freshGot, err := s.GetTask(fresh)
	if err != nil {
		t.Fatalf("GetTask(fresh): %v", err)
	}
	if freshGot.Assignee != "" || freshGot.AssignedAt != nil || freshGot.Priority != PriorityNone {
		t.Errorf("new task did not default to unassigned/none: %+v", freshGot)
	}
}
