package mainmenu

import (
	"strings"
	"testing"

	"github.com/filipemolina/farol/src/cmds"
)

func TestHeaderRendersWordmark(t *testing.T) {
	m := New()
	updated, _ := m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 80})

	out := updated.(Model).View().Content
	if !strings.Contains(out, "Farol") {
		t.Errorf("header output does not contain wordmark:\n%s", out)
	}
}

// TestHeaderDefaultsToAllView pins the header's view-mode indicator to
// "all" before any SetTaskTreeViewMsg arrives, matching the tree's own
// ViewAll default (docs/DESIGN.md §6) — the header must never claim a mode
// the tree has not actually selected.
func TestHeaderDefaultsToAllView(t *testing.T) {
	m := New()
	updated, _ := m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 80})

	out := updated.(Model).View().Content
	if !strings.Contains(out, "all") {
		t.Errorf("header output does not contain default view mode \"all\":\n%s", out)
	}
}

// TestHeaderTracksTaskTreeView proves the header re-renders the tree's
// reported mode, the only wiring that keeps the two in step (mainmenu.Model
// has no other way to learn the tree's view).
func TestHeaderTracksTaskTreeView(t *testing.T) {
	m := New()
	updated, _ := m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 80})
	updated, _ = updated.(Model).Update(cmds.SetTaskTreeViewMsg{View: "pending"})

	out := updated.(Model).View().Content
	if !strings.Contains(out, "pending") {
		t.Errorf("header output does not contain updated view mode \"pending\":\n%s", out)
	}
}

// TestHeaderShedsViewModeBeforeWordmark pins the terminal-width guard's
// shedding priority: narrowing from 80 columns, the view-mode indicator is
// the first thing to disappear, and the wordmark — the header's one
// non-negotiable element — is still standing once it does. The mode is the
// tree's transient state; the wordmark is the header's identity.
func TestHeaderShedsViewModeBeforeWordmark(t *testing.T) {
	m := New()

	var droppedAt int
	for w := 80; w > 0; w-- {
		updated, _ := m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: w})
		updated, _ = updated.(Model).Update(cmds.SetTaskTreeViewMsg{View: "pending"})
		out := updated.(Model).View().Content
		if !strings.Contains(out, "pending") {
			droppedAt = w
			break
		}
	}
	if droppedAt == 0 {
		t.Fatal("narrowing to 1 column never dropped the view mode")
	}

	updated, _ := m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: droppedAt})
	updated, _ = updated.(Model).Update(cmds.SetTaskTreeViewMsg{View: "pending"})
	out := updated.(Model).View().Content
	if !strings.Contains(out, "Farol") {
		t.Errorf("wordmark missing at width %d, the width the view mode first dropped at:\n%s", droppedAt, out)
	}
}
