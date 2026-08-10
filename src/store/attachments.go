package store

import (
	"fmt"
	"time"
)

// Attachment mirrors one row of the TaskAttachment table. It stores a file
// path (absolute or relative) associated with a task. No file content is
// stored in the database; the path is a reference the UI can display or open.
type Attachment struct {
	ID        string
	TaskID    string
	Path      string
	CreatedAt int64
}

// AddAttachment creates an attachment for taskID with the given file path and
// returns its id. The task must exist; the path is stored as-is without
// validation (the UI is responsible for opening/resolving it).
func (s *Store) AddAttachment(taskID, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("attachment path must not be empty")
	}

	// Verify task exists
	if _, err := s.GetTask(taskID); err != nil {
		return "", err
	}

	id := NewID()
	now := time.Now().Unix()

	if _, err := s.db.Exec(
		`INSERT INTO TaskAttachment (id, task_id, path, created_at) VALUES (?, ?, ?, ?)`,
		id, taskID, path, now,
	); err != nil {
		return "", err
	}
	return id, nil
}

// ListAttachments returns every attachment for taskID, ordered by created_at
// ascending (oldest first). Returns an empty slice when the task has no
// attachments.
func (s *Store) ListAttachments(taskID string) ([]Attachment, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, path, created_at FROM TaskAttachment WHERE task_id = ? ORDER BY created_at ASC`,
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.TaskID, &a.Path, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAttachment removes the attachment with the given id. Returns an error
// if the attachment does not exist.
func (s *Store) DeleteAttachment(id string) error {
	res, err := s.db.Exec(`DELETE FROM TaskAttachment WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireAffected(res, "attachment", id)
}
