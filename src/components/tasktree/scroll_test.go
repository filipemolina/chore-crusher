package tasktree

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
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

// TestFastMovementKeysJumpsToFirstAndLast verifies that g moves the selection to
// the first visible row and G to the last (Commit 1 of scroll-affordance task).
func TestFastMovementKeysJumpsToFirstAndLast(t *testing.T) {
	rows := nPending(30)
	m := driveTree(t, rows, 20, 48) // body height ~16

	// Selection starts on the first row.
	if got := m.SelectedID(); got != "p00" {
		t.Fatalf("initial selection = %q, want p00", got)
	}

	// G jumps to the last row.
	m = pressKey(m, "G", 'G')
	if got := m.SelectedID(); got != "p29" {
		t.Errorf("after G selection = %q, want p29", got)
	}

	// g jumps back to the first row.
	m = pressKey(m, "g", 'g')
	if got := m.SelectedID(); got != "p00" {
		t.Errorf("after g selection = %q, want p00", got)
	}

	// home and end also work as jump keys.
	m = pressKey(m, "", tea.KeyHome)
	if got := m.SelectedID(); got != "p00" {
		t.Errorf("after home selection = %q, want p00", got)
	}
	m = pressKey(m, "", tea.KeyEnd)
	if got := m.SelectedID(); got != "p29" {
		t.Errorf("after end selection = %q, want p29", got)
	}
}

// TestPageDownMovesByViewportHeight checks that pgdown moves the cursor by one
// viewport height (body height in rows) and clamps at the last row (Commit 1).
func TestPageDownMovesByViewportHeight(t *testing.T) {
	rows := nPending(30)
	m := driveTree(t, rows, 20, 48) // body height = 20 - 4 = 16

	// From the first row, pgdown moves bodyHeight rows down.
	m = pressKey(m, "", tea.KeyPgDown)
	wantIdx := m.bodyHeight() // 16
	wantID := fmt.Sprintf("p%02d", wantIdx)
	if got := m.SelectedID(); got != wantID {
		t.Fatalf("after pgdown selection = %q, want %q (bodyHeight=%d rows down)", got, wantID, wantIdx)
	}

	// Pressing pgdown again from there moves another viewport but clamps at the
	// last row rather than overshooting.
	m = pressKey(m, "", tea.KeyPgDown)
	if got := m.SelectedID(); got != "p29" {
		t.Errorf("after second pgdown selection = %q, want p29 (clamped at end)", got)
	}

	// G confirms we are at the last row.
	m = pressKey(m, "G", 'G')
	if got := m.SelectedID(); got != "p29" {
		t.Errorf("after G selection = %q, want p29", got)
	}

	// Pressing pgdown on the last row is a no-op (clamped).
	m = pressKey(m, "", tea.KeyPgDown)
	if got := m.SelectedID(); got != "p29" {
		t.Errorf("pgdown at the last row must not move, got %q", got)
	}
}

// TestPageUpMovesByViewportHeight checks that pgup moves the cursor up by one
// viewport height and clamps at the first row (Commit 1).
func TestPageUpMovesByViewportHeight(t *testing.T) {
	rows := nPending(30)
	m := driveTree(t, rows, 20, 48) // body height = 16

	// Jump to the end, then pgup should land bodyHeight rows above it.
	m = pressKey(m, "G", 'G')
	if got := m.SelectedID(); got != "p29" {
		t.Fatalf("after G selection = %q, want p29", got)
	}

	m = pressKey(m, "", tea.KeyPgUp)
	wantIdx := 29 - m.bodyHeight() // 29 - 16 = 13
	wantID := fmt.Sprintf("p%02d", wantIdx)
	if got := m.SelectedID(); got != wantID {
		t.Errorf("after pgup selection = %q, want %q (bodyHeight=%d rows up)", got, wantID, wantIdx)
	}

	// Pressing pgup again clamps at the first row.
	m = pressKey(m, "", tea.KeyPgUp)
	if got := m.SelectedID(); got != "p00" {
		t.Errorf("after second pgup selection = %q, want p00 (clamped at start)", got)
	}

	// Pressing pgup on the first row is a no-op.
	m = pressKey(m, "", tea.KeyPgUp)
	if got := m.SelectedID(); got != "p00" {
		t.Errorf("pgup at the first row must not move, got %q", got)
	}
}

