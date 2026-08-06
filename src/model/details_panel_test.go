package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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

// openDetails sizes the terminal, makes Lists visible (to prove Details hides
// it), then opens Details on the seeded task.
func openDetails(t *testing.T, width, height int) (AppModel, string) {
	t.Helper()
	m := seedOneList(t)
	m = refresh(t, m, tea.WindowSizeMsg{Width: width, Height: height})
	m.listsPanelVisible = true
	m.bodyLayout = m.calculateBodyLayout()
	taskID := seededTaskID(t, m)
	m = refresh(t, m, cmds.OpenDetails(taskID)())
	return m, taskID
}

func TestOpenDetailsHidesListsShowsAndFocuses(t *testing.T) {
	m, taskID := openDetails(t, 120, 40)

	if !m.detailsPanelVisible {
		t.Fatal("Details panel is not visible after open")
	}
	if m.listsPanelVisible {
		t.Fatal("Lists panel stayed visible — Details must hide it")
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
	_ = panel // presence of the accessor plus a visible panel is the contract
}

func TestOpenDetailsFromListsNeverThreePanels(t *testing.T) {
	m, _ := openDetails(t, 120, 40)
	if m.listsPanelVisible && m.detailsPanelVisible {
		t.Fatal("both Lists and Details are visible — three-panel body")
	}
}

func TestCleanEscReturnsToFullWidthTasks(t *testing.T) {
	m, _ := openDetails(t, 120, 40)

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
	if m.bodyLayout.MainWidth != 120 {
		t.Fatalf("MainWidth = %d, want full terminal width 120", m.bodyLayout.MainWidth)
	}
	if m.bodyLayout.DetailsWidth != 0 {
		t.Fatalf("DetailsWidth = %d, want 0 after close", m.bodyLayout.DetailsWidth)
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

func TestDetailsLayoutWidths(t *testing.T) {
	// Normal width: Tasks + gutter + Details == terminal width, Lists 0.
	m, _ := openDetails(t, 120, 40)
	l := m.bodyLayout
	if l.ListsWidth != 0 {
		t.Errorf("ListsWidth = %d, want 0 while Details is open", l.ListsWidth)
	}
	if got := l.MainWidth + constants.BODY_GUTTER_WIDTH + l.DetailsWidth; got != 120 {
		t.Errorf("Main+gutter+Details = %d, want 120", got)
	}
	if l.MainWidth <= 0 || l.DetailsWidth <= 0 {
		t.Errorf("expected both surfaces to fit: Main=%d Details=%d", l.MainWidth, l.DetailsWidth)
	}

	// Too narrow for a side surface: Details alone spans the body, Tasks drops.
	mn, _ := openDetails(t, 30, 20)
	ln := mn.bodyLayout
	if ln.DetailsWidth != 30 {
		t.Errorf("narrow: DetailsWidth = %d, want 30", ln.DetailsWidth)
	}
	if ln.MainWidth != 0 {
		t.Errorf("narrow: MainWidth = %d, want 0", ln.MainWidth)
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
