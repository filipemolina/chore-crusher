package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/cursor"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/constants"
)

// stepDiscard runs one message through Update and drops the returned
// command instead of resolving it through refresh(): both the lists panel's
// bubbles filter input and the listnamemodal's textinput re-issue a
// cursor-blink tick on every keystroke, and resolving that recursively — the
// way refresh() resolves every command — spins forever, the same trap
// detailspanel's stepKey helper exists to avoid.
func stepDiscard(t *testing.T, m AppModel, msg tea.Msg) AppModel {
	t.Helper()
	out, _ := m.Update(msg)
	return out.(AppModel)
}

// typeText feeds one KeyPressMsg per rune into whatever currently owns the
// keyboard (a modal's text input, or the lists panel's filter), via
// stepDiscard.
func typeText(t *testing.T, m AppModel, s string) AppModel {
	t.Helper()
	for _, r := range s {
		m = stepDiscard(t, m, tea.KeyPressMsg{Text: string(r), Code: r})
	}
	return m
}

// stepResolvingFilter is stepDiscard's counterpart for a keystroke whose
// effect on the *filtered results* actually matters to the test: bubbles'
// list filters asynchronously (a filterItems cmd, separate from the
// FilterInput's own synchronous value update), so a caller that needs
// AcceptWhileFiltering's "filtered down to nothing? reset instead of apply"
// check to see real results has to let that cmd run — stepDiscard's plain
// drop leaves filteredItems stale. It still must not recurse into the
// re-issued cursor.BlinkMsg chain (dropped instead, same trap stepDiscard
// exists to avoid), so this is not just refresh() with a rename.
func stepResolvingFilter(t *testing.T, m AppModel, msg tea.Msg) AppModel {
	t.Helper()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = stepResolvingFilter(t, m, c())
		}
		return m
	}
	if _, ok := msg.(cursor.BlinkMsg); ok {
		return m
	}
	updated, cmd := m.Update(msg)
	out, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("Update returned %T, want AppModel", updated)
	}
	if cmd == nil {
		return out
	}
	return stepResolvingFilter(t, out, cmd())
}

// openListsFocused opens the Lists panel and focuses it, the way a user's L
// keypress does — the standalone starting point every test below builds on.
// The caller's active list must have at least one task, or the empty-list
// auto-create-input claims the keyboard and L does nothing (docs/DESIGN.md
// §12 "Empty states").
func openListsFocused(t *testing.T, m AppModel) AppModel {
	t.Helper()
	m = refresh(t, m, tea.KeyPressMsg{Text: "L", Code: 'L'})
	if m.focusedZone != constants.COMPONENT_LISTS_PANEL || !m.listsPanelVisible {
		t.Fatalf("precondition: L should open and focus Lists (visible=%v zone=%d)", m.listsPanelVisible, m.focusedZone)
	}
	return m
}

// --- "Enter should select a list and close the lists panel" ---

// TestEnterOnListClosesPanelAndFocusesTasks pins the enter/"commit and
// close" contract: the highlighted list is already active (cursor-move
// live-switches it), so enter only needs to close the panel and hand focus
// back to Tasks.
func TestEnterOnListClosesPanelAndFocusesTasks(t *testing.T) {
	m := seedOneList(t)
	listA := m.activeListID
	if _, err := m.store.CreateList("Work", ""); err != nil {
		t.Fatalf("create list B: %v", err)
	}
	m = refresh(t, m, cmds.RefreshLists(m.store)())
	m = openListsFocused(t, m)

	m = refresh(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})

	if m.listsPanelVisible {
		t.Error("enter should have closed the Lists panel")
	}
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Errorf("focusedZone after enter = %d, want task tree", m.focusedZone)
	}
	// No navigation happened, so the highlighted (first) list stays active.
	if m.activeListID != listA {
		t.Errorf("activeListID after enter = %q, want %q (the highlighted list)", m.activeListID, listA)
	}
}

