package store

import "testing"

func TestSearchTasksMatchesTitleAndNotes(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "Buy paint", nil)
	if err := s.SetNotes(id, "matte finish, one coat"); err != nil {
		t.Fatalf("SetNotes: %v", err)
	}

	cases := []struct {
		query string
		want  int
	}{
		{"paint", 1},    // title match, case-insensitive
		{"PAINT", 1},    // case-insensitive
		{"matte", 1},    // notes match
		{"zzz", 0},      // no match
		{"coat", 1},     // notes match
		{"one coat", 1}, // contiguous across a notes phrase
	}
	for _, tc := range cases {
		got, err := s.SearchTasks(tc.query, &lid)
		if err != nil {
			t.Fatalf("SearchTasks(%q): %v", tc.query, err)
		}
		if len(got) != tc.want {
			t.Fatalf("SearchTasks(%q) returned %d rows, want %d", tc.query, len(got), tc.want)
		}
	}
}

func TestSearchTasksScopedByList(t *testing.T) {
	s := newTestStore(t)
	l1 := mustList(t, s, "list one")
	l2 := mustList(t, s, "list two")
	mustTask(t, s, l1, "shared keyword", nil)
	mustTask(t, s, l2, "shared keyword", nil)

	// Scoped to one list: exactly one match.
	got, err := s.SearchTasks("shared", &l1)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(got) != 1 || got[0].ListID != l1 {
		t.Fatalf("list-scoped search returned %d rows with list %v, want 1 in %s",
			len(got), got[0].ListID, l1)
	}

	// Unscoped: both.
	got, err = s.SearchTasks("shared", nil)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("unscoped search returned %d rows, want 2", len(got))
	}
}

func TestSearchTasksReturnsFullRows(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	id := mustTask(t, s, lid, "unique title", nil)
	if err := s.Complete(id); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	got, err := s.SearchTasks("unique", &lid)
	if err != nil {
		t.Fatalf("SearchTasks: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("SearchTasks returned %d rows, want 1", len(got))
	}
	if got[0].Status != StatusComplete || got[0].CompletedAt == nil {
		t.Fatalf("search result is not the full task row: %+v", got[0])
	}
}
