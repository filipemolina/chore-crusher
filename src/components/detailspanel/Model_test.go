package detailspanel

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/constants"
	"github.com/filipemolina/chore-crusher/src/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// loaded builds a panel hydrated with one task in a fresh store, plus the
// store and task id so save-path tests can inspect the writes.
func loaded(t *testing.T, notes string) (*Model, *store.Store, string) {
	t.Helper()
	s := openTestStore(t)
	listID, err := s.CreateList("Chores", "")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	taskID, err := s.CreateTask(listID, "Water plants", nil, notes)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	m := New(s).(*Model)
	m, _ = updateModel(m, cmds.SetFocus(constants.COMPONENT_DETAILS_PANEL)())
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	return m, s, taskID
}

func updateModel(m *Model, msg tea.Msg) (*Model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(*Model), cmd
}

// runCmd resolves a cmd to its message (nil-safe).
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func typeRune(t *testing.T, m *Model, r rune) *Model {
	t.Helper()
	m, _ = updateModel(m, tea.KeyPressMsg{Text: string(r), Code: r})
	return m
}

func TestCleanEscClosesPanel(t *testing.T) {
	m, _, _ := loaded(t, "")

	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "esc"})
	if _, ok := runCmd(cmd).(cmds.CloseDetailsSideMsg); !ok {
		t.Fatalf("clean esc: got %T, want CloseDetailsSideMsg", runCmd(cmd))
	}
}

func TestDirtyEscPromptsThenKeepsOrDiscards(t *testing.T) {
	m, _, _ := loaded(t, "")
	m = typeRune(t, m, 'x') // draft is now dirty

	// Esc alone must not close: it opens the discard prompt.
	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "esc"})
	if runCmd(cmd) != nil {
		t.Fatalf("dirty esc closed early: %T", runCmd(cmd))
	}
	if !m.confirmingDiscard {
		t.Fatal("dirty esc did not open the discard prompt")
	}

	// n keeps the draft.
	m, cmd = updateModel(m, tea.KeyPressMsg{Text: "n", Code: 'n'})
	if runCmd(cmd) != nil {
		t.Fatalf("n closed the panel: %T", runCmd(cmd))
	}
	if m.confirmingDiscard {
		t.Fatal("n did not dismiss the discard prompt")
	}
	if m.NotesValue() != "x" {
		t.Fatalf("n dropped the draft: NotesValue = %q", m.NotesValue())
	}

	// Esc again, then y discards and closes.
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "esc"})
	_, cmd = updateModel(m, tea.KeyPressMsg{Text: "y", Code: 'y'})
	if _, ok := runCmd(cmd).(cmds.CloseDetailsSideMsg); !ok {
		t.Fatalf("y after dirty esc: got %T, want CloseDetailsSideMsg", runCmd(cmd))
	}
}

func TestSaveWritesAndClosesWithRefresh(t *testing.T) {
	m, s, taskID := loaded(t, "old notes")

	// Type a character so the draft is dirty and distinct.
	m = typeRune(t, m, 'Z')

	_, cmd := updateModel(m, tea.KeyPressMsg{Text: "ctrl+s", Mod: tea.ModCtrl, Code: 's'})
	msg := runCmd(cmd)
	closeMsg, ok := msg.(cmds.CloseDetailsSideMsg)
	if !ok {
		t.Fatalf("ctrl+s: got %T, want CloseDetailsSideMsg", msg)
	}
	if closeMsg.Follow == nil {
		t.Fatal("save close carried no follow refresh command")
	}
	if _, ok := runCmd(closeMsg.Follow).(cmds.RefreshTasksMsg); !ok {
		t.Fatalf("save follow: got %T, want RefreshTasksMsg", runCmd(closeMsg.Follow))
	}

	got, err := s.GetTask(taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !strings.Contains(got.Notes, "Z") {
		t.Fatalf("save did not persist the edit: stored notes = %q", got.Notes)
	}
}

func TestRefreshUpdatesCleanButNotDirty(t *testing.T) {
	m, s, taskID := loaded(t, "first")

	// External change lands on a clean panel: it should show through.
	if err := s.SetNotes(taskID, "external"); err != nil {
		t.Fatalf("set notes: %v", err)
	}
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	if m.NotesValue() != "external" {
		t.Fatalf("clean refresh: NotesValue = %q, want %q", m.NotesValue(), "external")
	}

	// Now dirty the draft and push another external change: the draft wins.
	m = typeRune(t, m, 'Q')
	if err := s.SetNotes(taskID, "clobbered"); err != nil {
		t.Fatalf("set notes: %v", err)
	}
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	if m.NotesValue() != "externalQ" {
		t.Fatalf("dirty refresh overwrote the draft: NotesValue = %q", m.NotesValue())
	}
}

func TestLongTitleTruncatedWithinWidth(t *testing.T) {
	m, _, _ := loaded(t, "")
	m.title = strings.Repeat("verylongtitle ", 40)

	const width, height = 30, 24
	m, _ = updateModel(m, cmds.SetBodyLayout(height, 0, width, 0, width)())

	view := m.View().Content
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("line exceeds panel width %d: %d (%q)", width, w, line)
		}
	}
}

func TestNarrowViewFitsAndSeals(t *testing.T) {
	m, _, _ := loaded(t, "some notes\nover two lines")

	const width, height = 24, 18
	m, _ = updateModel(m, cmds.SetBodyLayout(height, 0, width, 0, width)())

	view := m.View().Content
	if w := lipgloss.Width(view); w > width {
		t.Fatalf("view width %d exceeds supplied %d", w, width)
	}
	if h := lipgloss.Height(view); h > height {
		t.Fatalf("view height %d exceeds supplied %d", h, height)
	}
	if appstyles.HasBackgroundBleed(view) {
		t.Fatal("view has background bleed")
	}
}
