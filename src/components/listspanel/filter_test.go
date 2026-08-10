package listspanel

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
)

// deliverAsync executes cmd (nil, a single leaf, or a tea.Batch of both) and
// feeds back into m any resulting message that resolves within a short
// window, recursing through whatever further commands that delivery
// produces. Typing into the panel's filter pairs the synchronous
// list.FilterMatchesMsg this package cares about with a cursor-blink
// reschedule (charm.land/bubbles/v2/cursor's ~530ms default speed) that
// chasing unconditionally (the way src/model's refresh() does) would pay
// for on every keystroke; capping the wait per hop lets the instant result
// through while abandoning the blink goroutine, which is purely cosmetic
// here and never asserted on.
func deliverAsync(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(100 * time.Millisecond):
		return m
	}
	if msg == nil {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = deliverAsync(t, m, c)
		}
		return m
	}
	updated, next := m.Update(msg)
	m = updated.(Model)
	return deliverAsync(t, m, next)
}

// newFilterablePanel builds a panel focused on three lists — Home, Work,
// Groceries — matching the bug's exact repro data (a query that should
// match exactly one of them by name).
func newFilterablePanel(t *testing.T) Model {
	t.Helper()
	m := New().(Model)
	items := []apptypes.ListSummary{
		{List: apptypes.List{ID: "L1", Name: "Home"}},
		{List: apptypes.List{ID: "L2", Name: "Work"}},
		{List: apptypes.List{ID: "L3", Name: "Groceries"}},
	}

	updated, _ := m.Update(cmds.SetBodyLayoutMsg{Height: 20, ListsWidth: 30})
	m = updated.(Model)
	updated, _ = m.Update(cmds.RefreshListsMsg{Lists: items})
	m = updated.(Model)
	updated, _ = m.Update(cmds.SetFocusMsg(focusedZoneID))
	m = updated.(Model)

	return m
}

// typeFilter opens the panel's filter and types query one rune at a time,
// the way a real terminal delivers keystrokes, delivering each keystroke's
// resulting command so filteredItems settles the way the real event loop
// would.
func typeFilter(t *testing.T, m Model, query string) Model {
	t.Helper()
	updated, cmd := m.Update(cmds.ActivateListFilterMsg{})
	m = updated.(Model)
	m = deliverAsync(t, m, cmd)

	for _, r := range query {
		updated, cmd := m.Update(tea.KeyPressMsg{Text: string(r), Code: r})
		m = updated.(Model)
		m = deliverAsync(t, m, cmd)
	}
	return m
}

// TestFilterNarrowsToMatchingList reproduces the bug's exact repro: typing
// "Work" against [Home, Work, Groceries] must narrow the panel to the one
// matching entry, not blank it (task 000037YRR5KAYVSHF3SCT1SY1C, case 1).
func TestFilterNarrowsToMatchingList(t *testing.T) {
	m := newFilterablePanel(t)
	m = typeFilter(t, m, "Work")

	visible := m.list.VisibleItems()
	if len(visible) != 1 {
		t.Fatalf("visible items after filtering \"Work\" = %d, want 1 (got %v)", len(visible), visible)
	}
	got, ok := visible[0].(apptypes.ListSummary)
	if !ok || got.List.Name != "Work" {
		t.Fatalf("visible item = %v, want the \"Work\" list", visible[0])
	}
}

// TestFilterWithNoMatchesShowsEmptyResult pins case 2 from the task notes:
// a query matching nothing must narrow to zero visible items rather than
// falling back to the full list.
func TestFilterWithNoMatchesShowsEmptyResult(t *testing.T) {
	m := newFilterablePanel(t)
	m = typeFilter(t, m, "zzz")

	if visible := m.list.VisibleItems(); len(visible) != 0 {
		t.Fatalf("visible items after a non-matching filter = %d, want 0 (got %v)", len(visible), visible)
	}
}

// TestFilterAcceptKeepsFilterAndSelection pins case 3: enter while a filter
// is narrowed keeps the filter applied and selects the highlighted list —
// the reported bug had enter discard the filter and restore the old
// (pre-filter) selection instead.
func TestFilterAcceptKeepsFilterAndSelection(t *testing.T) {
	m := newFilterablePanel(t)
	m = typeFilter(t, m, "Work")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	m = deliverAsync(t, m, cmd)

	if state := m.list.FilterState(); state != list.FilterApplied {
		t.Fatalf("filter state after enter = %v, want FilterApplied", state)
	}
	if got := m.SelectedListID(); got != "L2" {
		t.Fatalf("selected list after enter = %q, want %q (Work)", got, "L2")
	}
}

// TestFilterEscClearsAndRestoresFullList pins case 4: esc while filtering
// clears the filter and restores every list.
func TestFilterEscClearsAndRestoresFullList(t *testing.T) {
	m := newFilterablePanel(t)
	m = typeFilter(t, m, "Work")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	m = deliverAsync(t, m, cmd)

	if state := m.list.FilterState(); state != list.Unfiltered {
		t.Fatalf("filter state after esc = %v, want Unfiltered", state)
	}
	if visible := m.list.VisibleItems(); len(visible) != 3 {
		t.Fatalf("visible items after esc = %d, want 3 (got %v)", len(visible), visible)
	}
}

// TestFilterSurvivesPollRefresh is the actual root cause the bug report and
// the prior investigation traced: the poll's periodic RefreshListsMsg calls
// list.Model.SetItems, which resets filteredItems and returns a command to
// re-run the filter against the refreshed items. If that command is
// dropped, the panel goes blank mid-filter exactly as reported. This
// delivers a refresh carrying the same lists while "Work" is narrowed and
// asserts the narrowed view survives it.
func TestFilterSurvivesPollRefresh(t *testing.T) {
	m := newFilterablePanel(t)
	m = typeFilter(t, m, "Work")

	if visible := m.list.VisibleItems(); len(visible) != 1 {
		t.Fatalf("setup: visible items after filtering = %d, want 1", len(visible))
	}

	// The poll ticks again with the same lists — the exact scenario the bug
	// report and the prior session's investigation identified.
	items := []apptypes.ListSummary{
		{List: apptypes.List{ID: "L1", Name: "Home"}},
		{List: apptypes.List{ID: "L2", Name: "Work"}},
		{List: apptypes.List{ID: "L3", Name: "Groceries"}},
	}
	updated, cmd := m.Update(cmds.RefreshListsMsg{Lists: items})
	m = updated.(Model)
	m = deliverAsync(t, m, cmd)

	visible := m.list.VisibleItems()
	if len(visible) != 1 {
		t.Fatalf("visible items after a poll refresh mid-filter = %d, want 1 (panel went blank)", len(visible))
	}
	got, ok := visible[0].(apptypes.ListSummary)
	if !ok || got.List.Name != "Work" {
		t.Fatalf("visible item after refresh = %v, want the \"Work\" list still narrowed", visible[0])
	}
}
