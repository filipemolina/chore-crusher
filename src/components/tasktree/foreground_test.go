package tasktree

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/components/chrome"
)

// TestRowNeverDrawsDefaultForeground pins the foreground-bleed invariant for
// every row state: a title drawn with no foreground SGR renders in the
// terminal's own default color, which vanishes on a light theme's panel
// (farol-day made the bug visible: pending titles were white on warm
// off-white). The fix gave every state an explicit tier — TextPrimary for
// pending and in progress, TextMuted for complete — and styled the
// expand/collapse marker with the row's own tier rather than leaving it
// unstyled after the title's reset. This test renders all states plus a
// highlighted filtered row and asserts the invariant through
// appstyles.HasDefaultForeground rather than re-deriving the colors, so a
// future edit that drops a Foreground() anywhere in the row fails here.
func TestRowNeverDrawsDefaultForeground(t *testing.T) {
	m := New().(Model)
	m.collapsed = make(map[string]bool)

	parent := apptypes.Row{
		Task:        apptypes.Task{ID: "p", Title: "Parent task", Status: apptypes.StatusPending},
		HasChildren: true,
	}
	collapsedParent := parent
	m.collapsed["p"] = true

	rows := []struct {
		name string
		row  apptypes.Row
	}{
		{"pending", apptypes.Row{Task: apptypes.Task{ID: "1", Title: "Buy milk", Status: apptypes.StatusPending}}},
		{"in progress", apptypes.Row{Task: apptypes.Task{ID: "2", Title: "Half done", Status: apptypes.StatusInProgress}}},
		{"complete", apptypes.Row{Task: apptypes.Task{ID: "3", Title: "Done thing", Status: apptypes.StatusComplete}}},
		{"expanded parent", parent},
		{"collapsed parent", collapsedParent},
		{"filtered pending", apptypes.Row{Task: apptypes.Task{ID: "4", Title: "Match me", Status: apptypes.StatusPending}}},
	}

	for _, tc := range rows {
		var matched []int
		if strings.HasPrefix(tc.name, "filtered") {
			matched = []int{0, 1, 2} // a highlighted run inside the title
		}
		rendered := m.renderRow(tc.row, 80, testBg, matched)
		if appstyles.HasDefaultForeground(rendered) {
			t.Errorf("%s row draws glyphs in the terminal default foreground: %q", tc.name, ansi.Strip(rendered))
		}
	}
}

// TestCreateRowInputIsThemeSealed pins the same invariant for the inline
// create row: once the user has typed, the textinput's own View carries the
// text, and the bubbles default carries no foreground on focused text. The
// seal runs inside renderCreateRow, so rendering after sealing must be
// clean whether the input is empty (placeholder) or typed.
func TestCreateRowInputIsThemeSealed(t *testing.T) {
	m := New().(Model)
	m.StartCreating("")
	m.createInput.SetValue("new task title")

	rendered := m.renderCreateRow(80, testBg)
	if appstyles.HasDefaultForeground(rendered) {
		t.Errorf("create row with typed text draws default foreground: %q", ansi.Strip(rendered))
	}

	m.createInput.SetValue("")
	placeholder := m.renderCreateRow(80, testBg)
	if !strings.Contains(ansi.Strip(placeholder), "Add a task") {
		t.Fatalf("precondition: empty create row shows the placeholder, got %q", ansi.Strip(placeholder))
	}
	if appstyles.HasDefaultForeground(placeholder) {
		t.Errorf("create row placeholder draws default foreground: %q", ansi.Strip(placeholder))
	}
}

// TestFilterBarInputIsThemeSealed pins the invariant for the / filter bar:
// ViewInPanel seals the input onto the panel surface before the bar renders,
// so this test applies the same seal the way ViewInPanel does, then asserts
// the whole composed bar — slash, live input text, and suffix.
func TestFilterBarInputIsThemeSealed(t *testing.T) {
	m := New().(Model)
	m.filterTyping = true
	chrome.SealInput(&m.filterInput, testBg, testBg)

	m.filterInput.SetValue("query")
	bar := m.renderFilterBar(3)
	if appstyles.HasDefaultForeground(bar) {
		t.Errorf("filter bar draws default foreground: %q", ansi.Strip(bar))
	}
}
