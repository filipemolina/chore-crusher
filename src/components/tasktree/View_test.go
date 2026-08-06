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

// TestComputeTaskRowColsDropOrder pins the column budget's drop order
// (task-row redesign, docs/plan/task-row-redesign-and-inline-creation.md
// step 1): the progress column and the status+icon right block are each atomic
// (full width or zero) and progress is shed before the right block as the table
// narrows — never the reverse, and never as a fragment. The title and checkbox
// are never dropped: the title is always at least one column, and the checkbox
// keeps its fixed width. The status column is a fixed statusColWidth and the
// icon column a fixed detailsColWidth, reserved together regardless of notes.
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
		// drop order: progress before the right block -> a shed status implies a shed progress
		if cols.status == 0 && cols.progress != 0 {
			t.Fatalf("width %d: right block shed but progress kept (wrong drop order)", width)
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
		rendered := m.renderRow(row, width, testBg)

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
		if width >= 40 && !strings.HasSuffix(strings.TrimRight(stripped, " "), "IN PROGRESS") {
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

	expanded := ansi.Strip(m.renderRow(parent, 60, testBg))
	if !strings.Contains(expanded, "Project ▾") {
		t.Errorf("expanded row must render 'Project ▾', got: %q", expanded)
	}

	m.collapsed["1"] = true
	collapsed := ansi.Strip(m.renderRow(parent, 60, testBg))
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

	rootRendered := ansi.Strip(m.renderRow(root, 60, testBg))
	childRendered := ansi.Strip(m.renderRow(child, 60, testBg))

	if !strings.HasPrefix(rootRendered, "▌") {
		t.Fatalf("root card must open with the bar at column 0: %q", rootRendered)
	}
	if !strings.HasPrefix(childRendered, "  ▌") {
		t.Fatalf("subtask card must open with two indent columns then the bar: %q", childRendered)
	}
}

// TestDetailsIconInTrailingColumn pins the notes marker to the fixed trailing
// icon column (decision 2, docs/plan/ui-improvements.md; docs/DESIGN.md §12): a
// noted task ends in "PENDING 🗎" — the glyph is the row's last visible cell,
// right of the status column — while an un-noted task ends in "PENDING"
// followed by the reserved blank icon cell and never renders the glyph.
func TestDetailsIconInTrailingColumn(t *testing.T) {
	m := &Model{}
	withNotes := apptypes.Row{Task: apptypes.Task{ID: "1", Title: "has notes", Notes: "lots", Status: apptypes.StatusPending}}
	m.rows = []apptypes.Row{withNotes}

	rendered := ansi.Strip(m.renderRow(withNotes, 60, testBg))
	if !strings.Contains(rendered, "PENDING 🗎") {
		t.Errorf("noted row must render 'PENDING 🗎' (glyph in the trailing column), got: %q", rendered)
	}
	if !strings.HasSuffix(strings.TrimRight(rendered, " "), "🗎") {
		t.Errorf("noted row's document glyph must be the row's last visible cell, got: %q", rendered)
	}

	withoutNotes := apptypes.Row{Task: apptypes.Task{ID: "2", Title: "no notes", Status: apptypes.StatusPending}}
	rendered = ansi.Strip(m.renderRow(withoutNotes, 60, testBg))
	if strings.Contains(rendered, "🗎") {
		t.Errorf("un-noted row must not render the document glyph, got: %q", rendered)
	}
	if !strings.HasSuffix(strings.TrimRight(rendered, " "), "PENDING") {
		t.Errorf("un-noted row must end in the status label with the icon column left blank, got: %q", rendered)
	}
}

// TestStatusColumnIsFixedWidth pins the fixed status column: a short PENDING
// title, an eleven-character PENDING title, and an IN PROGRESS title rendered at
// the same width all place the document glyph at the same display column — so
// the status label and the trailing icon column line up across rows regardless
// of the label's length (decision 2, docs/plan/ui-improvements.md).
func TestStatusColumnIsFixedWidth(t *testing.T) {
	const width = 60
	m := &Model{}
	rows := []apptypes.Row{
		{Task: apptypes.Task{ID: "1", Title: "a", Notes: "n", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "2", Title: "eleven char", Notes: "n", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "3", Title: "b", Notes: "n", Status: apptypes.StatusInProgress}},
	}
	m.rows = rows

	glyphCol := -1
	for _, r := range rows {
		stripped := ansi.Strip(m.renderRow(r, width, testBg))
		gi := strings.Index(stripped, "🗎")
		if gi < 0 {
			t.Fatalf("row %q: expected a document glyph in %q", r.Task.Title, stripped)
		}
		col := lipgloss.Width(stripped[:gi]) // display column, not byte index
		if glyphCol == -1 {
			glyphCol = col
		} else if col != glyphCol {
			t.Errorf("document glyph column = %d for %q (status %v), want %d (fixed trailing column)",
				col, r.Task.Title, r.Task.Status, glyphCol)
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

	claimed := ansi.Strip(m.renderRow(m.rows[0], 80, testBg))
	if !strings.Contains(claimed, chrome.Spinner(3)+" claude") {
		t.Errorf("claimed row must render spinner %q + agent id, got: %q", chrome.Spinner(3), claimed)
	}
	if !strings.Contains(claimed, "PENDING") {
		t.Errorf("claimed row must keep its status label, got: %q", claimed)
	}

	m.work = map[string]apptypes.AgentActivity{}
	free := ansi.Strip(m.renderRow(m.rows[0], 80, testBg))
	if strings.Contains(free, chrome.Spinner(3)) {
		t.Errorf("unclaimed row must render no spinner, got: %q", free)
	}
}

// TestSpinnerUnitShedsBeforeProgress pins the right-block drop order with a
// claimed row (docs/plan/mcp-server-enhancement.md §3.7, §6): each unit is
// atomic (full width or zero, never a fragment), progress sheds before the
// agent-spinner unit, and the agent-spinner sheds before status.
func TestSpinnerUnitShedsBeforeProgress(t *testing.T) {
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
		// Drop order: progress first, then agent-spinner, then the status+icon
		// block. A unit shed while an earlier one is kept is the wrong order —
		// progress must not survive the agent-spinner, nor the agent-spinner the
		// right block.
		if cols.progress != 0 && cols.agentSpinner == 0 {
			t.Fatalf("width %d: progress kept but agent-spinner shed (wrong drop order)", width)
		}
		if cols.agentSpinner != 0 && cols.status == 0 {
			t.Fatalf("width %d: agent-spinner kept but status shed (wrong drop order)", width)
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
