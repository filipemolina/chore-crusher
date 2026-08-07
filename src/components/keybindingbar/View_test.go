package keybindingbar

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/constants"
	"github.com/filipemolina/chore-crusher/src/keys"
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

// TestFooterIsBlankWhileDetailsIsOpen verifies the footer says nothing at all
// while the Details modal owns the keyboard. The modal carries its own hint
// line next to the controls it describes; the footer used to render a second
// copy that listed a different subset in different words ("esc close" vs
// "esc cancel"), so the user read two contradictory lines at once. The bar
// still paints its full-width background, so the layout height does not move.
func TestFooterIsBlankWhileDetailsIsOpen(t *testing.T) {
	m := New()
	m, _ = m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 120})
	m, _ = m.(Model).Update(cmds.SetFooterContextMsg{
		Focused:             constants.COMPONENT_DETAILS_PANEL,
		DetailsPanelVisible: true,
		HasActiveList:       true,
	})

	out := m.(Model).View().Content
	if got := strings.TrimSpace(ansi.Strip(out)); got != "" {
		t.Errorf("footer must render no text while Details is open, got %q", got)
	}
	if lipgloss.Height(out) != 1 {
		t.Errorf("the blank footer must still occupy exactly one line, got %d", lipgloss.Height(out))
	}
	if got := lipgloss.Width(out); got != 120 {
		t.Errorf("the blank footer must still paint its full width, got %d columns, want 120", got)
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

// TestFooterWithContextShowsCreateHints proves WithContext renders the
// context handed to it directly, without going through the Update/
// SetFooterContextMsg round trip — the fix for the footer lagging the
// current mode by one keystroke: AppModel.View now calls WithContext with
// this frame's context instead of reading the bar's own (stale) ctx field.
func TestFooterWithContextShowsCreateHints(t *testing.T) {
	m := New()
	m, _ = m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 120})

	out := m.(Model).WithContext(keys.Context{
		Focused:       constants.COMPONENT_TASK_TREE,
		HasActiveList: true,
		Creating:      true,
	}).View().Content

	for _, label := range []string{"create", "cancel"} {
		if !strings.Contains(out, label) {
			t.Errorf("creating footer missing %q:\n%s", label, out)
		}
	}
	if strings.Contains(out, "navigate") {
		t.Errorf("creating footer must not advertise browse hints:\n%s", out)
	}
}

// TestFooterWithContextShowsFilterHints is the Filtering half of the same
// proof: while ctx.Filtering is true the bar must show the filter hints
// (confirm/cancel), not the browse or create hints.
func TestFooterWithContextShowsFilterHints(t *testing.T) {
	m := New()
	m, _ = m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 120})

	out := m.(Model).WithContext(keys.Context{
		Focused:       constants.COMPONENT_TASK_TREE,
		HasActiveList: true,
		Filtering:     true,
	}).View().Content

	if !strings.Contains(out, "confirm") {
		t.Errorf("filtering footer missing the confirm hint:\n%s", out)
	}
	if strings.Contains(out, "create") {
		t.Errorf("filtering footer must not advertise create hints:\n%s", out)
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
