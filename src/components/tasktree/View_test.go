package tasktree

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
)

// testBg is a concrete background the row renderer seals itself against so a
// width/overflow sweep can render without a theme.
var testBg = color.RGBA{R: 1, G: 1, B: 1, A: 1}

// testFg is a concrete foreground for geometry-only renders.
var testFg = color.RGBA{R: 200, G: 200, B: 200, A: 1}

func intPtr(v int) *int { return &v }

// TestComputeTaskRowColsDropOrder pins the column budget's drop order. The
// progress column and the status+icon right block are each atomic (full width
// or zero, never a fragment), and the right block sheds *before* progress as
// the table narrows: the title floor is what the shedding protects, and a row
// still shows its status through the ◻/◼ glyph, its colour, and its section,
// while the percentage appears nowhere else.
//
// That is the reverse of the order this test pinned when it was written for
// docs/plan/task-row-redesign-and-inline-creation.md step 1, which budgeted
// for overflow alone; docs/DESIGN.md §12 now records the floor-driven rule.
// The title and checkbox are never dropped: the title is always at least one
// column, and the checkbox keeps its fixed width. The status column is a fixed
// statusColWidth and the icon column a fixed detailsColWidth, reserved
// together regardless of notes.
func TestComputeTaskRowColsDropOrder(t *testing.T) {
	const checkbox = 1
	status := "in progress" // any label -> fixed status column, statusColWidth+1
	progress := "42%"       // 3 runes -> progress column = 4 (label + gap)

	statusFull := statusColWidth + 1
	detailsFull := detailsColWidth + 1
	progressFull := len(progress) + 1

	for width := 1; width <= 120; width++ {
		cols := computeTaskRowCols(width, checkbox, status, progress, "")

		// checkbox is fixed identity, never shed
		if cols.checkbox != checkbox {
			t.Fatalf("width %d: checkbox = %d, want %d", width, cols.checkbox, checkbox)
		}
		// title is never dropped below one column
		if cols.title < 1 {
			t.Fatalf("width %d: title = %d, want >= 1", width, cols.title)
		}
		// atomic columns: a non-zero column has its full width, never a fragment
		if cols.status != 0 && cols.status != statusFull {
			t.Fatalf("width %d: status = %d, want 0 or %d", width, cols.status, statusFull)
		}
		// the icon column is reserved whenever the status is, at its fixed width,
		// with or without notes — that is what keeps rows aligned
		if cols.details != 0 && cols.details != detailsFull {
			t.Fatalf("width %d: details = %d, want 0 or %d", width, cols.details, detailsFull)
		}
		// the status and icon columns are one right block: they are present or
		// absent together, never one without the other
		if (cols.status == 0) != (cols.details == 0) {
			t.Fatalf("width %d: status=%d and details=%d must shed together", width, cols.status, cols.details)
		}
		if cols.progress != 0 && cols.progress != progressFull {
			t.Fatalf("width %d: progress = %d, want 0 or %d", width, cols.progress, progressFull)
		}
		// drop order: the right block before progress -> a kept status implies a
		// kept progress, and the percentage outlives the label
		if cols.status != 0 && cols.progress == 0 {
			t.Fatalf("width %d: right block kept but progress shed (wrong drop order)", width)
		}
		// no overflow: title+status+details+progress fit the table budget (checkbox
		// lives in the prefix, not the table budget)
		if cols.title+cols.status+cols.details+cols.progress > width {
			t.Fatalf("width %d: cols sum %d > table width %d (overflow)", width,
				cols.title+cols.status+cols.details+cols.progress, width)
		}
	}
}

