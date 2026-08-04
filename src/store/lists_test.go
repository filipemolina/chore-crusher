package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateListAndListLists(t *testing.T) {
	s := newTestStore(t)

	l1 := mustList(t, s, "Home renovation")
	l2 := mustList(t, s, "Work")

	lists, err := s.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(lists) != 2 {
		t.Fatalf("ListLists returned %d lists, want 2", len(lists))
	}
	if lists[0].ID != l1 || lists[0].Name != "Home renovation" {
		t.Fatalf("lists[0] = %+v, want the first-created list", lists[0])
	}
	if lists[1].ID != l2 || lists[1].Name != "Work" {
		t.Fatalf("lists[1] = %+v, want the second-created list", lists[1])
	}

	// Counts: one pending task in the first list, nothing in the second.
	mustTask(t, s, l1, "Buy paint", nil)
	lists, err = s.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if lists[0].PendingCount != 1 || lists[0].CompleteCount != 0 {
		t.Fatalf("lists[0] counts = %d pending / %d complete, want 1/0",
			lists[0].PendingCount, lists[0].CompleteCount)
	}
	if lists[1].PendingCount != 0 || lists[1].CompleteCount != 0 {
		t.Fatalf("lists[1] counts = %d pending / %d complete, want 0/0",
			lists[1].PendingCount, lists[1].CompleteCount)
	}
}

func TestListCountsCompleteVsPending(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	t1 := mustTask(t, s, lid, "done task", nil)
	mustTask(t, s, lid, "doing task", nil)
	if err := s.Complete(t1); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	lists, err := s.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	// One complete, one pending — and a task in progress counts as pending.
	if lists[0].PendingCount != 1 || lists[0].CompleteCount != 1 {
		t.Fatalf("counts = %d/%d, want 1 pending / 1 complete",
			lists[0].PendingCount, lists[0].CompleteCount)
	}
}

func TestRenameList(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "old name")

	if err := s.RenameList(lid, "new name"); err != nil {
		t.Fatalf("RenameList: %v", err)
	}
	lists, err := s.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if lists[0].Name != "new name" {
		t.Fatalf("renamed list has name %q, want %q", lists[0].Name, "new name")
	}

	if err := s.RenameList("no-such-id", "x"); err == nil {
		t.Fatal("RenameList on a missing id did not error")
	}
	if err := s.RenameList(lid, "  "); err == nil {
		t.Fatal("RenameList accepted a blank name")
	}
}

func TestDeleteListRemovesEveryTask(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")
	root, child, grand := threeLevelTree(t, s, lid)
	if err := s.Complete(root); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if err := s.DeleteList(lid); err != nil {
		t.Fatalf("DeleteList: %v", err)
	}

	lists, err := s.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(lists) != 0 {
		t.Fatalf("ListLists returned %d lists after delete, want 0", len(lists))
	}
	for _, id := range []string{root, child, grand} {
		if _, err := s.GetTask(id); err == nil {
			t.Fatalf("task %s still readable after its list was deleted", id)
		}
	}
}

func TestGetOrCreateAgentList(t *testing.T) {
	s := newTestStore(t)

	// First call creates the list.
	id1, err := s.GetOrCreateAgentList("pi")
	if err != nil {
		t.Fatalf("first GetOrCreateAgentList: %v", err)
	}

	lists, err := s.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(lists) != 1 || lists[0].ID != id1 || lists[0].Name != "pi: Inbox" {
		t.Fatalf("after first call, lists = %+v, want one pi: Inbox with id %q", lists, id1)
	}

	// Second call returns the same list (idempotent, no duplicate).
	id2, err := s.GetOrCreateAgentList("pi")
	if err != nil {
		t.Fatalf("second GetOrCreateAgentList: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("GetOrCreateAgentList not idempotent: %q != %q", id1, id2)
	}

	lists, err = s.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(lists) != 1 {
		t.Fatalf("expected 1 list, got %d (duplicate created?)", len(lists))
	}

	// A different identity gets its own list.
	id3, err := s.GetOrCreateAgentList("claude")
	if err != nil {
		t.Fatalf("GetOrCreateAgentList(claude): %v", err)
	}
	if id3 == id1 || id3 == id2 {
		t.Fatalf("different identity returned the same list id %q", id3)
	}
	lists, err = s.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(lists) != 2 {
		t.Fatalf("expected 2 lists after second identity, got %d", len(lists))
	}
}

func TestDeleteListRequiresExisting(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteList("no-such-id")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("DeleteList on missing id error = %v, want a not-found error", err)
	}
}

