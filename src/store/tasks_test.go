package store

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/filipemolina/farol/src/mentions"
	"github.com/filipemolina/farol/src/store/migrations"
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

func TestSetNotesWithValidMention(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	target := mustTask(t, s, lid, "target task", nil)
	id := mustTask(t, s, lid, "task", nil)

	// SetNotes with a valid mention should succeed
	notes := "See @" + target + " for context"
	if err := s.SetNotes(id, notes); err != nil {
		t.Fatalf("SetNotes with valid mention failed: %v", err)
	}
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Notes != notes {
		t.Fatalf("notes not stored as-is: got %q, want %q", task.Notes, notes)
	}
}

func TestSetNotesWithInvalidMention(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	// SetNotes with a non-existent mention should fail
	notes := "See @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 for context"
	err := s.SetNotes(id, notes)
	if err == nil {
		t.Fatal("SetNotes with invalid mention should have failed")
	}
	if !strings.Contains(err.Error(), "mention @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 references non-existent task") {
		t.Fatalf("unexpected error message: %v", err)
	}
	// Verify notes were not changed
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Notes != "" {
		t.Fatalf("notes should be unchanged, got %q", task.Notes)
	}
}

func TestSetNotesWithMultipleMentionsOneInvalid(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	target := mustTask(t, s, lid, "target task", nil)
	id := mustTask(t, s, lid, "task", nil)

	// One valid, one invalid mention
	notes := "Related to @" + target + " and @01ARZ8X5Y6Z7A8B9C0D1E2F3G4"
	err := s.SetNotes(id, notes)
	if err == nil {
		t.Fatal("SetNotes with one invalid mention should have failed")
	}
	if !strings.Contains(err.Error(), "mention @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 references non-existent task") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestSetNotesWithNonULIDMentionIgnored(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "task", nil)

	// @user and @abc are not ULIDs, should be ignored
	notes := "Hello @user and @abc123"
	if err := s.SetNotes(id, notes); err != nil {
		t.Fatalf("SetNotes with non-ULID mentions should succeed: %v", err)
	}
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Notes != notes {
		t.Fatalf("notes not stored as-is: got %q, want %q", task.Notes, notes)
	}
}

// TestCreateTaskWithValidMentionInTitle verifies that creating a task with
// a valid mention in the title succeeds and stores the title as-is.
func TestCreateTaskWithValidMentionInTitle(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	target := mustTask(t, s, lid, "target task", nil)

	title := "See @" + target + " for context"
	id, err := s.CreateTask(lid, title, nil, "")
	if err != nil {
		t.Fatalf("CreateTask with valid mention failed: %v", err)
	}
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Title != title {
		t.Fatalf("title not stored as-is: got %q, want %q", task.Title, title)
	}
}

