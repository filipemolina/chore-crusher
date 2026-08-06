package store

import (
	"strings"
	"testing"
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