// TestEnterWhileFilteringAppliesFilterNotClose pins the precedence
// constraint: enter belongs to the filter (list.KeyMap's
// AcceptWhileFiltering) while one is being typed, and must not also close
// the panel.
func TestEnterWhileFilteringAppliesFilterNotClose(t *testing.T) {
	m := seedOneList(t)
	m = openListsFocused(t, m)

	m = refresh(t, m, tea.KeyPressMsg{Text: "/", Code: '/'})
	if !listsFilterActive(t, m) {
		t.Fatal("precondition: / should open the lists filter")
	}
	// Match seedOneList's default list name ("L") — bubbles resets the filter
	// instead of applying it on accept when nothing matches, so a filter
	// query has to actually match to exercise "applies, doesn't close".
	m = stepResolvingFilter(t, m, tea.KeyPressMsg{Text: "L", Code: 'L'})

	m = refresh(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})

	if !m.listsPanelVisible {
		t.Error("enter while filtering closed the panel; it should have applied the filter instead")
	}
	if m.focusedZone != constants.COMPONENT_LISTS_PANEL {
		t.Errorf("focusedZone after enter-while-filtering = %d, want lists panel", m.focusedZone)
	}
	if !listsFilterActive(t, m) {
		t.Error("enter while filtering should apply (not clear) the filter")
	}
}

// TestEnterCloseStaysClosedAcrossResize pins that an enter-close behaves
// exactly like an L-close: it flips the same listsPanelVisible preference,
// so a later resize to a wide terminal must not silently reopen it.
func TestEnterCloseStaysClosedAcrossResize(t *testing.T) {
	m := seedOneList(t)
	m = openListsFocused(t, m)
	m = refresh(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})
	if m.listsPanelVisible {
		t.Fatal("precondition: enter should have closed the panel")
	}

	m = refresh(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	if m.listsPanelVisible {
		t.Error("a resize to a wide terminal reopened the panel after an enter-close")
	}

	m = refresh(t, m, tea.KeyPressMsg{Text: "L", Code: 'L'})
	if !m.listsPanelVisible {
		t.Error("L should still reopen the panel after an enter-close")
	}
}

// --- "Esc should close the lists side panel" ---

// TestEscWithFilterClearsFilterOnly pins step 3 of the esc ladder
// (docs/DESIGN.md §5): an active lists filter claims esc for itself, and
// the panel must stay open.
func TestEscWithFilterClearsFilterOnly(t *testing.T) {
	m := seedOneList(t)
	m = openListsFocused(t, m)
	m = refresh(t, m, tea.KeyPressMsg{Text: "/", Code: '/'})
	m = stepDiscard(t, m, tea.KeyPressMsg{Text: "o", Code: 'o'})
	if !listsFilterActive(t, m) {
		t.Fatal("precondition: filter should be active")
	}

	m = refresh(t, m, tea.KeyPressMsg{Text: "esc"})

	if listsFilterActive(t, m) {
		t.Error("esc should have cleared the filter")
	}
	if !m.listsPanelVisible {
		t.Error("esc with an active filter closed the whole panel — should only clear the filter")
	}
	if m.focusedZone != constants.COMPONENT_LISTS_PANEL {
		t.Errorf("focusedZone after filter-clearing esc = %d, want lists panel", m.focusedZone)
	}
}

// TestEscWithNoFilterClosesPanel pins step 4: once nothing else claims esc,
// it closes the panel and returns focus to Tasks.
func TestEscWithNoFilterClosesPanel(t *testing.T) {
	m := seedOneList(t)
	m = openListsFocused(t, m)

	m = refresh(t, m, tea.KeyPressMsg{Text: "esc"})

	if m.listsPanelVisible {
		t.Error("esc with no filter should have closed the panel")
	}
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Errorf("focusedZone after esc-close = %d, want task tree", m.focusedZone)
	}
}

// TestTwoEscsFirstClearsFilterThenClosesPanel exercises the full ladder in
// sequence from a filtered state, the way a user would actually hit it.
func TestTwoEscsFirstClearsFilterThenClosesPanel(t *testing.T) {
	m := seedOneList(t)
	m = openListsFocused(t, m)
	m = refresh(t, m, tea.KeyPressMsg{Text: "/", Code: '/'})
	m = stepDiscard(t, m, tea.KeyPressMsg{Text: "o", Code: 'o'})

	m = refresh(t, m, tea.KeyPressMsg{Text: "esc"})
	if listsFilterActive(t, m) {
		t.Fatal("first esc should have cleared the filter")
	}
	if !m.listsPanelVisible {
		t.Fatal("first esc should not have closed the panel")
	}

	m = refresh(t, m, tea.KeyPressMsg{Text: "esc"})
	if m.listsPanelVisible {
		t.Error("second esc should have closed the panel")
	}
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Errorf("focusedZone after the second esc = %d, want task tree", m.focusedZone)
	}
}

