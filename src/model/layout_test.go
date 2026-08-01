package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-completer/src/appstyles"
	"github.com/filipemolina/chore-completer/src/config"
	"github.com/filipemolina/chore-completer/src/constants"
)

// drive feeds messages through the model the way the Bubble Tea loop would,
// discarding the commands. Layout is driven entirely by the messages listed
// here, so a test can reproduce any startup or resize ordering.
func drive(m tea.Model, msgs ...tea.Msg) AppModel {
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m.(AppModel)
}

// startup replays the startup sequence: the window size message first, then
// the layout broadcast the runtime would route back into the model.
func startup(width, height int) AppModel {
	m := drive(GetInitialModel(nil, config0()), tea.WindowSizeMsg{Width: width, Height: height})
	return drive(m, m.bodyLayout)
}

// config0 is the zero config; GetInitialModel needs one but these tests do
// not exercise the poll interval.
func config0() config.Config { return config.Config{} }

// The three-zone layout (docs/DESIGN.md §5) must always add up to the
// terminal width exactly — ListsWidth + gutter + MainWidth == width — and
// the main panel's vertical split must add up to the height.
func TestBodyLayoutFillsTerminalExactly(t *testing.T) {
	sizes := []struct{ width, height int }{
		{120, 40},
		{80, 24},
		{200, 60},
		{64, 20}, // narrower than two minimum panels: split evenly
		{300, 80},
	}

	for _, size := range sizes {
		layout := startup(size.width, size.height).bodyLayout
		if got := layout.ListsWidth + constants.BODY_GUTTER_WIDTH + layout.MainWidth; got != size.width {
			t.Errorf("%dx%d: ListsWidth+gutter+MainWidth = %d, want %d", size.width, size.height, got, size.width)
		}
		if got := layout.TreeHeight + layout.InputHeight; got != size.height {
			t.Errorf("%dx%d: TreeHeight+InputHeight = %d, want %d", size.width, size.height, got, size.height)
		}
	}
}

// Both visible panels are held at MIN_PANEL_WIDTH where the terminal allows
// it; below that the row is split evenly (docs/DESIGN.md §5).
// Width must be at least 2*MIN_PANEL_WIDTH + GUTTER to hold both at minimum.
func TestBodyLayoutFloorsPanels(t *testing.T) {
	minRequiredWidth := 2*constants.MIN_PANEL_WIDTH + constants.BODY_GUTTER_WIDTH

	// Widths below minimum should split evenly.
	for _, width := range []int{60} { // 60 < 62 (min required)
		layout := startup(width, 40).bodyLayout
		if layout.ListsWidth > 0 && layout.MainWidth > 0 {
			// Both panels are visible but narrow; they should be roughly equal.
			diff := layout.ListsWidth - layout.MainWidth
			if diff < -1 || diff > 1 {
				t.Errorf("width %d: panels not split evenly (lists=%d, main=%d)", width, layout.ListsWidth, layout.MainWidth)
			}
		}
	}

	// Widths above minimum should both be at MIN_PANEL_WIDTH or higher.
	for _, width := range []int{80, 120, 300} {
		if width < minRequiredWidth {
			t.Skipf("width %d is below minimum required (%d)", width, minRequiredWidth)
		}
		layout := startup(width, 40).bodyLayout
		if layout.ListsWidth < constants.MIN_PANEL_WIDTH {
			t.Errorf("width %d: ListsWidth = %d, want ≥ MIN_PANEL_WIDTH", width, layout.ListsWidth)
		}
		if layout.MainWidth < constants.MIN_PANEL_WIDTH {
			t.Errorf("width %d: MainWidth = %d, want ≥ MIN_PANEL_WIDTH", width, layout.MainWidth)
		}
	}
}

// Hiding the lists panel gives the whole row to the main panel and zero to
// the lists panel — and the panel leaves the layout without crashing at
// width 0.
func TestHiddenListsPanelGivesRowToMain(t *testing.T) {
	m := startup(120, 40)
	m.listsPanelVisible = false
	m.bodyLayout = m.calculateBodyLayout()

	if m.bodyLayout.ListsWidth != 0 {
		t.Errorf("hidden lists panel: ListsWidth = %d, want 0", m.bodyLayout.ListsWidth)
	}
	if m.bodyLayout.MainWidth != 120 {
		t.Errorf("hidden lists panel: MainWidth = %d, want 120", m.bodyLayout.MainWidth)
	}
}

// A terminal too narrow for any sidebar (guttered < MIN_PANEL_WIDTH)
// gives the whole row to the main panel rather than splitting into degenerate panels.
// This requires total width < MIN_PANEL_WIDTH + GUTTER.
func TestTooNarrowTerminalYieldsSidebar(t *testing.T) {
	maxWidth := constants.MIN_PANEL_WIDTH + constants.BODY_GUTTER_WIDTH - 1 // 31
	m := startup(maxWidth, 24)
	layout := m.bodyLayout
	if layout.ListsWidth != 0 {
		t.Errorf("%d cols: ListsWidth = %d, want 0 (too narrow for a sidebar)", maxWidth, layout.ListsWidth)
	}
	if layout.MainWidth != maxWidth {
		t.Errorf("%d cols: MainWidth = %d, want %d", maxWidth, layout.MainWidth, maxWidth)
	}
}

// The phase-3 verification: resizing never produces a background-bleed line.
// Render the full screen at several sizes, both panels visible and the
// lists panel hidden, and assert the finished frame is sealed.
func TestRenderedFrameHasNoBackgroundBleed(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 40}, {200, 60}} {
		for _, hidden := range []bool{false, true} {
			m := startup(size.width, size.height)
			m.listsPanelVisible = !hidden
			m.bodyLayout = m.calculateBodyLayout()

			rendered := m.View().Content
			if appstyles.HasBackgroundBleed(rendered) {
				t.Errorf("%dx%d hidden=%v: rendered frame has background bleed:\n%q",
					size.width, size.height, hidden, firstBleedLine(rendered))
			}
		}
	}
}

func firstBleedLine(block string) string {
	for _, line := range strings.Split(block, "\n") {
		if line != "" {
			return line
		}
	}
	return ""
}

// The layout broadcast and the SetFocusMsg both reach every zone, so the
// task tree learns it is focused on startup.
func TestTaskTreeStartsFocused(t *testing.T) {
	m := startup(120, 40)
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Errorf("focused zone = %d, want COMPONENT_TASK_TREE", m.focusedZone)
	}
}

// Tab moves focus through the computed cycle; hiding the lists panel removes
// it from the cycle, and hiding it while focused falls back to the tree.
func TestChangeFocusFollowsComputedCycle(t *testing.T) {
	m := startup(120, 40)

	// tree -> lists -> add input -> tree
	m.focusedZone = constants.COMPONENT_TASK_TREE
	m.ChangeFocus(1)
	if m.focusedZone != constants.COMPONENT_LISTS_PANEL {
		t.Errorf("tab from tree: got zone %d, want lists panel", m.focusedZone)
	}
	m.ChangeFocus(1)
	if m.focusedZone != constants.COMPONENT_ADD_INPUT {
		t.Errorf("tab from lists: got zone %d, want add input", m.focusedZone)
	}
	m.ChangeFocus(1)
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Errorf("tab from add input: got zone %d, want tree", m.focusedZone)
	}

	// With the lists panel hidden the cycle is tree -> add input.
	m.listsPanelVisible = false
	m.ChangeFocus(1)
	if m.focusedZone != constants.COMPONENT_ADD_INPUT {
		t.Errorf("hidden lists: tab from tree got zone %d, want add input", m.focusedZone)
	}
}
