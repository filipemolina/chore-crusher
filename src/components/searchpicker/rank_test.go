package searchpicker

import (
	"path/filepath"
	"testing"

	"github.com/filipemolina/chore-crusher/src/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// rank orders title matches first (by fuzzy score), then notes-only hits — the
// same decision the CLI's rankSearch makes. Each result carries its list's
// name so the picker can render "<list> › <title>" without a second lookup.
func TestRankTitlesFirstThenNotes(t *testing.T) {
	s := testStore(t)
	lid, err := s.CreateList("Errands", "")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	tid, err := s.CreateTask(lid, "Buy milk", nil, "")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	// A notes-only hit: its title does not match, its notes do.
	if _, err := s.CreateTask(lid, "Groceries", nil, "remember milk at the store"); err != nil {
		t.Fatalf("create notes task: %v", err)
	}

	results := rank(s, "milk")
	// "Buy milk" is a title match; "Groceries" only matches via notes.
	if len(results) != 2 {
		t.Fatalf("rank returned %d results, want 2", len(results))
	}
	if results[0].TaskID != tid {
		t.Errorf("results[0] = %q, want the title match first", results[0].TaskID)
	}

	for _, r := range results {
		if r.ListName != "Errands" {
			t.Errorf("result %q list name = %q, want Errands (context for the picker)", r.Title, r.ListName)
		}
	}
}

// The picker's jump needs the task's list id so AppModel can switch to it;
// rank must carry that through.
func TestRankCarriesListID(t *testing.T) {
	s := testStore(t)
	lid, err := s.CreateList("List", "")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	tid, _ := s.CreateTask(lid, "unicorn", nil, "")

	results := rank(s, "unicorn")
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].ListID != lid || results[0].TaskID != tid {
		t.Errorf("carried ListID=%q/TaskID=%q, want %q/%q", results[0].ListID, results[0].TaskID, lid, tid)
	}
}
