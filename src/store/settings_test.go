package store

import (
	"testing"
)

// TestSettingRoundTrip pins the Setting table's read/write contract: a value
// set is read back unchanged, and re-setting the same key overwrites in place
// (no duplicates).
func TestSettingRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetSetting(KeyLastListID, "01ARZ3NDEKTSV4RRFFQ69G5FAV"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	got, err := s.GetSetting(KeyLastListID)
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if got != "01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		t.Errorf("GetSetting = %q, want the stored id", got)
	}

	// Upsert: a second SetSetting with a new value must replace, not fail on
	// the primary key and not leave two rows.
	if err := s.SetSetting(KeyLastListID, "01ARZ3NDEKTSV4RRFFQ69G5FBW"); err != nil {
		t.Fatalf("SetSetting (upsert): %v", err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM Setting WHERE key = ?`, KeyLastListID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("rows for key after upsert = %d, want 1", n)
	}
	got, err = s.GetSetting(KeyLastListID)
	if err != nil {
		t.Fatalf("GetSetting after upsert: %v", err)
	}
	if got != "01ARZ3NDEKTSV4RRFFQ69G5FBW" {
		t.Errorf("GetSetting after upsert = %q, want the new value", got)
	}
}

// TestGetSettingMissingReturnsEmpty pins the "never set" contract: the empty
// string with a nil error, so first runs (no last list recorded) read as a
// plain default rather than an error callers must special-case.
func TestGetSettingMissingReturnsEmpty(t *testing.T) {
	s := newTestStore(t)

	got, err := s.GetSetting("never_written_key")
	if err != nil {
		t.Fatalf("GetSetting on a missing key: %v", err)
	}
	if got != "" {
		t.Errorf("GetSetting on a missing key = %q, want \"\"", got)
	}
}

// TestCollapsedTasksRoundTrip pins the per-list collapsed task state
// read/write contract.
func TestCollapsedTasksRoundTrip(t *testing.T) {
	s := newTestStore(t)
	listID := "list-1"

	// Initially empty.
	collapsed, err := s.GetCollapsedTasks(listID)
	if err != nil {
		t.Fatalf("GetCollapsedTasks (empty): %v", err)
	}
	if len(collapsed) != 0 {
		t.Errorf("GetCollapsedTasks (empty) = %v, want empty map", collapsed)
	}

	// Set some collapsed tasks.
	collapsed = map[string]bool{"task-a": true, "task-b": true}
	if err := s.SetCollapsedTasks(listID, collapsed); err != nil {
		t.Fatalf("SetCollapsedTasks: %v", err)
	}

	// Read back.
	got, err := s.GetCollapsedTasks(listID)
	if err != nil {
		t.Fatalf("GetCollapsedTasks (after set): %v", err)
	}
	if len(got) != 2 {
		t.Errorf("GetCollapsedTasks len = %d, want 2", len(got))
	}
	if !got["task-a"] || !got["task-b"] {
		t.Errorf("GetCollapsedTasks = %v, want task-a and task-b", got)
	}

	// Upsert: replace with a different set.
	collapsed = map[string]bool{"task-c": true}
	if err := s.SetCollapsedTasks(listID, collapsed); err != nil {
		t.Fatalf("SetCollapsedTasks (upsert): %v", err)
	}
	got, err = s.GetCollapsedTasks(listID)
	if err != nil {
		t.Fatalf("GetCollapsedTasks (after upsert): %v", err)
	}
	if len(got) != 1 || !got["task-c"] {
		t.Errorf("GetCollapsedTasks after upsert = %v, want task-c", got)
	}

	// Different list should be independent.
	collapsed2, err := s.GetCollapsedTasks("list-2")
	if err != nil {
		t.Fatalf("GetCollapsedTasks (list-2): %v", err)
	}
	if len(collapsed2) != 0 {
		t.Errorf("GetCollapsedTasks (list-2) = %v, want empty (independent)", collapsed2)
	}
}

// TestCollapsedTasksEmptyMapIsNotNil ensures GetCollapsedTasks returns an
// empty map (not nil) for a list with no saved state, so callers can safely
// range over it.
func TestCollapsedTasksEmptyMapIsNotNil(t *testing.T) {
	s := newTestStore(t)
	collapsed, err := s.GetCollapsedTasks("new-list")
	if err != nil {
		t.Fatalf("GetCollapsedTasks: %v", err)
	}
	if collapsed == nil {
		t.Error("GetCollapsedTasks returned nil, want empty map")
	}
}
