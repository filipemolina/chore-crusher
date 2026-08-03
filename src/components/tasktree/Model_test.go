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
// The selection sits at depth 1, so [ can still outdent all the way to the
// root without hitting the root-level no-op (docs/DESIGN.md §4).
func TestBracketChangesLevelOffset(t *testing.T) {
	m := &Model{}
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "1", Title: "1"}},
		{Task: apptypes.Task{ID: "2", Title: "2", ParentID: strPtr("1")}, Depth: 1},
	})
	m.selectedID = "2"
	m.StartCreating("2")

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
	// [ again: -1 (a sibling of the selection's parent, i.e. root)
	m.handleCreatingKey(tea.KeyPressMsg{Text: "[", Code: '['})
	if m.createLevelOffset != -1 {
		t.Errorf("second outdent: offset = %d, want -1", m.createLevelOffset)
	}
	// Third [ stays clamped at -1: the row already sits at the root, and
	// nothing sits above the root.
	m.handleCreatingKey(tea.KeyPressMsg{Text: "[", Code: '['})
	if m.createLevelOffset != -1 {
		t.Errorf("third outdent: offset = %d, want -1 (clamped)", m.createLevelOffset)
	}
}

// TestOutdentNoOpAtRoot verifies that [ is a no-op while the create row sits
// at the root level: a new task can never be its own parent's sibling one
// level above the root, so the only level choices at root are - (same level)
// and + (one level down) (docs/DESIGN.md §4).
func TestOutdentNoOpAtRoot(t *testing.T) {
	m := &Model{}
	m.applyRows(namedRows(3))
	m.selectedID = "1"
	m.StartCreating("1")

	// [ on a root-level create row changes nothing.
	for i := 0; i < 2; i++ {
		m.handleCreatingKey(tea.KeyPressMsg{Text: "[", Code: '['})
		if m.createLevelOffset != 0 {
			t.Fatalf("outdent at root: offset = %d, want 0 (no-op)", m.createLevelOffset)
		}
	}

	// ] still indents to a child...
	m.handleCreatingKey(tea.KeyPressMsg{Text: "]", Code: ']'})
	if m.createLevelOffset != 1 {
		t.Fatalf("indent: offset = %d, want 1", m.createLevelOffset)
	}
	// ...and [ steps that back to the sibling level.
	m.handleCreatingKey(tea.KeyPressMsg{Text: "[", Code: '['})
	if m.createLevelOffset != 0 {
		t.Fatalf("outdent from child: offset = %d, want 0", m.createLevelOffset)
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
	if m.createBeforeID != "2" || m.createLevelOffset != 0 {
		t.Errorf("state = before %q offset %d, want 2/0", m.createBeforeID, m.createLevelOffset)
	}
}

// An empty active list auto-enters inline creation mode: the input is the
// empty state's way in. It can be left with esc — see
// TestEscCancelsOnAutoEmpty (docs/plan/task-row-cards-and-status.md).
func TestEmptyListAutoCreates(t *testing.T) {
	m := &Model{}
	m.activeList = true
	m.applyRows(nil)

	if !m.creating {
		t.Fatal("empty active list should auto-enter creating mode")
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
	iSel := strings.Index(rendered, "◻ 2")
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
	iLastPending := strings.LastIndex(rendered, "◻ 2")
	iCreate := strings.Index(rendered, "Add a task")
	iFirstComplete := strings.Index(rendered, "◼ 3")
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
	// Selection at depth 1, so offset -1 (a sibling of the selection's
	// parent, at the root) is a reachable state; at a root selection it is
	// unreachable by design (docs/DESIGN.md §4).
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "1", Title: "1"}},
		{Task: apptypes.Task{ID: "2", Title: "2", ParentID: strPtr("1")}, Depth: 1},
	})
	m.selectedID = "2"
	m.StartCreating("2")
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

