package store

// SearchTasks returns candidate tasks matching query against title and notes,
// case-insensitively, scoped to listID when non-nil and across all lists when
// nil. Fuzzy ranking is the caller's job (sahilm/fuzzy, in phase 8 and the
// CLI's search command) over the candidate set this function returns — store
// only needs to return candidates efficiently, not to know about fuzzy
// matching (docs/plans/phase-1-storage.md step 9).
func (s *Store) SearchTasks(query string, listID *string) ([]Task, error) {
	q := `SELECT ` + taskColumns + ` FROM Task
	      WHERE (title LIKE ('%' || ? || '%') OR notes LIKE ('%' || ? || '%'))`
	args := []any{query, query}
	if listID != nil {
		q += ` AND list_id = ?`
		args = append(args, *listID)
	}
	q += ` ORDER BY position, created_at, id`

	rows, err := s.db.Query(q, args...)
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
