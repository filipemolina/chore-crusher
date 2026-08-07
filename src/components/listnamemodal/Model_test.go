package listnamemodal

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/cmds"
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

func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	out, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	return out, cmd
}

// runSubmit resolves a submit keypress's command chain fully: the outer cmd
// is cmds.CloseModal's, whose result is a CloseModalMsg carrying the actual
// create/rename/toggle work as Follow — running that is what performs the
// store write.
func runSubmit(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("submit produced no command")
	}
	closeMsg, ok := cmd().(cmds.CloseModalMsg)
	if !ok {
		t.Fatalf("submit produced %T, want cmds.CloseModalMsg", cmd())
	}
	if closeMsg.Follow == nil {
		t.Fatal("CloseModalMsg.Follow is nil")
	}
	return closeMsg.Follow()
}

func TestNewRenameSeedsCollaborativeFromStore(t *testing.T) {
	s := openTestStore(t)
	id, err := s.CreateList("Shared", "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if err := s.SetCollaborative(id, true); err != nil {
		t.Fatalf("SetCollaborative: %v", err)
	}

	m := New(ModeRename, id, s).(Model)
	if !m.collaborative {
		t.Error("collaborative = false, want true (seeded from the store)")
	}
	if !m.origCollaborative {
		t.Error("origCollaborative = false, want true")
	}
}

func TestNewRenameDefaultsCollaborativeFalse(t *testing.T) {
	s := openTestStore(t)
	id, err := s.CreateList("Private", "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}

	m := New(ModeRename, id, s).(Model)
	if m.collaborative {
		t.Error("collaborative = true, want false by default")
	}
}

// TestSpaceTypesIntoNameFieldWhenNotToggleFocused pins that repurposing
// space for the toggle must not break typing a space into the list name —
// the toggle only owns space once tab has moved focus onto it.
func TestSpaceTypesIntoNameFieldWhenNotToggleFocused(t *testing.T) {
	s := openTestStore(t)
	id, err := s.CreateList("Old name", "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	m := New(ModeRename, id, s).(Model)

	m, _ = step(t, m, tea.KeyPressMsg{Text: "A", Code: 'A'})
	m, _ = step(t, m, tea.KeyPressMsg{Text: " ", Code: ' '})
	m, _ = step(t, m, tea.KeyPressMsg{Text: "B", Code: 'B'})

	if m.input.Value() != "A B" {
		t.Errorf("input value = %q, want %q (space typed, not intercepted)", m.input.Value(), "A B")
	}
	if m.collaborative {
		t.Error("typing a space should not have toggled collaborative")
	}
}

// TestTabFocusesToggleAndSpaceFlipsIt pins the ModeRename-only interaction:
// tab moves focus onto the toggle, and space flips it there.
func TestTabFocusesToggleAndSpaceFlipsIt(t *testing.T) {
	s := openTestStore(t)
	id, err := s.CreateList("Shared", "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	m := New(ModeRename, id, s).(Model)

	m, _ = step(t, m, tea.KeyPressMsg{Text: "tab", Code: tea.KeyTab})
	if !m.toggleFocused {
		t.Fatal("tab should have moved focus onto the toggle")
	}
	if m.input.Focused() {
		t.Error("the name input should be blurred while the toggle has focus")
	}

	m, _ = step(t, m, tea.KeyPressMsg{Text: " ", Code: ' '})
	if !m.collaborative {
		t.Error("space should have flipped collaborative to true")
	}

	// tab back to the name field.
	m, _ = step(t, m, tea.KeyPressMsg{Text: "tab", Code: tea.KeyTab})
	if m.toggleFocused {
		t.Error("a second tab should have returned focus to the name field")
	}
	if !m.input.Focused() {
		t.Error("the name input should be focused again")
	}
}

// TestModeNewIgnoresToggleKeys pins that tab/space are ModeRename-only: in
// ModeNew there is no toggle, so space must always type into the name field
// and tab must not panic or do anything observable.
func TestModeNewIgnoresToggleKeys(t *testing.T) {
	s := openTestStore(t)
	m := New(ModeNew, "", s).(Model)

	m, _ = step(t, m, tea.KeyPressMsg{Text: "tab", Code: tea.KeyTab})
	if m.toggleFocused {
		t.Error("ModeNew has no toggle to focus")
	}

	m, _ = step(t, m, tea.KeyPressMsg{Text: "A", Code: 'A'})
	m, _ = step(t, m, tea.KeyPressMsg{Text: " ", Code: ' '})
	m, _ = step(t, m, tea.KeyPressMsg{Text: "B", Code: 'B'})
	if m.input.Value() != "A B" {
		t.Errorf("input value = %q, want %q", m.input.Value(), "A B")
	}
}

// TestSubmitPersistsCollaborativeWithoutRenaming is the feature's whole
// point: a human can open rename, toggle collaborative, and submit WITHOUT
// retyping the name — the name stays exactly as it was.
func TestSubmitPersistsCollaborativeWithoutRenaming(t *testing.T) {
	s := openTestStore(t)
	id, err := s.CreateList("Untouched name", "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	m := New(ModeRename, id, s).(Model)

	m, _ = step(t, m, tea.KeyPressMsg{Text: "tab", Code: tea.KeyTab})
	m, _ = step(t, m, tea.KeyPressMsg{Text: " ", Code: ' '})
	_, cmd := step(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})
	runSubmit(t, cmd)

	l, err := s.GetList(id)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if l.Name != "Untouched name" {
		t.Errorf("Name = %q, want unchanged %q", l.Name, "Untouched name")
	}
	if !l.Collaborative {
		t.Error("Collaborative = false, want true (toggled before submit)")
	}
}