// TestEscWithModalOpenGoesToModalNotPanel pins that a modal sits above the
// Lists panel in the esc ladder: opening the rename modal from the Lists
// panel and pressing esc must close the modal, not the panel underneath it.
func TestEscWithModalOpenGoesToModalNotPanel(t *testing.T) {
	m := seedOneList(t)
	m = openListsFocused(t, m)

	m = refresh(t, m, tea.KeyPressMsg{Text: "R", Code: 'R'})
	if m.activeModal == nil {
		t.Fatal("precondition: R should have opened the rename modal")
	}

	m = refresh(t, m, tea.KeyPressMsg{Text: "esc"})

	if m.activeModal != nil {
		t.Error("esc should have closed the modal")
	}
	if !m.listsPanelVisible {
		t.Error("esc closing the modal should not also close the Lists panel underneath it")
	}
}

// --- "Select a newly created List and close the lists panel" ---

// createListViaModal drives the full n -> type name -> enter sequence a user
// follows to create a list from the Lists panel, and returns the resulting
// model. The panel must already be open and focused.
func createListViaModal(t *testing.T, m AppModel, name string) AppModel {
	t.Helper()
	m = refresh(t, m, tea.KeyPressMsg{Text: "n", Code: 'n'})
	if m.activeModal == nil {
		t.Fatal("precondition: n should have opened the new-list modal")
	}
	m = typeText(t, m, name)
	m = refresh(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})
	return m
}

// TestCreateListSelectsItAndClosesPanel is the ordering guard: creating a
// list must show the NEW list's (empty) tasks, not the previously active
// list's — the failure mode of closing the panel before selecting the new
// list.
func TestCreateListSelectsItAndClosesPanel(t *testing.T) {
	m := seedOneList(t)
	oldList := m.activeListID
	m = openListsFocused(t, m)

	m = createListViaModal(t, m, "Groceries")

	// 1: active list is the new one, not the old one.
	if m.activeListID == oldList {
		t.Fatal("active list is still the old list — the new one was not selected")
	}
	newList, err := m.store.GetList(m.activeListID)
	if err != nil {
		t.Fatalf("GetList(active): %v", err)
	}
	if newList.Name != "Groceries" {
		t.Errorf("active list = %q, want Groceries", newList.Name)
	}

	// 2 + 3: panel closed, focus on Tasks.
	if m.listsPanelVisible {
		t.Error("creating a list should have closed the Lists panel")
	}
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Errorf("focusedZone after creating a list = %d, want task tree", m.focusedZone)
	}

	// 4: the Tasks panel shows the NEW (empty) list, not the old one's task.
	rows := treeRows(t, m)
	if len(rows) != 0 {
		t.Errorf("tree rows after creating an empty list = %v, want none (it shows the old list's task instead)", rows)
	}

	// 5: the empty new list auto-opens the inline create input.
	tree, ok := m.components.TaskPanel.(interface{ IsCreating() bool })
	if !ok {
		t.Fatal("TaskPanel has no IsCreating accessor")
	}
	if !tree.IsCreating() {
		t.Error("the new, empty list should auto-open the inline create input")
	}
}

// TestCancelledListCreationLeavesPanelOpen pins the "do not change what
// happens on cancel" rule: esc in the name modal must not trigger any of the
// select/close behavior above.
func TestCancelledListCreationLeavesPanelOpen(t *testing.T) {
	m := seedOneList(t)
	oldList := m.activeListID
	m = openListsFocused(t, m)

	m = refresh(t, m, tea.KeyPressMsg{Text: "n", Code: 'n'})
	m = typeText(t, m, "Abandoned")
	m = refresh(t, m, tea.KeyPressMsg{Text: "esc"})

	if m.activeModal != nil {
		t.Error("esc in the name modal should have closed it")
	}
	if !m.listsPanelVisible {
		t.Error("cancelling list creation should not close the Lists panel")
	}
	if m.activeListID != oldList {
		t.Errorf("active list changed after a cancelled creation: %q, want %q", m.activeListID, oldList)
	}
	lists, err := m.store.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	for _, l := range lists {
		if l.List.Name == "Abandoned" {
			t.Error("a cancelled creation should not have created the list at all")
		}
	}
}

