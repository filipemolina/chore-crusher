package tasktree

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
)

// rows builds a flat list of n root tasks with ids "1".."n".
func rows(n int) []apptypes.Row {
	out := make([]apptypes.Row, n)
	for i := range out {
		out[i] = apptypes.Row{Task: apptypes.Task{ID: string(rune('1' + i))}}
	}
	return out
}

// The cursor-preservation rule (docs/DESIGN.md §7): the selection is matched
// by task id across a refresh, so a row that keeps its id keeps the cursor
// even when its index moves.
func TestSelectionSurvivesByIdAcrossRefresh(t *testing.T) {
	m := Model{}
	m.applyRows(rows(3))
	m.selectedID = "2"

	// The selected task moved from index 1 to index 0.
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "2"}},
		{Task: apptypes.Task{ID: "1"}},
		{Task: apptypes.Task{ID: "3"}},
	})

	if m.selectedID != "2" {
		t.Errorf("selection = %q, want %q (must follow the id, not the index)", m.selectedID, "2")
	}
}

// When the selected id is gone, the selection falls back to the nearest
// surviving row: the old index clamped into the new list.
func TestSelectionFallsBackToNearestSurvivingRow(t *testing.T) {
	m := Model{}
	m.applyRows(rows(4))
	m.selectedID = "3" // index 2

	// Task 3 was deleted; the list is now two rows.
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "1"}},
		{Task: apptypes.Task{ID: "2"}},
	})

	if m.selectedID != "2" {
		t.Errorf("selection = %q, want %q (old index 2 clamped to the last row)", m.selectedID, "2")
	}
}

// A refresh that removes rows before the selection must not shift the
// cursor past the new end of the list.
func TestSelectionClampsToNewListEnd(t *testing.T) {
	m := Model{}
	m.applyRows(rows(4))
	m.selectedID = "4" // index 3, last row

	m.applyRows(rows(2)) // ids "1", "2"

	if m.selectedID != "2" {
		t.Errorf("selection = %q, want %q (clamped to the new last row)", m.selectedID, "2")
	}
}

func TestEmptyRefreshClearsSelection(t *testing.T) {
	m := Model{}
	m.applyRows(rows(3))
	m.selectedID = "2"

	m.applyRows(nil)

	if m.selectedID != "" || len(m.rows) != 0 {
		t.Errorf("empty refresh should clear rows and selection, got %d rows, selection %q", len(m.rows), m.selectedID)
	}
}

// A 3-level tree whose only title match is a leaf. The /-filter must keep the
// leaf's whole ancestor chain visible even though none of them match, so the
// leaf never floats with no visible parent (docs/plans/phase-8-search.md step 1).
func TestFilterKeepsAncestorsOfMatchedLeaf(t *testing.T) {
	root := apptypes.Row{Task: apptypes.Task{ID: "root", Title: "Project"}, Depth: 0, HasChildren: true}
	sub := apptypes.Row{Task: apptypes.Task{ID: "sub", ParentID: strPtr("root"), Title: "Milestone"}, Depth: 1, HasChildren: true}
	leaf := apptypes.Row{Task: apptypes.Task{ID: "leaf", ParentID: strPtr("sub"), Title: "Ship the zorb"}, Depth: 2}
	other := apptypes.Row{Task: apptypes.Task{ID: "other", Title: "Unrelated task"}, Depth: 0}

	m := Model{}
	m.applyRows([]apptypes.Row{root, sub, leaf, other})
	m.filterTyping = true
	m.filterQuery = "zorb"

	got := m.displayedRows()
	wantIDs := []string{"root", "sub", "leaf"}
	if len(got) != len(wantIDs) {
		t.Fatalf("filtered rows = %d, want %d", len(got), len(wantIDs))
	}
	for i, want := range wantIDs {
		if got[i].Task.ID != want {
			t.Errorf("filtered[%d].ID = %q, want %q", i, got[i].Task.ID, want)
		}
	}
}

// An unrelated root task with no match — and no matched descendant — drops out
// of the filtered view entirely.
func TestFilterDropsUnrelatedRoots(t *testing.T) {
	root := apptypes.Row{Task: apptypes.Task{ID: "root", Title: "Project"}, Depth: 0, HasChildren: true}
	sub := apptypes.Row{Task: apptypes.Task{ID: "sub", ParentID: strPtr("root"), Title: "Milestone"}, Depth: 1}
	m := Model{}
	m.applyRows([]apptypes.Row{root, sub})
	m.filterTyping = true
	m.filterQuery = "nowhere-nothing"

	got := m.displayedRows()
	if len(got) != 0 {
		t.Errorf("filtered rows = %d, want 0", len(got))
	}
}