// TestEscCancelsDiscardsText verifies single-press esc: with text in the
// input, one esc cancels creating outright and discards the text
// (docs/plan/task-row-cards-and-status.md).
func TestEscCancelsDiscardsText(t *testing.T) {
	m := &Model{}
	m.StartCreating("")
	m.createInput.SetValue("half typed")

	if _, _ = m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyEsc}); m.creating {
		t.Fatal("esc with text should cancel creating mode")
	}
	if m.createInput.Value() != "" {
		t.Errorf("esc should discard the typed text, got %q", m.createInput.Value())
	}
	if !m.createSuppressed {
		t.Error("esc should mark the session suppressed so a refresh does not re-open it")
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

// TestEscCancelsOnAutoEmpty verifies esc cancels even the empty-list auto
// input: the input is no longer the permanent empty state.
func TestEscCancelsOnAutoEmpty(t *testing.T) {
	m := &Model{}
	m.activeList = true
	m.applyRows(nil) // auto, empty input
	if m.createInput.Value() != "" {
		t.Fatalf("expected empty input, got %q", m.createInput.Value())
	}
	m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.creating {
		t.Error("esc on the empty-list input should cancel creating mode")
	}
}

// TestNoAutoCreateAfterEscCancel pins the createSuppressed flag: after an
// esc cancel on an empty list, the next refresh of the same list must not
// silently re-open the input, and n brings it back.
func TestNoAutoCreateAfterEscCancel(t *testing.T) {
	m := &Model{}
	m.activeList = true
	m.applyRows(nil)
	m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.creating {
		t.Fatal("setup: esc should have cancelled creating")
	}

	// Same list refreshes (e.g. a poll tick): rows still empty.
	m.applyRows(nil)
	if m.creating {
		t.Error("refresh after esc cancel must not re-open the input")
	}

	// But n brings it back.
	m.focused = true
	next, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !next.(Model).IsCreating() {
		t.Error("n after esc cancel should re-enter creating mode")
	}
}

// TestDeleteAllReopensInputAfterEscCancel pins the second way the empty-list
// input comes back: deleting every remaining task clears the esc suppression,
// so the refresh that empties the list auto-shows the input again (n is the
// other way — docs/plan/task-row-cards-and-status.md).
func TestDeleteAllReopensInputAfterEscCancel(t *testing.T) {
	m := &Model{}
	m.activeList = true
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "1", Title: "one"}},
		{Task: apptypes.Task{ID: "2", Title: "two"}},
	})
	m.StartCreating("2")
	m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.creating || !m.createSuppressed {
		t.Fatal("setup: esc should have cancelled creating and suppressed")
	}

	// The last remaining task is deleted: the refresh arrives with no rows.
	m.applyRows(nil)
	if m.createSuppressed {
		t.Error("deleting all items must clear the esc suppression")
	}
	if !m.creating {
		t.Error("an empty list whose last item was just deleted should re-open the input")
	}
}


// TestCreateSuppressedClearsOnListChange verifies the suppression is per
// list session, not global: switching to a different (empty) list re-enables
// the auto-input.
func TestCreateSuppressedClearsOnListChange(t *testing.T) {
	m := &Model{}
	next, _ := m.Update(cmds.RefreshTasksMsg{ListID: "list-a"})
	nm := next.(Model)
	m = &nm
	m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !m.createSuppressed {
		t.Fatal("setup: esc should have suppressed")
	}

	next, _ = m.Update(cmds.RefreshTasksMsg{ListID: "list-b"})
	nm = next.(Model)
	if nm.createSuppressed {
		t.Error("a list switch must clear the suppression")
	}
	if !nm.IsCreating() {
		t.Error("an empty list-b should auto-show its input")
	}
}

