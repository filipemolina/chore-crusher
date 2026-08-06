package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/constants"
)

// treeFilterActive reports whether the task tree's /-filter is open or
// applied, through the tasks panel accessor.
func treeFilterActive(t *testing.T, m AppModel) bool {
	t.Helper()
	panel, ok := m.components.TaskPanel.(interface{ FilterActive() bool })
	if !ok {
		t.Fatalf("TaskPanel is %T, want FilterActive accessor", m.components.TaskPanel)
	}
	return panel.FilterActive()
}

// listsFilterActive reports whether the lists panel's own filter is open or
// applied.
func listsFilterActive(t *testing.T, m AppModel) bool {
	t.Helper()
	panel, ok := m.components.ListsPanel.(interface{ FilterActive() bool })
	if !ok {
		t.Fatalf("ListsPanel is %T, want FilterActive accessor", m.components.ListsPanel)
	}
	return panel.FilterActive()
}

// seedOneList seeds the store with one list containing one task and returns
// the ids, with the app loaded onto that list.
func seedOneList(t *testing.T) AppModel {
	t.Helper()
	m := newTestModel(t, t.TempDir())
	lists, _ := m.store.ListLists()
	listID := ""
	if len(lists) > 0 {
		listID = lists[0].List.ID
	} else {
		id, err := m.store.CreateList("L", "")
		if err != nil {
			t.Fatalf("create list: %v", err)
		}
		listID = id
	}
	if _, err := m.store.CreateTask(listID, "one", nil, ""); err != nil {
		t.Fatalf("create task: %v", err)
	}
	m = refresh(t, m, cmds.RefreshLists(m.store)())
	m = refresh(t, m, cmds.RefreshTasks(m.store, listID)())
	// GetInitialModel does not run Init, so broadcast the startup focus to the
	// tree; without it KeepsEsc reads focused=false and the esc ladder
	// swallows every esc.
	m = refresh(t, m, cmds.SetFocus(constants.COMPONENT_TASK_TREE)())
	return m
}

// Bug 1 regression, driven through the esc ladder: esc while the tree's
// filter input is open must reach the tree and clear the filter, not be
// swallowed by AppModel.Update.
func TestEscClearsFilterWhileTyping(t *testing.T) {
	m := seedOneList(t)

	m = refresh(t, m, tea.KeyPressMsg{Text: "/", Code: '/'})
	if !treeFilterActive(t, m) {
		t.Fatal("/ did not open the tree filter")
	}
	m = refresh(t, m, tea.KeyPressMsg{Text: "z", Code: 'z'})
	if !treeFilterActive(t, m) {
		t.Fatal("typing a query should keep the filter open")
	}

	m = refresh(t, m, tea.KeyPressMsg{Text: "esc"})
	if treeFilterActive(t, m) {
		t.Error("esc while typing did not clear the filter (ladder swallowed it)")
	}
}

// Bug 2: / is contextual. Focused on the lists panel it opens the lists
// panel's own filter; focused on the tree it opens the tree's filter.
func TestListsFilterFollowsFocus(t *testing.T) {
	m := seedOneList(t)

	// Open the lists panel; focus lands on it.
	m = refresh(t, m, tea.KeyPressMsg{Text: "L", Code: 'L'})
	if m.focusedZone != constants.COMPONENT_LISTS_PANEL {
		t.Fatalf("after L, focused zone = %d, want lists panel", m.focusedZone)
	}

	// / while lists is focused: the lists filter opens, the tree's does not.
	m = refresh(t, m, tea.KeyPressMsg{Text: "/", Code: '/'})
	if !listsFilterActive(t, m) {
		t.Error("/ with lists focused did not open the lists filter")
	}
	if treeFilterActive(t, m) {
		t.Error("/ with lists focused leaked into the tree filter")
	}

	// esc clears the lists filter; tab back to the tree; / targets the tree.
	m = refresh(t, m, tea.KeyPressMsg{Text: "esc"})
	if listsFilterActive(t, m) {
		t.Fatal("esc did not clear the lists filter")
	}
	m = refresh(t, m, tea.KeyPressMsg{Text: "tab"})
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Fatalf("after tab, focused zone = %d, want task tree", m.focusedZone)
	}
	m = refresh(t, m, tea.KeyPressMsg{Text: "/", Code: '/'})
	if !treeFilterActive(t, m) {
		t.Error("/ with tree focused did not open the tree filter")
	}
	if listsFilterActive(t, m) {
		t.Error("/ with tree focused leaked into the lists filter")
	}
}

