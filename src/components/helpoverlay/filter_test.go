package helpoverlay

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/keys"
)

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		u, _ := m.Update(tea.KeyPressMsg{Text: k, Code: rune(k[0])})
		m = u.(Model)
	}
	return m
}

// TestSlashOpensFilterAndNarrowsLive types a query one key at a time, the way
// a real keystroke stream arrives, and checks the catalog narrows after every
// character without needing enter.
func TestSlashOpensFilterAndNarrowsLive(t *testing.T) {
	m := New(keys.Context{}, 100, 200).(Model)
	m = press(m, "/")
	if !m.filterTyping {
		t.Fatal("/ did not open the filter input")
	}

	m = press(m, "d", "e", "l", "e", "t", "e")
	if m.filterQuery != "delete" {
		t.Fatalf("filterQuery = %q, want %q", m.filterQuery, "delete")
	}

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "delete") {
		t.Errorf("filtered view does not contain a matching entry:\n%s", view)
	}
	if strings.Contains(view, "Overlays") {
		t.Errorf("filtered view still shows a scope with no matching entry:\n%s", view)
	}
}

// TestFilterDropsScopesWithNoMatch checks that a scope with zero matching
// entries disappears entirely rather than rendering an orphaned title over
// nothing (docs/DESIGN.md's "clear separation between sections" only means
// something if a section without content is not shown at all).
func TestFilterDropsScopesWithNoMatch(t *testing.T) {
	m := New(keys.Context{}, 100, 200).(Model)
	m.filterQuery = "does-not-match-anything-zzz"

	catalog := m.filteredCatalog()
	if len(catalog) != 0 {
		t.Fatalf("filteredCatalog = %d scopes, want 0", len(catalog))
	}

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "No shortcuts match") {
		t.Errorf("view does not report the empty filter result:\n%s", view)
	}
}

// TestEnterAppliesFilterAndArrowsScrollAgain checks that enter blurs the
// input and hands ↑/↓ back to scrolling, the same handoff the task tree's own
// filter makes.
func TestEnterAppliesFilterAndArrowsScrollAgain(t *testing.T) {
	m := New(keys.Context{}, 100, 200).(Model)
	m = press(m, "/", "d")
	u, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = u.(Model)

	if m.filterTyping {
		t.Fatal("enter did not blur the filter input")
	}
	if !m.filterApplied {
		t.Fatal("enter did not apply the filter")
	}

	u, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = u.(Model)
	if m.filterQuery != "d" {
		t.Fatalf("arrow key leaked into the filter query: %q", m.filterQuery)
	}
}

// TestEscWhileTypingClearsFilter checks the first esc clears an in-progress
// query rather than closing the overlay.
func TestEscWhileTypingClearsFilter(t *testing.T) {
	m := New(keys.Context{}, 100, 200).(Model)
	m = press(m, "/", "d")

	u, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = u.(Model)

	if m.filterTyping || m.filterQuery != "" {
		t.Fatalf("esc did not clear the filter: typing=%v query=%q", m.filterTyping, m.filterQuery)
	}
	if cmd != nil {
		t.Fatal("esc while typing should clear, not close the overlay")
	}
}

// TestEscOnAppliedFilterClearsBeforeClosing checks that once a filter is
// applied, esc clears it first; only a second esc (with no filter left)
// closes the overlay.
func TestEscOnAppliedFilterClearsBeforeClosing(t *testing.T) {
	m := New(keys.Context{}, 100, 200).(Model)
	m = press(m, "/", "d")
	u, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = u.(Model)

	u, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = u.(Model)
	if m.filterApplied || m.filterQuery != "" {
		t.Fatal("esc on an applied filter should clear it")
	}
	if cmd != nil {
		t.Fatal("clearing an applied filter should not close the overlay")
	}

	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc with no filter left should close the overlay")
	}
}
