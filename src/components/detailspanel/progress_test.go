package detailspanel

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/store"
)

// Each progress mode renders a self-describing label. "simple" told the user
// nothing; §3's actual claim is "being worked on, no number attached".
func TestProgressModesRenderSelfDescribingLabels(t *testing.T) {
	for _, tc := range []struct {
		kind apptypes.ProgressKind
		want string
	}{
		{apptypes.ProgressSimple, "in progress (flag)"},
		{apptypes.ProgressSubtasks, "from subtasks"},
		{apptypes.ProgressPercentage, "percentage"},
	} {
		if got := progressModeLabel(tc.kind); got != tc.want {
			t.Errorf("progressModeLabel(%q) = %q, want %q", tc.kind, got, tc.want)
		}
	}

	// The mode name leads the zone, so a bare internal name would show up as
	// the prefix. Checking the prefix (rather than "contains") is what catches
	// "simple" while still allowing "from subtasks" to contain "subtasks".
	m, _, _ := loaded(t, "")
	m = zoneFor(t, m, focusProgress)
	for _, tc := range []struct {
		kind apptypes.ProgressKind
		want string
	}{
		{apptypes.ProgressSimple, "in progress (flag)"},
		{apptypes.ProgressSubtasks, "from subtasks"},
		{apptypes.ProgressPercentage, "percentage"},
	} {
		m.progressKind = tc.kind
		rendered := ansi.Strip(m.renderProgressZone())
		if !strings.HasPrefix(rendered, tc.want) {
			t.Errorf("%q mode must lead with %q, got: %q", tc.kind, tc.want, rendered)
		}
	}

	// "simple" in particular must not survive anywhere in the zone.
	m.progressKind = apptypes.ProgressSimple
	if rendered := ansi.Strip(m.renderProgressZone()); strings.Contains(rendered, "simple") {
		t.Errorf("the internal name %q must not reach the screen, got: %q", "simple", rendered)
	}
}

// The "(no children)" annotation for a subtasks-mode task with nothing to
// derive from is deliberate (docs/DESIGN.md §3: the mode is kept, only the
// display falls back) and survives the relabelling.
func TestSubtasksWithNoChildrenKeepsItsAnnotation(t *testing.T) {
	m, _, _ := loaded(t, "")
	m = zoneFor(t, m, focusProgress)
	m.progressKind = apptypes.ProgressSubtasks
	m.displayAsSimple = true

	rendered := ansi.Strip(m.renderProgressZone())
	if !strings.Contains(rendered, "from subtasks") {
		t.Errorf("expected the new label, got: %q", rendered)
	}
	if !strings.Contains(rendered, "(no children)") {
		t.Errorf("the (no children) annotation must survive relabelling, got: %q", rendered)
	}
}

// The relabelling is display-only. These are the values the DB column, the
// CLI's `crush progress --mode`, and the MCP tool's parameter all speak
// (docs/DESIGN.md §9); renaming one would break every caller outside the TUI.
func TestStoredModeVocabularyIsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		got  string
		want string
	}{
		{string(apptypes.ProgressSimple), "simple"},
		{string(apptypes.ProgressSubtasks), "subtasks"},
		{string(apptypes.ProgressPercentage), "percentage"},
		{string(store.ProgressSimple), "simple"},
		{string(store.ProgressSubtasks), "subtasks"},
		{string(store.ProgressPercentage), "percentage"},
	} {
		if tc.got != tc.want {
			t.Errorf("stored mode value = %q, want %q — this is a public contract (CLI + MCP), not a display string", tc.got, tc.want)
		}
	}
}