// TestFastMovementKeysWithFilter checks that g/G/pgup/pgdown operate on the
// filtered row set, not the full unfiltered list (Commit 1).
func TestFastMovementKeysWithFilter(t *testing.T) {
	rows := nPending(30)
	m := driveTree(t, rows, 20, 48)

	// Enter filter mode and type a query that narrows to a handful of rows.
	u, _ := m.Update(cmds.ActivateFilterMsg{})
	m = u.(Model)
	for _, r := range "Pending-1" {
		m = pressKey(m, string(r), r)
	}
	// Apply the filter (enter).
	m = pressKey(m, "", tea.KeyEnter)

	// The filtered set must be narrowed (fuzzy "Pending-1" matches p01,
	// p10-p19, p21, etc. — the exact set depends on fuzzy scoring, but it
	// is strictly smaller than the full 30 rows).
	filtered := m.displayedRows()
	if len(filtered) == 0 || len(filtered) >= len(rows) {
		t.Fatalf("expected a narrowed filter set, got %d rows", len(filtered))
	}
	lastFiltered := filtered[len(filtered)-1].Task.ID
	firstFiltered := filtered[0].Task.ID

	// G jumps to the last filtered row.
	m = pressKey(m, "G", 'G')
	if got := m.SelectedID(); got != lastFiltered {
		t.Errorf("after G in filter selection = %q, want %q (last filtered match)", got, lastFiltered)
	}

	// g jumps back to the first filtered row.
	m = pressKey(m, "g", 'g')
	if got := m.SelectedID(); got != firstFiltered {
		t.Errorf("after g in filter selection = %q, want %q (first filtered match)", got, firstFiltered)
	}

	// Clear the filter to restore the full set.
	m = pressKey(m, "", tea.KeyEsc)

	// g still works on the unfiltered set.
	m = pressKey(m, "g", 'g')
	if got := m.SelectedID(); got != "p00" {
		t.Errorf("after clearing filter, g selection = %q, want p00", got)
	}
}

// TestSectionHeaderIsPinnedAtTop navigates from the first task into the
// Complete section, where the Pending header has scrolled past the top of the
// viewport. The Complete header should then be pinned as the first line so the
// user still knows which section they are in (Commit 5, Part 2).
func TestSectionHeaderIsPinnedAtTop(t *testing.T) {
	rows := nPending(20)
	for i := 0; i < 20; i++ {
		rows = append(rows, apptypes.Row{Task: apptypes.Task{ID: fmt.Sprintf("c%02d", i), Title: fmt.Sprintf("Complete-%02d", i), Status: apptypes.StatusComplete}})
	}
	m := driveTree(t, rows, 20, 48) // body height = 16

	// Jump to the last complete task so the Complete header is above the window.
	m = pressKey(m, "G", 'G')
	if got := m.SelectedID(); got != "c19" {
		t.Fatalf("after G selection = %q, want c19", got)
	}
	body := ansi.Strip(treeBody(m))

	// The "Complete" header must appear — it should be pinned at the top.
	if !strings.Contains(body, "Complete") {
		t.Errorf("Complete section header not visible (not pinned) after scrolling into Complete:\n%s", body)
	}

	// The pinned header should be the first non-empty line.
	lines := strings.Split(body, "\n")
	firstNonEmpty := ""
	for _, l := range lines {
		stripped := strings.TrimSpace(l)
		if stripped != "" {
			firstNonEmpty = stripped
			break
		}
	}
	if !strings.Contains(firstNonEmpty, "Complete") {
		t.Errorf("first visible line is %q, want it to contain the Complete header (pinned at top):\n%s", firstNonEmpty, body)
	}
}

