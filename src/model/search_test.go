package model

import (
	"testing"

	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
)

// selectedID reports the task tree's current selection, which the picker's
// jump must move onto.
func selectedID(t *testing.T, m AppModel) string {
	t.Helper()
	tree, ok := m.components.TaskPanel.(interface{ SelectedID() string })
	if !ok {
		t.Fatalf("TaskPanel is %T, want selected-ID accessor", m.components.TaskPanel)
	}
	return tree.SelectedID()
}

// The global picker jumped to a task in another list: the active list must
// switch to the result's list and the tree's selection must land on that exact
// task. The jump handler switches the list synchronously and issues a refresh
// plus a select; those two commands may arrive in either order, and the
// selection must end on the target either way.
func TestJumpToTaskSwitchesListAndSelects(t *testing.T) {
	m := newTestModel(t, t.TempDir())
	// GetInitialModel creates a default list when the store is empty; remove it
	// so this test's first list is the one it creates below.
	lists, _ := m.store.ListLists()
	if len(lists) > 0 {
		m.store.DeleteList(lists[0].ID)
	}

	listA, err := m.store.CreateList("Alpha", "")
	if err != nil {
		t.Fatalf("create list A: %v", err)
	}
	listB, err := m.store.CreateList("Beta", "")
	if err != nil {
		t.Fatalf("create list B: %v", err)
	}
	if _, err := m.store.CreateTask(listA, "First", nil, ""); err != nil {
		t.Fatalf("create task A: %v", err)
	}
	target, err := m.store.CreateTask(listB, "Needle", nil, "")
	if err != nil {
		t.Fatalf("create task B: %v", err)
	}

	// Seed the app into list A as active with its tasks loaded: the first
	// lists refresh adopts the first list and requests its tasks.
	m = refresh(t, m, cmds.RefreshLists(m.store)())
	m = refresh(t, m, cmds.RefreshTasks(m.store, listA, apptypes.SortManual)())
	if m.activeListID != listA {
		t.Fatalf("seed: activeListID = %q, want %q", m.activeListID, listA)
	}

	// The picker's enter delivers the jump to AppModel, which switches the
	// active list synchronously.
	out, _ := m.Update(cmds.JumpToTaskMsg{TaskID: target, ListID: listB})
	m = out.(AppModel)
	if m.activeListID != listB {
		t.Errorf("activeListID = %q, want %q (switched to the result's list)", m.activeListID, listB)
	}

	// The jump queued a refresh and a select; drive them in both orders. The
	// selection must be the matched task either way.
	t.Run("select-then-refresh", func(t *testing.T) {
		// Select against the old (list A) rows: the target is not present, so
		// the tree must remember the request and honour it once list B loads.
		m2 := refresh(t, m, cmds.SelectTask(target)())
		m2 = refresh(t, m2, cmds.RefreshTasks(m2.store, listB, apptypes.SortManual)())
		if got := selectedID(t, m2); got != target {
			t.Errorf("selection = %q, want %q", got, target)
		}
	})

	t.Run("refresh-then-select", func(t *testing.T) {
		m2 := refresh(t, m, cmds.RefreshTasks(m.store, listB, apptypes.SortManual)())
		m2 = refresh(t, m2, cmds.SelectTask(target)())
		if got := selectedID(t, m2); got != target {
			t.Errorf("selection = %q, want %q", got, target)
		}
	})
}
