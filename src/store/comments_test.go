package store

import (
	"strings"
	"testing"
	"time"
)

// TestAddCommentAndListComments verifies the round-trip: comments are
// stored and returned oldest-first (docs/plan/task-comments.md §1).
func TestAddCommentAndListComments(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	taskID := mustTask(t, s, lid, "task", nil)

	// A task with notes empty still has no comments yet.
	got, err := s.ListComments(taskID)
	if err != nil {
		t.Fatalf("ListComments on empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 comments, got %d", len(got))
	}

	id1, err := s.AddComment(taskID, "human", "first")
	if err != nil {
		t.Fatalf("AddComment 1: %v", err)
	}
	if id1 == "" {
		t.Fatal("AddComment returned empty id")
	}
	id2, err := s.AddComment(taskID, "pi", "second")
	if err != nil {
		t.Fatalf("AddComment 2: %v", err)
	}

	got, err = s.ListComments(taskID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 comments, got %d", len(got))
	}
	// Sorted by created_at ASC — oldest first, regardless of insertion in
	// this test (they're close enough in time; if they tie, the order is
	// unspecified but the count is what we pin here).
	if got[0].Author != "human" || got[0].Note != "first" {
		t.Errorf("first comment = (%q, %q), want (human, first)", got[0].Author, got[0].Note)
	}
	if got[0].ID != id1 {
		t.Errorf("first comment id = %q, want %q", got[0].ID, id1)
	}
	if got[0].TaskID != taskID {
		t.Errorf("first comment task_id = %q, want %q", got[0].TaskID, taskID)
	}
	// id2 must appear somewhere.
	found := false
	for _, c := range got {
		if c.ID == id2 {
			found = true
			if c.Author != "pi" || c.Note != "second" {
				t.Errorf("second comment = (%q, %q), want (pi, second)", c.Author, c.Note)
			}
		}
	}
	if !found {
		t.Error("second comment not found in ListComments result")
	}
}

// TestListCommentsOnNoCommentsReturnsEmpty pins the empty-case contract:
// a task with no comments is not an error.
func TestListCommentsOnNoCommentsReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	taskID := mustTask(t, s, lid, "task", nil)

	got, err := s.ListComments(taskID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 comments, got %d", len(got))
	}
}

// TestAddCommentOnNonexistentTask verifies the existence check.
func TestAddCommentOnNonexistentTask(t *testing.T) {
	s := newTestStore(t)

	_, err := s.AddComment("no-such-task", "human", "note")
	if err == nil {
		t.Fatal("AddComment on a nonexistent task should fail")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should mention 'not found'", err.Error())
	}
}

// TestAddCommentOnEmptyFields pins the input validation contract.
func TestAddCommentOnEmptyFields(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	taskID := mustTask(t, s, lid, "task", nil)

	cases := []struct {
		name    string
		taskID  string
		author  string
		wantErr string
	}{
		{"empty task id", "", "human", "task_id must not be empty"},
		{"empty author", taskID, "", "author must not be empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.AddComment(tc.taskID, tc.author, "note")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestAddCommentRefusedWhenDisabled verifies the per-list disable flag.
func TestAddCommentRefusedWhenDisabled(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	// Flip comments_disabled directly — no store setter exists yet (that is
	// a CLI/TUI surface decision deferred to a later commit per
	// docs/plan/task-comments.md §1).
	if _, err := s.db.Exec(`UPDATE List SET comments_disabled = 1 WHERE id = ?`, lid); err != nil {
		t.Fatalf("enable comments_disabled: %v", err)
	}

	taskID := mustTask(t, s, lid, "task", nil)

	_, err := s.AddComment(taskID, "human", "note")
	if err != ErrCommentsDisabled {
		t.Errorf("expected ErrCommentsDisabled, got %v", err)
	}

	// ListComments still works on a disabled list — it reads comments that
	// were there before the flag was flipped, if any.
	got, err := s.ListComments(taskID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 comments, got %d", len(got))
	}
}

// TestTaskIDsWithComments pins the batch predicate used by RefreshTasks to
// set HasComments on every row of a list (docs/plan/task-comments.md §6,
// Commit 4): only tasks that actually have a comment appear in the map,
// comments on tasks in other lists are excluded, and a list with no comments
// at all yields an empty (non-nil) map.
func TestTaskIDsWithComments(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	taskWith := mustTask(t, s, lid, "with comments", nil)
	taskWithout := mustTask(t, s, lid, "no comments", nil)

	otherList := mustList(t, s, "other list")
	otherTask := mustTask(t, s, otherList, "other comment", nil)

	if _, err := s.AddComment(taskWith, "a", "c1"); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	if _, err := s.AddComment(otherTask, "a", "c2"); err != nil {
		t.Fatalf("add comment: %v", err)
	}

	got, err := s.TaskIDsWithComments(lid)
	if err != nil {
		t.Fatalf("TaskIDsWithComments: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("list query = %v, want exactly 1 entry (%q has a comment)", got, taskWith)
	}
	if !got[taskWith] {
		t.Errorf("expected %q in result, got %v", taskWith, got)
	}
	if got[taskWithout] {
		t.Errorf("task with no comments (%q) leaked into result", taskWithout)
	}
	if got[otherTask] {
		t.Error("other list task leaked into result")
	}

	// A list with no comments at all yields an empty map, not an error.
	cleanList := mustList(t, s, "clean")
	mustTask(t, s, cleanList, "no comments here", nil)
	got, err = s.TaskIDsWithComments(cleanList)
	if err != nil {
		t.Fatalf("clean list TaskIDsWithComments: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("clean list: want 0 entries, got %d", len(got))
	}
}

// TestAddCommentBumpsTaskUpdatedAt pins the contract documented in
// docs/plan/mcp-list-changes-since.md §1: a new comment is activity on the
// task, so AddComment must advance the task's updated_at. This is what makes
// list_changes(since) surface new comments.
func TestAddCommentBumpsTaskUpdatedAt(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	taskID := mustTask(t, s, lid, "task", nil)

	before, err := s.GetTask(taskID)
	if err != nil {
		t.Fatalf("GetTask before: %v", err)
	}

	// Sleep past updated_at's 1-second resolution so the bump is observable
	// without rewinding (which would bypass the real code path).
	cutoff := time.Now().Unix()
	time.Sleep(1100 * time.Millisecond)

	cid, err := s.AddComment(taskID, "pi", "hi")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}

	after, err := s.GetTask(taskID)
	if err != nil {
		t.Fatalf("GetTask after comment: %v", err)
	}
	if after.UpdatedAt <= before.UpdatedAt {
		t.Errorf("AddComment should bump updated_at: before=%d after=%d (comment=%s)",
			before.UpdatedAt, after.UpdatedAt, cid)
	}
	if after.UpdatedAt <= cutoff {
		t.Errorf("updated_at (%d) should be after cutoff (%d)", after.UpdatedAt, cutoff)
	}
}
