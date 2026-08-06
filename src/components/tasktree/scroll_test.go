package tasktree

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
)

// driveTree builds a focused tree loaded with rows at a given panel size.
func driveTree(t *testing.T, rows []apptypes.Row, panelHeight, mainWidth int) Model {
	t.Helper()
	m := New().(Model)
	u, _ := m.Update(cmds.SetBodyLayoutMsg{Height: panelHeight, MainWidth: mainWidth})
	m = u.(Model)
	u, _ = m.Update(cmds.RefreshTasksMsg{ListID: "L", Rows: rows})
	m = u.(Model)
	u, _ = m.Update(cmds.SetFocusMsg(focusedZoneID))
	return u.(Model)
}

func pressKey(m Model, text string, code rune) Model {
	u, _ := m.Update(tea.KeyPressMsg{Text: text, Code: code})
	return u.(Model)
}

// treeBody renders the windowed tree body the way taskspanel does.
func treeBody(m Model) string { return m.View().Content }

// nPending builds n pending tasks p00..p(n-1).
func nPending(n int) []apptypes.Row {
	rows := make([]apptypes.Row, n)
	for i := range rows {
		rows[i] = apptypes.Row{Task: apptypes.Task{ID: fmt.Sprintf("p%02d", i), Title: fmt.Sprintf("Pending-%02d", i), Status: apptypes.StatusPending}}
	}
	return rows
}

// assertNoLineOverflows checks no rendered line is wider than the body width.
func assertNoLineOverflows(t *testing.T, m Model, body string) {
	t.Helper()
	w := m.bodyWidth()
	for i, line := range strings.Split(body, "\n") {
		if got := lipgloss.Width(line); got > w {
			t.Errorf("line %d width %d exceeds panel width %d: %q", i, got, w, ansi.Strip(line))
		}
	}
}

// TestScrollKeepsSelectionReachable drives the selection from the first task to
// the final pending task and then to the final complete task in a panel far
// shorter than the content, asserting the selected row is rendered at each step
// and no row overflows the panel width (Commit 5 acceptance).
func TestScrollKeepsSelectionReachable(t *testing.T) {
	rows := nPending(10)
	for i := 0; i < 5; i++ {
		rows = append(rows, apptypes.Row{Task: apptypes.Task{ID: fmt.Sprintf("c%02d", i), Title: fmt.Sprintf("Complete-%02d", i), Status: apptypes.StatusComplete}})
	}
	m := driveTree(t, rows, 12, 40) // body height ~8, well under 15 rows + chrome

	// The selection starts on the first pending row; it is visible.
	if got := m.SelectedID(); got != "p00" {
		t.Fatalf("initial selection = %q, want p00", got)
	}
	body := ansi.Strip(treeBody(m))
	if !strings.Contains(body, "Pending-00") {
		t.Fatalf("first task not visible initially:\n%s", body)
	}

	// Navigate down through the whole flat order (pending then complete);
	// stop at the last pending and assert it scrolled into view.
	for i := 0; i < 9; i++ {
		m = pressKey(m, "j", 'j')
	}
	if got := m.SelectedID(); got != "p09" {
		t.Fatalf("after 9×down selection = %q, want p09", got)
	}
	body = ansi.Strip(treeBody(m))
	if !strings.Contains(body, "Pending-09") {
		t.Errorf("last pending task not visible after scrolling to it:\n%s", body)
	}
	assertNoLineOverflows(t, m, treeBody(m))

	// Continue to the very last complete task.
	for i := 0; i < 20; i++ {
		m = pressKey(m, "j", 'j')
	}
	if got := m.SelectedID(); got != "c04" {
		t.Fatalf("after navigating to the end selection = %q, want c04", got)
	}
	body = ansi.Strip(treeBody(m))
	if !strings.Contains(body, "Complete-04") {
		t.Errorf("last complete task not visible after scrolling to it:\n%s", body)
	}
	assertNoLineOverflows(t, m, treeBody(m))
}

