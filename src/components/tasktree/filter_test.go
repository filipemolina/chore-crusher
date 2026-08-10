package tasktree

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
)

// gardenRows is the demo shape the /-filter bug was reported against: one
// pending parent whose title does not contain "beds", over three children that
// do. Filtering on "beds" therefore keeps the parent only as an ancestor,
// which is what the elision guard below pins.
func gardenRows() []apptypes.Row {
	parent := strPtr("P")
	return []apptypes.Row{
		{Task: apptypes.Task{ID: "P", Title: "Plan the garden", Status: apptypes.StatusPending}, HasChildren: true, Depth: 0},
		{Task: apptypes.Task{ID: "C1", Title: "Prep the beds", Status: apptypes.StatusPending, ParentID: parent}, Depth: 1},
		{Task: apptypes.Task{ID: "C2", Title: "Build the beds", Status: apptypes.StatusPending, ParentID: parent}, Depth: 1},
		{Task: apptypes.Task{ID: "D", Title: "Refinish the deck", Status: apptypes.StatusPending}, Depth: 0},
	}
}

// filtering returns a focused tree with the /-input open and query typed one
// keystroke at a time through Update, so the test exercises the real live path
// rather than assigning filter state directly. enter is never sent: everything
// asserted afterwards is the typing state.
func filtering(t *testing.T, query string) Model {
	t.Helper()
	m := New().(Model)
	m.focused = true
	m.activeList = true
	m.applyRows(gardenRows())

	next, _ := m.Update(cmds.ActivateFilterMsg{})
	m = next.(Model)
	if !m.filterTyping {
		t.Fatal("ActivateFilterMsg should open the filter input")
	}
	for _, r := range query {
		next, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(Model)
	}
	if m.filterApplied {
		t.Fatal("typing must not apply the filter — enter is what commits it")
	}
	return m
}

func rowIDs(rows []apptypes.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Task.ID
	}
	return out
}

// REGRESSION GUARD (docs: task "/ should filter the task tree live"). An
// applied filter still elides a non-matching ancestor to "[…] Plan the
// garden": the matched children keep their tree context without the parent
// pretending to be a match. Live filtering must not change this.
func TestAppliedFilterStillElidesNonMatchingAncestor(t *testing.T) {
	m := &Model{collapsed: make(map[string]bool)}
	m.applyRows(gardenRows())
	m.activeList = true
	m.filterApplied = true
	m.filterQuery = "beds"

	rendered := ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))

	if !strings.Contains(rendered, "[…] Plan the garden") {
		t.Errorf("non-matching ancestor must render elided as %q, got:\n%s", "[…] Plan the garden", rendered)
	}
	for _, title := range []string{"Prep the beds", "Build the beds"} {
		if !strings.Contains(rendered, title) {
			t.Errorf("matched child %q must stay visible, got:\n%s", title, rendered)
		}
	}
	if strings.Contains(rendered, "Refinish the deck") {
		t.Errorf("unrelated root must drop out of the filtered view, got:\n%s", rendered)
	}
}

// REGRESSION GUARD. A query nothing matches still shows the "No tasks match"
// card rather than an empty panel, both once applied and while still typing.
func TestNoMatchQueryStillShowsNoMatchCard(t *testing.T) {
	applied := &Model{collapsed: make(map[string]bool)}
	applied.applyRows(gardenRows())
	applied.activeList = true
	applied.filterApplied = true
	applied.filterQuery = "nowhere-nothing"

	typing := filtering(t, "nowhere-nothing")

	for name, m := range map[string]Model{"applied": *applied, "typing": typing} {
		rendered := ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))
		if !strings.Contains(rendered, "No tasks match") {
			t.Errorf("%s: no-match query must render the %q card, got:\n%s", name, "No tasks match", rendered)
		}
	}
}

// The bug this task fixes: typing "gard" after / narrows the tree on every
// keystroke, with no enter. Only "Plan the garden" matches, and it has no
// matched descendants, so it stands alone.
func TestTypingNarrowsRowsWithoutEnter(t *testing.T) {
	m := filtering(t, "gard")

	got := rowIDs(m.displayedRows())
	if len(got) != 1 || got[0] != "P" {
		t.Fatalf("typing %q narrowed to %v, want [P] (\"Plan the garden\")", "gard", got)
	}

	rendered := ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))
	for _, gone := range []string{"Prep the beds", "Build the beds", "Refinish the deck"} {
		if strings.Contains(rendered, gone) {
			t.Errorf("non-matching row %q must be gone while typing, got:\n%s", gone, rendered)
		}
	}
}

// Every keystroke re-narrows: the row set tracks the query as it grows, so the
// user sees the list shrink under their fingers instead of after enter.
func TestEachKeystrokeRenarrowsTheTree(t *testing.T) {
	// "b" matches both "beds" children and "Build the beds"; "beds" keeps the
	// two children (plus their ancestor); "bedsx" matches nothing.
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"", []string{"P", "C1", "C2", "D"}},
		{"beds", []string{"P", "C1", "C2"}},
		{"gard", []string{"P"}},
		{"bedsx", nil},
	} {
		m := filtering(t, tc.query)
		if got := rowIDs(m.displayedRows()); strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("typing %q showed %v, want %v", tc.query, got, tc.want)
		}
	}
}

