package store

import (
	"fmt"
	"time"
)

// Comment mirrors one row of the TaskComment table. It is the
// append-only v1 shape (docs/plan/task-comments.md §1): no edit, no delete,
// only a notes field — fields the spec explicitly defers.
type Comment struct {
	ID        string
	TaskID    string
	Author    string
	Note      string
	CreatedAt int64
}

// ErrCommentsDisabled is returned when AddComment targets a task whose list
// has comments_disabled set. The caller (CLI, TUI, MCP) is responsible for
// surfacing its text to the user.
var ErrCommentsDisabled = fmt.Errorf("comments are disabled for this list")

// AddComment appends a comment to taskID by author with the given note. It
// validates the task exists, refuses when the task's list has
// comments_disabled = 1, and returns the new comment's ULID. Comments are
// insert-only: there is no update or delete path (docs/plan/task-comments.md
// §1). The returned id is generated before the insert so the caller can
// reference the row without a re-query, matching CreateTask's convention.
func (s *Store) AddComment(taskID, author, note string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("add comment: task_id must not be empty")
	}
	if author == "" {
		return "", fmt.Errorf("add comment: author must not be empty")
	}

	// Validate the task exists and read whether its list disables comments
	// in a single join — no second round-trip.
	var disabled int
	err := s.db.QueryRow(
		`SELECT l.comments_disabled FROM Task t JOIN List l ON l.id = t.list_id WHERE t.id = ?`,
		taskID,
	).Scan(&disabled)
	if err != nil {
		if isNoRows(err) {
			return "", fmt.Errorf("task %q not found", taskID)
		}
		return "", err
	}
	if disabled == 1 {
		return "", ErrCommentsDisabled
	}

	id := NewID()
	now := time.Now().Unix()
	if _, err := s.db.Exec(
		`INSERT INTO TaskComment (id, task_id, author, note, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, taskID, author, note, now,
	); err != nil {
		return "", err
	}
	return id, nil
}

// ListComments returns all comments on taskID, oldest first
// (ORDER BY created_at ASC — the sort order the spec requires,
// docs/plan/task-comments.md §1). A task with no comments returns an
// empty slice, not an error.
func (s *Store) ListComments(taskID string) ([]Comment, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, author, note, created_at FROM TaskComment WHERE task_id = ? ORDER BY created_at ASC`,
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.TaskID, &c.Author, &c.Note, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetCommentsDisabled toggles the list-level comments_disabled flag. The
// plan (docs/plan/task-comments.md §1) leaves the "how a human turns it on"
// question as a follow-up — this store method exists so the flag can be set
// for testing and so a future CLI/TUI toggle has the one writer it needs,
// rather than every caller reaching into raw SQL.
func (s *Store) SetCommentsDisabled(listID string, disabled bool) error {
	flag := 0
	if disabled {
		flag = 1
	}
	res, err := s.db.Exec(
		`UPDATE List SET comments_disabled = ? WHERE id = ?`,
		flag, listID,
	)
	if err != nil {
		return err
	}
	return requireAffected(res, "list", listID)
}
