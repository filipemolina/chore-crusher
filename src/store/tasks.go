package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CreateTask creates a task in listID, with the given title, notes, and
// optional parent, appends it after the current last sibling, and returns its
// id. The id is generated before the transaction opens so the caller can
// reference the new task without a re-query (docs/DESIGN.md §2).
//
// The parent, when given, must exist and belong to the same list — a task
// belongs to exactly one List, and a cross-list parent would orphan the task
// from every tree reader that scopes by list_id.
func (s *Store) CreateTask(listID, title string, parentID *string, notes string) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", fmt.Errorf("task title must not be empty")
	}

	id := NewID()
	now := time.Now().Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var one int
	if err := tx.QueryRow(`SELECT 1 FROM List WHERE id = ?`, listID).Scan(&one); err != nil {
		if isNoRows(err) {
			return "", fmt.Errorf("list %q not found", listID)
		}
		return "", err
	}

	if parentID != nil {
		var parentList string
		err := tx.QueryRow(`SELECT list_id FROM Task WHERE id = ?`, *parentID).Scan(&parentList)
		if err != nil {
			if isNoRows(err) {
				return "", fmt.Errorf("parent task %q not found", *parentID)
			}
			return "", err
		}
		if parentList != listID {
			return "", fmt.Errorf("parent task %q belongs to a different list", *parentID)
		}
	}

	var position int
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(position), -1) + 1 FROM Task WHERE list_id = ? AND parent_id IS ?`,
		listID, parentID,
	).Scan(&position); err != nil {
		return "", err
	}

	if _, err := tx.Exec(
		`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind,
		                  progress_pct, position, created_at, updated_at, completed_at)
		 VALUES (?, ?, ?, ?, ?, 'pending', 'none', NULL, ?, ?, ?, NULL)`,
		id, listID, parentID, title, notes, position, now, now,
	); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// GetTask returns the task with the given id.
func (s *Store) GetTask(id string) (Task, error) {
	return getTask(s.db, id)
}

// RenameTask sets a task's title.
func (s *Store) RenameTask(id, title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("task title must not be empty")
	}
	res, err := s.db.Exec(`UPDATE Task SET title = ?, updated_at = ? WHERE id = ?`, title, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	return requireAffected(res, "task", id)
}

// SetNotes replaces a task's notes wholesale (the CLI's notes command is
// "replace, not append"; clearing notes is setting the empty string).
func (s *Store) SetNotes(id, notes string) error {
	res, err := s.db.Exec(`UPDATE Task SET notes = ?, updated_at = ? WHERE id = ?`, notes, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	return requireAffected(res, "task", id)
}

// DeleteTask deletes the task and, via the parent_id foreign key's ON DELETE
// CASCADE, every descendant at every depth. Sibling subtrees are untouched.
func (s *Store) DeleteTask(id string) error {
	res, err := s.db.Exec(`DELETE FROM Task WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireAffected(res, "task", id)
}

// ListTasks returns every task in listID as flat rows, each carrying its own
// ParentID — building the tree shape is the caller's job (src/apptypes in a
// later phase), so the CLI's flat mode and the TUI's tree renderer read the
// exact same query. Rows are ordered by sibling position (creation order),
// then creation time.
func (s *Store) ListTasks(listID string) ([]Task, error) {
	rows, err := s.db.Query(
		`SELECT id, list_id, parent_id, title, notes, status, progress_kind, progress_pct,
		        position, created_at, updated_at, completed_at
		 FROM Task WHERE list_id = ?
		 ORDER BY position, created_at, id`,
		listID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// taskColumns is shared by every query that reads a full Task row.
const taskColumns = `id, list_id, parent_id, title, notes, status, progress_kind, progress_pct,
	position, created_at, updated_at, completed_at`

// getTask reads one Task row, converting sql.Null* columns to pointers.
func getTask(q querier, id string) (Task, error) {
	t, err := scanTask(q.QueryRow(`SELECT `+taskColumns+` FROM Task WHERE id = ?`, id))
	if err != nil && isNoRows(err) {
		return Task{}, fmt.Errorf("task %q not found", id)
	}
	return t, err
}

// scanTask converts one result row into a Task.
func scanTask(r rowScanner) (Task, error) {
	var t Task
	var parent sql.NullString
	var pct sql.NullInt64
	var done sql.NullInt64
	err := r.Scan(
		&t.ID, &t.ListID, &parent, &t.Title, &t.Notes,
		&t.Status, &t.ProgressKind, &pct,
		&t.Position, &t.CreatedAt, &t.UpdatedAt, &done,
	)
	if err != nil {
		return Task{}, err
	}
	if parent.Valid {
		t.ParentID = &parent.String
	}
	if pct.Valid {
		v := int(pct.Int64)
		t.ProgressPct = &v
	}
	if done.Valid {
		v := done.Int64
		t.CompletedAt = &v
	}
	return t, nil
}

// getParentID returns the parent of id, or nil for a root-level task.
func getParentID(q querier, id string) (*string, error) {
	var parent sql.NullString
	if err := q.QueryRow(`SELECT parent_id FROM Task WHERE id = ?`, id).Scan(&parent); err != nil {
		return nil, err
	}
	if !parent.Valid {
		return nil, nil
	}
	return &parent.String, nil
}
