package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/constants"
)

// quits reports whether cmd leaves the app: tea.Quit returns a QuitMsg
// directly, and a batched cmd returns a BatchMsg holding the cmds it fans
// out, so a nested quit still counts. Nothing here executes a store write —
// the cmds under test are quit and the ordinary UI batches.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		for _, c := range msg {
			if quits(c) {
				return true
			}
		}
	}
	return false
}

// press runs one keypress through Update without running the resulting
// command, and reports whether that command would quit. The command is
// deliberately not executed: a test that asserts "q does not quit" must not
// have its assertion depend on what the follow-up commands do.
func press(t *testing.T, m AppModel, msg tea.KeyPressMsg) (AppModel, bool) {
	t.Helper()
	updated, cmd := m.Update(msg)
	out, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("Update returned %T, want AppModel", updated)
	}
	return out, quits(cmd)
}

// keyQ is the keypress the terminal delivers for a lowercase q.
var keyQ = tea.KeyPressMsg{Text: "q", Code: 'q'}

// keyCtrlC is ctrl+c, the unconditional escape hatch.
var keyCtrlC = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}

// showLists opens the lists panel and moves focus onto it, so the lists-panel
// cases can be driven the way a user reaches them.
func showLists(t *testing.T, m AppModel) AppModel {
	t.Helper()
	m = refresh(t, m, tea.KeyPressMsg{Text: "L", Code: 'L'})
	if m.focusedZone != constants.COMPONENT_LISTS_PANEL {
		t.Fatalf("focus after L = %d, want the lists panel (%d)", m.focusedZone, constants.COMPONENT_LISTS_PANEL)
	}
	return m
}

func TestQuitsFromTaskTree(t *testing.T) {
	m := seedOneList(t)

	if _, quit := press(t, m, keyQ); !quit {
		t.Error("q on the task tree did not quit")
	}
	if _, quit := press(t, m, keyCtrlC); !quit {
		t.Error("ctrl+c on the task tree did not quit")
	}
}

func TestQuitsFromListsPanel(t *testing.T) {
	m := showLists(t, seedOneList(t))

	if _, quit := press(t, m, keyQ); !quit {
		t.Error("q on the lists panel did not quit")
	}
	if _, quit := press(t, m, keyCtrlC); !quit {
		t.Error("ctrl+c on the lists panel did not quit")
	}
}

// TestQTypesALiteralWhileCreating is the case a weak version of this change
// breaks: q is a printable character, so while the inline create input is
// open it must reach the input as text instead of quitting.
func TestQTypesALiteralWhileCreating(t *testing.T) {
	m := seedOneList(t)
	m = refresh(t, m, m.bodyLayout)

	m = refresh(t, m, tea.KeyPressMsg{Text: "n", Code: 'n'})
	if !m.helpContext().Creating {
		t.Fatal("n did not open the inline create input")
	}

	for _, r := range "quick" {
		var quit bool
		m, quit = press(t, m, tea.KeyPressMsg{Text: string(r), Code: r})
		if quit {
			t.Fatalf("typing %q in the create input quit the app", r)
		}
	}

	out := ansi.Strip(m.View().Content)
	if !strings.Contains(out, "quick") {
		t.Errorf("create draft does not contain the typed text:\n%s", out)
	}

	// ctrl+c still leaves, even mid-draft.
	if _, quit := press(t, m, keyCtrlC); !quit {
		t.Error("ctrl+c while creating did not quit")
	}
}

// TestQTypesALiteralWhileFiltering covers the other typing surface: a
// /-filter being typed in the task tree.
func TestQTypesALiteralWhileFiltering(t *testing.T) {
	m := seedOneList(t)
	m = refresh(t, m, m.bodyLayout)

	m = refresh(t, m, tea.KeyPressMsg{Text: "/", Code: '/'})
	if !treeFilterActive(t, m) {
		t.Fatal("/ did not open the tree filter")
	}

	var quit bool
	if m, quit = press(t, m, keyQ); quit {
		t.Fatal("q while typing a filter quit the app")
	}

	// The filter bar renders as "/ > <query>", the textinput's own prompt
	// included, so a q that reached the input shows up as "> q" rather than
	// anywhere else on the frame.
	out := ansi.Strip(m.View().Content)
	if !strings.Contains(out, "> q") || !treeFilterActive(t, m) {
		t.Errorf("q while typing a filter did not reach the filter input:\n%s", out)
	}

	if _, quit := press(t, m, keyCtrlC); !quit {
		t.Error("ctrl+c while filtering did not quit")
	}
}

// TestQTypesALiteralWhileFilteringLists is the same case on the lists panel,
// whose filter is the bubbles list's own.
func TestQTypesALiteralWhileFilteringLists(t *testing.T) {
	m := showLists(t, seedOneList(t))
	m = refresh(t, m, m.bodyLayout)

	m = refresh(t, m, tea.KeyPressMsg{Text: "/", Code: '/'})
	if !listsFilterActive(t, m) {
		t.Fatal("/ did not open the lists filter")
	}

	if _, quit := press(t, m, keyQ); quit {
		t.Error("q while typing a lists filter quit the app")
	}
	if _, quit := press(t, m, keyCtrlC); !quit {
		t.Error("ctrl+c while filtering the lists panel did not quit")
	}
}

// TestQDoesNotQuitWhileAModalIsOpen: a modal owns the keyboard, so q belongs
// to it, not to the quit handler.
func TestQDoesNotQuitWhileAModalIsOpen(t *testing.T) {
	m := seedOneList(t)

	m = refresh(t, m, tea.KeyPressMsg{Text: "?", Code: '?'})
	if m.activeModal == nil {
		t.Fatal("? did not open the help overlay")
	}

	if _, quit := press(t, m, keyQ); quit {
		t.Error("q quit the app while the help overlay was open")
	}
	if _, quit := press(t, m, keyCtrlC); !quit {
		t.Error("ctrl+c did not quit while the help overlay was open")
	}
}

// TestQDoesNotQuitWhileDetailsIsOpen: the Details modal owns every keypress
// except ctrl+c, and its comment compose card takes typed characters.
func TestQDoesNotQuitWhileDetailsIsOpen(t *testing.T) {
	m := seedOneList(t)

	selected := ""
	if tree, ok := m.components.TaskPanel.(interface{ SelectedID() string }); ok {
		selected = tree.SelectedID()
	}
	if selected == "" {
		t.Fatal("no task selected to open Details on")
	}
	m = refresh(t, m, cmds.OpenDetails(selected)())
	if !m.detailsPanelVisible {
		t.Fatal("Details did not open")
	}

	if _, quit := press(t, m, keyQ); quit {
		t.Error("q quit the app while the Details modal was open")
	}
	if _, quit := press(t, m, keyCtrlC); !quit {
		t.Error("ctrl+c did not quit while the Details modal was open")
	}
}
