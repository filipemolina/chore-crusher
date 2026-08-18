package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/constants"
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
	m = refresh(t, m, cmds.RefreshTasks(m.store, listID, apptypes.SortManual)())
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
	m.bodyLayout = m.calculateBodyLayout()
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

// taskParent returns a task's parent id, or "" for a root task.
func taskParent(t *testing.T, m AppModel, id string) string {
	t.Helper()
	task, err := m.store.GetTask(id)
	if err != nil {
		t.Fatalf("get task %s: %v", id, err)
	}
	if task.ParentID == nil {
		return ""
	}
	return *task.ParentID
}

// Bug 4: a second indent pressed inside the first indent's refresh window
// must act on the post-change state, not on the stale rows. The report: 1,
// 2, 3; ] on 2 (2 becomes a child of 1); then ] on 3 — if the second ]
// lands before the first refresh applies, the tree computes 3's previous
// sibling from the pre-reparent rows (task 2) and nests 3 under 2 instead
// of under 1. Reproduced in the real TUI by sending j]j] with no delay;
// this test forces the same window by holding the first ]'s command
// instead of chasing it.
func TestIndentDuringRefreshWindowTargetsPostChangeState(t *testing.T) {
	m := newTestModel(t, t.TempDir())
	// GetInitialModel no longer creates the default list synchronously; the
	// first Lists refresh does. Drive it so the empty store gets its default list
	// and activeListID is populated before this test seeds tasks into it.
	m = refresh(t, m, cmds.RefreshLists(m.store)())
	listID := m.activeListID
	if listID == "" {
		t.Fatal("no initial list to seed into")
	}
	one, err := m.store.CreateTask(listID, "1", nil, "")
	if err != nil {
		t.Fatalf("create 1: %v", err)
	}
	two, err := m.store.CreateTask(listID, "2", nil, "")
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	three, err := m.store.CreateTask(listID, "3", nil, "")
	if err != nil {
		t.Fatalf("create 3: %v", err)
	}
	m = refresh(t, m, cmds.RefreshLists(m.store)())
	m = refresh(t, m, cmds.RefreshTasks(m.store, listID, apptypes.SortManual)())
	m = refresh(t, m, cmds.SetFocus(constants.COMPONENT_TASK_TREE)())

	// Select 2 and start its indent, holding the command so the tree's rows
	// stay stale — the store has not seen the reparent yet.
	m = refresh(t, m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	held, cmd := m.Update(tea.KeyPressMsg{Text: "]", Code: ']'})
	var ok bool
	if m, ok = held.(AppModel); !ok {
		t.Fatalf("Update returned %T, want AppModel", held)
	}

	// Move to 3 and press ] while the rows are still stale.
	m = refresh(t, m, tea.KeyPressMsg{Text: "j", Code: 'j'})
	m = refresh(t, m, tea.KeyPressMsg{Text: "]", Code: ']'})

	// Let the held reparent land; the refresh replays the deferred ] on 3.
	m = refresh(t, m, cmd())

	if got := taskParent(t, m, three); got != one {
		t.Errorf("task 3 parent = %q, want %q (task 1): the second ] used stale rows", got, one)
	}
	if got := taskParent(t, m, two); got != one {
		t.Errorf("task 2 parent = %q, want %q (task 1)", got, one)
	}
	// The deferred gesture kept its target task, and the cursor stayed where
	// the user left it (task 3).
	panel, ok := m.components.TaskPanel.(interface{ SelectedID() string })
	if !ok {
		t.Fatalf("TaskPanel is %T, want SelectedID accessor", m.components.TaskPanel)
	}
	if got := panel.SelectedID(); got != three {
		t.Errorf("selection = %q, want %q (task 3)", got, three)
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
	m.bodyLayout = m.calculateBodyLayout()
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
	m.bodyLayout = m.calculateBodyLayout()
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
	m.bodyLayout = m.calculateBodyLayout()
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

// Bug 3: a paste message while the Details panel is open must reach the
// notes textarea. A paste is a non-key message, so it flows through the
// ordinary component fan-out (the Details key-ownership branch captures only
// keypresses) and lands on the focused panel's textarea.
func TestPasteReachesDetailsNotes(t *testing.T) {
	m := seedOneList(t)
	lists, _ := m.store.ListLists()
	tasks, _ := m.store.ListTasks(lists[0].List.ID)
	if len(tasks) == 0 {
		t.Fatal("seed: no task rows")
	}
	taskID := tasks[0].ID

	m = refresh(t, m, cmds.OpenDetails(taskID)())
	if !m.detailsPanelVisible {
		t.Fatal("details panel did not open")
	}
	// Move focus from the Title entry field into Notes so the paste lands there.
	m = stepKey(t, m, tea.KeyPressMsg{Text: "tab"})
	// Drive the paste through AppModel.Update directly: the panel forwards
	// non-key messages to the focused textarea, whose blink command is a timed
	// loop the synchronous refresh helper cannot run.
	out, _ := m.Update(tea.PasteMsg{Content: "hello world"})
	m = out.(AppModel)

	panel, ok := m.components.DetailsPanel.(interface{ NotesValue() string })
	if !ok {
		t.Fatalf("details panel is %T, want NotesValue accessor", m.components.DetailsPanel)
	}
	if got := panel.NotesValue(); !strings.Contains(got, "hello world") {
		t.Errorf("notes = %q, want pasted content", got)
	}
}

// Footer key hints must describe the mode the app is in right now, not the
// mode it was in one keystroke ago: pressing n opens the inline create
// input, and the very first rendered frame after that keypress — not the
// second one — must already show the create hints (bug: footer key hints
// lag the current mode by one keystroke). The stale rendering came from
// View() reading the keybinding bar's own ctx field, which the bar only
// updates once SetFooterContextMsg arrives on a later Update cycle;
// AppModel.View now hands the bar this frame's context directly.
func TestFooterShowsCreateHintsOnTheFrameAfterN(t *testing.T) {
	m := seedOneList(t)
	// The keybinding bar only learns the terminal width via a fanned-out
	// SetBodyLayoutMsg (newTestModel's WindowSizeMsg discards the command
	// that would normally deliver it) and renders empty at width 0.
	// Re-broadcast the model's own already-correct bodyLayout — the same
	// trick layout_test.go's startup() uses — rather than fabricating one,
	// since a hand-built Height overrides the header/footer-adjusted body
	// height and pushes the footer off the bottom of the frame.
	m = refresh(t, m, m.bodyLayout)

	updated, _ := m.Update(tea.KeyPressMsg{Text: "n", Code: 'n'})
	m = updated.(AppModel)

	out := m.View().Content
	for _, label := range []string{"create", "cancel"} {
		if !strings.Contains(out, label) {
			t.Errorf("footer on the frame right after n = %q missing, want create hints:\n%s", label, out)
		}
	}
	if strings.Contains(out, "navigate") {
		t.Errorf("footer on the frame right after n still shows browse hints:\n%s", out)
	}
}

// Bug (task 00003803MAY3SMPHM9YPS8VAXR): typing e or i into the lists
// panel's /-filter still opened the export/import modals. The lists CRUD
// cases in AppModel.Update were guarded only on visibility+focus, not on
// keyboardOwned(), so while the filter input owned the keyboard the
// printable letters n, R, d, e and i matched their modal-openers instead of
// typing into the query — unlike every global key (already suppressed) and
// Lists.Select's enter (already guarded). The suppressed keypress falls
// through to the component fan-out, where the list's own filter input
// consumes it.
func TestListsFilterTypingDoesNotOpenCRUDModals(t *testing.T) {
	m := seedOneList(t)

	// Open the lists panel; focus lands on it.
	m = refresh(t, m, tea.KeyPressMsg{Text: "L", Code: 'L'})
	if m.focusedZone != constants.COMPONENT_LISTS_PANEL {
		t.Fatalf("after L, focused zone = %d, want lists panel", m.focusedZone)
	}

	// / opens the lists panel's own filter; the panel now owns the keyboard.
	m = refresh(t, m, tea.KeyPressMsg{Text: "/", Code: '/'})
	if !listsFilterActive(t, m) {
		t.Fatal("/ with lists focused did not open the lists filter")
	}

	// applyOnce delivers msg without chasing the returned cmd: every
	// keystroke into the focused filter input returns a cursor-blink
	// rescheduling cmd (~530ms per hop) that refresh() would chase forever.
	applyOnce := func(m AppModel, msg tea.Msg) AppModel {
		t.Helper()
		updated, _ := m.Update(msg)
		out, ok := updated.(AppModel)
		if !ok {
			t.Fatalf("Update returned %T, want AppModel", updated)
		}
		return out
	}

	// Every printable shortcut bound in the Lists context — n (new), R
	// (rename), d (delete), e (export), i (import) — must type into the
	// filter query and open no modal while the filter input is live.
	typed := ""
	for _, k := range []string{"n", "R", "d", "e", "i"} {
		m = applyOnce(m, tea.KeyPressMsg{Text: k, Code: rune(k[0])})
		typed += k
		if m.activeModal != nil {
			t.Fatalf("typing %q in the lists filter opened a modal; the key must be a query character", k)
		}
		if !listsFilterActive(t, m) {
			t.Fatalf("typing %q closed the lists filter", k)
		}
	}

	// The keystrokes landed in the filter input, not just nowhere.
	panel, ok := m.components.ListsPanel.(interface{ FilterValue() string })
	if !ok {
		t.Fatalf("ListsPanel is %T, want FilterValue accessor", m.components.ListsPanel)
	}
	if got := panel.FilterValue(); got != typed {
		t.Errorf("lists filter query = %q, want %q (all five keys must type)", got, typed)
	}

	// esc clears the filter; with the keyboard back in the rows, e opens the
	// export modal as always — the suppression is scoped to typing only.
	m = refresh(t, m, tea.KeyPressMsg{Text: "esc"})
	if listsFilterActive(t, m) {
		t.Fatal("esc did not clear the lists filter")
	}
	m = refresh(t, m, tea.KeyPressMsg{Text: "e", Code: 'e'})
	if m.activeModal == nil {
		t.Error("e with the lists panel focused and unfiltered must still open the export modal")
	}
}

// The task-tree /-filter input never had the leak: its handleFilterKey
// routes every non-enter/esc keystroke into the input, and every global key
// AppModel handles is guarded on keyboardOwned(). Pin it so the two filters
// cannot drift: typing shortcut letters into the tree filter must neither
// open a modal nor start an action.
func TestTreeFilterTypingDoesNotTriggerShortcuts(t *testing.T) {
	m := seedOneList(t)

	m = refresh(t, m, tea.KeyPressMsg{Text: "/", Code: '/'})
	if !treeFilterActive(t, m) {
		t.Fatal("/ did not open the tree filter")
	}

	// Same blink caveat as the lists test: keystrokes into the focused
	// filter input reschedule a cursor blink, so deliver without chasing.
	applyOnce := func(m AppModel, msg tea.Msg) AppModel {
		t.Helper()
		updated, _ := m.Update(msg)
		out, ok := updated.(AppModel)
		if !ok {
			t.Fatalf("Update returned %T, want AppModel", updated)
		}
		return out
	}

	// e and i are the reported keys (modal openers in the Lists context,
	// unbound in the tree's); n is Tree.New, the tree action most likely to
	// leak if the filter input ever stopped swallowing keys.
	typed := ""
	for _, k := range []string{"e", "i", "n"} {
		m = applyOnce(m, tea.KeyPressMsg{Text: k, Code: rune(k[0])})
		typed += k
		if m.activeModal != nil {
			t.Fatalf("typing %q in the tree filter opened a modal", k)
		}
		if !treeFilterActive(t, m) {
			t.Fatalf("typing %q closed the tree filter", k)
		}
	}
	if tasks, ok := m.components.TaskPanel.(interface{ IsCreating() bool }); ok && tasks.IsCreating() {
		t.Fatal("typing n in the tree filter started inline creation")
	}

	tree, ok := m.components.TaskPanel.(interface{ FilterValue() string })
	if !ok {
		t.Fatalf("TaskPanel is %T, want FilterValue accessor", m.components.TaskPanel)
	}
	if got := tree.FilterValue(); got != typed {
		t.Errorf("tree filter query = %q, want %q (shortcut letters must type)", got, typed)
	}
}
