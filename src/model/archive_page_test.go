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

// TestOpeningArchivePageLoadsRealArchivedLists proves AppModel actually
// drives the archived-set query on open (cmds.RefreshArchivedLists), not
// just the visibility flag — an archived list's name must show up in the
// rendered body without any further interaction.
func TestOpeningArchivePageLoadsRealArchivedLists(t *testing.T) {
	m := seedOneList(t)
	lists, err := m.store.ListLists()
	if err != nil || len(lists) == 0 {
		t.Fatalf("seed: no lists (err=%v)", err)
	}
	if err := m.store.ArchiveList(lists[0].List.ID); err != nil {
		t.Fatalf("archive list: %v", err)
	}
	m = refresh(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	m = refresh(t, m, cmds.OpenArchivePage()())

	body := m.renderBody()
	if !strings.Contains(body, lists[0].List.Name) {
		t.Fatalf("rendered body does not show the archived list %q:\n%s", lists[0].List.Name, body)
	}
}

// TestPollTickRefreshesArchivePageWhileOpen proves the Archive page gets the
// same live-refresh contract as every other open surface (docs/DESIGN.md
// §7): a list archived by another process shows up after a poll tick without
// the page being reopened.
//
// PollTickMsg's own batch includes cmds.PollTick itself, a real tea.Tick
// that blocks for the poll interval — resolving the batch's children (the
// only way to see what RefreshArchivedLists actually produced) means that
// blocking is unavoidable here, so the interval is dropped to 1ms first.
// Nothing recurses back into PollTickMsg (mirrors TestPollTickReissuesItself's
// own note: that message re-issues itself forever, so a test must never feed
// it back into Update).
func TestPollTickRefreshesArchivePageWhileOpen(t *testing.T) {
	m := openArchivePage(t, 100, 40)
	m.cfg.PollIntervalMs = 1

	lists, err := m.store.ListLists()
	if err != nil || len(lists) == 0 {
		t.Fatalf("seed: no lists (err=%v)", err)
	}
	if err := m.store.ArchiveList(lists[0].List.ID); err != nil {
		t.Fatalf("archive list: %v", err)
	}

	updated, cmd := m.Update(cmds.PollTickMsg{})
	m = updated.(AppModel)
	if cmd == nil {
		t.Fatal("PollTickMsg returned no command")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("PollTickMsg command produced %T, want tea.BatchMsg", cmd())
	}
	found := false
	for _, c := range batch {
		if c == nil {
			continue
		}
		if msg, ok := c().(cmds.RefreshArchivedListsMsg); ok {
			found = true
			m = refresh(t, m, msg)
		}
	}
	if !found {
		t.Fatal("PollTickMsg's batch did not include a RefreshArchivedListsMsg while the Archive page was open")
	}

	body := m.renderBody()
	if !strings.Contains(body, lists[0].List.Name) {
		t.Fatalf("poll tick did not refresh the Archive page with the newly archived list %q:\n%s", lists[0].List.Name, body)
	}
}
