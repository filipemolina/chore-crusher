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

// TestViewModeNext pins the v-cycle order: All (default) -> Pending ->
// Complete -> All, wrapping indefinitely.
func TestViewModeNext(t *testing.T) {
	cases := []struct {
		from ViewMode
		want ViewMode
	}{
		{ViewAll, ViewPending},
		{ViewPending, ViewComplete},
		{ViewComplete, ViewAll},
	}
	for _, c := range cases {
		if got := c.from.Next(); got != c.want {
			t.Errorf("%v.Next() = %v, want %v", c.from, got, c.want)
		}
	}

	// Three presses from any starting point return to that same mode.
	for _, start := range []ViewMode{ViewAll, ViewPending, ViewComplete} {
		v := start
		for range 3 {
			v = v.Next()
		}
		if v != start {
			t.Errorf("three Next() calls from %v landed on %v, want back at %v", start, v, start)
		}
	}
}

// TestDefaultViewModeIsAll regression-proofs "default is All, not Pending":
// ViewAll must be the zero value so a freshly constructed Model shows both
// sections before any key is pressed. If ViewPending were ever declared
// first in the iota block, this would silently become the default and every
// existing user's tree would go blank on load.
func TestDefaultViewModeIsAll(t *testing.T) {
	m := Model{}
	if m.CurrentView() != ViewAll {
		t.Errorf("zero-value Model.view = %v, want ViewAll", m.CurrentView())
	}
	if got := New().(Model).CurrentView(); got != ViewAll {
		t.Errorf("New().CurrentView() = %v, want ViewAll", got)
	}
}

// viewModeTestRows is a small fixture with two pending root tasks and two
// complete root tasks, used across the splitSections/selection tests below.
func viewModeTestRows() []apptypes.Row {
	return []apptypes.Row{
		{Task: apptypes.Task{ID: "p1", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "p2", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "c1", Status: apptypes.StatusComplete}},
		{Task: apptypes.Task{ID: "c2", Status: apptypes.StatusComplete}},
	}
}

// TestSplitSectionsFiltersByViewMode is the core of the feature: splitSections
// is the single choke point the view mode filters through, so this pins its
// return value directly for all three modes.
func TestSplitSectionsFiltersByViewMode(t *testing.T) {
	m := &Model{collapsed: make(map[string]bool)}
	m.rows = viewModeTestRows()

	m.view = ViewAll
	pending, complete := m.splitSections()
	if got := rowIDs(pending); len(got) != 2 || got[0] != "p1" || got[1] != "p2" {
		t.Errorf("ViewAll pending = %v, want [p1 p2]", got)
	}
	if got := rowIDs(complete); len(got) != 2 || got[0] != "c1" || got[1] != "c2" {
		t.Errorf("ViewAll complete = %v, want [c1 c2]", got)
	}

	m.view = ViewPending
	pending, complete = m.splitSections()
	if got := rowIDs(pending); len(got) != 2 || got[0] != "p1" || got[1] != "p2" {
		t.Errorf("ViewPending pending = %v, want [p1 p2]", got)
	}
	if complete != nil {
		t.Errorf("ViewPending complete = %v, want nil (hidden)", rowIDs(complete))
	}

	m.view = ViewComplete
	pending, complete = m.splitSections()
	if pending != nil {
		t.Errorf("ViewComplete pending = %v, want nil (hidden)", rowIDs(pending))
	}
	if got := rowIDs(complete); len(got) != 2 || got[0] != "c1" || got[1] != "c2" {
		t.Errorf("ViewComplete complete = %v, want [c1 c2]", got)
	}
}

// TestSplitSectionsFilterEdgeCases covers a filtered-out section that was
// already empty (nothing left to hide) and a wholly empty list — neither
// should panic or behave differently from the ViewAll case.
func TestSplitSectionsFilterEdgeCases(t *testing.T) {
	allPending := []apptypes.Row{{Task: apptypes.Task{ID: "p1", Status: apptypes.StatusPending}}}
	allComplete := []apptypes.Row{{Task: apptypes.Task{ID: "c1", Status: apptypes.StatusComplete}}}

	m := &Model{collapsed: make(map[string]bool), view: ViewComplete}
	m.rows = allPending
	pending, complete := m.splitSections()
	if pending != nil || complete != nil {
		t.Errorf("all-pending list in ViewComplete = pending %v complete %v, want nil/nil", rowIDs(pending), rowIDs(complete))
	}

	m.view = ViewPending
	m.rows = allComplete
	pending, complete = m.splitSections()
	if pending != nil || complete != nil {
		t.Errorf("all-complete list in ViewPending = pending %v complete %v, want nil/nil", rowIDs(pending), rowIDs(complete))
	}

	for _, mode := range []ViewMode{ViewAll, ViewPending, ViewComplete} {
		m := &Model{collapsed: make(map[string]bool), view: mode}
		pending, complete := m.splitSections()
		if pending != nil || complete != nil {
			t.Errorf("empty list in mode %v = pending %v complete %v, want nil/nil", mode, rowIDs(pending), rowIDs(complete))
		}
	}
}

