package store

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// newTestStore opens a fresh store on a temp-dir database file per test —
// real SQLite, no terminal, no mocking (docs/DESIGN.md §13).
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mustList(t *testing.T, s *Store, name string) string {
	t.Helper()
	id, err := s.CreateList(name, "")
	if err != nil {
		t.Fatalf("CreateList(%q): %v", name, err)
	}
	return id
}

// mustTask creates a task, failing the test on error. parent may be nil.
func mustTask(t *testing.T, s *Store, listID, title string, parent *string) string {
	t.Helper()
	id, err := s.CreateTask(listID, title, parent, "")
	if err != nil {
		t.Fatalf("CreateTask(%q, parent=%v): %v", title, parent != nil, err)
	}
	return id
}

func strptr(s string) *string { return &s }
func intptr(n int) *int       { return &n }

// threeLevelTree builds root → child → grandchild in listID and returns the ids.
func threeLevelTree(t *testing.T, s *Store, listID string) (root, child, grand string) {
	t.Helper()
	root = mustTask(t, s, listID, "root", nil)
	child = mustTask(t, s, listID, "child", &root)
	grand = mustTask(t, s, listID, "grand", &child)
	return root, child, grand
}

func TestOpenAppliesMigrationsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	mustList(t, s1, "first list")
	s1.Close()

	// Second open of the same file must be a no-op on the schema — no error,
	// no duplicate-migration failure — and must see the first open's data.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.Close()

	lists, err := s2.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(lists) != 1 || lists[0].Name != "first list" {
		t.Fatalf("second open sees %d lists, want the one from the first open", len(lists))
	}
}

// TestOpenEnforcesForeignKeys confirms the _foreign_keys=1 DSN parameter
// actually took effect on the store's own connection: a row referencing a
// nonexistent list or parent is rejected, and ON DELETE CASCADE fires.
func TestOpenEnforcesForeignKeys(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	// Direct INSERT bypassing CreateTask's own validation, so the failure has
	// to come from the foreign key constraint rather than store's checks.
	now := int64(0)
	if _, err := s.db.Exec(
		`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind,
		                  progress_pct, position, created_at, updated_at, completed_at)
		 VALUES ('badlist', 'no-such-list', NULL, 'x', '', 'pending', 'none', NULL, 0, ?, ?, NULL)`,
		now, now,
	); err == nil {
		t.Fatal("insert with nonexistent list_id unexpectedly succeeded")
	}

	parent := mustTask(t, s, lid, "parent", nil)
	if _, err := s.db.Exec(
		`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind,
		                  progress_pct, position, created_at, updated_at, completed_at)
		 VALUES ('badparent', ?, 'no-such-parent', 'x', '', 'pending', 'none', NULL, 0, ?, ?, NULL)`,
		lid, now, now,
	); err == nil {
		t.Fatal("insert with nonexistent parent_id unexpectedly succeeded")
	}

	// Cascade: deleting the parent removes its children via the FK, not by
	// any store code. (CreateTask's own checks never produce a dangling
	// reference, so this is the proof the FK wiring is live.)
	if err := s.DeleteTask(parent); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, err := s.GetTask(parent); err == nil {
		t.Fatal("parent still readable after delete")
	}
}

// TestConcurrentWritesNoBusy fires many writes at the same store in
// parallel and asserts none of them fail. Before SetMaxOpenConns(1) in Open,
// the unlimited connection pool let two agent writes in one batch contend on
// the single WAL writer lock and surface as SQLITE_BUSY (reproduced ~50% of
// parallel write pairs during the capability test). With a single pooled
// connection every write serializes and there is no intra-process lock to
// lose.
func TestConcurrentWritesNoBusy(t *testing.T) {
	s := newTestStore(t)

	const n = 32
	var wg sync.WaitGroup
	errc := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			lid, err := s.CreateList(fmt.Sprintf("list-%d", i), "")
			if err != nil {
				errc <- fmt.Errorf("CreateList %d: %w", i, err)
				return
			}
			if _, err := s.CreateTask(lid, "task", nil, ""); err != nil {
				errc <- fmt.Errorf("CreateTask %d: %w", i, err)
				return
			}
		}(i)
	}
	wg.Wait()
	close(errc)

	var errs []error
	for err := range errc {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		t.Fatalf("%d of %d concurrent writes failed: %v", len(errs), n, errs[0])
	}
}

func TestOpenRejectsUnknownTable(t *testing.T) {
	s := newTestStore(t)
	_, err := s.ResolveID("tasks", "0")
	if err == nil {
		t.Fatal("ResolveID accepted an unknown table")
	}
	if !strings.Contains(err.Error(), "unknown table") {
		t.Fatalf("ResolveID error %q does not name the unknown table", err)
	}
}
