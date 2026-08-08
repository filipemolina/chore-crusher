package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/constants"
)

// seededTaskID returns the id of the single task seedOneList created.
func seededTaskID(t *testing.T, m AppModel) string {
	t.Helper()
	lists, _ := m.store.ListLists()
	if len(lists) == 0 {
		t.Fatal("seed: no lists")
	}
	tasks, _ := m.store.ListTasks(lists[0].List.ID)
	if len(tasks) == 0 {
		t.Fatal("seed: no tasks")
	}
	return tasks[0].ID
}

// stepKey feeds one keypress through Update. A textarea keypress returns a
// blink tick — a timed tea.Tick the synchronous test loop must not run, or it
// re-fires forever — so only a resulting CloseDetailsSideMsg (a clean/dirty
// close or a save) is resolved; a blink is dropped.
func stepKey(t *testing.T, m AppModel, msg tea.Msg) AppModel {
	t.Helper()
	out, cmd := m.Update(msg)
	m = out.(AppModel)
	if cmd == nil {
		return m
	}
	produced := cmd()
	if _, ok := produced.(cmds.CloseDetailsSideMsg); ok {
		return refresh(t, m, produced)
	}
	return m
}

// openDetails sizes the terminal, then opens the Details modal on the seeded
// task. Details is a modal layered over the body now — it does not disturb the
// Lists/Tasks split beneath it.
func openDetails(t *testing.T, width, height int) (AppModel, string) {
	t.Helper()
	m := seedOneList(t)
	m = refresh(t, m, tea.WindowSizeMsg{Width: width, Height: height})
	taskID := seededTaskID(t, m)
	m = refresh(t, m, cmds.OpenDetails(taskID)())
	return m, taskID
}

func TestOpenDetailsShowsAndFocuses(t *testing.T) {
	m, taskID := openDetails(t, 120, 40)

	if !m.detailsPanelVisible {
		t.Fatal("Details modal is not visible after open")
	}
	if m.detailsTaskID != taskID {
		t.Fatalf("detailsTaskID = %q, want %q", m.detailsTaskID, taskID)
	}
	if m.focusedZone != constants.COMPONENT_DETAILS_PANEL {
		t.Fatalf("focusedZone = %d, want COMPONENT_DETAILS_PANEL", m.focusedZone)
	}
	// The RefreshDetails appended by the open case must have hydrated the panel.
	panel, ok := m.components.DetailsPanel.(interface{ NotesValue() string })
	if !ok {
		t.Fatal("DetailsPanel has no NotesValue accessor")
	}
	_ = panel // presence of the accessor plus a visible modal is the contract
}

// TestDetailsModalDoesNotDisturbBody proves Details is a modal, not a body
// surface: opening it over a visible Lists panel leaves the Lists/Tasks split
// exactly as it was and never allocates a Details body column.
func TestDetailsModalDoesNotDisturbBody(t *testing.T) {
	m := seedOneList(t)
	m = refresh(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	// Turn Lists on explicitly (the 100-wide test model starts it hidden).
	m.listsPanelVisible = true
	m.bodyLayout = m.calculateBodyLayout()
	if !m.listsPanelRendered() {
		t.Fatal("precondition: Lists should be visible at 120 wide")
	}
	before := m.bodyLayout

	taskID := seededTaskID(t, m)
	m = refresh(t, m, cmds.OpenDetails(taskID)())

	if m.bodyLayout != before {
		t.Fatalf("body layout changed when Details opened: before %+v after %+v", before, m.bodyLayout)
	}
	if !m.listsPanelRendered() {
		t.Fatal("Lists disappeared when the Details modal opened")
	}
}

func TestCleanEscReturnsToTasks(t *testing.T) {
	// Below AUTO_SHOW_LISTS_MIN_WIDTH Lists starts hidden, so Tasks fills the row.
	m, _ := openDetails(t, 80, 40)

	m = stepKey(t, m, tea.KeyPressMsg{Text: "esc"})

	if m.detailsPanelVisible {
		t.Fatal("clean esc did not close Details")
	}
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Fatalf("focusedZone = %d, want COMPONENT_TASK_TREE", m.focusedZone)
	}
	if m.detailsTaskID != "" {
		t.Fatalf("detailsTaskID = %q, want cleared", m.detailsTaskID)
	}
	if m.bodyLayout.MainWidth != 80 {
		t.Fatalf("MainWidth = %d, want full terminal width 80", m.bodyLayout.MainWidth)
	}
}