// TestSelectionOrderRespectsViewMode proves the cursor walk (selectionOrder,
// which concatenates splitSections' two return values) never offers a row
// from the hidden section.
func TestSelectionOrderRespectsViewMode(t *testing.T) {
	m := &Model{collapsed: make(map[string]bool), view: ViewPending}
	m.rows = viewModeTestRows()
	if got := rowIDs(m.selectionOrder()); len(got) != 2 || got[0] != "p1" || got[1] != "p2" {
		t.Errorf("ViewPending selectionOrder = %v, want [p1 p2]", got)
	}

	m.view = ViewComplete
	if got := rowIDs(m.selectionOrder()); len(got) != 2 || got[0] != "c1" || got[1] != "c2" {
		t.Errorf("ViewComplete selectionOrder = %v, want [c1 c2]", got)
	}
}

// TestMoveSelectionStaysInFilteredSet proves navigation clamps to the
// visible section instead of crossing into the hidden one — the same clamp
// TestNavigationTracksSectionsSeparately pins for the unfiltered walk, now
// under a view mode that removes one whole side of the boundary.
func TestMoveSelectionStaysInFilteredSet(t *testing.T) {
	m := &Model{collapsed: make(map[string]bool), view: ViewPending}
	m.rows = viewModeTestRows()
	m.selectedID = "p1"

	m.moveSelection(1)
	if m.selectedID != "p2" {
		t.Errorf("down from p1 in ViewPending = %q, want p2", m.selectedID)
	}
	// Past the last pending row: clamps, never lands on a complete task even
	// though c1/c2 exist in m.rows.
	m.moveSelection(1)
	if m.selectedID != "p2" {
		t.Errorf("down past last pending in ViewPending = %q, want p2 (clamped, not c1)", m.selectedID)
	}

	m.view = ViewComplete
	m.selectedID = "c2"
	m.moveSelection(-1)
	if m.selectedID != "c1" {
		t.Errorf("up from c2 in ViewComplete = %q, want c1", m.selectedID)
	}
	m.moveSelection(-1)
	if m.selectedID != "c1" {
		t.Errorf("up past first complete in ViewComplete = %q, want c1 (clamped, not p2)", m.selectedID)
	}
}

// TestMoveToFirstAndLastRespectViewMode proves g/G land inside the visible
// section only.
func TestMoveToFirstAndLastRespectViewMode(t *testing.T) {
	m := &Model{collapsed: make(map[string]bool), view: ViewPending}
	m.rows = viewModeTestRows()

	m.moveToFirst()
	if m.selectedID != "p1" {
		t.Errorf("moveToFirst in ViewPending = %q, want p1", m.selectedID)
	}
	m.moveToLast()
	if m.selectedID != "p2" {
		t.Errorf("moveToLast in ViewPending = %q, want p2 (not c2)", m.selectedID)
	}

	m.view = ViewComplete
	m.moveToFirst()
	if m.selectedID != "c1" {
		t.Errorf("moveToFirst in ViewComplete = %q, want c1 (not p1)", m.selectedID)
	}
	m.moveToLast()
	if m.selectedID != "c2" {
		t.Errorf("moveToLast in ViewComplete = %q, want c2", m.selectedID)
	}
}

// TestViewModeHidesSectionHeader proves the render side of the single choke
// point: a section hidden by the view mode drops its header the same way an
// already-empty section's header is omitted (TestPendingHeaderCountsStatusesNotSectionRows).
func TestViewModeHidesSectionHeader(t *testing.T) {
	m := &Model{collapsed: make(map[string]bool), view: ViewPending}
	m.rows = viewModeTestRows()
	m.activeList = true

	rendered := ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))
	if !strings.Contains(rendered, "Pending") {
		t.Errorf("ViewPending must still render the Pending header, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "Complete") {
		t.Errorf("ViewPending must hide the Complete header, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "c1") || strings.Contains(rendered, "c2") {
		t.Errorf("ViewPending must hide complete rows, got:\n%s", rendered)
	}

	m.view = ViewComplete
	rendered = ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))
	if strings.Contains(rendered, "Pending") {
		t.Errorf("ViewComplete must hide the Pending header, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Complete") {
		t.Errorf("ViewComplete must still render the Complete header, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "p1") || strings.Contains(rendered, "p2") {
		t.Errorf("ViewComplete must hide pending rows, got:\n%s", rendered)
	}
}

// TestViewKeyUpdatesModeAndEmitsHeaderMsg pins the key handler: pressing v
// three times cycles m.view through All -> Pending -> Complete -> All,
// returning the cmds.SetTaskTreeViewMsg the header listens for each time.
func TestViewKeyUpdatesModeAndEmitsHeaderMsg(t *testing.T) {
	m := Model{collapsed: make(map[string]bool), focused: true}
	m.rows = viewModeTestRows()
	m.selectedID = "p1"

	wantModes := []ViewMode{ViewPending, ViewComplete, ViewAll}
	wantStrings := []string{"pending", "complete", "all"}

	for i, want := range wantModes {
		updated, cmd := m.Update(tea.KeyPressMsg{Text: "v", Code: 'v'})
		tm := updated.(Model)
		if tm.CurrentView() != want {
			t.Fatalf("press %d: view = %v, want %v", i+1, tm.CurrentView(), want)
		}
		if cmd == nil {
			t.Fatalf("press %d returned a nil cmd, want cmds.SetTaskTreeView", i+1)
		}
		msg := cmd()
		setMsg, ok := msg.(cmds.SetTaskTreeViewMsg)
		if !ok {
			t.Fatalf("press %d's cmd produced %T, want cmds.SetTaskTreeViewMsg", i+1, msg)
		}
		if setMsg.View != wantStrings[i] {
			t.Errorf("press %d: SetTaskTreeViewMsg.View = %q, want %q", i+1, setMsg.View, wantStrings[i])
		}
		m = tm
	}
}
