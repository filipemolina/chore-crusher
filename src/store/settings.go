package store

// KeyLastListID is the Setting key holding the id of the last list the user
// had active, restored at the next launch (docs/DESIGN.md §7). An empty value
// means "no preference recorded"; the first list wins.
const KeyLastListID = "last_list_id"

// GetSetting returns the value stored under key, or "" with a nil error when
// the key has never been set. A nil error for a missing key is deliberate:
// settings are app-written state with a natural default per key, so callers
// should not have to special-case first runs.
func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM Setting WHERE key = ?`, key).Scan(&value)
	if isNoRows(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// SetSetting upserts the value under key. It is a single short write
// transaction like every other store mutator, so the TUI's long-lived read
// connection never blocks on it (docs/DESIGN.md §8).
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO Setting (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}