// --- Export / Import modal wiring ---

// TestPressEOpensExportModal verifies that pressing 'e' in the Lists panel
// opens the export modal with whole-store mode (when no list is highlighted)
// or this-list mode (when a list is highlighted).
func TestPressEOpensExportModal(t *testing.T) {
	m := seedOneList(t)
	m = openListsFocused(t, m)

	m = refresh(t, m, tea.KeyPressMsg{Text: "e", Code: 'e'})

	if m.activeModal == nil {
		t.Fatal("pressing 'e' should have opened the export modal")
	}
	// Verify it's actually an export modal by checking its View
	view := ansi.Strip(m.activeModal.View().Content)
	if !strings.Contains(view, "Export") {
		t.Errorf("modal view should contain \"Export\", got: %q", view)
	}
}

// TestPressIOpensImportModal verifies that pressing 'i' in the Lists panel
// opens the import modal.
func TestPressIOpensImportModal(t *testing.T) {
	m := seedOneList(t)
	m = openListsFocused(t, m)

	m = refresh(t, m, tea.KeyPressMsg{Text: "i", Code: 'i'})

	if m.activeModal == nil {
		t.Fatal("pressing 'i' should have opened the import modal")
	}
	view := ansi.Strip(m.activeModal.View().Content)
	if !strings.Contains(view, "Import") {
		t.Errorf("modal view should contain \"Import\", got: %q", view)
	}
}

// TestExportImportKeysInertWhenPanelNotFocused verifies that 'e' and 'i'
// do nothing when the Lists panel is not focused (matching the guard used
// for Lists.New/Rename/Delete).
func TestExportImportKeysInertWhenPanelNotFocused(t *testing.T) {
	m := seedOneList(t)
	// Tasks panel is focused initially, not Lists
	if m.focusedZone == constants.COMPONENT_LISTS_PANEL && m.listsPanelVisible {
		t.Fatal("precondition: Lists should not be focused")
	}

	m = refresh(t, m, tea.KeyPressMsg{Text: "e", Code: 'e'})
	if m.activeModal != nil {
		t.Error("'e' should be inert when Lists panel is not focused")
	}

	m = refresh(t, m, tea.KeyPressMsg{Text: "i", Code: 'i'})
	if m.activeModal != nil {
		t.Error("'i' should be inert when Lists panel is not focused")
	}
}

// TestExportImportRoundTripThroughTUI verifies the full flow: pressing 'e'
// opens the export modal, typing a path and submitting writes a file; then
// opening the import modal and feeding it that file recreates the list.
func TestExportImportRoundTripThroughTUI(t *testing.T) {
	m := seedOneList(t)
	lists, _ := m.store.ListLists()
	srcID := lists[0].List.ID

	// Create a task so the list has content
	if _, err := m.store.CreateTask(srcID, "Task one", nil, ""); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	m = refresh(t, m, cmds.RefreshLists(m.store)())

	m = openListsFocused(t, m)

	// Press 'e' to open export modal
	m = refresh(t, m, tea.KeyPressMsg{Text: "e", Code: 'e'})
	if m.activeModal == nil {
		t.Fatal("'e' should have opened the export modal")
	}

	// Type a temp path and submit
	expPath := filepath.Join(t.TempDir(), "export.json")
	m = typeText(t, m, expPath)
	m = refresh(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})

	// The export modal's follow command should have written the file
	// and closed the modal (we can verify the file exists)
	if _, err := os.Stat(expPath); err != nil {
		t.Fatalf("export file not created at %q: %v", expPath, err)
	}

	// Now open the import modal
	m = refresh(t, m, tea.KeyPressMsg{Text: "i", Code: 'i'})
	if m.activeModal == nil {
		t.Fatal("'i' should have opened the import modal")
	}

	// Type the path and submit
	m = typeText(t, m, expPath)
	m = refresh(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})

	// Verify the list was recreated (additive - original plus imported)
	after, err := m.store.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(after) != len(lists)+1 {
		t.Errorf("after import, lists count = %d, want %d (original + imported)", len(after), len(lists)+1)
	}

	// Find the imported list (different ID from srcID)
	var found bool
	for _, l := range after {
		if l.List.Name == lists[0].List.Name && l.List.ID != srcID {
			found = true
			// Verify tasks were recreated (seedOneList + the one we added = 2)
			tasks, err := m.store.ListTasks(l.List.ID)
			if err != nil {
				t.Fatalf("ListTasks on imported list: %v", err)
			}
			if len(tasks) != 2 {
				t.Errorf("imported list has %d tasks, want 2", len(tasks))
			}
			break
		}
	}
	if !found {
		t.Error("imported copy of the list not found")
	}

	// Modal should be closed after the follow command
	if m.activeModal != nil {
		t.Error("modal should be closed after import completes")
	}
}

