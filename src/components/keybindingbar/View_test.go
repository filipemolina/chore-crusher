package keybindingbar

import (
	"strings"
	"testing"

	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/constants"
)

func TestFooterRendersContextAndGlobalKeys(t *testing.T) {
	m := New()
	m, _ = m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 120})
	m, _ = m.(Model).Update(cmds.SetFooterContextMsg{
		Focused:           constants.COMPONENT_TASK_TREE,
		HasActiveList:     true,
		TaskTreeEmpty:     false,
		ListsPanelVisible: true,
	})

	out := m.(Model).View().Content
	// navigate, delete and new (n) are the task tree's context hints; help
	// is a global pinned on the right.
	for _, label := range []string{"navigate", "delete", "new", "help"} {
		if !strings.Contains(out, label) {
			t.Errorf("footer output missing %q:\n%s", label, out)
		}
	}
}

// TestFooterInDetailsContextShowsOnlyDetailsHints verifies that while the
// Details side panel owns the keyboard, the footer advertises its bindings
// (save/next field/mode/cancel) and none of the task-tree or global keys that
// are intentionally routed only to Details (docs/DESIGN.md §5).
func TestFooterInDetailsContextShowsOnlyDetailsHints(t *testing.T) {
	m := New()
	m, _ = m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 120})
	m, _ = m.(Model).Update(cmds.SetFooterContextMsg{
		Focused:             constants.COMPONENT_DETAILS_PANEL,
		DetailsPanelVisible: true,
		HasActiveList:       true,
	})

	out := m.(Model).View().Content
	for _, label := range []string{"save", "next field", "cancel"} {
		if !strings.Contains(out, label) {
			t.Errorf("Details footer missing %q:\n%s", label, out)
		}
	}
	for _, absent := range []string{"lists", "help", "navigate", "search", "theme"} {
		if strings.Contains(out, absent) {
			t.Errorf("Details footer must not advertise %q:\n%s", absent, out)
		}
	}
}

// TestFooterEmptyTreeAdvertisesNew verifies the empty task tree advertises
// only n (new) as its context hint: the inline input is the empty state's
// way in, and navigation/toggle keys have nothing to act on
// (docs/plan/task-row-cards-and-status.md).
func TestFooterEmptyTreeAdvertisesNew(t *testing.T) {
	m := New()
	m, _ = m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 120})
	m, _ = m.(Model).Update(cmds.SetFooterContextMsg{
		Focused:           constants.COMPONENT_TASK_TREE,
		HasActiveList:     true,
		TaskTreeEmpty:     true,
		ListsPanelVisible: true,
	})

	out := m.(Model).View().Content
	if !strings.Contains(out, "new") {
		t.Errorf("empty-tree footer missing the new hint:\n%s", out)
	}
	for _, absent := range []string{"navigate", "delete", "toggle"} {
		if strings.Contains(out, absent) {
			t.Errorf("empty-tree footer must not advertise %q:\n%s", absent, out)
		}
	}
}

func TestFooterShedsHintsOnNarrowTerminal(t *testing.T) {
	m := New()
	m, _ = m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 20})
	m, _ = m.(Model).Update(cmds.SetFooterContextMsg{
		Focused:           constants.COMPONENT_TASK_TREE,
		HasActiveList:     true,
		TaskTreeEmpty:     false,
		ListsPanelVisible: true,
	})

	out := m.(Model).View().Content
	// The bar should never wrap onto multiple physical lines; with only 20
	// columns it must have shed the longer context hints.
	if strings.Count(out, "\n") > 1 {
		t.Errorf("narrow footer wrapped onto multiple lines:\n%s", out)
	}
	if strings.Contains(out, "expand") {
		t.Errorf("narrow footer unexpectedly contains long hint:\n%s", out)
	}
}