// TestNewKeyStartsCreatingOnEmptyTree verifies n starts creating when the
// tree has no rows and is not already creating: esc can leave the surface
// bare, and n is the way back in.
func TestNewKeyStartsCreatingOnEmptyTree(t *testing.T) {
	m := &Model{}
	m.focused = true
	m.activeList = true
	m.applyRows(nil)
	m.handleCreatingKey(tea.KeyPressMsg{Code: tea.KeyEsc}) // leave creating

	next, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !next.(Model).IsCreating() {
		t.Error("n on an empty tree should start creating")
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

// TestNavigationTracksSectionsSeparately pins the two-section cursor rule
// (docs/DESIGN.md §6): pending and complete rows keep their own order, so a
// completed task no longer hijacks a pending row's index. The store positions
// interleave (p1, c1, p2, c2) but the cursor walks Pending then Complete.
func TestNavigationTracksSectionsSeparately(t *testing.T) {
	m := &Model{}
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "p1", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "c1", Status: apptypes.StatusComplete}},
		{Task: apptypes.Task{ID: "p2", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "c2", Status: apptypes.StatusComplete}},
	})

	m.selectedID = "p1"
	m.moveSelection(1)
	if m.selectedID != "p2" {
		t.Errorf("down from p1 = %q, want p2 (the complete row is not on the path)", m.selectedID)
	}

	// Down past the last pending row lands on the first complete row.
	m.moveSelection(1)
	if m.selectedID != "c1" {
		t.Errorf("down from last pending = %q, want c1", m.selectedID)
	}

	// Up from the first complete row returns to the last pending row.
	m.moveSelection(-1)
	if m.selectedID != "p2" {
		t.Errorf("up from first complete = %q, want p2", m.selectedID)
	}

	// The ends clamp: no wrap from the top of Pending or the bottom of
	// Complete.
	m.selectedID = "c2"
	m.moveSelection(1)
	if m.selectedID != "c2" {
		t.Errorf("down from last complete = %q, want c2 (clamped)", m.selectedID)
	}
	m.selectedID = "p1"
	m.moveSelection(-1)
	if m.selectedID != "p1" {
		t.Errorf("up from first pending = %q, want p1 (clamped)", m.selectedID)
	}
}

// TestOutdentSelectedEmitsMoveTask pins [ on a selected task: it resolves to
// MoveTask(task, parent) so the task becomes its parent's next sibling. A
// root task has nothing above it — no-op.
func TestOutdentSelectedEmitsMoveTask(t *testing.T) {
	m := &Model{}
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "1", Title: "1"}},
		{Task: apptypes.Task{ID: "2", Title: "2", ParentID: strPtr("1")}, Depth: 1},
	})
	m.selectedID = "2"

	cmd := m.outdentSelected()
	if cmd == nil {
		t.Fatal("outdent on a child must emit a MoveTask command")
	}
	msg, ok := cmd().(cmds.MoveTaskMsg)
	if !ok {
		t.Fatalf("outdent produced %T, want cmds.MoveTaskMsg", cmd())
	}
	if msg.TaskID != "2" || msg.AfterID != "1" {
		t.Errorf("outdent = %+v, want TaskID 2 AfterID 1 (the parent)", msg)
	}

	m.selectedID = "1"
	if cmd := m.outdentSelected(); cmd != nil {
		t.Errorf("outdent at root must be a no-op, got %v", cmd)
	}
}

// TestIndentSelectedEmitsReparent pins ] on a selected task: it resolves to
// ReparentTask(task, previous sibling), making the task the last child of
// its previous sibling. A first sibling is a no-op, and so is a pending task
// whose previous sibling is complete (§3 forbids that parentage).
func TestIndentSelectedEmitsReparent(t *testing.T) {
	m := &Model{}
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "1", Title: "1"}},
		{Task: apptypes.Task{ID: "2", Title: "2"}},
	})
	m.selectedID = "2"

	cmd := m.indentSelected()
	if cmd == nil {
		t.Fatal("indent on a second sibling must emit a ReparentTask command")
	}
	msg, ok := cmd().(cmds.ReparentTaskMsg)
	if !ok {
		t.Fatalf("indent produced %T, want cmds.ReparentTaskMsg", cmd())
	}
	if msg.TaskID != "2" || msg.ParentID == nil || *msg.ParentID != "1" {
		t.Errorf("indent = %+v, want parent 1", msg)
	}

	m.selectedID = "1"
	if cmd := m.indentSelected(); cmd != nil {
		t.Errorf("indent on the first sibling must be a no-op, got %v", cmd)
	}

	// A pending task cannot move under a complete sibling (§3).
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "3", Status: apptypes.StatusComplete}},
		{Task: apptypes.Task{ID: "4", Status: apptypes.StatusPending}},
	})
	m.selectedID = "4"
	if cmd := m.indentSelected(); cmd != nil {
		t.Errorf("indent under a complete sibling must be a no-op, got %v", cmd)
	}
}

