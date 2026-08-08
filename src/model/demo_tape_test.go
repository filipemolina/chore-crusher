package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/constants"
)

// TestDemoTapeCreateSequence replays the exact keystroke sequence the
// demo/demo.tape GIF records, so a broken tape — the lists panel never opening,
// brackets leaking into a title, a missing Enter dropping a task, or a level
// offset that lands the task at the wrong depth — surfaces as a test failure
// rather than as a GIF a human has to eyeball.
//
// The sequence mirrors demo/demo.tape: L opens the lists side panel, Up/Down
// navigates it once, n opens the new-list name modal (type name + Enter
// creates the list, auto-closes the panel, and lands focus on the empty task
// tree whose inline create input has auto-opened), so the first task is typed
// straight into the live input (no n); ] indents the next create to a child
// of that task; Enter saves it; Esc closes the rapid-entry input.
func TestDemoTapeCreateSequence(t *testing.T) {
	// 100 columns is below AUTO_SHOW_LISTS_MIN_WIDTH (120), so the lists panel
	// starts hidden — the tape opens it explicitly with L, so this matches the
	// tape's starting condition (panel closed, focus on the task tree).
	m := newTestModel(t, t.TempDir())

	// L opens the lists panel and moves focus onto it.
	m = refresh(t, m, tea.KeyPressMsg{Text: "L", Code: 'L'})
	if !m.listsPanelVisible {
		t.Fatal("L should have opened the lists panel")
	}
	if m.focusedZone != constants.COMPONENT_LISTS_PANEL {
		t.Fatalf("after L, focusedZone = %d, want lists panel (%d)",
			m.focusedZone, constants.COMPONENT_LISTS_PANEL)
	}

	// Navigate up then down once in the lists panel (a no-op on a 2-list seed,
	// but it exercises the panel cursor the way the GIF shows).
	m = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})

	// n opens the new-list name modal; type the name; Enter submits it. The app
	// auto-closes the panel and selects the new (empty) list, whose inline
	// create input auto-opens.
	m = refresh(t, m, tea.KeyPressMsg{Text: "n", Code: 'n'})
	if m.activeModal == nil {
		t.Fatal("n in the lists panel should open the new-list name modal, not reach the task tree")
	}
	for _, r := range "Weekend chores" {
		m = update(m, tea.KeyPressMsg{Text: string(r), Code: r})
	}
	m = refresh(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.listsPanelVisible {
		t.Error("creating a list should auto-close the lists panel (the tape's L-to-close step is redundant)")
	}
	tree, ok := m.components.TaskPanel.(interface{ IsCreating() bool })
	if !ok {
		t.Fatalf("TaskPanel missing IsCreating")
	}
	if !tree.IsCreating() {
		t.Fatal("the new empty list's create input should be live (so the first task needs no n)")
	}

	// --- Root task -------------------------------------------------------
	// The empty list's input is already live — type the title, Enter saves it.
	for _, r := range "Grocery run" {
		m = update(m, tea.KeyPressMsg{Text: string(r), Code: r})
	}
	m = refresh(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	// --- Nested subtask (one level deep) ----------------------------------
	// Rapid entry left the input live and anchored on "Grocery run". ] indents
	// the create level to +1 (child), then type the title and Enter.
	m = update(m, tea.KeyPressMsg{Text: "]", Code: ']'})
	for _, r := range "Pick up milk" {
		m = update(m, tea.KeyPressMsg{Text: string(r), Code: r})
	}
	m = refresh(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	// Esc closes the rapid-entry input (the tape's "remove the new task input").
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})

	// --- Assertions on the final tree ------------------------------------
	rows := taskRowsFor(m)
	wantTitles := []string{"Grocery run", "Pick up milk"}
	if len(rows) != len(wantTitles) {
		t.Fatalf("tree rows = %d, want %d (%s)", len(rows), len(wantTitles), rowSummary(rows))
	}
	titleFor := func(title string) *apptypes.Row {
		for i := range rows {
			if rows[i].Task.Title == title {
				return &rows[i]
			}
		}
		return nil
	}
	for _, want := range wantTitles {
		row := titleFor(want)
		if row == nil {
			t.Errorf("missing task %q; rows: %s", want, rowSummary(rows))
			continue
		}
		if strings.ContainsAny(row.Task.Title, "[]") {
			t.Errorf("task title %q leaked a bracket character", row.Task.Title)
		}
	}

	// Depth + parent: "Grocery run" is a root (depth 0), "Pick up milk" is its
	// child (depth 1).
	root := titleFor("Grocery run")
	child := titleFor("Pick up milk")
	if root == nil || child == nil {
		t.Fatalf("missing rows; rows: %s", rowSummary(rows))
	}
	if root.Depth != 0 || root.Task.ParentID != nil {
		t.Errorf("\"Grocery run\" depth=%d parent=%v, want depth 0 root", root.Depth, root.Task.ParentID)
	}
	if child.Depth != 1 {
		t.Errorf("\"Pick up milk\" depth = %d, want 1", child.Depth)
	}
	if child.Task.ParentID == nil || *child.Task.ParentID != root.Task.ID {
		t.Errorf("\"Pick up milk\" parent = %v, want %q", child.Task.ParentID, root.Task.ID)
	}
}

// update runs a message through AppModel.Update without chasing the returned
// command. Typed runes carry a cursor-blink reschedule command (see
// refresh_test.go's TestInlineCreateCircuitEndToEnd) that is cosmetic and
// irrelevant to the create cascade, which is driven separately through refresh.
func update(m AppModel, msg tea.Msg) AppModel {
	updated, _ := m.Update(msg)
	return updated.(AppModel)
}

func rowSummary(rows []apptypes.Row) string {
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(r.Task.Title)
	}
	return b.String()
}