// TestRenderTaskRowNeverOverflows sweeps a full card across panel widths and
// asserts the card never exceeds the panel, spans it exactly, keeps the bar
// column and checkbox identity, and — when there is room — ends with the
// right-aligned status label. This is the mechanical backstop for
// docs/DESIGN.md §12's width sweep, extended for the card chrome
// (docs/plan/task-row-cards-and-status.md).
func TestRenderTaskRowNeverOverflows(t *testing.T) {
	m := &Model{}
	title := "Water the ferns before the weekend meeting"
	row := apptypes.Row{Task: apptypes.Task{
		ID: "1", Title: title, Status: apptypes.StatusInProgress,
		ProgressKind: apptypes.ProgressPercentage, ProgressPct: intPtr(63),
	}}
	m.rows = []apptypes.Row{row}
	m.selectedID = "1"
	// The sweep row is claimed: the spinner+agent unit is part of the budget
	// this overflow sweep must exercise (§6 #1 of
	// docs/plan/mcp-server-enhancement.md).
	m.work = map[string]apptypes.AgentActivity{
		"1": {EntityType: "task", EntityID: "1", AgentID: "claude", Kind: "working"},
	}
	m.animFrame = 2
	spinnerUnit := chrome.Spinner(2) + " claude"

	for width := 20; width <= 200; width += 10 {
		rendered := m.renderRow(row, width, testBg, nil)

		if got := lipgloss.Width(rendered); got > width {
			t.Fatalf("width %d: rendered row = %d columns (overflow)", width, got)
		}
		// The card spans the panel exactly: a shorter render would mean the
		// selected-row ModalBg does not cover the full width (the old
		// text-run highlight).
		if got := lipgloss.Width(rendered); got != width {
			t.Fatalf("width %d: rendered row = %d columns, want exactly %d", width, got, width)
		}

		stripped := ansi.Strip(rendered)
		if !strings.HasPrefix(stripped, "▌") {
			t.Fatalf("width %d: bar column missing from card: %q", width, stripped)
		}
		// The sweep row is in progress: its checkbox is the pending square —
		// only complete rows use the filled square.
		if !strings.Contains(stripped, "◻") {
			t.Fatalf("width %d: checkbox lost from row: %q", width, stripped)
		}
		if strings.Contains(stripped, "◼") || strings.Contains(stripped, "✔") {
			t.Fatalf("width %d: in-progress row must not use a complete glyph: %q", width, stripped)
		}
		// At a generous width the whole title survives unmodified.
		if width >= 120 && !strings.Contains(stripped, title) {
			t.Fatalf("width %d: expected full title %q in row: %q", width, title, stripped)
		}
		// When there is room for the right block, the status is the last
		// thing on the content line (right-aligned; the card's right-padding
		// space is trimmed here). The card is a single line now that the
		// vertical padding is gone.
		//
		// The threshold is 50 rather than 40 because this sweep row is
		// claimed: its spinner unit costs nine columns, and with the
		// percentage and the gutter on top, a 40-column card would leave the
		// title under titleFloor — so the label sheds by design (§12). An
		// unclaimed row still keeps its label at 40.
		if width >= 50 && !strings.HasSuffix(strings.TrimRight(stripped, " "), "IN PROGRESS") {
			t.Fatalf("width %d: expected right-aligned IN PROGRESS suffix in: %q", width, stripped)
		}
		// A claimed row shows the full spinner+agent unit un-clipped when there
		// is room — never a fragment.
		if width >= 120 && !strings.Contains(stripped, spinnerUnit) {
			t.Fatalf("width %d: expected claimed spinner unit %q in: %q", width, spinnerUnit, stripped)
		}
	}
}

// TestExpandMarkerSitsAtTitleEnd pins the expanded/collapsed marker (▾/▸) to
// the end of the title: a parent row's title starts at its own depth — the
// marker never occupies a leading column (docs/DESIGN.md §12).
func TestExpandMarkerSitsAtTitleEnd(t *testing.T) {
	m := &Model{collapsed: make(map[string]bool)}
	parent := apptypes.Row{Task: apptypes.Task{ID: "1", Title: "Project"}, HasChildren: true}
	m.rows = []apptypes.Row{parent}
	m.selectedID = "1"

	expanded := ansi.Strip(m.renderRow(parent, 60, testBg, nil))
	if !strings.Contains(expanded, "Project ▾") {
		t.Errorf("expanded row must render 'Project ▾', got: %q", expanded)
	}

	m.collapsed["1"] = true
	collapsed := ansi.Strip(m.renderRow(parent, 60, testBg, nil))
	if !strings.Contains(collapsed, "Project ▸") {
		t.Errorf("collapsed row must render 'Project ▸', got: %q", collapsed)
	}
}

