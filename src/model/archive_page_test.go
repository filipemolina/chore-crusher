package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/constants"
)

// openArchivePage sizes the terminal, then opens the Archive page.
func openArchivePage(t *testing.T, width, height int) AppModel {
	t.Helper()
	m := seedOneList(t)
	m = refresh(t, m, tea.WindowSizeMsg{Width: width, Height: height})
	m = refresh(t, m, cmds.OpenArchivePage()())
	return m
}

// TestArchiveKeyOpensPage proves the A binding (keys.Global.ArchivePage) is
// actually wired in AppModel.Update, not just the OpenArchivePageMsg plumbing.
func TestArchiveKeyOpensPage(t *testing.T) {
	m := seedOneList(t)
	m = refresh(t, m, tea.WindowSizeMsg{Width: 80, Height: 40})

	m = refresh(t, m, tea.KeyPressMsg{Text: "A"})

	if !m.archivePageVisible {
		t.Fatal("A did not open the Archive page")
	}
	if m.focusedZone != constants.COMPONENT_ARCHIVE_PAGE {
		t.Fatalf("focusedZone = %d, want COMPONENT_ARCHIVE_PAGE", m.focusedZone)
	}
}

func TestOpenArchivePageShowsAndFocuses(t *testing.T) {
	m := openArchivePage(t, 120, 40)

	if !m.archivePageVisible {
		t.Fatal("Archive page is not visible after open")
	}
	if m.focusedZone != constants.COMPONENT_ARCHIVE_PAGE {
		t.Fatalf("focusedZone = %d, want COMPONENT_ARCHIVE_PAGE", m.focusedZone)
	}
}

// TestArchivePageReplacesBody proves the Archive page is a full-body takeover
// (docs/DESIGN.md §5), not a side surface composed alongside Tasks/Lists: with
// Lists forced on, opening the Archive page must still render only the
// archive surface, not the Tasks/Lists split beneath it.
func TestArchivePageReplacesBody(t *testing.T) {
	m := seedOneList(t)
	m = refresh(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m.listsPanelVisible = true
	m.bodyLayout = m.calculateBodyLayout()
	if !m.listsPanelRendered() {
		t.Fatal("precondition: Lists should be visible at 120 wide")
	}

	m = refresh(t, m, cmds.OpenArchivePage()())

	body := m.renderBody()
	if !strings.Contains(body, "Archived Lists") {
		t.Fatalf("rendered body does not contain the Archive page title:\n%s", body)
	}
	if strings.Contains(body, "Tasks") {
		t.Fatalf("rendered body still shows the Tasks panel while the Archive page is open:\n%s", body)
	}
}

func TestArchivePageEscReturnsToTasks(t *testing.T) {
	m := openArchivePage(t, 80, 40)

	out, cmd := m.Update(tea.KeyPressMsg{Text: "esc"})
	m = out.(AppModel)
	if cmd != nil {
		m = refresh(t, m, cmd())
	}

	if m.archivePageVisible {
		t.Fatal("esc did not close the Archive page")
	}
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Fatalf("focusedZone = %d, want COMPONENT_TASK_TREE", m.focusedZone)
	}
}

// TestArchivePageOwnsKeyboard proves the Archive page intercepts keypresses
// exclusively while open, the same way Details does: a key that would
// otherwise toggle a global surface (here, T for the theme picker) must not
// open anything while the Archive page owns the keyboard.
func TestArchivePageOwnsKeyboard(t *testing.T) {
	m := openArchivePage(t, 80, 40)

	out, _ := m.Update(tea.KeyPressMsg{Text: "T"})
	m = out.(AppModel)

	if m.activeModal != nil {
		t.Fatal("a global key opened a modal while the Archive page owned the keyboard")
	}
	if !m.archivePageVisible {
		t.Fatal("the Archive page closed on an unrelated keypress")
	}
}

// TestArchivePageForceQuitStillWorks proves ctrl+c quits from the Archive
// page exactly as it does from every other surface (docs/DESIGN.md §5).
func TestArchivePageForceQuitStillWorks(t *testing.T) {
	m := openArchivePage(t, 80, 40)

	_, cmd := m.Update(tea.KeyPressMsg{Text: "ctrl+c"})
	if cmd == nil {
		t.Fatal("ctrl+c produced no command while the Archive page was open")
	}
}
