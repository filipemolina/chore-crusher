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
	// A comment is activity on the task: bump updated_at so list_tasks(since=…)
	// reports it as changed (docs/plan/mcp-list-changes-since.md §1, decision (a);
	// the list_changes tool that plan added is now that since parameter).
	if _, err := s.db.Exec(
		`UPDATE Task SET updated_at = ? WHERE id = ?`, now, taskID,
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

// TaskIDsWithComments returns the set of task ids in listID that have at
// least one comment — a single batch query so the TUI can mark the comments
// glyph on every row of a list without N+1 round-trips (Commit 4,
// docs/plan/task-comments.md). A task with zero comments is simply absent from
// the map.
func (s *Store) TaskIDsWithComments(listID string) (map[string]bool, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT tc.task_id
		 FROM TaskComment tc JOIN Task t ON t.id = tc.task_id
		 WHERE t.list_id = ?`,
		listID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// GetComment returns one comment by id. Callers that must decide whether a
// delete is allowed (the MCP server's "own comments only" rule) read the
// author here before calling DeleteComment.
func (s *Store) GetComment(id string) (Comment, error) {
	var c Comment
	err := s.db.QueryRow(
		`SELECT id, task_id, author, note, created_at FROM TaskComment WHERE id = ?`,
		id,
	).Scan(&c.ID, &c.TaskID, &c.Author, &c.Note, &c.CreatedAt)
	if err != nil {
		if isNoRows(err) {
			return Comment{}, fmt.Errorf("comment %q not found", id)
		}
		return Comment{}, err
	}
	return c, nil
}

// DeleteComment hard-deletes one comment — no soft-delete, undo, or
// tombstone, the same append-then-remove shape DeleteTask uses. The store
// does not enforce who may delete what; that is a caller decision (the MCP
// server gates on author, the CLI and TUI are deliberately unenforced),
// exactly as ownership works for lists today.
func (s *Store) DeleteComment(id string) error {
	res, err := s.db.Exec(`DELETE FROM TaskComment WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireAffected(res, "comment", id)
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