// TestSubtaskCardIsIndented pins the whole-card indent: a depth-1 row's card
// (bar column included) starts two columns right of a depth-0 row, so the
// bars step right and no continuous vertical line forms
// (docs/DESIGN.md §12).
func TestSubtaskCardIsIndented(t *testing.T) {
	m := &Model{}
	root := apptypes.Row{Task: apptypes.Task{ID: "1", Title: "root"}}
	child := apptypes.Row{Task: apptypes.Task{ID: "2", Title: "child", ParentID: strPtr("1")}, Depth: 1}
	m.rows = []apptypes.Row{root, child}

	rootRendered := ansi.Strip(m.renderRow(root, 60, testBg, nil))
	childRendered := ansi.Strip(m.renderRow(child, 60, testBg, nil))

	if !strings.HasPrefix(rootRendered, "▌") {
		t.Fatalf("root card must open with the bar at column 0: %q", rootRendered)
	}
	if !strings.HasPrefix(childRendered, "  ▌") {
		t.Fatalf("subtask card must open with two indent columns then the bar: %q", childRendered)
	}
}

// TestDetailsIconInTrailingColumn pins the notes and comments markers to the
// fixed two-cell trailing icon column (decision 2, docs/plan/ui-improvements.md;
// docs/DESIGN.md §12): the column holds the notes glyph on the left and the
// comments glyph on the right, each one cell, an absent glyph rendered as a
// single space. A noted-only task ends in "PENDING 🗎 " (notes glyph, then a
// blank for the absent comments glyph); a commented-only task ends in
// "PENDING  🗨" (a blank for absent notes, then the comments glyph); a task
// with both ends in "PENDING 🗎🗨"; and a clean task ends in "PENDING" with
// two reserved blank cells after it. The glyph is the row's last visible cell.
func TestDetailsIconInTrailingColumn(t *testing.T) {
	testCases := []struct {
		label         string
		row           apptypes.Row
		wantSuffix    string // right-trimmed last visible cell — the comments glyph or the status
		wantSubstring string // a fixed slice to assert the two-glyph column renders in order
		notWant       string // a glyph that must NOT appear given this row's flags
	}{
		{
			label:         "notes only",
			row:           apptypes.Row{Task: apptypes.Task{ID: "1", Title: "has notes", Notes: "lots", Status: apptypes.StatusPending}},
			wantSubstring: "PENDING 🗎 ", // notes glyph, then blank for absent comments
			wantSuffix:    detailsIcon,  // notes glyph is the last non-blank cell
			notWant:       commentsIcon,
		},
		{
			label:         "comments only",
			row:           apptypes.Row{Task: apptypes.Task{ID: "2", Title: "has comments", Status: apptypes.StatusPending}, HasComments: true},
			wantSubstring: "PENDING  " + commentsIcon, // blank for absent notes, then comments glyph
			wantSuffix:    commentsIcon,
			notWant:       detailsIcon,
		},
		{
			label:         "notes and comments",
			row:           apptypes.Row{Task: apptypes.Task{ID: "3", Title: "both", Notes: "n", Status: apptypes.StatusPending}, HasComments: true},
			wantSubstring: "PENDING 🗎" + commentsIcon,
			wantSuffix:    commentsIcon,
		},
		{
			label:         "neither",
			row:           apptypes.Row{Task: apptypes.Task{ID: "4", Title: "plain", Status: apptypes.StatusPending}},
			wantSubstring: "PENDING   ", // two reserved blank cells
			wantSuffix:    "PENDING",
			notWant:       detailsIcon, // neither glyph may appear
		},
	}

	for _, tc := range testCases {
		m := &Model{}
		m.rows = []apptypes.Row{tc.row}
		m.selectedID = tc.row.Task.ID

		rendered := ansi.Strip(m.renderRow(tc.row, 60, testBg, nil))
		if !strings.Contains(rendered, tc.wantSubstring) {
			t.Errorf("[%s] row must render %q in the trailing column, got: %q", tc.label, tc.wantSubstring, rendered)
		}
		if !strings.HasSuffix(strings.TrimRight(rendered, " "), tc.wantSuffix) {
			t.Errorf("[%s] row must end in %q (last visible cell), got: %q", tc.label, tc.wantSuffix, rendered)
		}
		if tc.notWant != "" && strings.Contains(rendered, tc.notWant) {
			t.Errorf("[%s] row must not render %q, got: %q", tc.label, tc.notWant, rendered)
		}
	}
}