// An empty query shows everything: typing / and nothing yet is a no-op filter.
func TestEmptyQueryDoesNotFilter(t *testing.T) {
	root := apptypes.Row{Task: apptypes.Task{ID: "a", Title: "Alpha"}}
	sub := apptypes.Row{Task: apptypes.Task{ID: "b", Title: "Beta"}}
	m := Model{}
	m.applyRows([]apptypes.Row{root, sub})
	m.filterQuery = ""

	got := m.displayedRows()
	if len(got) != 2 {
		t.Errorf("empty query filtered to %d rows, want 2", len(got))
	}
}

// A directly-matched row is distinguishable from an ancestor-only row: only
// real matches land in the matched set used to dim ancestors.
func TestMatchedVisibleSeparatesMatchesFromAncestors(t *testing.T) {
	root := apptypes.Row{Task: apptypes.Task{ID: "root", Title: "Project"}, HasChildren: true}
	leaf := apptypes.Row{Task: apptypes.Task{ID: "leaf", ParentID: strPtr("root"), Title: "Zorble"}}

	_, matched := matchVisible([]apptypes.Row{root, leaf}, "zorble")
	if !matched["leaf"] {
		t.Errorf("leaf should be a direct match")
	}
	if matched["root"] {
		t.Errorf("root should not count as a direct match, only an ancestor")
	}
}

func strPtr(s string) *string { return &s }

// A first load with no prior selection picks the first row.
func TestFirstLoadSelectsFirstRow(t *testing.T) {
	m := Model{}
	m.applyRows(rows(3))

	if m.selectedID != "1" {
		t.Errorf("selection = %q, want %q (first row)", m.selectedID, "1")
	}
}

// namedRows is like rows but gives each task a title equal to its id, so the
// rendered text is distinguishable for ordering assertions.
func namedRows(n int) []apptypes.Row {
	out := make([]apptypes.Row, n)
	for i := range out {
		out[i] = apptypes.Row{Task: apptypes.Task{ID: string(rune('1' + i)), Title: string(rune('1' + i))}}
	}
	return out
}

// TestBracketChangesLevelOffset verifies that [ / ] adjust the create-level
// offset, clamped to [-1, +1], replacing the old tab / shift+tab behavior.
func TestBracketChangesLevelOffset(t *testing.T) {
	m := &Model{}
	m.applyRows(namedRows(3))
	m.selectedID = "1"
	m.StartCreating("1")

	// ] indents (child), clamped at +1
	m.handleCreatingKey(tea.KeyPressMsg{Text: "]", Code: ']'})
	if m.createLevelOffset != 1 {
		t.Errorf("indent ]: offset = %d, want 1", m.createLevelOffset)
	}
	// Second ] stays clamped at +1
	m.handleCreatingKey(tea.KeyPressMsg{Text: "]", Code: ']'})
	if m.createLevelOffset != 1 {
		t.Errorf("second indent: offset = %d, want 1 (clamped)", m.createLevelOffset)
	}
	// [ outdents (parent)
	m.handleCreatingKey(tea.KeyPressMsg{Text: "[", Code: '['})
	if m.createLevelOffset != 0 {
		t.Errorf("outdent [: offset = %d, want 0", m.createLevelOffset)
	}
	// [ again: -1
	m.handleCreatingKey(tea.KeyPressMsg{Text: "[", Code: '['})
	if m.createLevelOffset != -1 {
		t.Errorf("second outdent: offset = %d, want -1", m.createLevelOffset)
	}
	// Third [ stays clamped at -1
	m.handleCreatingKey(tea.KeyPressMsg{Text: "[", Code: '['})
	if m.createLevelOffset != -1 {
		t.Errorf("third outdent: offset = %d, want -1 (clamped)", m.createLevelOffset)
	}
}