// TestCreateTaskWithInvalidMentionInTitle verifies that creating a task with
// an invalid mention in the title fails with a clear error.
func TestCreateTaskWithInvalidMentionInTitle(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	title := "See @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 for context"
	_, err := s.CreateTask(lid, title, nil, "")
	if err == nil {
		t.Fatal("CreateTask with invalid mention should have failed")
	}
	if !strings.Contains(err.Error(), "mention @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 references non-existent task") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestCreateTaskWithMultipleMentionsOneInvalidInTitle verifies that creating
// a task with multiple mentions where one is invalid fails.
func TestCreateTaskWithMultipleMentionsOneInvalidInTitle(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	target := mustTask(t, s, lid, "target task", nil)

	title := "Related to @" + target + " and @01ARZ8X5Y6Z7A8B9C0D1E2F3G4"
	_, err := s.CreateTask(lid, title, nil, "")
	if err == nil {
		t.Fatal("CreateTask with one invalid mention should have failed")
	}
	if !strings.Contains(err.Error(), "mention @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 references non-existent task") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestCreateTaskWithNonULIDMentionInTitleIgnored verifies that non-ULID
// @patterns in the title are ignored (not treated as mentions).
func TestCreateTaskWithNonULIDMentionInTitleIgnored(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	title := "Hello @user and @abc123"
	id, err := s.CreateTask(lid, title, nil, "")
	if err != nil {
		t.Fatalf("CreateTask with non-ULID mentions should succeed: %v", err)
	}
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Title != title {
		t.Fatalf("title not stored as-is: got %q, want %q", task.Title, title)
	}
}

// TestRenameTaskWithValidMentionInTitle verifies that renaming a task to
// include a valid mention succeeds.
func TestRenameTaskWithValidMentionInTitle(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	target := mustTask(t, s, lid, "target task", nil)
	id := mustTask(t, s, lid, "original title", nil)

	newTitle := "Updated to reference @" + target
	if err := s.RenameTask(id, newTitle); err != nil {
		t.Fatalf("RenameTask with valid mention failed: %v", err)
	}
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Title != newTitle {
		t.Fatalf("title not stored as-is: got %q, want %q", task.Title, newTitle)
	}
}

// TestRenameTaskWithInvalidMentionInTitle verifies that renaming a task to
// include an invalid mention fails with a clear error.
func TestRenameTaskWithInvalidMentionInTitle(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "original title", nil)

	newTitle := "See @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 for context"
	err := s.RenameTask(id, newTitle)
	if err == nil {
		t.Fatal("RenameTask with invalid mention should have failed")
	}
	if !strings.Contains(err.Error(), "mention @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 references non-existent task") {
		t.Fatalf("unexpected error message: %v", err)
	}
	// Verify title was not changed
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Title != "original title" {
		t.Fatalf("title should be unchanged, got %q", task.Title)
	}
}

// TestCreateTaskAfterWithValidMentionInTitle verifies that CreateTaskAfter
// also validates mentions in titles.
func TestCreateTaskAfterWithValidMentionInTitle(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	target := mustTask(t, s, lid, "target task", nil)

	title := "Follow up on @" + target
	id, err := s.CreateTaskAfter(lid, title, nil, "", "")
	if err != nil {
		t.Fatalf("CreateTaskAfter with valid mention failed: %v", err)
	}
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Title != title {
		t.Fatalf("title not stored as-is: got %q, want %q", task.Title, title)
	}
}

// TestCreateTaskAfterWithInvalidMentionInTitle verifies that CreateTaskAfter
// rejects invalid mentions in titles.
func TestCreateTaskAfterWithInvalidMentionInTitle(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	title := "See @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 for context"
	_, err := s.CreateTaskAfter(lid, title, nil, "", "")
	if err == nil {
		t.Fatal("CreateTaskAfter with invalid mention should have failed")
	}
	if !strings.Contains(err.Error(), "mention @01ARZ8X5Y6Z7A8B9C0D1E2F3G4 references non-existent task") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestSelfReferenceMention verifies that a task can mention itself.
// This is a valid use case (e.g., "See @self for details" where @self
// is the task's own ID). The validation checks if the task exists
// at write time, and since the task exists, it should pass.
func TestSelfReferenceMention(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	// Create a task first
	id := mustTask(t, s, lid, "original title", nil)

	// Rename the task to mention itself
	newTitle := "Updated to reference @" + id
	if err := s.RenameTask(id, newTitle); err != nil {
		t.Fatalf("RenameTask with self-reference failed: %v", err)
	}

	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Title != newTitle {
		t.Fatalf("title not stored as-is: got %q, want %q", task.Title, newTitle)
	}

	// Also test in notes
	notes := "See @" + id + " for context"
	if err := s.SetNotes(id, notes); err != nil {
		t.Fatalf("SetNotes with self-reference failed: %v", err)
	}

	task, err = s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Notes != notes {
		t.Fatalf("notes not stored as-is: got %q, want %q", task.Notes, notes)
	}

	// Also test in comments
	cid, err := s.AddComment(id, "human", "Self reference: @"+id)
	if err != nil {
		t.Fatalf("AddComment with self-reference failed: %v", err)
	}
	if cid == "" {
		t.Fatal("AddComment returned empty id")
	}

	got, err := s.ListComments(id)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(got) != 1 || got[0].Note != "Self reference: @"+id {
		t.Fatalf("comment not stored as-is: got %q", got[0].Note)
	}
}

// TestCrossListMention verifies that a task in one list can mention
// a task in a different list. Task IDs are globally unique, so
// cross-list mentions should work without special handling.
func TestCrossListMention(t *testing.T) {
	s := newTestStore(t)
	listA := mustList(t, s, "list A")
	listB := mustList(t, s, "list B")

	// Create a task in list B to be mentioned
	targetInB := mustTask(t, s, listB, "target in list B", nil)

	// Create a task in list A that mentions the task in list B
	title := "Related to @" + targetInB
	id, err := s.CreateTask(listA, title, nil, "")
	if err != nil {
		t.Fatalf("CreateTask with cross-list mention failed: %v", err)
	}

	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Title != title {
		t.Fatalf("title not stored as-is: got %q, want %q", task.Title, title)
	}

	// Also test in notes
	notes := "See @" + targetInB + " for context"
	if err := s.SetNotes(id, notes); err != nil {
		t.Fatalf("SetNotes with cross-list mention failed: %v", err)
	}

	// Also test in comments
	cid, err := s.AddComment(id, "human", "Cross-list ref: @"+targetInB)
	if err != nil {
		t.Fatalf("AddComment with cross-list mention failed: %v", err)
	}
	if cid == "" {
		t.Fatal("AddComment returned empty id")
	}
}

// TestEndToEndMentionWorkflow verifies the complete mention workflow:
// create task with mention -> verify stored -> show resolves it.
func TestEndToEndMentionWorkflow(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	// Create a target task
	target := mustTask(t, s, lid, "target task", nil)

	// Create a task with a mention in the title
	title := "Follow up on @" + target
	id, err := s.CreateTask(lid, title, nil, "")
	if err != nil {
		t.Fatalf("CreateTask with mention failed: %v", err)
	}

	// Verify the task was stored with the mention intact
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Title != title {
		t.Fatalf("title not stored as-is: got %q, want %q", task.Title, title)
	}

	// Verify we can resolve the mention (simulating CLI show --json)
	parsedMentions := mentions.ParseMentions(task.Title)
	if len(parsedMentions) != 1 {
		t.Fatalf("expected 1 mention in stored title, got %d", len(parsedMentions))
	}
	if parsedMentions[0].ID != target {
		t.Fatalf("stored mention ID = %q, want %q", parsedMentions[0].ID, target)
	}

	// Verify the resolved metadata
	metadata := mentions.BuildMentionMetadata(task.Title, func(id string) string {
		if id == target {
			return "target task"
		}
		return ""
	})
	if len(metadata) != 1 {
		t.Fatalf("expected 1 metadata entry, got %d", len(metadata))
	}
	if metadata[0].ID != target {
		t.Fatalf("metadata ID = %q, want %q", metadata[0].ID, target)
	}
	if metadata[0].Title == nil || *metadata[0].Title != "target task" {
		t.Fatalf("metadata Title = %v, want 'target task'", metadata[0].Title)
	}
	if metadata[0].Deleted {
		t.Fatalf("metadata Deleted = true, want false")
	}
}

// TestMentionInAllThreeSurfaces verifies that mentions work in all
// three surfaces: title, notes, and comments.
func TestMentionInAllThreeSurfaces(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	// Create target tasks to mention
	target1 := mustTask(t, s, lid, "target one", nil)
	target2 := mustTask(t, s, lid, "target two", nil)
	target3 := mustTask(t, s, lid, "target three", nil)

	// Create a task with mention in title
	title := "Related to @" + target1
	id, err := s.CreateTask(lid, title, nil, "")
	if err != nil {
		t.Fatalf("CreateTask with mention in title failed: %v", err)
	}

	// Add mention in notes
	notes := "See @" + target2 + " for details"
	if err := s.SetNotes(id, notes); err != nil {
		t.Fatalf("SetNotes with mention failed: %v", err)
	}

	// Add mention in comment
	cid, err := s.AddComment(id, "human", "Also see @"+target3)
	if err != nil {
		t.Fatalf("AddComment with mention failed: %v", err)
	}
	if cid == "" {
		t.Fatal("AddComment returned empty id")
	}

	// Verify all three surfaces have the mentions stored correctly
	task, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	// Check title mention
	titleMentions := mentions.ParseMentions(task.Title)
	if len(titleMentions) != 1 || titleMentions[0].ID != target1 {
		t.Fatalf("title mentions = %+v, want 1 mention to %s", titleMentions, target1)
	}

	// Check notes mention
	notesMentions := mentions.ParseMentions(task.Notes)
	if len(notesMentions) != 1 || notesMentions[0].ID != target2 {
		t.Fatalf("notes mentions = %+v, want 1 mention to %s", notesMentions, target2)
	}

	// Check comment mention
	comments, err := s.ListComments(id)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	commentMentions := mentions.ParseMentions(comments[0].Note)
	if len(commentMentions) != 1 || commentMentions[0].ID != target3 {
		t.Fatalf("comment mentions = %+v, want 1 mention to %s", commentMentions, target3)
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

// TestReparentCompleteParentReopens ensures that reparenting a pending task
// under a complete parent reopens the parent (docs/DESIGN.md §3: a complete
// task with a pending child is forbidden). The parent is switched to subtasks
// mode so it derives progress from its children.
func TestReparentCompleteParentReopens(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	done := mustTask(t, s, lid, "done", nil)
	if err := s.Complete(done); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	task := mustTask(t, s, lid, "still working", nil)

	// Reparent should succeed by reopening the parent
	if err := s.Reparent(task, &done); err != nil {
		t.Fatalf("Reparent: %v", err)
	}

	// Parent should be reopened and switched to subtasks, but stay pending —
	// creating/reparenting a subtask is planning, not starting (docs/DESIGN.md §3).
	d, _ := s.GetTask(done)
	if d.Status != StatusPending || d.ProgressKind != ProgressSubtasks {
		t.Fatalf("Parent should be reopened to pending/subtasks, got %s/%s", d.Status, d.ProgressKind)
	}

	// Task should be reparented
	tk, _ := s.GetTask(task)
	if tk.ParentID == nil || *tk.ParentID != done {
		t.Fatalf("Task not reparented: parent=%v", tk.ParentID)
	}

	// A complete subtree may move under a complete parent (both sides complete).
	// The parent stays complete since the child is also complete.
	// Use a fresh complete parent for this test.
	done2 := mustTask(t, s, lid, "done2", nil)
	if err := s.Complete(done2); err != nil {
		t.Fatalf("Complete(done2): %v", err)
	}
	doneSub := mustTask(t, s, lid, "done subtree", nil)
	if err := s.Complete(doneSub); err != nil {
		t.Fatalf("Complete(doneSub): %v", err)
	}
	if err := s.Reparent(doneSub, &done2); err != nil {
		t.Fatalf("moving a complete subtree under a complete parent should be allowed: %v", err)
	}
	d2, _ := s.GetTask(done2)
	if d2.Status != StatusComplete || d2.ProgressKind != ProgressNone {
		t.Fatalf("Parent should stay complete when moving complete subtree, got %s/%s", d2.Status, d2.ProgressKind)
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

// TestCreateTaskReopensCompleteParent ensures that creating a child under a
// complete parent reopens the parent (docs/DESIGN.md §3: a complete task with
// a pending child is forbidden). This covers both CreateTask (append) and
// CreateTaskAfter (positioned) paths.
func TestCreateTaskReopensCompleteParent(t *testing.T) {
	tests := []struct {
		name       string
		createFunc func(*Store, string, string, *string, string, string) (string, error)
	}{
		{
			name: "CreateTask (append)",
			createFunc: func(s *Store, lid, title string, parentID *string, notes, afterID string) (string, error) {
				return s.CreateTask(lid, title, parentID, notes)
			},
		},
		{
			name: "CreateTaskAfter (append via empty afterID)",
			createFunc: func(s *Store, lid, title string, parentID *string, notes, afterID string) (string, error) {
				return s.CreateTaskAfter(lid, title, parentID, notes, "")
			},
		},
		{
			name: "CreateTaskAfter (positioned after sibling)",
			createFunc: func(s *Store, lid, title string, parentID *string, notes, afterID string) (string, error) {
				return s.CreateTaskAfter(lid, title, parentID, notes, afterID)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			lid := mustList(t, s, "list")

			parent := mustTask(t, s, lid, "parent", nil)
			child1 := mustTask(t, s, lid, "child1", &parent)
			child2 := mustTask(t, s, lid, "child2", &parent)

			if err := s.Complete(child1); err != nil {
				t.Fatalf("Complete(child1): %v", err)
			}
			if err := s.Complete(child2); err != nil {
				t.Fatalf("Complete(child2): %v", err)
			}

			// Parent should be complete (auto-promoted via subtasks mode)
			p, _ := s.GetTask(parent)
			if p.Status != StatusComplete {
				t.Fatalf("Parent should be complete, got %s", p.Status)
			}

			// Create a new child
			var newChild string
			var err error
			if tc.name == "CreateTaskAfter (positioned after sibling)" {
				newChild, err = tc.createFunc(s, lid, "newchild", &parent, "", child1)
			} else {
				newChild, err = tc.createFunc(s, lid, "newchild", &parent, "", "")
			}
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			// Parent should be reopened and switched to subtasks, but stay
			// pending — creating a subtask is planning, not starting (§3).
			p, _ = s.GetTask(parent)
			if p.Status != StatusPending || p.ProgressKind != ProgressSubtasks {
				t.Fatalf("Parent should be reopened to pending/subtasks, got %s/%s", p.Status, p.ProgressKind)
			}

			// New child should be pending
			nc, _ := s.GetTask(newChild)
			if nc.Status != StatusPending {
				t.Fatalf("New child should be pending, got %s", nc.Status)
			}

			// MoveTask should work (the bug: moving was impossible before fix)
			var moveAfter string
			if tc.name == "CreateTaskAfter (positioned after sibling)" {
				moveAfter = child2
			} else {
				moveAfter = child1
			}
			if err := s.MoveTask(newChild, moveAfter); err != nil {
				t.Fatalf("MoveTask failed: %v", err)
			}

			// Verify move worked
			tasks, _ := s.ListTasks(lid)
			childOrder := []string{}
			for _, task := range tasks {
				if task.ParentID != nil && *task.ParentID == parent {
					childOrder = append(childOrder, task.ID)
				}
			}
			if len(childOrder) != 3 {
				t.Fatalf("Expected 3 children after move, got %d", len(childOrder))
			}
		})
	}
}

// TestReparentReopensCompleteParent ensures Reparent also reopens a complete
// parent when moving a task under it, for consistency with CreateTask.
func TestReparentReopensCompleteParent(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	done := mustTask(t, s, lid, "done", nil)
	if err := s.Complete(done); err != nil {
		t.Fatalf("Complete(done): %v", err)
	}

	task := mustTask(t, s, lid, "still working", nil)

	// Reparent should reopen the complete parent
	if err := s.Reparent(task, &done); err != nil {
		t.Fatalf("Reparent: %v", err)
	}

	d, _ := s.GetTask(done)
	if d.Status != StatusPending || d.ProgressKind != ProgressSubtasks {
		t.Fatalf("Parent should be reopened to pending/subtasks, got %s/%s", d.Status, d.ProgressKind)
	}

	tk, _ := s.GetTask(task)
	if tk.ParentID == nil || *tk.ParentID != done {
		t.Fatalf("Task not reparented: parent=%v", tk.ParentID)
	}
}
