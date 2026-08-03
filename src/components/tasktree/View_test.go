package tasktree

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
)

// testBg is a concrete background the row renderer seals itself against so a
// width/overflow sweep can render without a theme.
var testBg = color.RGBA{R: 1, G: 1, B: 1, A: 1}

// testFg is a concrete foreground for geometry-only renders.
var testFg = color.RGBA{R: 200, G: 200, B: 200, A: 1}

func intPtr(v int) *int { return &v }

// TestComputeTaskRowColsDropOrder pins the column budget's drop order
// (task-row redesign, docs/plan/task-row-redesign-and-inline-creation.md
// step 1): the progress and status columns are each atomic (full width or
// zero) and progress is shed before status as the table narrows — never the
// reverse, and never as a fragment. The title and checkbox are never dropped:
// the title is always at least one column, and the checkbox keeps its fixed
// width. The details marker widens the status column by two cells when
// present, and sheds with it.
func TestComputeTaskRowColsDropOrder(t *testing.T) {
	const checkbox = 1
	status := "in progress" // 11 runes -> status column = 12 (label + gap)
	progress := "42%"       // 3 runes -> progress column = 4 (label + gap)

	statusFull := len(status) + 1
	statusFullDetails := statusFull + 2 // "🗎 " before the label
	progressFull := len(progress) + 1

	for width := 1; width <= 120; width++ {
		for _, details := range []bool{false, true} {
			cols := computeTaskRowCols(width, checkbox, status, progress, details)

			// checkbox is fixed identity, never shed
			if cols.checkbox != checkbox {
				t.Fatalf("width %d: checkbox = %d, want %d", width, cols.checkbox, checkbox)
			}
			// title is never dropped below one column
			if cols.title < 1 {
				t.Fatalf("width %d: title = %d, want >= 1", width, cols.title)
			}
			// atomic columns: a non-zero column has its full width, never a fragment
			want := statusFull
			if details {
				want = statusFullDetails
			}
			if cols.status != 0 && cols.status != want {
				t.Fatalf("width %d details=%v: status = %d, want 0 or %d", width, details, cols.status, want)
			}
			if cols.progress != 0 && cols.progress != progressFull {
				t.Fatalf("width %d: progress = %d, want 0 or %d", width, cols.progress, progressFull)
			}
			// drop order: progress before status -> a shed status implies a shed progress
			if cols.status == 0 && cols.progress != 0 {
				t.Fatalf("width %d: status shed but progress kept (wrong drop order)", width)
			}
			// no overflow: title+status+progress fit the table budget (checkbox lives
			// in the prefix, not the table budget)
			if cols.title+cols.status+cols.progress > width {
				t.Fatalf("width %d: cols sum %d > table width %d (overflow)", width,
					cols.title+cols.status+cols.progress, width)
			}
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

// TestDetailsIconSitsLeftOfStatus pins the notes marker: a task with notes
// renders 🗎 immediately left of its status label in the right block, and a
// task without notes renders none (docs/DESIGN.md §12 glyph table).
func TestDetailsIconSitsLeftOfStatus(t *testing.T) {
	m := &Model{}
	withNotes := apptypes.Row{Task: apptypes.Task{ID: "1", Title: "has notes", Notes: "lots", Status: apptypes.StatusPending}}
	m.rows = []apptypes.Row{withNotes}

	rendered := ansi.Strip(m.renderRow(withNotes, 60, testBg))
	if !strings.Contains(rendered, "🗎 PENDING") {
		t.Errorf("row with notes must render '🗎 PENDING', got: %q", rendered)
	}

	withoutNotes := apptypes.Row{Task: apptypes.Task{ID: "2", Title: "no notes", Status: apptypes.StatusPending}}
	rendered = ansi.Strip(m.renderRow(withoutNotes, 60, testBg))
	if strings.Contains(rendered, "🗎") {
		t.Errorf("row without notes must not render the details marker, got: %q", rendered)
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