// TestHardAllowlistSwallowsNonCreateKeys verifies that while creating,
// keys outside the allowlist (Tab, arrows) are swallowed rather than
// reaching the text input or triggering app shortcuts.
func TestHardAllowlistSwallowsNonCreateKeys(t *testing.T) {
	m := &Model{}
	m.StartCreating("")
	m.createInput.SetValue("buy milk")

	// Tab should be swallowed, not typed into the input
	m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.createInput.Value() != "buy milk" {
		t.Errorf("tab should be swallowed, input = %q", m.createInput.Value())
	}

	// Down arrow should be swallowed
	m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.createInput.Value() != "buy milk" {
		t.Errorf("down arrow should be swallowed, input = %q", m.createInput.Value())
	}

	// Up arrow should be swallowed
	m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.createInput.Value() != "buy milk" {
		t.Errorf("up arrow should be swallowed, input = %q", m.createInput.Value())
	}

	// Regular typing should still work
	m.handleCreatingKey(tea.KeyPressMsg{Text: "!", Code: '!'})
	if m.createInput.Value() != "buy milk!" {
		t.Errorf("typing ! should work, input = %q", m.createInput.Value())
	}
}

func TestStartCreatingEntersCreateMode(t *testing.T) {
	m := &Model{}
	m.applyRows(rows(3))
	m.selectedID = "2"
	m.StartCreating("2")

	if !m.creating {
		t.Error("StartCreating should enter creating mode")
	}
	if !m.createManual {
		t.Error("StartCreating should mark the mode manual (cancelable via esc)")
	}
	if m.createBeforeID != "2" || m.createLevelOffset != 0 {
		t.Errorf("state = before %q offset %d, want 2/0", m.createBeforeID, m.createLevelOffset)
	}
}

// An empty active list auto-enters the non-cancelable inline creation mode:
// the input row is the empty state, so esc on an empty input must not leave.
func TestEmptyListAutoCreates(t *testing.T) {
	m := &Model{}
	m.activeList = true
	m.applyRows(nil)

	if !m.creating {
		t.Fatal("empty active list should auto-enter creating mode")
	}
	if m.createManual {
		t.Error("auto-create for an empty list must not be manual")
	}
	if m.createBeforeID != "" {
		t.Errorf("createBeforeID = %q, want \"\" (append at end)", m.createBeforeID)
	}
}

func TestOwnsKeyboardAndKeepsEscWhileCreating(t *testing.T) {
	m := &Model{}
	m.focused = true
	m.StartCreating("1")

	if !m.OwnsKeyboard() {
		t.Error("creating tree should own the keyboard so globals are suppressed")
	}
	if !m.KeepsEsc() {
		t.Error("creating tree should keep esc to cancel/clear")
	}
}

func TestRenderCreateRowShowsPlaceholder(t *testing.T) {
	m := &Model{}
	m.activeList = true
	m.applyRows(nil) // auto-creating on an empty list

	rendered := ansi.Strip(m.ViewInPanel(60, 24, appstyles.Active.BackgroundPanel))
	if !strings.Contains(rendered, "Add a task") {
		t.Errorf("expected 'Add a task' placeholder in empty-list create row, got: %q", rendered)
	}
}

func TestCreateRowAppearsAfterSelected(t *testing.T) {
	m := &Model{}
	m.activeList = true
	m.applyRows(namedRows(3))
	m.selectedID = "2"
	m.StartCreating("2")

	rendered := ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))
	// The create row must come after the selected task's row in the
	// rendered output. Find each section by its unique text and compare
	// positions rather than relying on strings.Index of a shared digit.
	iSel := strings.Index(rendered, "[ ] 2")
	iCreate := strings.Index(rendered, "Add a task")
	if iSel < 0 || iCreate < 0 {
		t.Fatalf("rendered = %q", rendered)
	}
	if iCreate < iSel {
		t.Errorf("create row rendered before selected task: selected@%d create@%d", iSel, iCreate)
	}
}

