package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/config"
	"github.com/filipemolina/chore-crusher/src/constants"
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

// The two-surface layout (docs/DESIGN.md §5) must always add up to the
// terminal width exactly — ListsWidth + gutter + MainWidth == width (when
// Lists is visible) — and expose one title-adjusted Tasks body height.
func TestBodyLayoutFillsTerminalExactly(t *testing.T) {
	sizes := []struct{ width, height int }{
		{120, 40},
		{80, 24},
		{200, 60},
		{64, 20}, // narrower than two minimum panels: split evenly
		{300, 80},
	}

	for _, size := range sizes {
		m := startup(size.width, size.height)
		m.listsPanelVisible = true
		m.bodyLayout = m.calculateBodyLayout()
		layout := m.bodyLayout
		if got := layout.ListsWidth + constants.BODY_GUTTER_WIDTH + layout.MainWidth; got != size.width {
			t.Errorf("%dx%d: ListsWidth+gutter+MainWidth = %d, want %d", size.width, size.height, got, size.width)
		}
		bodyHeight := size.height - constants.HEADER_HEIGHT - constants.FOOTER_HEIGHT
		if got := layout.Height; got != bodyHeight {
			t.Errorf("%dx%d: body height = %d, want %d", size.width, size.height, got, bodyHeight)
		}
		if got := chrome.PanelBodyHeight(layout.Height); got < 0 {
			t.Errorf("%dx%d: Tasks inner height = %d, want non-negative", size.width, size.height, got)
		}
	}
}

// Both visible panels are held at MIN_PANEL_WIDTH where the terminal allows
// it; below that the row is split evenly (docs/DESIGN.md §5).
// Width must be at least 2*MIN_PANEL_WIDTH + GUTTER to hold both at minimum.
func TestBodyLayoutFloorsPanels(t *testing.T) {
	minRequiredWidth := 2*constants.MIN_PANEL_WIDTH + constants.BODY_GUTTER_WIDTH

	// Widths below minimum should split evenly (when lists panel is visible).
	for _, width := range []int{60} { // 60 < 62 (min required)
		m := startup(width, 40)
		m.listsPanelVisible = true
		m.bodyLayout = m.calculateBodyLayout()
		layout := m.bodyLayout
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
		m := startup(width, 40)
		m.listsPanelVisible = true
		m.bodyLayout = m.calculateBodyLayout()
		layout := m.bodyLayout
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

// The task tree is the startup focus zone and must be functionally focused
// from the first frame — its keys would be ignored at launch if Init never
// broadcast the initial focus (the zone's flag reads false until the first
// ctrl+arrow). This pins that Init's command batch carries the broadcast.
func TestInitBroadcastsTaskTreeFocus(t *testing.T) {
	// Init runs its RefreshLists command against the real store, so give the
	// model one (newTestModel) rather than nil.
	m := newTestModel(t, t.TempDir())
	var msgs []tea.Msg
	expandBatch(m.Init(), &msgs)
	for _, msg := range msgs {
		if sf, ok := msg.(cmds.SetFocusMsg); ok && int(sf) == constants.COMPONENT_TASK_TREE {
			return
		}
	}
	t.Errorf("Init command batch does not broadcast SetFocus(TASK_TREE)")
}

// expandBatch flattens a command (expanding tea.BatchMsg into its parts) into
// the list of messages it would deliver to the loop.
func expandBatch(cmd tea.Cmd, acc *[]tea.Msg) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			expandBatch(c, acc)
		}
		return
	}
	*acc = append(*acc, msg)
}

// The layout broadcast and the SetFocusMsg both reach every zone, so the
// task tree learns it is focused on startup.
func TestTaskTreeStartsFocused(t *testing.T) {
	m := startup(120, 40)
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Errorf("focused zone = %d, want COMPONENT_TASK_TREE", m.focusedZone)
	}
}

// L opens the hidden Lists panel and moves focus to it, so its list keys work
// immediately without requiring a separate tab keypress.
func TestToggleListsPanelFocusesLists(t *testing.T) {
	m := startup(120, 40)

	m = refresh(t, m, tea.KeyPressMsg{Text: "L", Code: 'L'})

	if !m.listsPanelVisible {
		t.Fatal("lists panel is hidden after L")
	}
	if m.focusedZone != constants.COMPONENT_LISTS_PANEL {
		t.Errorf("focused zone after L = %d, want lists panel (%d)", m.focusedZone, constants.COMPONENT_LISTS_PANEL)
	}
}

// tab/shift+tab move focus through the computed cycle: the task tree and,
// when visible, the lists panel. With the lists panel hidden the cycle is a
// single zone, so cycling is a no-op rather than landing on an invisible
// panel (docs/DESIGN.md §5). There is no add-input zone anymore: inline
// creation lives inside the tree and takes keystrokes via OwnsKeyboard.
func TestChangeFocusFollowsComputedCycle(t *testing.T) {
	m := startup(120, 40)

	// Hidden lists: only the task tree is focusable, so cycling stays put.
	m.listsPanelVisible = false
	m.focusedZone = constants.COMPONENT_TASK_TREE
	m.ChangeFocus(1)
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Errorf("tab from tree (hidden lists): got zone %d, want task tree", m.focusedZone)
	}
	m.ChangeFocus(-1)
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Errorf("shift+tab from tree (hidden lists): got zone %d, want task tree", m.focusedZone)
	}

	// Visible lists: tree <-> lists.
	m.listsPanelVisible = true
	m.focusedZone = constants.COMPONENT_TASK_TREE
	m.ChangeFocus(1)
	if m.focusedZone != constants.COMPONENT_LISTS_PANEL {
		t.Errorf("tab from tree: got zone %d, want lists panel", m.focusedZone)
	}
	m.ChangeFocus(-1)
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Errorf("shift+tab from lists: got zone %d, want task tree", m.focusedZone)
	}
}