func TestCreateListStoresOwner(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateList("Inbox", "pi")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	l, err := s.GetList(id)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if l.CreatedBy != "pi" {
		t.Fatalf("CreatedBy = %q, want %q", l.CreatedBy, "pi")
	}
}

func TestGetListReturnsCreatedBy(t *testing.T) {
	s := newTestStore(t)

	// An explicitly unowned (human-created) list stays empty.
	id, err := s.CreateList("Groceries", "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	l, err := s.GetList(id)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if l.CreatedBy != "" {
		t.Fatalf("CreatedBy = %q, want empty (owned by nobody)", l.CreatedBy)
	}
}

func TestGetListNotFound(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.GetList("no-such-id"); err == nil {
		t.Fatal("GetList on a missing id did not error")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("GetList error = %q, want a not-found error", err)
	}
}

// TestBackfillAdoptsTaggedLists exercises the Open-time backfill: lists named
// with A's "<tag>:" convention but empty created_by (the pre-B shape, as
// CreateList writes when no owner is given) are adopted into created_by, an
// untagged list stays owned by nobody, and the pass runs automatically on the
// second Open.
func TestBackfillAdoptsTaggedLists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	piList := mustList(t, s1, "pi: Main")
	claudeList := mustList(t, s1, "claude: Backlog")
	untagged := mustList(t, s1, "Groceries")
	s1.Close()

	// Reopen: Open runs the backfill, adopting the "<tag>:" convention.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open (backfill): %v", err)
	}
	t.Cleanup(func() { s2.Close() })

	assertOwner := func(id, want string) {
		t.Helper()
		l, err := s2.GetList(id)
		if err != nil {
			t.Fatalf("GetList(%q): %v", id, err)
		}
		if l.CreatedBy != want {
			t.Fatalf("list %q (name %q) CreatedBy = %q, want %q", id, l.Name, l.CreatedBy, want)
		}
	}
	assertOwner(piList, "pi")
	assertOwner(claudeList, "claude")
	assertOwner(untagged, "") // no tag → owned by nobody
}

// TestBackfillIsIdempotent confirms repeated passes never clobber an owner
// that is already set: the backfill only touches rows where created_by = ”
// (so a hand-set owner survives), and an already-adopted tag is not re-applied.
func TestBackfillIsIdempotent(t *testing.T) {
	s := newTestStore(t)

	piList := mustList(t, s, "pi: Main")
	if err := s.backfillOwners(); err != nil {
		t.Fatalf("first backfillOwners: %v", err)
	}
	if err := s.backfillOwners(); err != nil {
		t.Fatalf("second backfillOwners: %v", err)
	}
	l, err := s.GetList(piList)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if l.CreatedBy != "pi" {
		t.Fatalf("after idempotent passes CreatedBy = %q, want %q", l.CreatedBy, "pi")
	}

	// A manually-set owner (not via the tag) must survive a later pass: the
	// backfill only adopts rows whose created_by is still empty.
	if _, err := s.db.Exec(`UPDATE List SET created_by = 'claude' WHERE id = ?`, piList); err != nil {
		t.Fatalf("override owner: %v", err)
	}
	if err := s.backfillOwners(); err != nil {
		t.Fatalf("third backfillOwners: %v", err)
	}
	l, err = s.GetList(piList)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if l.CreatedBy != "claude" {
		t.Fatalf("backfill overwrote a manually-set owner: CreatedBy = %q, want %q", l.CreatedBy, "claude")
	}
}
