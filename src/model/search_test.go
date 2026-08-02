package model

import (
	"testing"

	"github.com/filipemolina/chore-completer/src/cmds"
	"github.com/filipemolina/chore-completer/src/components/tasktree"
)

// selectedID reports the task tree's current selection, which the picker's
// jump must move onto.
func selectedID(t *testing.T, m AppModel) string {
	t.Helper()
	tree, ok := m.components.TaskTree.(tasktree.Model)
	if !ok {
		t.Fatalf("TaskTree is %T, want tasktree.Model", m.components.TaskTree)
	}
	return tree.SelectedID()
}

// The global picker jumped to a task in another list: the active list must
// switch to the result's list and the tree's selection must land on that exact
// task. The jump handler switches the list synchronously and issues a refresh
// plus a select; those two commands may arrive in either order, and the
// selection must end on the target either way (docs/plans/phase-8-search.md
// verification).
func TestJumpToTaskSwitchesListAndSelects(t *testing.T) {
	m := newTestModel(t, t.TempDir())

	listA, err := m.store.CreateList("Alpha")
	if err != nil {
		t.Fatalf("create list A: %v", err)
	}
	listB, err := m.store.CreateList("Beta")
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
	m = refresh(t, m, cmds.RefreshTasks(m.store, listA)())
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
		m2 = refresh(t, m2, cmds.RefreshTasks(m2.store, listB)())
		if got := selectedID(t, m2); got != target {
			t.Errorf("selection = %q, want %q", got, target)
		}
	})

	t.Run("refresh-then-select", func(t *testing.T) {
		m2 := refresh(t, m, cmds.RefreshTasks(m.store, listB)())
		m2 = refresh(t, m2, cmds.SelectTask(target)())
		if got := selectedID(t, m2); got != target {
			t.Errorf("selection = %q, want %q", got, target)
		}
	})
}