// TestStatusColumnIsFixedWidth pins the fixed status column: rows of varying
// status label length all place the trailing icon column at the same display
// column — so the status label and the two-cell notes+comments glyph column
// line up across rows regardless of the label's length (decision 2,
// docs/plan/ui-improvements.md; docs/DESIGN.md §12). Every row here carries
// notes, so the notes glyph is always present at a fixed column; commented
// rows must additionally place the comments glyph immediately after it, so
// the two share the fixed 2-cell trailing column.
func TestStatusColumnIsFixedWidth(t *testing.T) {
	const width = 60
	m := &Model{}
	rows := []apptypes.Row{
		{Task: apptypes.Task{ID: "1", Title: "a", Notes: "n", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "2", Title: "eleven char", Notes: "n", Status: apptypes.StatusPending}, HasComments: true},
		{Task: apptypes.Task{ID: "3", Title: "b", Notes: "n", Status: apptypes.StatusInProgress}, HasComments: true},
		{Task: apptypes.Task{ID: "4", Title: "c", Notes: "n", Status: apptypes.StatusComplete}},
	}
	m.rows = rows

	// The notes glyph (present on every row) must land at the same display
	// column regardless of the status label's length — that is the fixed
	// trailing column invariant from ui-improvements Commit 4.
	notesCol := -1
	for _, r := range rows {
		stripped := ansi.Strip(m.renderRow(r, width, testBg, nil))
		gi := strings.Index(stripped, detailsIcon)
		if gi < 0 {
			t.Fatalf("row %q: expected a notes glyph in %q", r.Task.Title, stripped)
		}
		col := lipgloss.Width(stripped[:gi])
		if notesCol == -1 {
			notesCol = col
		} else if col != notesCol {
			t.Errorf("notes glyph column = %d for %q (status %v), want %d (fixed trailing column)",
				col, r.Task.Title, r.Task.Status, notesCol)
		}
	}

	// On the commented rows the comments glyph must sit immediately after the
	// notes glyph — both one-cell runes, adjacent at the byte level (each is a
	// 4-byte UTF-8 sequence), so together they occupy the fixed 2-cell column
	// with no gap between them.
	for _, r := range rows {
		if !r.HasComments {
			continue
		}
		stripped := ansi.Strip(m.renderRow(r, width, testBg, nil))
		gi := strings.Index(stripped, detailsIcon)
		ci := strings.Index(stripped, commentsIcon)
		// Both glyphs are single 4-byte UTF-8 runes, so adjacency is gi+4 bytes.
		if ci != gi+len(detailsIcon) {
			t.Errorf("row %q: comments glyph at byte %d is not immediately after notes glyph at %d; rendered: %q",
				r.Task.Title, ci, gi, stripped)
		}
	}
}

// TestStatusLabelCaps pins the all-caps status labels.
func TestStatusLabelCaps(t *testing.T) {
	cases := []struct {
		status apptypes.Status
		want   string
	}{
		{apptypes.StatusPending, "PENDING"},
		{apptypes.StatusInProgress, "IN PROGRESS"},
		{apptypes.StatusComplete, "COMPLETE"},
	}
	for _, c := range cases {
		if got := statusLabel(c.status); got != c.want {
			t.Errorf("statusLabel(%v) = %q, want %q", c.status, got, c.want)
		}
	}
}

// TestStatusFgComesFromTheme pins the status-label colors to the active
// theme's tokens — never a literal hex (docs/plan/task-row-cards-and-status.md).
func TestStatusFgComesFromTheme(t *testing.T) {
	if got := statusFg(apptypes.StatusPending); got != appstyles.Active.TextMuted {
		t.Errorf("pending fg = %v, want TextMuted", got)
	}
	if got := statusFg(apptypes.StatusInProgress); got != appstyles.Active.StatusInProgress {
		t.Errorf("in-progress fg = %v, want StatusInProgress", got)
	}
	if got := statusFg(apptypes.StatusComplete); got != appstyles.Active.StatusComplete {
		t.Errorf("complete fg = %v, want StatusComplete", got)
	}
}

