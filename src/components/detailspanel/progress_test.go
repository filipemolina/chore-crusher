package detailspanel

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/store"
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

// The relabelling is display-only. These are the values the DB column and the
// CLI's `farol progress --mode` both speak
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

// Up/down step the percentage by 5 and clamp at both ends, so holding a key
// settles at 0 or 100 rather than erroring or wrapping.
func TestPercentNudgeStepsByFiveAndClamps(t *testing.T) {
	for _, tc := range []struct {
		start string
		key   string
		want  string
	}{
		{"60", "up", "65"},
		{"60", "down", "55"},
		{"100", "up", "100"}, // clamped at the top
		{"98", "up", "100"},  // clamps rather than overshooting
		{"0", "down", "0"},   // clamped at the bottom
		{"3", "down", "0"},   // clamps rather than going negative
		{"", "up", "5"},      // unset counts as 0
		{"", "down", "0"},    // and cannot go below it
	} {
		m, _, _ := loaded(t, "")
		m = zoneFor(t, m, focusProgress)
		m.progressKind = apptypes.ProgressPercentage
		m.percentInput = tc.start

		m, _ = updateModel(m, tea.KeyPressMsg{Code: keyCodeFor(tc.key), Text: tc.key})
		if m.percentInput != tc.want {
			t.Errorf("%q then %s = %q, want %q", tc.start, tc.key, m.percentInput, tc.want)
		}
	}
}

func keyCodeFor(name string) rune {
	if name == "up" {
		return tea.KeyUp
	}
	return tea.KeyDown
}

// Nudging is percentage-only: the other two modes have no value the user owns,
// so up/down must not invent one.
func TestPercentNudgeIgnoredInOtherModes(t *testing.T) {
	for _, kind := range []apptypes.ProgressKind{apptypes.ProgressSimple, apptypes.ProgressSubtasks} {
		m, _, _ := loaded(t, "")
		m = zoneFor(t, m, focusProgress)
		m.progressKind = kind
		m.percentInput = ""

		m, _ = updateModel(m, tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
		if m.percentInput != "" {
			t.Errorf("%q mode: up set percentInput to %q, want it untouched", kind, m.percentInput)
		}
	}
}

// Typing digits still works exactly as before — the nudge keys are an addition,
// not a replacement.
func TestTypingDigitsStillSetsThePercentage(t *testing.T) {
	m, _, _ := loaded(t, "")
	m = zoneFor(t, m, focusProgress)
	m.progressKind = apptypes.ProgressPercentage

	m = typeRune(t, m, '6')
	m = typeRune(t, m, '0')
	if m.percentInput != "60" {
		t.Errorf("typing \"60\" gave %q, want %q", m.percentInput, "60")
	}

	// Out of range still reports rather than silently clamping typed input:
	// the user can see what they typed, unlike a held arrow key.
	m = typeRune(t, m, '0') // -> "600"
	if m.errMsg == "" {
		t.Error("typing past 100 must report an out-of-range error")
	}
	if m.percentInput != "60" {
		t.Errorf("a rejected digit must not land in the buffer, got %q", m.percentInput)
	}
}

// The value reads as an editable field, not a status annotation: TextPrimary,
// no parentheses, "0%" rather than "(—)" when unset, and lifted onto
// BackgroundElevated while the Progress zone holds focus.
func TestPercentageRendersAsAFieldNotAnAnnotation(t *testing.T) {
	m, _, _ := loaded(t, "")
	m = zoneFor(t, m, focusProgress)
	m.progressKind = apptypes.ProgressPercentage

	unset := ansi.Strip(m.renderProgressZone())
	if strings.Contains(unset, "(—)") {
		t.Errorf("an unset percentage must not render the em-dash annotation, got: %q", unset)
	}
	if !strings.Contains(unset, "0%") {
		t.Errorf("an unset percentage must render as %q, got: %q", "0%", unset)
	}

	m.percentInput = "60"
	set := ansi.Strip(m.renderProgressZone())
	if strings.Contains(set, "(60%)") {
		t.Errorf("the value must not render as a dim parenthetical, got: %q", set)
	}
	if !strings.Contains(set, "60%") {
		t.Errorf("the value must render, got: %q", set)
	}

	// Focused, the field carries the elevated-tier lift this app uses to show
	// focus; blurred, it does not.
	focused := m.renderProgressZone()
	m.focus = focusTitle
	blurred := m.renderProgressZone()
	if focused == blurred {
		t.Error("the percentage field must look different when the Progress zone has focus")
	}
}

// The Progress zone advertises how to enter a value, but only in the mode where
// typing and the arrows actually do something.
func TestPercentInputMethodsAdvertisedOnlyInPercentageMode(t *testing.T) {
	m, _, _ := loaded(t, "")
	m = zoneFor(t, m, focusProgress)

	m.progressKind = apptypes.ProgressPercentage
	footer := ansi.Strip(m.renderFooter())
	for _, want := range []string{"type a number", "±5"} {
		if !strings.Contains(footer, want) {
			t.Errorf("percentage mode must advertise %q, got: %q", want, footer)
		}
	}

	m.progressKind = apptypes.ProgressSimple
	footer = ansi.Strip(m.renderFooter())
	for _, gone := range []string{"type a number", "±5"} {
		if strings.Contains(footer, gone) {
			t.Errorf("simple mode must not advertise %q, got: %q", gone, footer)
		}
	}
}
