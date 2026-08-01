package store

import (
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

func TestDeleteListRequiresExisting(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteList("no-such-id")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("DeleteList on missing id error = %v, want a not-found error", err)
	}
}