// TestOpenExportModalMsgOpensExportModal verifies that the cmds message
// OpenExportModalMsg opens the export modal.
func TestOpenExportModalMsgOpensExportModal(t *testing.T) {
	m := seedOneList(t)
	m = openListsFocused(t, m)

	m = refresh(t, m, cmds.OpenExportModal()())

	if m.activeModal == nil {
		t.Fatal("OpenExportModalMsg should have opened the export modal")
	}
	view := ansi.Strip(m.activeModal.View().Content)
	if !strings.Contains(view, "Export") {
		t.Errorf("modal view should contain \"Export\", got: %q", view)
	}
}

// TestOpenImportModalMsgOpensImportModal verifies that the cmds message
// OpenImportModalMsg opens the import modal.
func TestOpenImportModalMsgOpensImportModal(t *testing.T) {
	m := seedOneList(t)
	m = openListsFocused(t, m)

	m = refresh(t, m, cmds.OpenImportModal()())

	if m.activeModal == nil {
		t.Fatal("OpenImportModalMsg should have opened the import modal")
	}
	view := ansi.Strip(m.activeModal.View().Content)
	if !strings.Contains(view, "Import") {
		t.Errorf("modal view should contain \"Import\", got: %q", view)
	}
}

// --- lastError rendering (Task 6) ---

// TestFailedImportRendersErrorInStatusLine verifies that a failed import
// (bad path) surfaces its error through the rendered View, not just in
// lastError (test-visible only). The error text should appear in the
// status line between the body and the footer.
func TestFailedImportRendersErrorInStatusLine(t *testing.T) {
	m := seedOneList(t)
	m = openListsFocused(t, m)

	// Open the import modal
	m = refresh(t, m, tea.KeyPressMsg{Text: "i", Code: 'i'})
	if m.activeModal == nil {
		t.Fatal("pressing 'i' should have opened the import modal")
	}

	// Type a non-existent path and submit
	expPath := filepath.Join(t.TempDir(), "does-not-exist.json")
	m = typeText(t, m, expPath)
	m = refresh(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})

	// Resolve the terminal layout so View renders a full frame
	m = refresh(t, m, m.bodyLayout)

	rendered := ansi.Strip(m.View().Content)
	if !strings.Contains(rendered, "no such file") && !strings.Contains(rendered, m.lastError) {
		t.Errorf("View should contain the failed import error text; lastError = %q", m.lastError)
	}
	if m.lastError == "" {
		t.Error("failed import should have set lastError")
	}
}

// TestSuccessfulRoundTripClearsLastError verifies that after a failed
// operation, a successful one (refresh, which clears lastError) removes
// the error from the status line.
func TestSuccessfulRoundTripClearsLastError(t *testing.T) {
	m := seedOneList(t)
	m = openListsFocused(t, m)

	// Trigger an error via a failed import
	m = refresh(t, m, tea.KeyPressMsg{Text: "i", Code: 'i'})
	m = typeText(t, m, filepath.Join(t.TempDir(), "nope.json"))
	m = refresh(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})

	if m.lastError == "" {
		t.Fatal("precondition: failed import should have set lastError")
	}

	// Refresh clears lastError (RefreshListsMsg with no Err)
	m = refresh(t, m, cmds.RefreshLists(m.store)())

	if m.lastError != "" {
		t.Errorf("after a clean refresh, lastError = %q, want empty", m.lastError)
	}

	m = refresh(t, m, m.bodyLayout)
	rendered := ansi.Strip(m.View().Content)
	if strings.Contains(rendered, "no such file") {
		t.Error("View should not contain the stale error after refresh")
	}
}
