package store

import "testing"

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
