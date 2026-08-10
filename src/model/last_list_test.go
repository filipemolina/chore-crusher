package model

import (
	"testing"

	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/store"
)

// lastListSetting reads the persisted last-active-list id straight from the
// store, bypassing the model, so a test can assert what a *fresh* launch
// would restore.
func lastListSetting(t *testing.T, m AppModel) string {
	t.Helper()
	got, err := m.store.GetSetting(store.KeyLastListID)
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	return got
}

// seedTwoLists drops the constructor's default list and creates two lists
// (A first, B second) inside the given data dir, returning the model and the
// ids. The first lists refresh adopts A (docs/DESIGN.md §7), so a test can
// then switch to B and back. The dir is passed in so a test can later build
// a *fresh* model over the same store for a relaunch.
func seedTwoLists(t *testing.T, dataDir string) (AppModel, string, string) {
	t.Helper()
	m := newTestModel(t, dataDir)
	lists, _ := m.store.ListLists()
	for _, l := range lists {
		m.store.DeleteList(l.ID)
	}
	listA, err := m.store.CreateList("A", "")
	if err != nil {
		t.Fatalf("create list A: %v", err)
	}
	listB, err := m.store.CreateList("B", "")
	if err != nil {
		t.Fatalf("create list B: %v", err)
	}
	m = refresh(t, m, cmds.RefreshLists(m.store)())
	if m.activeListID != listA {
		t.Fatalf("setup: activeListID = %q, want %q (first list)", m.activeListID, listA)
	}
	return m, listA, listB
}

// The first lists refresh restores the list the user last had active, not
// always the first list: a saved last-list id that still exists wins over
// msg.Lists[0] (docs/DESIGN.md §7). The persisted id is written by the same
// model on a switch, so this round-trips through the store like a real
// relaunch.
func TestFirstRefreshRestoresLastList(t *testing.T) {
	dir := t.TempDir()
	m, _, listB := seedTwoLists(t, dir)

	// Switch to B the way a user would; the switch persists B as the last
	// active list.
	m = refresh(t, m, cmds.SelectListMsg{ListID: listB})
	if got := lastListSetting(t, m); got != listB {
		t.Fatalf("persisted last list after switch = %q, want %q", got, listB)
	}

	// Relaunch: a fresh model over the same store must come up on B.
	fresh := newTestModel(t, dir)
	fresh = refresh(t, fresh, cmds.RefreshLists(fresh.store)())
	if fresh.activeListID != listB {
		t.Errorf("fresh launch activeListID = %q, want %q (the last list)", fresh.activeListID, listB)
	}
}

// A saved last list that no longer exists (the list was deleted while the
// app was closed) falls back to the first remaining list, exactly like a
// first run with no saved id at all.
func TestFirstRefreshFallsBackWhenLastListDeleted(t *testing.T) {
	dir := t.TempDir()
	m, listA, listB := seedTwoLists(t, dir)
	m = refresh(t, m, cmds.SelectListMsg{ListID: listB})
	if got := lastListSetting(t, m); got != listB {
		t.Fatalf("setup: persisted last list = %q, want %q", got, listB)
	}

	// The last list is gone; relaunch over the same store.
	if err := m.store.DeleteList(listB); err != nil {
		t.Fatalf("DeleteList: %v", err)
	}
	fresh := newTestModel(t, dir)
	fresh = refresh(t, fresh, cmds.RefreshLists(fresh.store)())
	if fresh.activeListID != listA {
		t.Errorf("fresh launch activeListID = %q, want %q (fallback to the first list)", fresh.activeListID, listA)
	}
}

// A first run with no saved id (fresh store, or the setting never written)
// keeps the pre-existing behavior: the first list wins.
func TestFirstRefreshSelectsFirstListWhenNothingSaved(t *testing.T) {
	m := newTestModel(t, t.TempDir())
	// A fresh store has no lists until the first refresh creates a default
	// one; create the list explicitly so this test's "first list" is its own.
	listID, err := m.store.CreateList("Errands", "")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	if got := lastListSetting(t, m); got != "" {
		t.Fatalf("setup: last list already saved = %q, want \"\"", got)
	}

	m = refresh(t, m, cmds.RefreshLists(m.store)())
	if m.activeListID != listID {
		t.Errorf("activeListID = %q, want %q (first list, nothing saved)", m.activeListID, listID)
	}
	// The fallback choice is persisted, so even an unsaved first run records
	// where the user landed.
	if got := lastListSetting(t, m); got != listID {
		t.Errorf("persisted last list = %q, want %q", got, listID)
	}
}

// Selecting a list persists it as the last active list, so the next launch
// reopens it (docs/DESIGN.md §7).
func TestSelectListPersistsLastList(t *testing.T) {
	m, _, listB := seedTwoLists(t, t.TempDir())

	m = refresh(t, m, cmds.SelectListMsg{ListID: listB})
	if m.activeListID != listB {
		t.Fatalf("activeListID = %q, want %q", m.activeListID, listB)
	}
	if got := lastListSetting(t, m); got != listB {
		t.Errorf("persisted last list = %q, want %q", got, listB)
	}
}

// Creating a list lands on it and persists it, so a launch after a session
// that created a list reopens that list, not the one that existed before.
func TestListCreatedPersistsLastList(t *testing.T) {
	m, _, _ := seedTwoLists(t, t.TempDir())
	newID, err := m.store.CreateList("C", "")
	if err != nil {
		t.Fatalf("create list C: %v", err)
	}

	m = refresh(t, m, cmds.ListCreatedMsg{ID: newID})
	if got := lastListSetting(t, m); got != newID {
		t.Errorf("persisted last list after creation = %q, want %q", got, newID)
	}
}

// Jumping to a task in another list switches and persists the destination
// list, the same way a plain select does.
func TestJumpToTaskPersistsLastList(t *testing.T) {
	m, _, listB := seedTwoLists(t, t.TempDir())
	if _, err := m.store.CreateTask(listB, "target", nil, ""); err != nil {
		t.Fatalf("create task: %v", err)
	}

	m = refresh(t, m, cmds.JumpToTaskMsg{ListID: listB, TaskID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"})
	if m.activeListID != listB {
		t.Fatalf("activeListID = %q, want %q", m.activeListID, listB)
	}
	if got := lastListSetting(t, m); got != listB {
		t.Errorf("persisted last list = %q, want %q", got, listB)
	}
}
