package listspanel

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
)

// TestListsPanelScrollsToSelection proves the existing bubbles/list already
// keeps the selected row reachable when there are more lists than fit in the
// panel body: Update forwards navigation to m.list, which scrolls its own
// viewport. This is Commit 5 step 1 — a passing regression here means the Lists
// panel needs no second scrolling system.
func TestListsPanelScrollsToSelection(t *testing.T) {
	m := New().(Model)

	// Far more four-row list cards than a short panel can show at once.
	const n = 20
	items := make([]apptypes.ListSummary, n)
	for i := range items {
		items[i] = apptypes.ListSummary{List: apptypes.List{ID: fmt.Sprintf("L%02d", i), Name: fmt.Sprintf("Listname%02d", i)}}
	}

	updated, _ := m.Update(cmds.SetBodyLayoutMsg{Height: 16, ListsWidth: 30})
	m = updated.(Model)
	updated, _ = m.Update(cmds.RefreshListsMsg{Lists: items})
	m = updated.(Model)
	updated, _ = m.Update(cmds.SetFocusMsg(focusedZoneID))
	m = updated.(Model)

	// Precondition: the whole list cannot fit, so reaching the last item must
	// scroll rather than merely reveal an always-visible row.
	body := ansi.Strip(m.View().Content)
	if strings.Contains(body, items[n-1].List.Name) {
		t.Fatalf("precondition: last list already visible before scrolling:\n%s", body)
	}

	// Jump to the last list the way a user would (G = GoToEnd).
	updated, _ = m.Update(tea.KeyPressMsg{Text: "G", Code: 'G'})
	m = updated.(Model)

	if got := m.SelectedListID(); got != items[n-1].List.ID {
		t.Fatalf("selected list = %q, want the last %q", got, items[n-1].List.ID)
	}
	body = ansi.Strip(m.View().Content)
	if !strings.Contains(body, items[n-1].List.Name) {
		t.Errorf("last list %q not visible after scrolling to it:\n%s", items[n-1].List.Name, body)
	}
	if strings.Contains(body, items[0].List.Name) {
		t.Errorf("first list %q still visible after scrolling to the end; the panel did not scroll:\n%s", items[0].List.Name, body)
	}
}

// TestListsPanelShowsBelowOverflow checks that when there are more lists than
// fit in the panel, the panel shows an "N below" footer instead of the bubbles
// list's built-in pagination dots (Commit 5, Part 3 — same treatment as the
// task tree overflow indicator).
func TestListsPanelShowsBelowOverflow(t *testing.T) {
	m := New().(Model)

	const n = 20
	items := make([]apptypes.ListSummary, n)
	for i := range items {
		items[i] = apptypes.ListSummary{List: apptypes.List{ID: fmt.Sprintf("L%02d", i), Name: fmt.Sprintf("Listname%02d", i)}}
	}

	updated, _ := m.Update(cmds.SetBodyLayoutMsg{Height: 16, ListsWidth: 30})
	m = updated.(Model)
	updated, _ = m.Update(cmds.RefreshListsMsg{Lists: items})
	m = updated.(Model)
	updated, _ = m.Update(cmds.SetFocusMsg(focusedZoneID))
	m = updated.(Model)

	body := ansi.Strip(m.View().Content)
	if !strings.Contains(body, "below") {
		t.Errorf("expected 'N below' overflow indicator in lists panel:\n%s", body)
	}
}

// TestListsPanelNoOverflowWhenAllFit checks that when all lists fit in the
// panel, no "below" indicator is shown.
func TestListsPanelNoOverflowWhenAllFit(t *testing.T) {
	m := New().(Model)

	items := []apptypes.ListSummary{
		{List: apptypes.List{ID: "L01", Name: "First"}},
		{List: apptypes.List{ID: "L02", Name: "Second"}},
	}

	updated, _ := m.Update(cmds.SetBodyLayoutMsg{Height: 20, ListsWidth: 30})
	m = updated.(Model)
	updated, _ = m.Update(cmds.RefreshListsMsg{Lists: items})
	m = updated.(Model)
	updated, _ = m.Update(cmds.SetFocusMsg(focusedZoneID))
	m = updated.(Model)

	body := ansi.Strip(m.View().Content)
	if strings.Contains(body, "below") {
		t.Errorf("should not show overflow when all lists fit:\n%s", body)
	}
}

// TestListsPanelNoPaginationDots checks that the bubbles list's built-in
// pagination dots are not rendered (they were replaced with the text footer).
// The dots character is "•" (U+2022).
func TestListsPanelNoPaginationDots(t *testing.T) {
	m := New().(Model)

	const n = 20
	items := make([]apptypes.ListSummary, n)
	for i := range items {
		items[i] = apptypes.ListSummary{List: apptypes.List{ID: fmt.Sprintf("L%02d", i), Name: fmt.Sprintf("Listname%02d", i)}}
	}

	updated, _ := m.Update(cmds.SetBodyLayoutMsg{Height: 16, ListsWidth: 30})
	m = updated.(Model)
	updated, _ = m.Update(cmds.RefreshListsMsg{Lists: items})
	m = updated.(Model)
	updated, _ = m.Update(cmds.SetFocusMsg(focusedZoneID))
	m = updated.(Model)

	body := ansi.Strip(m.View().Content)
	if strings.Contains(body, "•") {
		t.Errorf("pagination dots should not be rendered in lists panel:\n%s", body)
	}
}