// TestBarFgRule pins the bar-column color rule: accent on the selected row,
// the row's own status color otherwise.
func TestBarFgRule(t *testing.T) {
	for _, s := range []apptypes.Status{apptypes.StatusPending, apptypes.StatusInProgress, apptypes.StatusComplete} {
		if got := barFgFor(s, true); got != appstyles.Active.Accent {
			t.Errorf("selected %v bar = %v, want Accent", s, got)
		}
		if got := barFgFor(s, false); got != statusFg(s) {
			t.Errorf("unselected %v bar = %v, want its status color", s, got)
		}
	}
}

// TestSpinnerFgRule pins the spinner color rule: Accent on the selected row,
// TextDim otherwise (docs/plan/mcp-server-enhancement.md §3.7).
func TestSpinnerFgRule(t *testing.T) {
	if got := spinnerFg(true); got != appstyles.Active.Accent {
		t.Errorf("selected spinner fg = %v, want Accent", got)
	}
	if got := spinnerFg(false); got != appstyles.Active.TextDim {
		t.Errorf("unselected spinner fg = %v, want TextDim", got)
	}
}

// TestRenderRowShowsSpinnerWhenClaimed pins the agent-presence render
// (docs/plan/mcp-server-enhancement.md §3.7): a row whose task is in m.work
// appends the animated spinner glyph for m.animFrame plus the short agent id
// after the status label, and an unclaimed row renders no spinner.
func TestRenderRowShowsSpinnerWhenClaimed(t *testing.T) {
	m := &Model{
		rows: []apptypes.Row{{Task: apptypes.Task{ID: "1", Title: "claimed", Status: apptypes.StatusPending}}},
		work: map[string]apptypes.AgentActivity{
			"1": {EntityType: "task", EntityID: "1", AgentID: "claude", Kind: "working"},
		},
		animFrame: 3,
	}

	claimed := ansi.Strip(m.renderRow(m.rows[0], 80, testBg, nil))
	if !strings.Contains(claimed, chrome.Spinner(3)+" claude") {
		t.Errorf("claimed row must render spinner %q + agent id, got: %q", chrome.Spinner(3), claimed)
	}
	if !strings.Contains(claimed, "PENDING") {
		t.Errorf("claimed row must keep its status label, got: %q", claimed)
	}

	m.work = map[string]apptypes.AgentActivity{}
	free := ansi.Strip(m.renderRow(m.rows[0], 80, testBg, nil))
	if strings.Contains(free, chrome.Spinner(3)) {
		t.Errorf("unclaimed row must render no spinner, got: %q", free)
	}
}