// listsPanelSelectedID reads the lists panel's highlighted list id.
func listsPanelSelectedID(t *testing.T, m AppModel) string {
	t.Helper()
	lists, ok := m.components.ListsPanel.(interface{ SelectedListID() string })
	if !ok {
		t.Fatalf("ListsPanel is %T, want SelectedListID accessor", m.components.ListsPanel)
	}
	return lists.SelectedListID()
}

// Bug 6: the delete handler must remove the list highlighted in the lists
// panel, not the list currently open in the tasks panel. The two diverge
// whenever the active list changes without the panel cursor moving — the
// global picker (F) jumping to a task in another list is the concrete path
// (JumpToTaskMsg switches activeListID; the panel cursor only moves on its
// own keypresses). Before the fix the handler read m.activeListID, so
// confirming deleted the open list while the user was pointing at the
// highlighted one.
func TestDeleteDeletesHighlightedListNotActive(t *testing.T) {
	m := newTestModel(t, t.TempDir())
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
	taskB, err := m.store.CreateTask(listB, "task in B", nil, "")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	// First refresh adopts A; the panel cursor lands on A.
	m = refresh(t, m, cmds.RefreshLists(m.store)())
	if m.activeListID != listA {
		t.Fatalf("setup: activeListID = %q, want A", m.activeListID)
	}

	// The picker jumps to a task in B: the active list becomes B while the
	// panel's cursor stays on A.
	m = refresh(t, m, cmds.JumpToTaskMsg{TaskID: taskB, ListID: listB})
	if m.activeListID != listB {
		t.Fatalf("setup: after picker jump activeListID = %q, want B", m.activeListID)
	}

	// Open and focus the lists panel: A is highlighted, B is active.
	m.listsPanelVisible = true
	m = refresh(t, m, m.ChangeFocus(1)())
	if got := listsPanelSelectedID(t, m); got != listA {
		t.Fatalf("setup: panel highlight = %q, want A", got)
	}

	// Delete and confirm: the highlighted list (A) must go, B must survive.
	m = refresh(t, m, tea.KeyPressMsg{Text: "d"})
	m = refresh(t, m, tea.KeyPressMsg{Text: "y"})

	remaining, _ := m.store.ListLists()
	ids := make(map[string]bool, len(remaining))
	for _, l := range remaining {
		ids[l.List.ID] = true
	}
	if ids[listA] {
		t.Error("highlighted list A was not deleted")
	}
	if !ids[listB] {
		t.Error("active list B was deleted; delete must target the highlighted list")
	}
}

