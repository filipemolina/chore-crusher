package tasktree

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// testBg is a concrete background the row renderer seals itself against so a
// width/overflow sweep can render without a theme.
var testBg = color.RGBA{R: 1, G: 1, B: 1, A: 1}

// TestComputeTaskRowColsDropOrder pins the column budget's drop order
// (task-row redesign, docs/plan/task-row-redesign-and-inline-creation.md
// step 1): the progress and status columns are each atomic (full width or
// zero) and progress is shed before status as the table narrows — never the
// reverse, and never as a fragment. The title and checkbox are never dropped:
// the title is always at least one column, and the checkbox keeps its fixed
// width.
func TestComputeTaskRowColsDropOrder(t *testing.T) {
	const checkbox = 3
	status := "in progress" // 11 runes -> status column = 12 (label + gap)
	progress := "42%"       // 3 runes -> progress column = 4 (label + gap)

	statusFull := len(status) + 1
	progressFull := len(progress) + 1

	for width := 1; width <= 120; width++ {
		cols := computeTaskRowCols(width, checkbox, status, progress)

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

// TestRenderTaskRowNeverOverflows sweeps a full row across panel widths and
// asserts the rendered row never exceeds the panel, the checkbox identity is
// always present, and — when there is room — the full title is readable.
// This is the mechanical backstop for docs/DESIGN.md §12's width sweep.
func TestRenderTaskRowNeverOverflows(t *testing.T) {
	m := &Model{}
	title := "Water the ferns before the weekend meeting"
	status := "in progress"
	progress := "63%"

	for width := 20; width <= 200; width += 10 {
		rendered := m.renderTaskRowBase("", "▾", "[ ]", title, status, progress, 3, width, testBg, false)

		if got := lipgloss.Width(rendered); got > width {
			t.Fatalf("width %d: rendered row = %d columns (overflow)", width, got)
		}

		stripped := ansi.Strip(rendered)
		// The checkbox is the row's identity and is never shed.
		if !strings.Contains(stripped, "[ ]") {
			t.Fatalf("width %d: checkbox lost from row: %q", width, stripped)
		}

		// At a generous width the whole title survives unmodified.
		if width >= 120 && !strings.Contains(stripped, title) {
			t.Fatalf("width %d: expected full title %q in row: %q", width, title, stripped)
		}
	}
}