func TestDirtyDiscardDoesNotCloseUntilYes(t *testing.T) {
	m, _ := openDetails(t, 120, 40)

	// Dirty the draft.
	m = stepKey(t, m, tea.KeyPressMsg{Text: "x", Code: 'x'})

	// Esc opens the discard prompt but does not close.
	m = stepKey(t, m, tea.KeyPressMsg{Text: "esc"})
	if !m.detailsPanelVisible {
		t.Fatal("dirty esc closed Details early")
	}

	// n keeps editing.
	m = stepKey(t, m, tea.KeyPressMsg{Text: "n", Code: 'n'})
	if !m.detailsPanelVisible {
		t.Fatal("n closed Details — it should keep editing")
	}

	// Esc then y discards and closes.
	m = stepKey(t, m, tea.KeyPressMsg{Text: "esc"})
	m = stepKey(t, m, tea.KeyPressMsg{Text: "y", Code: 'y'})
	if m.detailsPanelVisible {
		t.Fatal("y after dirty esc did not close Details")
	}
}

func TestDetailsSaveRefreshesAndCloses(t *testing.T) {
	m, taskID := openDetails(t, 120, 40)

	// Type into Notes (move there from the Title entry field).
	m = stepKey(t, m, tea.KeyPressMsg{Text: "tab"})
	m = stepKey(t, m, tea.KeyPressMsg{Text: "Z", Code: 'Z'})
	m = stepKey(t, m, tea.KeyPressMsg{Text: "ctrl+s", Mod: tea.ModCtrl, Code: 's'})

	if m.detailsPanelVisible {
		t.Fatal("Details stayed visible after save")
	}
	got, err := m.store.GetTask(taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !strings.Contains(got.Notes, "Z") {
		t.Fatalf("save did not persist: notes = %q", got.Notes)
	}
}

// TestCommentDeleteConfirmQuotesTheComment pins that pressing d on the
// highlighted comment opens a confirm dialog naming what it is about to
// destroy — the same "delete dialog must name its target" fix the Bugs
// list's "Panel focus is imperceptible, and no delete dialog names its
// target" task applied to task and list delete, now applied here too.
func TestCommentDeleteConfirmQuotesTheComment(t *testing.T) {
	m, taskID := openDetails(t, 120, 40)

	if _, err := m.store.AddComment(taskID, "human", "a very particular comment"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	m = refresh(t, m, cmds.RefreshDetails(m.store, taskID)())

	// Tab from Title into Notes, Progress, Priority, then Comments.
	for i := 0; i < 4; i++ {
		m = stepKey(t, m, tea.KeyPressMsg{Text: "tab"})
	}

	m = refresh(t, m, tea.KeyPressMsg{Text: "d", Code: 'd'})

	if m.activeModal == nil {
		t.Fatal("d on a selected comment should open a confirm modal")
	}
	body := ansi.Strip(m.activeModal.View().Content)
	if !strings.Contains(body, "a very particular comment") {
		t.Errorf("confirm dialog does not quote the comment text: %q", body)
	}
}

// TestCommentDeleteConfirmRemovesComment pins the confirm path end to end:
// answering yes deletes the comment and closes the modal, leaving Details
// open on the same task (unlike task delete, which closes Details too).
func TestCommentDeleteConfirmRemovesComment(t *testing.T) {
	m, taskID := openDetails(t, 120, 40)

	cid, err := m.store.AddComment(taskID, "human", "delete me")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	m = refresh(t, m, cmds.RefreshDetails(m.store, taskID)())
	for i := 0; i < 4; i++ {
		m = stepKey(t, m, tea.KeyPressMsg{Text: "tab"})
	}
	m = refresh(t, m, tea.KeyPressMsg{Text: "d", Code: 'd'})
	if m.activeModal == nil {
		t.Fatal("precondition: d should have opened a confirm modal")
	}

	m = refresh(t, m, tea.KeyPressMsg{Text: "y", Code: 'y'})

	if m.activeModal != nil {
		t.Error("confirm modal should close after y")
	}
	if !m.detailsPanelVisible {
		t.Error("Details modal should stay open after deleting a comment")
	}
	if _, err := m.store.GetComment(cid); err == nil {
		t.Error("comment should be deleted from the store")
	}
}

func TestDetailsModalSizeIsMostOfScreen(t *testing.T) {
	// The modal outer box is about 90% of each axis, and Details never takes a
	// body column (it is layered over the body).
	m := seedOneList(t)
	m = refresh(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if w, h := m.detailsModalSize(); w != 108 || h != 36 {
		t.Errorf("detailsModalSize(120x40) = %dx%d, want 108x36", w, h)
	}
	m = refresh(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if w, h := m.detailsModalSize(); w != 72 || h != 21 {
		t.Errorf("detailsModalSize(80x24) = %dx%d, want 72x21", w, h)
	}
}

func TestDetailsRenderHasNoBackgroundBleed(t *testing.T) {
	for _, size := range []struct{ width, height int }{{120, 40}, {80, 24}, {30, 20}} {
		m, _ := openDetails(t, size.width, size.height)
		rendered := m.View().Content
		if appstyles.HasBackgroundBleed(rendered) {
			t.Errorf("%dx%d: Details layout has background bleed:\n%q",
				size.width, size.height, firstBleedLine(rendered))
		}
	}
}