// TestOverflowSuffixShowsBelow checks that when the cursor is on the first
// task of the Pending section (no tasks above), the pinned header shows only
// "N below" (Commit 5, Part 3).
func TestOverflowSuffixShowsBelow(t *testing.T) {
	rows := nPending(40)
	m := driveTree(t, rows, 20, 48) // body height = 16

	// Scroll down two pages so tasks are hidden above and below.
	m = pressKey(m, "", tea.KeyPgDown)
	m = pressKey(m, "", tea.KeyPgDown)

	body := ansi.Strip(treeBody(m))
	// The pinned header (Pending) should carry an overflow suffix.
	if !strings.Contains(body, "below") {
		t.Errorf("pinned header does not show an overflow suffix:\n%s", body)
	}
}

// TestOverflowSuffixShowsAboveAndBelow checks that when the cursor is in the
// middle of a long section, the pinned header shows both "N above" and
// "N below" joined by " . " (Commit 5, Part 3).
func TestOverflowSuffixShowsAboveAndBelow(t *testing.T) {
	rows := nPending(40)
	m := driveTree(t, rows, 20, 48)

	// Jump to the end then page up to land roughly in the middle.
	m = pressKey(m, "G", 'G')
	m = pressKey(m, "", tea.KeyPgUp)

	body := ansi.Strip(treeBody(m))
	if !strings.Contains(body, "above") {
		t.Errorf("pinned header does not show 'N above':\n%s", body)
	}
	if !strings.Contains(body, "below") {
		t.Errorf("pinned header does not show 'N below':\n%s", body)
	}
}

// TestNoOverflowSuffixWhenSectionFits checks that when the entire section fits
// within the viewport, the pinned header carries no overflow suffix (Commit 5,
// Part 3).
func TestNoOverflowSuffixWhenSectionFits(t *testing.T) {
	rows := nPending(3) // only 3 pending, no complete section
	m := driveTree(t, rows, 20, 48)

	body := ansi.Strip(treeBody(m))
	// The Pending header should be visible but without any overflow suffix.
	if strings.Contains(body, "below") {
		t.Errorf("pinned header should not show 'below' when the section fits:\n%s", body)
	}
	if strings.Contains(body, "above") {
		t.Errorf("pinned header should not show 'above' when the section fits:\n%s", body)
	}
}

// TestPinnedHeaderDoesNotDuplicateWhenAtTop checks that when the section header
// is already at the top of the window (cursor near the top of the section), it
// is not duplicated as a pinned line (Commit 5, Part 2).
func TestPinnedHeaderDoesNotDuplicateWhenAtTop(t *testing.T) {
	rows := nPending(20)
	for i := 0; i < 5; i++ {
		rows = append(rows, apptypes.Row{Task: apptypes.Task{ID: fmt.Sprintf("c%02d", i), Title: fmt.Sprintf("Complete-%02d", i), Status: apptypes.StatusComplete}})
	}
	m := driveTree(t, rows, 20, 48)

	// Selection starts on the first pending task; the Pending header is
	// already at the top of the window. It should appear exactly once.
	body := ansi.Strip(treeBody(m))
	// "Pending (" only matches the section header line, not task rows like
	// "Pending-00" — so it must appear exactly once.
	count := strings.Count(body, "Pending (")
	if count != 1 {
		t.Errorf("Pending header appears %d times, want exactly 1 (no duplicate pin at top):\n%s", count, body)
	}
}

// TestPinAndOverflowWithFilter ensures that under an active filter (which
// renders a flat plan with no section headers), no header is pinned and no
// overflow suffix appears — the header-pinning logic is section-only (Commit
// 5, Part 3).
func TestPinAndOverflowWithFilter(t *testing.T) {
	rows := nPending(30)
	m := driveTree(t, rows, 20, 48)

	u, _ := m.Update(cmds.ActivateFilterMsg{})
	m = u.(Model)
	for _, r := range "Pending-1" {
		m = pressKey(m, string(r), r)
	}
	m = pressKey(m, "", tea.KeyEnter)

	body := ansi.Strip(treeBody(m))
	if strings.Contains(body, "below") {
		t.Errorf("filtered view should not show overflow suffix:\n%s", body)
	}
}
