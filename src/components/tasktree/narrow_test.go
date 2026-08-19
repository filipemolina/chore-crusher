package tasktree

import (
	"strings"
	"testing"

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
		"▌◼ Reach the feature parity milestone                                   45%     ",
		"▌◻ Short one                                                                    ",
		"▌◼ A completed task with a long title                                        🗎  ",
		"▌◻ Parent ▾                                                                     ",
		"  ▌◻ Child of the parent row                                                    ",
	}

	m := narrowModel()
	for i, row := range m.rows {
		got := ansi.Strip(m.renderRow(row, 80, testBg, nil))
		if got != want[i] {
			t.Errorf("row %d at 80 columns changed:\n got %q\nwant %q", i, got, want[i])
		}
	}
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

// TestTitleFloorIsHonouredWhileTheIconColumnIsShown pins the floor itself: a
// row that still shows its trailing icon column must have had at least
// titleFloor columns of title to show it in.
func TestTitleFloorIsHonouredWhileTheIconColumnIsShown(t *testing.T) {
	const checkbox = 1
	for width := 1; width <= 120; width++ {
		cols := computeTaskRowCols(width, checkbox, "45%", "", "", "")
		if cols.details != 0 && cols.title-cols.gutter < titleFloor {
			t.Errorf("table width %d: icon column kept with only %d title columns, want >= %d",
				width, cols.title-cols.gutter, titleFloor)
		}
	}
}
