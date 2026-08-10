package tasktree

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/apptypes"
)

// narrowRows is the fixture the width tests share: an in-progress row with a
// percentage and a title long enough to ellipsise, a short pending row, a
// complete row carrying notes, and a parent/child pair for the depth indent.
func narrowRows() []apptypes.Row {
	pct := 45
	return []apptypes.Row{
		{Task: apptypes.Task{ID: "a", Title: "Reach the feature parity milestone", Status: apptypes.StatusInProgress, ProgressKind: apptypes.ProgressPercentage, ProgressPct: &pct}},
		{Task: apptypes.Task{ID: "b", Title: "Short one", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "c", Title: "A completed task with a long title", Notes: "n", Status: apptypes.StatusComplete}},
		{Task: apptypes.Task{ID: "d", Title: "Parent", Status: apptypes.StatusPending}, HasChildren: true},
		{Task: apptypes.Task{ID: "e", Title: "Child of the parent row", Status: apptypes.StatusPending}, Depth: 1},
	}
}

func narrowModel() *Model {
	m := &Model{collapsed: map[string]bool{}}
	m.rows = narrowRows()
	m.selectedID = "a"
	return m
}

// TestEightyColumnRowLayoutIsUnchanged is the regression guard for the
// narrow-width work: 80 columns is the width the row layout was tuned at, and
// nothing done for the small-terminal cases may move a cell here. The literals
// are the rendering as it stood before that work, captured cell for cell.
func TestEightyColumnRowLayoutIsUnchanged(t *testing.T) {
	want := []string{
		"▌◻ Reach the feature parity milestone                       45%  IN PROGRESS    ",
		"▌◻ Short one                                                         PENDING    ",
		"▌◼ A completed task with a long title                               COMPLETE 🗎  ",
		"▌◻ Parent ▾                                                          PENDING    ",
		"  ▌◻ Child of the parent row                                         PENDING    ",
	}

	m := narrowModel()
	for i, row := range m.rows {
		got := ansi.Strip(m.renderRow(row, 80, testBg, nil))
		if got != want[i] {
			t.Errorf("row %d at 80 columns changed:\n got %q\nwant %q", i, got, want[i])
		}
	}
}

// statusEndCol returns the display column the row's status label ends at, or
// -1 when the row carries no label. The label is right-aligned inside a fixed
// column, so labels of different lengths start at different columns by design
// — the end is the offset every row shares.
func statusEndCol(stripped string) int {
	for _, label := range []string{"IN PROGRESS", "PENDING", "COMPLETE"} {
		if i := strings.Index(stripped, label); i >= 0 {
			return lipgloss.Width(stripped[:i]) + lipgloss.Width(label)
		}
	}
	return -1
}

// TestNarrowRowsKeepTitleOffTheStatusGroup covers defect 1: at 40 columns the
// ellipsised title ran straight into the percentage ("Reach the fe…45%") with
// no separator. Every width from the minimum up must keep at least one blank
// cell between the title and whatever follows it.
func TestNarrowRowsKeepTitleOffTheStatusGroup(t *testing.T) {
	m := narrowModel()

	for width := 20; width <= 120; width++ {
		for _, row := range m.rows {
			stripped := strings.TrimRight(ansi.Strip(m.renderRow(row, width, testBg, nil)), " ")
			// The title is ellipsised with "…" only when it did not fit; when
			// it is, the very next cell must be a space, never a digit or a
			// status letter.
			if i := strings.Index(stripped, "…"); i >= 0 {
				rest := stripped[i+len("…"):]
				if rest != "" && !strings.HasPrefix(rest, " ") {
					t.Errorf("width %d, row %q: ellipsised title touches what follows it: %q",
						width, row.Task.Title, stripped)
				}
			}
		}
	}
}

// TestStatusGroupEndsAtOneColumnForEveryRow covers the second half of defect
// 1: at 40 columns the in-progress row's status group overflowed past the
// right edge every other row aligned to, so no two rows ended at the same
// place. Rows at the same depth must end their status label at the same
// column, at every width.
func TestStatusGroupEndsAtOneColumnForEveryRow(t *testing.T) {
	m := narrowModel()

	for width := 20; width <= 120; width++ {
		col := -1
		for _, row := range m.rows {
			if row.Depth != 0 {
				continue // an indented card is offset by design
			}
			stripped := ansi.Strip(m.renderRow(row, width, testBg, nil))
			got := statusEndCol(stripped)
			if got < 0 {
				continue // this row shed its label at this width
			}
			if col == -1 {
				col = got
			} else if got != col {
				t.Errorf("width %d: row %q ends its status label at column %d, want %d",
					width, row.Task.Title, got, col)
			}
		}
	}
}

// TestStatusLabelShedsBeforeTheTitleFloor covers defect 2: once the reserved
// right-hand cells would squeeze the title below titleFloor columns, the
// status label (and the icon column it is bracketed with) drops. The checkbox
// glyph and the percentage never drop for it — status is still carried by the
// glyph, the row colour, and the section the row sits in, whereas the
// percentage appears nowhere else on the row.
func TestStatusLabelShedsBeforeTheTitleFloor(t *testing.T) {
	m := narrowModel()
	inProgress := m.rows[0]

	shedAt := -1
	for width := 60; width >= 20; width-- {
		stripped := ansi.Strip(m.renderRow(inProgress, width, testBg, nil))
		if !strings.Contains(stripped, "IN PROGRESS") {
			shedAt = width
			break
		}
	}
	if shedAt < 0 {
		t.Fatal("the status label never shed between 60 and 20 columns")
	}

	stripped := ansi.Strip(m.renderRow(inProgress, shedAt, testBg, nil))
	if !strings.Contains(stripped, "◻") {
		t.Errorf("checkbox glyph dropped with the status label at %d columns: %q", shedAt, stripped)
	}
	if !strings.Contains(stripped, "45%") {
		t.Errorf("percentage dropped with the status label at %d columns: %q", shedAt, stripped)
	}
}

// TestTitleFloorIsHonouredWhileTheStatusLabelIsShown pins the floor itself: a
// row that still shows its status label must have had at least titleFloor
// columns of title to show it in.
func TestTitleFloorIsHonouredWhileTheStatusLabelIsShown(t *testing.T) {
	const checkbox = 1
	for width := 1; width <= 120; width++ {
		cols := computeTaskRowCols(width, checkbox, "IN PROGRESS", "45%", "", "", "")
		if cols.status != 0 && cols.title-cols.gutter < titleFloor {
			t.Errorf("table width %d: status label kept with only %d title columns, want >= %d",
				width, cols.title-cols.gutter, titleFloor)
		}
	}
}