// The header carries the match count from the first character typed — the
// signal that tells a user their query is too narrow before they give up on
// it. The count is direct matches only: the ancestors kept for tree context
// are not matches and must not inflate it.
func TestHeaderShowsMatchCountWhileTyping(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  string
	}{
		{"gard", "1 match"},   // "Plan the garden" only
		{"beds", "2 matches"}, // both children; the ancestor is not counted
	} {
		m := filtering(t, tc.query)
		rendered := ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))
		if !strings.Contains(rendered, tc.want) {
			t.Errorf("typing %q must show %q in the header, got:\n%s", tc.query, tc.want, rendered)
		}
		if !strings.Contains(rendered, "esc to clear") {
			t.Errorf("typing %q must show %q from the first keystroke, got:\n%s", tc.query, "esc to clear", rendered)
		}
	}

	// The count agrees in number: one match is never "1 matches", and a query
	// that matches nothing says so rather than falling back to a stale count.
	for _, tc := range []struct{ query, want, reject string }{
		{"gard", "1 match", "1 matches"},
		{"beds", "2 matches", "2 match "},
		{"bedsx", "0 matches", ""},
	} {
		m := filtering(t, tc.query)
		rendered := ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))
		if !strings.Contains(rendered, tc.want) {
			t.Errorf("typing %q must show %q, got:\n%s", tc.query, tc.want, rendered)
		}
		if tc.reject != "" && strings.Contains(rendered, tc.reject) {
			t.Errorf("typing %q must not show %q, got:\n%s", tc.query, tc.reject, rendered)
		}
	}

	// With / open but nothing typed yet there is no query to count, so the bar
	// offers the escape hatch without inventing a number.
	empty := ansi.Strip(filtering(t, "").ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))
	if strings.Contains(empty, "match") {
		t.Errorf("an empty query must not show a match count, got:\n%s", empty)
	}
	if !strings.Contains(empty, "esc to clear") {
		t.Errorf("an empty query must still offer %q, got:\n%s", "esc to clear", empty)
	}
}

// enter still means something: it commits the query, blurs the input and
// leaves the same rows filtered, so the cursor can move through them.
func TestEnterStillCommitsTheSameFilter(t *testing.T) {
	m := filtering(t, "beds")
	before := rowIDs(m.displayedRows())

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)

	if m.filterTyping || !m.filterApplied {
		t.Fatalf("enter should commit: filterTyping=%v filterApplied=%v, want false/true", m.filterTyping, m.filterApplied)
	}
	if m.filterQuery != "beds" {
		t.Errorf("committed query = %q, want %q", m.filterQuery, "beds")
	}
	if after := rowIDs(m.displayedRows()); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Errorf("committing changed the row set: %v -> %v", before, after)
	}
	rendered := ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))
	if !strings.Contains(rendered, "2 matches") || !strings.Contains(rendered, "esc to clear") {
		t.Errorf("applied header must keep the count and the esc hint, got:\n%s", rendered)
	}
}

// Esc while typing abandons the filter outright and restores every row.
func TestEscWhileTypingClearsFilterAndRestoresRows(t *testing.T) {
	m := filtering(t, "gard")
	if len(m.displayedRows()) != 1 {
		t.Fatalf("precondition: %q should have narrowed the tree", "gard")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape, Text: "esc"})
	m = next.(Model)

	if m.filterTyping || m.filterApplied || m.filterQuery != "" {
		t.Errorf("esc must clear all filter state, got typing=%v applied=%v query=%q",
			m.filterTyping, m.filterApplied, m.filterQuery)
	}
	if got := rowIDs(m.displayedRows()); len(got) != 4 {
		t.Errorf("esc restored %v, want all four rows", got)
	}
	rendered := ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))
	for _, title := range []string{"Plan the garden", "Prep the beds", "Build the beds", "Refinish the deck"} {
		if !strings.Contains(rendered, title) {
			t.Errorf("esc must restore %q, got:\n%s", title, rendered)
		}
	}
}

// The matched substring is highlighted inside a matching row: the styled
// render carries escape sequences the unfiltered render does not, while the
// visible text is unchanged. Ancestor-only rows stay dim and unhighlighted.
func TestMatchedSubstringIsHighlighted(t *testing.T) {
	m := filtering(t, "gard")
	row := m.displayedRows()[0]
	_, matched := matchVisible(m.rows, m.filterQuery)

	highlighted := m.renderRow(row, 80, appstyles.Active.BackgroundPanel, matched[row.Task.ID])
	plain := m.renderRow(row, 80, appstyles.Active.BackgroundPanel, nil)

	if highlighted == plain {
		t.Error("a matched row must render differently from an unmatched one (no highlight applied)")
	}
	if got, want := ansi.Strip(highlighted), ansi.Strip(plain); got != want {
		t.Errorf("highlighting must not change the visible text:\n got %q\nwant %q", got, want)
	}
}

// Highlighting must not widen a row past its column budget: the styling is
// zero-width escapes, so the same overflow contract holds as for a plain row.
func TestHighlightedRowNeverOverflows(t *testing.T) {
	m := &Model{collapsed: make(map[string]bool)}
	m.applyRows(gardenRows())
	m.filterQuery = "gard"
	_, matched := matchVisible(m.rows, "gard")
	row := m.rows[0]

	for width := 20; width <= 200; width += 10 {
		rendered := m.renderRow(row, width, appstyles.Active.BackgroundPanel, matched[row.Task.ID])
		if got := ansi.StringWidth(rendered); got > width {
			t.Fatalf("width %d: highlighted row = %d columns (overflow)", width, got)
		}
	}
}
