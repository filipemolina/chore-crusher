package store

import (
	"path/filepath"
	"slices"
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

// TestRenameListAdoptsTagIntoOwner pins adopt-on-tag: a rename into the
// "<tag>:" convention adopts the tag as owner in the same
// write (no store.Open needed), and an existing owner is never overwritten
// by a rename.
func TestRenameListAdoptsTagIntoOwner(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateList("Groceries", "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if err := s.RenameList(id, "pi: Groceries"); err != nil {
		t.Fatalf("RenameList into tag: %v", err)
	}
	got, err := s.GetList(id)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if got.CreatedBy != "pi" || got.Name != "pi: Groceries" {
		t.Fatalf("after rename-into-tag: %+v, want name pi: Groceries owned by pi", got)
	}

	// A rename never transfers ownership: claude's list keeps claude even
	// when renamed into a pi: name.
	id2, err := s.CreateList("claude: Backlog", "claude")
	if err != nil {
		t.Fatalf("CreateList(owned): %v", err)
	}
	if err := s.RenameList(id2, "pi: Backlog"); err != nil {
		t.Fatalf("RenameList over owner: %v", err)
	}
	got2, err := s.GetList(id2)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if got2.CreatedBy != "claude" {
		t.Fatalf("rename transferred ownership: %+v, want owner claude untouched", got2)
	}

	// A tagless rename stays untagged.
	id3, err := s.CreateList("Errands", "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if err := s.RenameList(id3, "Shopping"); err != nil {
		t.Fatalf("RenameList tagless: %v", err)
	}
	got3, err := s.GetList(id3)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if got3.CreatedBy != "" {
		t.Fatalf("tagless rename adopted an owner: %+v", got3)
	}
}

// TestGetOrCreateAgentListIsOwnerFirst pins H3: the lookup
// must be by created_by, not by the "<identity>:" name prefix. A list the
// human created in the CLI/TUI with a pi: name but an empty owner is foreign
// to every agent — returning it would make the next add_task fail with
// "owned by no one".
func TestGetOrCreateAgentListIsOwnerFirst(t *testing.T) {
	s := newTestStore(t)

	// TUI/CLI-shaped list: pi: name, empty owner.
	untagged, err := s.CreateList("pi: Notes", "")
	if err != nil {
		t.Fatalf("CreateList(pi: Notes, \"\"): %v", err)
	}

	// GetOrCreateAgentList must NOT return it; it creates a real owned list.
	id, err := s.GetOrCreateAgentList("pi")
	if err != nil {
		t.Fatalf("GetOrCreateAgentList(pi): %v", err)
	}
	if id == untagged {
		t.Fatalf("GetOrCreateAgentList returned the untagged name-prefixed list %q; it must not be adopted", id)
	}

	got, err := s.GetList(id)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if got.CreatedBy != "pi" || got.Name != "pi: Inbox" {
		t.Fatalf("GetOrCreateAgentList created list = %+v, want pi: Inbox owned by pi", got)
	}

	// Owner-first also the other way: on a fresh store whose only owned list is
	// named differently (e.g. a handoff from `crush lists add --owner pi`), the
	// lookup returns it instead of creating a second pi: Inbox.
	s2 := newTestStore(t)
	handoff, err := s2.CreateList("Sprint", "pi")
	if err != nil {
		t.Fatalf("CreateList(Sprint, pi): %v", err)
	}
	id2, err := s2.GetOrCreateAgentList("pi")
	if err != nil {
		t.Fatalf("GetOrCreateAgentList(pi) on owned-only store: %v", err)
	}
	if id2 != handoff {
		t.Fatalf("GetOrCreateAgentList = %q, want the pi-owned list %q (created_by, not name)", id2, handoff)
	}
	lists, err := s2.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(lists) != 1 {
		t.Fatalf("expected no pi: Inbox duplicate, got %d lists", len(lists))
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

// TestCollaborativeDefaultsFalse pins the migration's default: every list,
// new or pre-existing, starts non-collaborative until a human opts it in.
// newTestStore always opens through the full migration chain, so this also
// stands in for "the migration applies and defaults existing rows to false."
func TestCollaborativeDefaultsFalse(t *testing.T) {
	s := newTestStore(t)
	id := mustList(t, s, "Groceries")

	l, err := s.GetList(id)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if l.Collaborative {
		t.Error("Collaborative = true, want false by default")
	}

	summaries, err := s.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(summaries) != 1 || summaries[0].Collaborative {
		t.Errorf("ListLists = %+v, want one summary with Collaborative=false", summaries)
	}
}

// TestSetCollaborativeRoundTrips pins the write path through both read
// shapes — GetList and ListLists must agree.
func TestSetCollaborativeRoundTrips(t *testing.T) {
	s := newTestStore(t)
	id := mustList(t, s, "Shared backlog")

	if err := s.SetCollaborative(id, true); err != nil {
		t.Fatalf("SetCollaborative(true): %v", err)
	}
	l, err := s.GetList(id)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if !l.Collaborative {
		t.Error("Collaborative = false after SetCollaborative(true)")
	}
	summaries, err := s.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(summaries) != 1 || !summaries[0].Collaborative {
		t.Errorf("ListLists = %+v, want Collaborative=true", summaries)
	}

	if err := s.SetCollaborative(id, false); err != nil {
		t.Fatalf("SetCollaborative(false): %v", err)
	}
	l, err = s.GetList(id)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if l.Collaborative {
		t.Error("Collaborative = true after SetCollaborative(false)")
	}
}

// TestSetCollaborativeRequiresExisting mirrors TestDeleteListRequiresExisting.
func TestSetCollaborativeRequiresExisting(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetCollaborative("no-such-id", true); err == nil {
		t.Fatal("SetCollaborative on a missing id did not error")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("SetCollaborative error = %q, want a not-found error", err)
	}
}

// listOrderIDs returns the ids of every list, in ordering position.
func listOrderIDs(t *testing.T, s *Store) []string {
	t.Helper()
	ls, err := s.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	ids := make([]string, len(ls))
	for i, l := range ls {
		ids[i] = l.ID
	}
	return ids
}

// TestMoveListReordersWithinOrdering: moving a list after a later list shifts
// it down within the same ordering.
func TestMoveListReordersWithinOrdering(t *testing.T) {
	s := newTestStore(t)
	a := mustList(t, s, "a")
	b := mustList(t, s, "b")
	c := mustList(t, s, "c")
	d := mustList(t, s, "d")

	if err := s.MoveList(b, d); err != nil {
		t.Fatalf("MoveList: %v", err)
	}
	want := []string{a, c, d, b}
	if got := listOrderIDs(t, s); !slices.Equal(got, want) {
		t.Errorf("order after move = %v, want %v", got, want)
	}
}

// TestMoveListToFront: an empty afterID moves the list to the front of the
// ordering.
func TestMoveListToFront(t *testing.T) {
	s := newTestStore(t)
	a := mustList(t, s, "a")
	b := mustList(t, s, "b")
	c := mustList(t, s, "c")

	if err := s.MoveList(c, ""); err != nil {
		t.Fatalf("MoveList: %v", err)
	}
	want := []string{c, a, b}
	if got := listOrderIDs(t, s); !slices.Equal(got, want) {
		t.Errorf("order after move-to-front = %v, want %v", got, want)
	}
}

// TestMoveListMovesToFrontOfOrdering: after moving to the front, a second
// move down lands the list after the first list, not at the front again.
func TestMoveListMovesToFrontOfOrdering(t *testing.T) {
	s := newTestStore(t)
	a := mustList(t, s, "a")
	b := mustList(t, s, "b")
	c := mustList(t, s, "c")

	if err := s.MoveList(c, ""); err != nil {
		t.Fatalf("MoveList to front: %v", err)
	}
	if err := s.MoveList(c, a); err != nil {
		t.Fatalf("MoveList after a: %v", err)
	}
	want := []string{a, c, b}
	if got := listOrderIDs(t, s); !slices.Equal(got, want) {
		t.Errorf("order after moves = %v, want %v", got, want)
	}
}

// TestMoveListValidation: the target must exist and not be the list itself.
func TestMoveListValidation(t *testing.T) {
	s := newTestStore(t)
	a := mustList(t, s, "a")

	if err := s.MoveList(a, a); err == nil {
		t.Error("MoveList after itself did not error")
	}
	if err := s.MoveList(a, "no-such-id"); err == nil {
		t.Error("MoveList after a missing list did not error")
	}
	if err := s.MoveList("no-such-id", ""); err == nil {
		t.Error("MoveList of a missing list did not error")
	}
}
