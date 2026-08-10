package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/constants"
)

// The startup width policy (docs/DESIGN.md §5): Lists auto-shows at
// AUTO_SHOW_LISTS_MIN_WIDTH or wider and starts hidden below it, and either
// way the first focus is Tasks — showing Lists by width never steals focus.
func TestListsStartupPolicyByWidth(t *testing.T) {
	for _, tc := range []struct {
		width       int
		wantVisible bool
	}{
		{80, false},
		{119, false},
		{120, true},
		{160, true},
	} {
		m := startup(tc.width, 40)
		if m.listsPanelVisible != tc.wantVisible {
			t.Errorf("width %d: listsPanelVisible = %v, want %v", tc.width, m.listsPanelVisible, tc.wantVisible)
		}
		if m.focusedZone != constants.COMPONENT_TASK_TREE {
			t.Errorf("width %d: startup focus = %d, want Tasks", tc.width, m.focusedZone)
		}
	}
}

// A resize too narrow for any sidebar drops the rendered Lists panel and pulls
// focus back to Tasks without discarding the preference, so widening restores
// the panel.
func TestNarrowResizeDropsListsAndRestores(t *testing.T) {
	m := startup(160, 40) // Lists auto-shown
	if !m.listsPanelRendered() {
		t.Fatal("expected Lists rendered at 160 columns")
	}
	m = drive(m, tea.KeyPressMsg{Code: tea.KeyTab}) // focus Lists
	if m.focusedZone != constants.COMPONENT_LISTS_PANEL {
		t.Fatalf("tab did not focus Lists: zone = %d", m.focusedZone)
	}

	// Shrink below MIN_PANEL_WIDTH+gutter: no sidebar can render.
	m = drive(m, tea.WindowSizeMsg{Width: 31, Height: 40})
	if m.listsPanelRendered() {
		t.Error("Lists still rendered at 31 columns")
	}
	if m.bodyLayout.ListsWidth != 0 {
		t.Errorf("ListsWidth = %d at 31 columns, want 0", m.bodyLayout.ListsWidth)
	}
	if m.bodyLayout.MainWidth != 31 {
		t.Errorf("MainWidth = %d, want the full 31 columns", m.bodyLayout.MainWidth)
	}
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Error("focus not pulled to Tasks when Lists became unavailable")
	}
	if !m.listsPanelVisible {
		t.Error("stored Lists preference must survive a too-narrow resize")
	}

	// Widen again: the preference is still on, so Lists returns (focus stays Tasks).
	m = drive(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	if !m.listsPanelRendered() {
		t.Error("Lists did not return after widening with the preference on")
	}
	if m.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Error("focus should stay Tasks after Lists is restored by width")
	}
}

// L is the sole per-session override: toggling it off stays off across a later
// resize, and toggling it on while too narrow records the preference but never
// renders (or focuses) a zero-width panel until there is room.
func TestListsToggleOverridesWidthPolicy(t *testing.T) {
	// Off stays off after widening.
	m := startup(160, 40) // on by policy
	m = drive(m, tea.KeyPressMsg{Text: "L", Code: 'L'})
	if m.listsPanelVisible {
		t.Fatal("L did not hide the auto-shown Lists panel")
	}
	m = drive(m, tea.WindowSizeMsg{Width: 200, Height: 40})
	if m.listsPanelVisible {
		t.Error("a resize re-showed Lists against the user's L toggle")
	}

	// On while narrow: preference set, but not rendered or focused until space.
	n := startup(31, 40) // hidden by policy and too narrow to render
	n = drive(n, tea.KeyPressMsg{Text: "L", Code: 'L'})
	if !n.listsPanelVisible {
		t.Fatal("L did not set the Lists preference on")
	}
	if n.listsPanelRendered() {
		t.Error("Lists rendered at 31 columns despite no space")
	}
	if n.focusedZone != constants.COMPONENT_TASK_TREE {
		t.Error("focus landed on an unrendered Lists panel")
	}
	n = drive(n, tea.WindowSizeMsg{Width: 160, Height: 40})
	if !n.listsPanelRendered() {
		t.Error("Lists did not render once widened with the preference on")
	}
}

// The rendered frame stays sealed across both policy outcomes and the narrow
// transition, so no resize leaks the terminal background.
func TestResponsiveResizeHasNoBackgroundBleed(t *testing.T) {
	m := startup(160, 40)
	for _, w := range []int{160, 119, 31, 80, 120} {
		m = drive(m, tea.WindowSizeMsg{Width: w, Height: 40})
		if appstyles.HasBackgroundBleed(m.View().Content) {
			t.Errorf("width %d: rendered frame has background bleed", w)
		}
	}
}
