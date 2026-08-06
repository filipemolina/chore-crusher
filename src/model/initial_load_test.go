package model

import (
	"testing"

	"github.com/filipemolina/chore-crusher/src/cmds"
)

// GetInitialModel must do no database work: it constructs components and
// returns with no active list, so Bubble Tea can paint the first frame (the
// Tasks panel's loading animation) before the opening Lists query runs. The
// default-list creation moved to the first RefreshListsMsg.
func TestConstructorDoesNoDatabaseWork(t *testing.T) {
	// newTestModel builds via GetInitialModel and delivers one WindowSizeMsg;
	// neither touches the store, so an empty store stays empty until a refresh.
	m := newTestModel(t, t.TempDir())

	lists, err := m.store.ListLists()
	if err != nil {
		t.Fatalf("list lists: %v", err)
	}
	if len(lists) != 0 {
		t.Errorf("constructor created %d lists, want 0 (no DB work before the first refresh)", len(lists))
	}
	if m.activeListID != "" {
		t.Errorf("constructor set activeListID = %q, want empty before the first refresh", m.activeListID)
	}
}

// The first Lists refresh against an empty store creates exactly one default
// "New List" and adopts it as the active list, preserving the old constructor's
// invariant without a second stray list from the follow-up refresh it issues.
func TestFirstRefreshCreatesExactlyOneDefaultList(t *testing.T) {
	m := newTestModel(t, t.TempDir())

	m = refresh(t, m, cmds.RefreshLists(m.store)())

	lists, _ := m.store.ListLists()
	if len(lists) != 1 {
		t.Fatalf("first refresh created %d lists, want exactly 1 default list", len(lists))
	}
	if lists[0].List.Name != "New List" {
		t.Errorf("default list name = %q, want %q", lists[0].List.Name, "New List")
	}
	if m.activeListID != lists[0].List.ID {
		t.Errorf("activeListID = %q, want the new default list %q", m.activeListID, lists[0].List.ID)
	}
}