// TestSpinnerUnitShedsAfterStatus pins the drop order with a claimed row: each
// unit is atomic (full width or zero, never a fragment), the status+icon block
// sheds first, then the agent-spinner unit, and the percentage last.
//
// docs/plan/mcp-server-enhancement.md §3.7 ordered these the other way round
// (progress, then spinner, then status) when the only constraint was overflow.
// The title floor reverses it: the label is the cheapest thing to lose and the
// percentage the dearest. The spinner's position relative to status and
// progress is unchanged — it is still the middle one — so §3.7's actual point,
// that the spinner unit is atomic and never clipped, still holds here.
func TestSpinnerUnitShedsAfterStatus(t *testing.T) {
	const checkbox = 1
	status := "IN PROGRESS" // any label -> fixed status column, statusColWidth+1
	progress := "42%"       // 3 runes -> progress column = 4 (label + gap)
	agent := chrome.Spinner(1) + " claude"

	statusFull := statusColWidth + 1
	detailsFull := detailsColWidth + 1
	progressFull := len(progress) + 1
	agentFull := len(agent) + 1

	for width := 1; width <= 120; width++ {
		cols := computeTaskRowCols(width, checkbox, status, progress, agent)

		if cols.progress != 0 && cols.progress != progressFull {
			t.Fatalf("width %d: progress = %d, want 0 or %d", width, cols.progress, progressFull)
		}
		if cols.agentSpinner != 0 && cols.agentSpinner != agentFull {
			t.Fatalf("width %d: agent-spinner = %d, want 0 or %d", width, cols.agentSpinner, agentFull)
		}
		if cols.status != 0 && cols.status != statusFull {
			t.Fatalf("width %d: status = %d, want 0 or %d", width, cols.status, statusFull)
		}
		if cols.details != 0 && cols.details != detailsFull {
			t.Fatalf("width %d: details = %d, want 0 or %d", width, cols.details, detailsFull)
		}
		// Drop order: the status+icon block first, then agent-spinner, then
		// progress. A unit kept while an earlier one is shed is the wrong order —
		// status must not outlive the agent-spinner, nor the agent-spinner the
		// percentage.
		if cols.status != 0 && cols.agentSpinner == 0 {
			t.Fatalf("width %d: status kept but agent-spinner shed (wrong drop order)", width)
		}
		if cols.agentSpinner != 0 && cols.progress == 0 {
			t.Fatalf("width %d: agent-spinner kept but progress shed (wrong drop order)", width)
		}
		if cols.title+cols.progress+cols.agentSpinner+cols.status+cols.details > width {
			t.Fatalf("width %d: cols sum %d > table width %d (overflow)", width,
				cols.title+cols.progress+cols.agentSpinner+cols.status+cols.details, width)
		}
	}
}

// TestRenderCreateRowCard checks the create row's card: it spans the panel
// width, opens with the bar column, shows the placeholder, and no longer
// renders the → prompt (docs/plan/task-row-cards-and-status.md).
func TestRenderCreateRowCard(t *testing.T) {
	m := &Model{}
	m.activeList = true
	m.applyRows(nil) // auto-creating on an empty list

	rendered := m.renderCreateRow(60, testBg)
	if got := lipgloss.Width(rendered); got != 60 {
		t.Fatalf("create card width = %d, want 60", got)
	}
	stripped := ansi.Strip(rendered)
	if !strings.HasPrefix(stripped, "▌") {
		t.Errorf("create card must open with the bar column: %q", stripped)
	}
	if strings.Contains(stripped, "→") {
		t.Errorf("create card must not render the → prompt: %q", stripped)
	}
	if !strings.Contains(stripped, "Add a task") {
		t.Errorf("expected 'Add a task' placeholder in create card, got: %q", stripped)
	}
}

// TestEmptyListCreateRowUnderPending pins where the empty list's create card
// opens: under the Pending header. The input creates a pending task
// (store.CreateTask inserts status 'pending'), so the card belongs to the
// Pending section even while it has nothing in it yet — it must not float at
// the top of the panel with no section above it
// (docs/plan/task-row-cards-and-status.md).
func TestEmptyListCreateRowUnderPending(t *testing.T) {
	m := &Model{}
	m.activeList = true
	m.applyRows(nil) // auto-creating on an empty list

	rendered := ansi.Strip(m.ViewInPanel(60, 24, appstyles.Active.BackgroundPanel))
	iHeader := strings.Index(rendered, "Pending")
	iCard := strings.Index(rendered, "Add a task")
	if iHeader < 0 {
		t.Fatalf("empty list must render the Pending header, got: %q", rendered)
	}
	if iCard < 0 {
		t.Fatalf("empty list must render the create card, got: %q", rendered)
	}
	if iCard < iHeader {
		t.Errorf("create card must render after the Pending header: header@%d card@%d", iHeader, iCard)
	}
	if strings.Contains(rendered, "Complete") {
		t.Errorf("empty list must not render the Complete header, got: %q", rendered)
	}
}