// TestMoveSelectedStaysInSection pins the move gesture: it swaps the selected
// task with the previous/next same-status sibling and never crosses the
// Pending/Complete boundary — a task at its run's edge stays put
// (docs/DESIGN.md §6).
func TestMoveSelectedStaysInSection(t *testing.T) {
	m := &Model{}
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "p1", Title: "p1", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "c1", Title: "c1", Status: apptypes.StatusComplete}},
		{Task: apptypes.Task{ID: "p2", Title: "p2", Status: apptypes.StatusPending}},
	})

	// Move p2 up: it swaps with p1; p1 is first in the run, so p2 goes to
	// the front (afterID "").
	m.selectedID = "p2"
	cmd := m.moveSelected(-1)
	if cmd == nil {
		t.Fatal("move up with a same-status predecessor must emit a MoveTask command")
	}
	msg, ok := cmd().(cmds.MoveTaskMsg)
	if !ok {
		t.Fatalf("move up produced %T, want cmds.MoveTaskMsg", cmd())
	}
	if msg.TaskID != "p2" || msg.AfterID != "" {
		t.Errorf("move p2 up = %+v, want AfterID \"\" (front of run)", msg)
	}

	// Move down from the last pending row: no same-status successor — no-op.
	m.selectedID = "p2"
	if cmd := m.moveSelected(1); cmd != nil {
		t.Errorf("move down past the pending boundary must be a no-op, got %v", cmd)
	}

	// The complete run has one task: neither direction moves.
	m.selectedID = "c1"
	if cmd := m.moveSelected(-1); cmd != nil {
		t.Errorf("move up past the complete boundary must be a no-op, got %v", cmd)
	}
	if cmd := m.moveSelected(1); cmd != nil {
		t.Errorf("move down from the only complete task must be a no-op, got %v", cmd)
	}
}

// TestMoveSelectedDownUsesNextSibling pins the down-gesture resolution: the
// task lands right after its next same-status sibling.
func TestMoveSelectedDownUsesNextSibling(t *testing.T) {
	m := &Model{}
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "a", Title: "a", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "b", Title: "b", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "c", Title: "c", Status: apptypes.StatusComplete}},
	})

	m.selectedID = "a"
	cmd := m.moveSelected(1)
	msg, ok := cmd().(cmds.MoveTaskMsg)
	if !ok {
		t.Fatalf("move down produced %T, want cmds.MoveTaskMsg", cmd())
	}
	if msg.TaskID != "a" || msg.AfterID != "b" {
		t.Errorf("move a down = %+v, want AfterID b", msg)
	}
}

// TestBracketKeysRestructureSelectedTask verifies [ / ] act on the selected
// task when the tree is focused and not creating — they no longer only pick a
// level while the inline input is active (docs/DESIGN.md §5).
func TestBracketKeysRestructureSelectedTask(t *testing.T) {
	m := &Model{}
	m.focused = true
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "1", Title: "1"}},
		{Task: apptypes.Task{ID: "2", Title: "2"}},
	})
	m.selectedID = "2"

	_, cmd := m.Update(tea.KeyPressMsg{Text: "]", Code: ']'})
	if cmd == nil {
		t.Fatal("] on a selected task must emit a command")
	}
	if _, ok := cmd().(cmds.ReparentTaskMsg); !ok {
		t.Errorf("] produced %T, want cmds.ReparentTaskMsg", cmd())
	}
}

// TestAltMoveKeysReachMoveHandler verifies alt+↑/alt+↓ (and alt+k/alt+j)
// route to the move gesture through Update (docs/DESIGN.md §5).
func TestAltMoveKeysReachMoveHandler(t *testing.T) {
	m := &Model{}
	m.focused = true
	m.applyRows([]apptypes.Row{
		{Task: apptypes.Task{ID: "1", Title: "1", Status: apptypes.StatusPending}},
		{Task: apptypes.Task{ID: "2", Title: "2", Status: apptypes.StatusPending}},
	})
	m.selectedID = "2"

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModAlt})
	if cmd == nil {
		t.Fatal("alt+k must emit a move command")
	}
	if _, ok := cmd().(cmds.MoveTaskMsg); !ok {
		t.Errorf("alt+k produced %T, want cmds.MoveTaskMsg", cmd())
	}
}