// TestCreateRowAfterLastPendingWhenCompleteSelected verifies phase B step 4:
// when the selected task is complete, the create row is placed after the
// last pending task (at the same depth) rather than under the complete row.
func TestCreateRowAfterLastPendingWhenCompleteSelected(t *testing.T) {
	m := &Model{}
	m.activeList = true
	// Pending: 1, 2; Complete: 3, 4. Select complete task 3.
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "1", Title: "1", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "2", Title: "2", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "3", Title: "3", Status: apptypes.StatusComplete}},
		{Task: apptypes.Task{ID: "4", Title: "4", Status: apptypes.StatusComplete}},
	})
	m.selectedID = "3"
	m.StartCreating("3")

	// createBeforeID should have been redirected to the last pending task (2).
	if m.createBeforeID != "2" {
		t.Errorf("createBeforeID = %q, want \"2\" (last pending task)", m.createBeforeID)
	}

	// Visual: create row must appear after task 2 and before task 3.
	// When createBeforeID is "2" but selectedID is "3", the create row
	// is inserted after task 2 (in the Pending section), not after task 3.
	rendered := ansi.Strip(m.ViewInPanel(80, 24, appstyles.Active.BackgroundPanel))
	iLastPending := strings.LastIndex(rendered, "[ ] 2")
	iCreate := strings.Index(rendered, "Add a task")
	iFirstComplete := strings.Index(rendered, "[x] 3")
	if iLastPending < 0 || iCreate < 0 || iFirstComplete < 0 {
		t.Fatalf("rendered = %q", rendered)
	}
	if !(iLastPending < iCreate && iCreate < iFirstComplete) {
		t.Errorf("create row should be between last pending (2@%d) and first complete (3@%d): create@%d",
			iLastPending, iFirstComplete, iCreate)
	}
}

// TestCreateRowAtRootWhenNoPending verifies that when all tasks are complete
// (zero pending), the create row lands at root depth at the top of the
// Pending section (the section shows only the input).
func TestCreateRowAtRootWhenNoPending(t *testing.T) {
	m := &Model{}
	m.activeList = true
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "1", Title: "1", Status: apptypes.StatusComplete}},
		{Task: apptypes.Task{ID: "2", Title: "2", Status: apptypes.StatusComplete}},
	})
	m.selectedID = "1"
	m.StartCreating("1")

	if m.createBeforeID != "" {
		t.Errorf("createBeforeID = %q, want \"\" (root/append when no pending)", m.createBeforeID)
	}
	if m.createLevelOffset != 0 {
		t.Errorf("createLevelOffset = %d, want 0 (root depth)", m.createLevelOffset)
	}
}

func TestCreateRowGlyphForLevelOffset(t *testing.T) {
	m := &Model{}
	m.activeList = true
	m.applyRows(nil)
	for _, c := range []struct {
		offset int
		glyph  string
	}{
		{0, "-"}, {1, "+"}, {-1, "^"},
	} {
		m.createLevelOffset = c.offset
		rendered := ansi.Strip(m.ViewInPanel(60, 24, appstyles.Active.BackgroundPanel))
		if !strings.Contains(rendered, c.glyph) {
			t.Errorf("offset %d: expected glyph %q in %q", c.offset, c.glyph, rendered)
		}
	}
}

func TestEscWithTextStaysCreating(t *testing.T) {
	m := &Model{}
	m.StartCreating("")
	m.createInput.SetValue("half typed")

	if _, _ = m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyEsc}); !m.creating {
		t.Fatal("esc with text should keep creating mode")
	}
	if m.createInput.Value() != "" {
		t.Errorf("esc with text should clear input, got %q", m.createInput.Value())
	}
}

func TestEscCancelsManualEmpty(t *testing.T) {
	m := &Model{}
	m.StartCreating("") // manual, empty input
	m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.creating {
		t.Error("esc on empty manual input should cancel creating mode")
	}
}

func TestEscStaysOnAutoEmpty(t *testing.T) {
	m := &Model{}
	m.activeList = true
	m.applyRows(nil) // auto, empty input
	if m.createInput.Value() != "" {
		t.Fatalf("expected empty input, got %q", m.createInput.Value())
	}
	m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !m.creating {
		t.Error("esc on empty auto (empty-list) input should NOT cancel creating")
	}
}

func TestEnterEmitsCreateTaskFromInput(t *testing.T) {
	m := &Model{}
	m.StartCreating("")
	m.createInput.SetValue("buy milk")

	_, cmd := m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter with text should emit a CreateTaskFromInput command")
	}
	msg := cmd()
	msgTyped, ok := msg.(cmds.CreateTaskFromInputMsg)
	if !ok {
		t.Fatalf("command produced %T, want cmds.CreateTaskFromInputMsg", msg)
	}
	if msgTyped.Title != "buy milk" {
		t.Errorf("Title = %q, want %q", msgTyped.Title, "buy milk")
	}
}