// TestCreateRowRendersAfterAnchorSubtree pins the create-row ghost position
// (bug 5): when the selected task has visible children, the inline create row
// must render AFTER the anchor's last visible descendant — matching the
// committed task's position — not between the anchor and its first child
// ("the new task card appears inside the task with children"). Selecting the
// parent B (with children B1, B2) and pressing n must place the card after
// B2, not between B and B1.
func TestCreateRowRendersAfterAnchorSubtree(t *testing.T) {
	m := &Model{collapsed: make(map[string]bool)}
	bID := strPtr("B")
	m.rows = []apptypes.Row{
		{Task: apptypes.Task{ID: "A", Title: "A", Status: apptypes.StatusPending}, Depth: 0},
		{Task: apptypes.Task{ID: "B", Title: "B", Status: apptypes.StatusPending}, HasChildren: true, Depth: 0},
		{Task: apptypes.Task{ID: "B1", Title: "B1", Status: apptypes.StatusPending, ParentID: bID}, Depth: 1},
		{Task: apptypes.Task{ID: "B2", Title: "B2", Status: apptypes.StatusPending, ParentID: bID}, Depth: 1},
	}
	m.selectedID = "B"
	m.activeList = true
	m.StartCreating("B")

	rendered := ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))

	// lineIndexOf returns the first line number containing needle, or -1.
	lineIndexOf := func(needle string) int {
		for i, line := range strings.Split(rendered, "\n") {
			if strings.Contains(line, needle) {
				return i
			}
		}
		return -1
	}
	iA := lineIndexOf("▌◻ A")
	iB := lineIndexOf("▌◻ B ▾")
	iB1 := lineIndexOf("▌◻ B1")
	iB2 := lineIndexOf("▌◻ B2")
	iCreate := lineIndexOf("▌- Add a task")
	if iA < 0 || iB < 0 || iB1 < 0 || iB2 < 0 || iCreate < 0 {
		t.Fatalf("missing a rendered line (A=%d B=%d B1=%d B2=%d create=%d)\n%s",
			iA, iB, iB1, iB2, iCreate, rendered)
	}
	// Order in the pending section must be A, B, B1, B2; the create card
	// must render AFTER B2 (after the anchor's whole subtree).
	if !(iA < iB && iB < iB1 && iB1 < iB2) {
		t.Errorf("expected A<B<B1<B2 line order, got %d,%d,%d,%d", iA, iB, iB1, iB2)
	}
	if iCreate < iB2 {
		t.Errorf("create card at line %d must render after B's children (B1@%d, B2@%d); "+
			"it renders between the anchor and its first child", iCreate, iB1, iB2)
	}
}

// TestPendingHeaderCountsStatusesNotSectionRows pins the section-header count
// fix (bug: the same list shows two different counts on one screen). A
// completed subtask of a pending parent renders inside the Pending section
// (splitSections' deliberate behaviour, preserved here), but the Pending
// header must count only rows whose own status is not complete — the same
// thing apptypes.ListSummary.PendingCount counts — so the header matches
// what the Lists panel would show for the same list, instead of counting
// every row in the section regardless of status.
func TestPendingHeaderCountsStatusesNotSectionRows(t *testing.T) {
	m := &Model{collapsed: make(map[string]bool)}
	parentID := strPtr("P")
	m.rows = []apptypes.Row{
		{Task: apptypes.Task{ID: "P", Title: "Plan the garden", Status: apptypes.StatusPending}, HasChildren: true, Depth: 0},
		{Task: apptypes.Task{ID: "C1", Title: "Prep the beds", Status: apptypes.StatusComplete, ParentID: parentID}, Depth: 1},
		{Task: apptypes.Task{ID: "C2", Title: "Build the beds", Status: apptypes.StatusComplete, ParentID: parentID}, Depth: 1},
		{Task: apptypes.Task{ID: "C3", Title: "Water the beds", Status: apptypes.StatusPending, ParentID: parentID}, Depth: 1},
	}
	m.activeList = true

	rendered := ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))

	// Statuses: P pending, C1 complete, C2 complete, C3 pending -> 2 not-complete.
	if !strings.Contains(rendered, "Pending (2)") {
		t.Errorf("Pending header must count statuses (2 not-complete of 4 rows), got:\n%s", rendered)
	}
	if strings.Contains(rendered, "Complete") {
		t.Errorf("no row's root status is complete, so no Complete section should render, got:\n%s", rendered)
	}
	for _, title := range []string{"Plan the garden", "Prep the beds", "Build the beds", "Water the beds"} {
		if !strings.Contains(rendered, title) {
			t.Errorf("all four rows must still render inside the Pending section, missing %q:\n%s", title, rendered)
		}
	}
}