// Bug 6 follow-on: bubbles' list does not clamp a stale cursor on SetItems,
// so after deleting the highlighted list the panel cursor can point past the
// end of the surviving items, leaving no selection at all. The panel must
// recover the highlight onto the new last item and re-broadcast it so the
// active list follows (the app's highlight==active invariant).
func TestDeleteRecoversPanelCursor(t *testing.T) {
	m := newTestModel(t, t.TempDir())
	lists, _ := m.store.ListLists()
	for _, l := range lists {
		m.store.DeleteList(l.ID)
	}
	listA, _ := m.store.CreateList("A", "")
	listB, _ := m.store.CreateList("B", "")
	listC, _ := m.store.CreateList("C", "")
	_ = listA

	m.listsPanelVisible = true
	m = refresh(t, m, cmds.RefreshLists(m.store)())
	m = refresh(t, m, m.ChangeFocus(1)())

	// Move the cursor down to C (index 2).
	m = refresh(t, m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	m = refresh(t, m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	if got := listsPanelSelectedID(t, m); got != listC {
		t.Fatalf("setup: panel highlight = %q, want C", got)
	}

	// Delete C: the cursor was on the last item, so after the refresh it is
	// past the end of the two survivors; the panel must move the highlight
	// to B (the new last item) and the active list must follow.
	m = refresh(t, m, tea.KeyPressMsg{Text: "d"})
	m = refresh(t, m, tea.KeyPressMsg{Text: "y"})

	if got := listsPanelSelectedID(t, m); got != listB {
		t.Errorf("panel highlight after delete = %q, want B (stale cursor left no selection)", got)
	}
	if m.activeListID != listB {
		t.Errorf("activeListID after delete = %q, want B (must follow the recovered highlight)", m.activeListID)
	}
	remaining, _ := m.store.ListLists()
	if len(remaining) != 2 {
		t.Errorf("remaining lists = %d, want 2", len(remaining))
	}
}

// Bug 6 sibling: the rename handler targets m.activeListID with the same
// "open list, not highlighted list" mistake. Renaming must rename the
// highlighted list.
func TestRenameTargetsHighlightedList(t *testing.T) {
	m := newTestModel(t, t.TempDir())
	lists, _ := m.store.ListLists()
	for _, l := range lists {
		m.store.DeleteList(l.ID)
	}
	listA, _ := m.store.CreateList("A", "")
	listB, _ := m.store.CreateList("B", "")
	taskB, _ := m.store.CreateTask(listB, "task in B", nil, "")

	m = refresh(t, m, cmds.RefreshLists(m.store)())
	m = refresh(t, m, cmds.JumpToTaskMsg{TaskID: taskB, ListID: listB})
	m.listsPanelVisible = true
	m = refresh(t, m, m.ChangeFocus(1)())
	if got := listsPanelSelectedID(t, m); got != listA {
		t.Fatalf("setup: panel highlight = %q, want A", got)
	}

	// applyOnce: typing into the rename input returns a cursor-blink cmd the
	// synchronous refresh helper cannot chase (Blink -> BlinkMsg loop).
	applyOnce := func(m AppModel, msg tea.Msg) AppModel {
		t.Helper()
		updated, _ := m.Update(msg)
		out, ok := updated.(AppModel)
		if !ok {
			t.Fatalf("Update returned %T, want AppModel", updated)
		}
		return out
	}

	m = refresh(t, m, tea.KeyPressMsg{Text: "R"})
	m = applyOnce(m, tea.KeyPressMsg{Text: "X", Code: 'X'})
	m = refresh(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	remaining, _ := m.store.ListLists()
	for _, l := range remaining {
		switch l.List.ID {
		case listA:
			if l.List.Name != "X" {
				t.Errorf("highlighted list A renamed to %q, want %q", l.List.Name, "X")
			}
		case listB:
			if l.List.Name != "B" {
				t.Errorf("active list B was renamed to %q instead of the highlighted A", l.List.Name)
			}
		}
	}
}

// Bug 6: a failing DeleteList must reach the model's error channel instead
// of being swallowed (the old handler returned nil on error). The confirm
// callback reports through the same RefreshListsMsg{Err} the refresh command
// itself uses, which AppModel records in lastError.
func TestDeleteErrorSurfacesInLastError(t *testing.T) {
	m := newTestModel(t, t.TempDir())
	lists, _ := m.store.ListLists()
	for _, l := range lists {
		m.store.DeleteList(l.ID)
	}
	listA, _ := m.store.CreateList("A", "")

	m.listsPanelVisible = true
	m = refresh(t, m, cmds.RefreshLists(m.store)())
	m = refresh(t, m, m.ChangeFocus(1)())

	// Open the confirm modal for A, then delete A behind the app's back so
	// the confirm's DeleteList hits a missing row.
	m = refresh(t, m, tea.KeyPressMsg{Text: "d"})
	if err := m.store.DeleteList(listA); err != nil {
		t.Fatalf("delete behind app's back: %v", err)
	}
	m = refresh(t, m, tea.KeyPressMsg{Text: "y"})

	if m.lastError == "" {
		t.Error("failed delete left lastError empty; the error was swallowed")
	}
}

// Bug 3: a paste message while the details modal is open must reach the
// notes textarea. AppModel previously forwarded only key presses to modals,
// so the paste was dropped before the textarea ever saw it.
func TestPasteReachesDetailsNotes(t *testing.T) {
	m := seedOneList(t)
	lists, _ := m.store.ListLists()
	tasks, _ := m.store.ListTasks(lists[0].List.ID)
	if len(tasks) == 0 {
		t.Fatal("seed: no task rows")
	}
	taskID := tasks[0].ID

	m = refresh(t, m, cmds.OpenDetails(taskID)())
	if m.activeModal == nil {
		t.Fatal("details modal did not open")
	}
	// Drive the paste through AppModel.Update directly: the modal forwards
	// every message to the focused textarea, whose blink command is a timed
	// loop the synchronous refresh helper cannot run.
	out, _ := m.Update(tea.PasteMsg{Content: "hello world"})
	m = out.(AppModel)

	modal, ok := m.activeModal.(interface{ NotesValue() string })
	if !ok {
		t.Fatalf("active modal is %T, want NotesValue accessor", m.activeModal)
	}
	if got := modal.NotesValue(); !strings.Contains(got, "hello world") {
		t.Errorf("notes = %q, want pasted content", got)
	}
}
