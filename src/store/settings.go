package store

import (
	"encoding/json"
)

// KeyLastListID is the Setting key holding the id of the last list the user
// had active, restored at the next launch (docs/DESIGN.md §7). An empty value
// means "no preference recorded"; the first list wins.
const KeyLastListID = "last_list_id"

// KeyCollapsedTasksPrefix is the Setting key prefix for per-list collapsed
// task state. The full key is "collapsed_tasks:<list_id>". The value is a
// JSON array of task IDs that are collapsed.
const KeyCollapsedTasksPrefix = "collapsed_tasks:"

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

// GetCollapsedTasks returns the set of collapsed task IDs for the given list.
// Returns an empty map (not nil) if no state has been saved for the list.
func (s *Store) GetCollapsedTasks(listID string) (map[string]bool, error) {
	key := KeyCollapsedTasksPrefix + listID
	value, err := s.GetSetting(key)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return make(map[string]bool), nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(value), &ids); err != nil {
		return nil, err
	}
	collapsed := make(map[string]bool, len(ids))
	for _, id := range ids {
		collapsed[id] = true
	}
	return collapsed, nil
}

// SetCollapsedTasks saves the set of collapsed task IDs for the given list.
func (s *Store) SetCollapsedTasks(listID string, collapsed map[string]bool) error {
	key := KeyCollapsedTasksPrefix + listID
	ids := make([]string, 0, len(collapsed))
	for id := range collapsed {
		ids = append(ids, id)
	}
	value, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return s.SetSetting(key, string(value))
}