// TestScrollClampsOnShrinkAndRestoresOnGrow scrolls to the bottom, shrinks the
// panel (the old offset must clamp so the selection stays visible), then grows
// it back (all content returns), with no background bleed at any size.
func TestScrollClampsOnShrinkAndRestoresOnGrow(t *testing.T) {
	rows := nPending(20)
	m := driveTree(t, rows, 12, 40)

	// Scroll to the last row.
	for i := 0; i < 25; i++ {
		m = pressKey(m, "j", 'j')
	}
	if got := m.SelectedID(); got != "p19" {
		t.Fatalf("selection = %q, want p19 after scrolling down", got)
	}
	if m.scrollOffset == 0 {
		t.Fatalf("expected a non-zero offset after scrolling to the bottom")
	}

	// Shrink the panel: the offset must clamp and the selection stay visible.
	u, _ := m.Update(cmds.SetBodyLayoutMsg{Height: 8, MainWidth: 40})
	m = u.(Model)
	body := treeBody(m)
	if !strings.Contains(ansi.Strip(body), "Pending-19") {
		t.Errorf("shrinking the panel hid the selection:\n%s", ansi.Strip(body))
	}
	if got, want := lipgloss.Height(body), m.bodyHeight(); got != want {
		t.Errorf("shrunk body height = %d, want exactly %d", got, want)
	}
	if appstyles.HasBackgroundBleed(body) {
		t.Errorf("shrunk window has background bleed:\n%q", body)
	}

	// Grow the panel tall enough for everything: content is fully restored.
	u, _ = m.Update(cmds.SetBodyLayoutMsg{Height: 60, MainWidth: 40})
	m = u.(Model)
	body = treeBody(m)
	full := ansi.Strip(body)
	if !strings.Contains(full, "Pending-00") || !strings.Contains(full, "Pending-19") {
		t.Errorf("growing the panel did not restore the full content:\n%s", full)
	}
	if got, want := lipgloss.Height(body), m.bodyHeight(); got != want {
		t.Errorf("grown body height = %d, want exactly %d", got, want)
	}
	if appstyles.HasBackgroundBleed(body) {
		t.Errorf("grown window has background bleed:\n%q", body)
	}
}

// TestScrollAdjustsOnFilterAndCreate checks the offset tracks the two other
// state transitions the plan calls out: applying then clearing a /-filter keeps
// the matched row visible, and starting inline creation on the last task scrolls
// the create row into view; cancelling returns to a visible selection.
func TestScrollAdjustsOnFilterAndCreate(t *testing.T) {
	rows := nPending(20)
	m := driveTree(t, rows, 12, 40)

	// Filter to a single deep match; it must be visible in the filtered window.
	u, _ := m.Update(cmds.ActivateFilterMsg{})
	m = u.(Model)
	for _, r := range "Pending-18" {
		m = pressKey(m, string(r), r)
	}
	body := ansi.Strip(treeBody(m))
	if !strings.Contains(body, "Pending-18") {
		t.Errorf("filtered match not visible:\n%s", body)
	}

	// Clear the filter (esc): back to the sections with a visible selection.
	m = pressKey(m, "", tea.KeyEsc)
	if m.FilterActive() {
		t.Fatalf("esc did not clear the filter")
	}
	body = ansi.Strip(treeBody(m))
	if sel := m.SelectedID(); sel != "" && !strings.Contains(body, titleFor(rows, sel)) {
		t.Errorf("selection %q not visible after clearing the filter:\n%s", sel, body)
	}

	// Select the last task, then start inline creation: the create row is
	// spliced after it and must scroll into view.
	u, _ = m.Update(cmds.SelectTaskMsg{TaskID: "p19"})
	m = u.(Model)
	u, _ = m.Update(cmds.StartCreatingMsg{})
	m = u.(Model)
	if !m.IsCreating() {
		t.Fatalf("StartCreating did not enter creating mode")
	}
	body = ansi.Strip(treeBody(m))
	if !strings.Contains(body, "Add a task") {
		t.Errorf("inline create row not visible after starting creation at the bottom:\n%s", body)
	}

	// Cancel: the selection is visible again.
	m = pressKey(m, "", tea.KeyEsc)
	if m.IsCreating() {
		t.Fatalf("esc did not cancel creating")
	}
	body = ansi.Strip(treeBody(m))
	if !strings.Contains(body, "Pending-19") {
		t.Errorf("selection not visible after cancelling creation:\n%s", body)
	}
}

// TestScrollWindowSealsShortPanel renders a long list in a short panel and a
// short list in a tall panel, asserting the window is exactly the panel height
// and never bleeds the background on its padded tail (Commit 5, extends the
// no-bleed sweep for scrolling).
func TestScrollWindowSealsShortPanel(t *testing.T) {
	cases := []struct {
		name          string
		count, height int
	}{
		{"long list, short panel", 30, 10},
		{"short list, tall panel", 3, 40},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := driveTree(t, nPending(c.count), c.height, 48)
			body := m.View().Content
			if got, want := lipgloss.Height(body), m.bodyHeight(); got != want {
				t.Errorf("body height = %d, want exactly the panel body height %d", got, want)
			}
			if appstyles.HasBackgroundBleed(body) {
				t.Errorf("window has background bleed:\n%q", body)
			}
			assertNoLineOverflows(t, m, body)
		})
	}
}

func titleFor(rows []apptypes.Row, id string) string {
	for _, r := range rows {
		if r.Task.ID == id {
			return r.Task.Title
		}
	}
	return ""
}
