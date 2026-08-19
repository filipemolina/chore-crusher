package archivepage

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/constants"
	"github.com/filipemolina/farol/src/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// step feeds one message through Update and unwraps the *Model result. It
// deliberately does NOT execute any returned command: a key press can return
// a textinput-originated command (a cursor blink continuation, or
// textinput.Blink itself on "/"), and those are real timed tea.Tick values —
// calling them blocks the synchronous test loop for real wall-clock time
// (mirrors detailspanel's stepKey and its own note about the same trap).
// previewListID is set synchronously inside Update regardless (see
// loadPreviewIfSelectionChanged), so tests that need it need no chase; tests
// that need actual preview rows build and send RefreshArchivedListPreviewMsg
// themselves (previewRowsFor).
func step(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	updated, _ := m.Update(msg)
	out, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	return out
}

// readyModel builds a Model sized and focused, with two archived lists
// loaded: "Groceries" (one pending, one complete task) and "Chores" (empty).
// Archived most-recently-first, so entries[0] is whichever the store's own
// ListArchivedLists ordering puts first (archived_at DESC) — the two are
// archived in the same second in tests, so callers that care about order
// look the lists up by name rather than assuming a position.
func readyModel(t *testing.T) (Model, *store.Store) {
	t.Helper()
	s := openTestStore(t)

	groceries, err := s.CreateList("Groceries", "")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	if _, err := s.CreateTask(groceries, "Buy milk", nil, ""); err != nil {
		t.Fatalf("create task: %v", err)
	}
	doneID, err := s.CreateTask(groceries, "Buy eggs", nil, "")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := s.Toggle(doneID); err != nil {
		t.Fatalf("toggle task: %v", err)
	}
	if err := s.ArchiveList(groceries); err != nil {
		t.Fatalf("archive list: %v", err)
	}

	chores, err := s.CreateList("Chores", "")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	if err := s.ArchiveList(chores); err != nil {
		t.Fatalf("archive list: %v", err)
	}

	m := New(s).(Model)
	m = step(t, m, cmds.SetBodyLayoutMsg{Height: 30, TerminalWidth: 100})
	m = step(t, m, cmds.SetFocusMsg(constants.COMPONENT_ARCHIVE_PAGE))
	m = step(t, m, cmds.OpenArchivePageMsg{})
	m = step(t, m, cmds.RefreshArchivedListsMsg{Lists: apptypes.FromStoreLists(mustArchivedLists(t, s))})
	return m, s
}

// manyArchivedListsModel builds a Model with n archived, empty lists — enough
// to overflow a small terminal's list-column viewport for the scroll tests,
// which readyModel's fixed two entries never would.
func manyArchivedListsModel(t *testing.T, n int) (Model, *store.Store) {
	t.Helper()
	s := openTestStore(t)
	for i := range n {
		id, err := s.CreateList(fmt.Sprintf("List %02d", i), "")
		if err != nil {
			t.Fatalf("create list: %v", err)
		}
		if err := s.ArchiveList(id); err != nil {
			t.Fatalf("archive list: %v", err)
		}
	}

	m := New(s).(Model)
	m = step(t, m, cmds.SetBodyLayoutMsg{Height: 30, TerminalWidth: 100})
	m = step(t, m, cmds.SetFocusMsg(constants.COMPONENT_ARCHIVE_PAGE))
	m = step(t, m, cmds.OpenArchivePageMsg{})
	m = step(t, m, cmds.RefreshArchivedListsMsg{Lists: apptypes.FromStoreLists(mustArchivedLists(t, s))})
	return m, s
}

func mustArchivedLists(t *testing.T, s *store.Store) []store.ListSummary {
	t.Helper()
	ls, err := s.ListArchivedLists("")
	if err != nil {
		t.Fatalf("list archived lists: %v", err)
	}
	return ls
}

func TestLoadingStateShowsBeforeFirstRefresh(t *testing.T) {
	s := openTestStore(t)
	m := New(s).(Model)
	m = step(t, m, cmds.SetBodyLayoutMsg{Height: 30, TerminalWidth: 100})
	m = step(t, m, cmds.SetFocusMsg(constants.COMPONENT_ARCHIVE_PAGE))
	m = step(t, m, cmds.OpenArchivePageMsg{})

	out := ansi.Strip(m.View().Content)
	if !strings.Contains(out, "Loading archived lists") {
		t.Errorf("missing loading state:\n%s", out)
	}
}

func TestEmptyStateWhenNoArchivedLists(t *testing.T) {
	s := openTestStore(t)
	m := New(s).(Model)
	m = step(t, m, cmds.SetBodyLayoutMsg{Height: 30, TerminalWidth: 100})
	m = step(t, m, cmds.SetFocusMsg(constants.COMPONENT_ARCHIVE_PAGE))
	m = step(t, m, cmds.OpenArchivePageMsg{})
	m = step(t, m, cmds.RefreshArchivedListsMsg{Lists: nil})

	out := ansi.Strip(m.View().Content)
	if !strings.Contains(out, "No archived lists yet") {
		t.Errorf("missing empty state:\n%s", out)
	}
}

func TestLoadErrorIsShown(t *testing.T) {
	s := openTestStore(t)
	m := New(s).(Model)
	m = step(t, m, cmds.SetBodyLayoutMsg{Height: 30, TerminalWidth: 100})
	m = step(t, m, cmds.OpenArchivePageMsg{})
	m = step(t, m, cmds.RefreshArchivedListsMsg{Err: errBoom})

	out := ansi.Strip(m.View().Content)
	if !strings.Contains(out, "Could not load archived lists") {
		t.Errorf("missing error state:\n%s", out)
	}
}

var errBoom = errTest("boom")

type errTest string

func (e errTest) Error() string { return string(e) }

// TestRefreshShowsBothArchivedLists proves the loaded entries render and the
// selection lands on the first one, loading its preview automatically —
// opening the page should never require a keypress before anything shows.
func TestRefreshShowsBothArchivedLists(t *testing.T) {
	m, _ := readyModel(t)

	out := ansi.Strip(m.View().Content)
	if !strings.Contains(out, "Groceries") || !strings.Contains(out, "Chores") {
		t.Errorf("both archived lists should be listed:\n%s", out)
	}
	if m.previewListID == "" {
		t.Error("no preview load was triggered for the initial selection")
	}
}

// TestPreviewShowsTaskTitlesAndEmptyState proves the read-only preview
// renders the selected list's tasks, and shows a distinct message for a
// selected list that has none.
func TestPreviewShowsTaskTitlesAndEmptyState(t *testing.T) {
	m, _ := readyModel(t)

	// Whichever entry is selected initially, resolve the preview for it.
	sel, ok := m.selectedEntry()
	if !ok {
		t.Fatal("nothing selected after refresh")
	}
	m = step(t, m, cmds.RefreshArchivedListPreviewMsg{
		ListID: sel.List.ID,
		Rows:   previewRowsFor(t, m, sel.List.ID),
	})

	out := ansi.Strip(m.View().Content)
	if strings.HasPrefix(sel.List.Name, "Groceries") {
		if !strings.Contains(out, "Buy milk") || !strings.Contains(out, "Buy eggs") {
			t.Errorf("preview missing Groceries' tasks:\n%s", out)
		}
	} else {
		if !strings.Contains(out, "No tasks in this list") {
			t.Errorf("preview should show the empty-list message for Chores:\n%s", out)
		}
	}
}

// previewRowsFor loads and flattens a list's tasks the same way
// cmds.RefreshArchivedListPreview does, for tests that want to hand the
// message to Update directly rather than executing the real command.
func previewRowsFor(t *testing.T, m Model, listID string) []apptypes.Row {
	t.Helper()
	tasks, err := m.store.ListTasks(listID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	return apptypes.Flatten(apptypes.FromStoreTasks(tasks))
}

// TestStalePreviewResponseIsDropped proves a slow preview response for a list
// the user has since navigated away from does not clobber the newer
// selection's (possibly already-loaded) preview.
func TestStalePreviewResponseIsDropped(t *testing.T) {
	m, _ := readyModel(t)
	firstSelected := m.previewListID

	// Move the selection so previewListID changes.
	m = step(t, m, tea.KeyPressMsg{Text: "down"})
	if m.previewListID == firstSelected {
		t.Skip("only one entry in this run's ordering; nothing to navigate to")
	}
	newSelected := m.previewListID

	// A stale response for the old selection arrives after the move.
	m = step(t, m, cmds.RefreshArchivedListPreviewMsg{ListID: firstSelected, Rows: []apptypes.Row{{Task: apptypes.Task{Title: "stale"}}}})

	if m.previewListID != newSelected {
		t.Errorf("stale response overwrote the current selection: previewListID = %q, want %q", m.previewListID, newSelected)
	}
	for _, r := range m.previewRows {
		if r.Task.Title == "stale" {
			t.Error("stale preview response leaked into the current preview")
		}
	}
}

// TestFilterNarrowsVisibleEntries proves typing after / narrows the list live
// and the count label reflects it.
func TestFilterNarrowsVisibleEntries(t *testing.T) {
	m, _ := readyModel(t)

	m = step(t, m, tea.KeyPressMsg{Text: "/"})
	if !m.filtering {
		t.Fatal("/ did not enter filtering mode")
	}
	for _, r := range "Groc" {
		m = step(t, m, tea.KeyPressMsg{Text: string(r)})
	}

	visible := m.visibleEntries()
	if len(visible) != 1 || !strings.HasPrefix(visible[0].List.Name, "Groceries") {
		t.Fatalf("visibleEntries after filtering \"Groc\" = %v, want just Groceries", visible)
	}

	out := ansi.Strip(m.View().Content)
	if strings.Contains(out, "Chores") {
		t.Errorf("filtered-out list still rendered:\n%s", out)
	}
}

// TestEscWhileTypingClearsInOneStep proves esc pressed while the filter
// input is focused clears the query and exits typing in a single press,
// exactly mirroring the task tree's own /-filter ("esc clears it",
// docs/DESIGN.md §5) rather than a separate "commit, then a further esc
// clears" step of its own.
func TestEscWhileTypingClearsInOneStep(t *testing.T) {
	m, _ := readyModel(t)

	m = step(t, m, tea.KeyPressMsg{Text: "/"})
	m = step(t, m, tea.KeyPressMsg{Text: "z"}) // matches nothing
	m = step(t, m, tea.KeyPressMsg{Text: "esc"})

	if m.filtering {
		t.Error("esc should have stopped typing")
	}
	if m.filterInput.Value() != "" {
		t.Errorf("esc while typing should clear the query in one step, got %q", m.filterInput.Value())
	}
}

// TestEscClearsAppliedFilterBeforeClosingPage proves the esc-ladder
// precedence once a filter is applied (enter committed it, keyboard already
// elsewhere): a first esc clears the applied filter, and only a second esc —
// with nothing left to clear — closes the page.
func TestEscClearsAppliedFilterBeforeClosingPage(t *testing.T) {
	m, _ := readyModel(t)

	m = step(t, m, tea.KeyPressMsg{Text: "/"})
	m = step(t, m, tea.KeyPressMsg{Text: "z"}) // matches nothing
	m = step(t, m, tea.KeyPressMsg{Text: "enter"})
	if m.filtering || m.filterInput.Value() != "z" {
		t.Fatalf("precondition: enter should commit \"z\" without clearing it (filtering=%v, value=%q)", m.filtering, m.filterInput.Value())
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "esc"})
	m = updated.(Model)
	if m.filterInput.Value() != "" {
		t.Fatalf("first esc should clear the applied filter; value = %q", m.filterInput.Value())
	}
	if cmd != nil {
		if _, ok := cmd().(cmds.CloseArchivePageMsg); ok {
			t.Fatal("first esc closed the page instead of clearing the filter")
		}
	}

	_, cmd = m.Update(tea.KeyPressMsg{Text: "esc"})
	if cmd == nil {
		t.Fatal("esc with no filter and no other claim should close the page")
	}
	if _, ok := cmd().(cmds.CloseArchivePageMsg); !ok {
		t.Errorf("esc command = %T, want cmds.CloseArchivePageMsg", cmd())
	}
}

// TestUnarchiveKeyRequestsConfirmationWithName proves u does not restore
// anything itself — it only asks AppModel to confirm, carrying the selected
// list's name so the (AppModel-owned) dialog can name what it is about to
// restore, matching Delete's own request shape (docs/DESIGN.md §9).
func TestUnarchiveKeyRequestsConfirmationWithName(t *testing.T) {
	m, _ := readyModel(t)
	sel, ok := m.selectedEntry()
	if !ok {
		t.Fatal("nothing selected after refresh")
	}

	_, cmd := m.Update(tea.KeyPressMsg{Text: "u"})
	if cmd == nil {
		t.Fatal("u produced no command")
	}
	msg, ok := cmd().(cmds.UnarchiveArchivedListMsg)
	if !ok {
		t.Fatalf("u command produced %T, want cmds.UnarchiveArchivedListMsg", cmd())
	}
	if msg.ListID != sel.List.ID {
		t.Errorf("ListID = %q, want %q", msg.ListID, sel.List.ID)
	}
	if msg.ListName != sel.List.Name {
		t.Errorf("ListName = %q, want %q", msg.ListName, sel.List.Name)
	}
}

// TestUnarchiveKeyDoesNotItselfWriteToTheStore proves u, like d, performs no
// store write on its own — the list must still be archived immediately
// after the keypress, since only AppModel's confirm modal may call
// store.UnarchiveList.
func TestUnarchiveKeyDoesNotItselfWriteToTheStore(t *testing.T) {
	m, s := readyModel(t)
	sel, ok := m.selectedEntry()
	if !ok {
		t.Fatal("nothing selected after refresh")
	}

	m.Update(tea.KeyPressMsg{Text: "u"})

	lists, err := s.ListLists()
	if err != nil {
		t.Fatalf("list lists: %v", err)
	}
	for _, l := range lists {
		if l.List.ID == sel.List.ID {
			t.Errorf("list %q reappeared in normal discovery after u alone (no confirmation happened)", sel.List.Name)
		}
	}
}

// TestDeleteKeyRequestsConfirmationWithNameAndTaskCount proves d does not
// delete anything itself — it only asks AppModel to confirm, carrying the
// selected list's name and task count so the (AppModel-owned) dialog can
// name what it is about to destroy, matching Lists.Delete's own body text
// style (docs/DESIGN.md §9).
func TestDeleteKeyRequestsConfirmationWithNameAndTaskCount(t *testing.T) {
	m, _ := readyModel(t)
	sel, ok := m.selectedEntry()
	if !ok {
		t.Fatal("nothing selected after refresh")
	}

	_, cmd := m.Update(tea.KeyPressMsg{Text: "d"})
	if cmd == nil {
		t.Fatal("d produced no command")
	}
	msg, ok := cmd().(cmds.DeleteArchivedListMsg)
	if !ok {
		t.Fatalf("d command produced %T, want cmds.DeleteArchivedListMsg", cmd())
	}
	if msg.ListID != sel.List.ID {
		t.Errorf("ListID = %q, want %q", msg.ListID, sel.List.ID)
	}
	if msg.ListName != sel.List.Name {
		t.Errorf("ListName = %q, want %q", msg.ListName, sel.List.Name)
	}
	if msg.TaskCount != sel.PendingCount+sel.CompleteCount {
		t.Errorf("TaskCount = %d, want %d", msg.TaskCount, sel.PendingCount+sel.CompleteCount)
	}
}

// TestDeleteKeyDoesNotItselfWriteToTheStore proves d, like u, performs no
// store write on its own — the list must still exist (archived or not)
// immediately after the keypress, since only AppModel's confirm modal may
// call store.DeleteList.
func TestDeleteKeyDoesNotItselfWriteToTheStore(t *testing.T) {
	m, s := readyModel(t)
	sel, ok := m.selectedEntry()
	if !ok {
		t.Fatal("nothing selected after refresh")
	}

	m.Update(tea.KeyPressMsg{Text: "d"})

	if _, err := s.GetList(sel.List.ID); err != nil {
		t.Errorf("list %q no longer resolves after d alone (no confirmation happened): %v", sel.List.Name, err)
	}
}

// TestOpenArchivePageMsgResetsStaleState proves reopening the page after it
// was left mid-filter starts clean rather than resuming a stale query and
// selection.
func TestOpenArchivePageMsgResetsStaleState(t *testing.T) {
	m, _ := readyModel(t)
	m = step(t, m, tea.KeyPressMsg{Text: "/"})
	m = step(t, m, tea.KeyPressMsg{Text: "z"})

	m = step(t, m, cmds.OpenArchivePageMsg{})

	if m.filterInput.Value() != "" || m.filtering {
		t.Error("OpenArchivePageMsg did not reset the filter")
	}
	if !m.loading || len(m.entries) != 0 {
		t.Error("OpenArchivePageMsg did not reset to a loading, empty state")
	}
}

// TestFocusPreviewTogglesWithTabAndShiftTab proves both tab and shift+tab
// cycle keyboard focus between the archived-list column and the task
// preview — with exactly two columns, either key just toggles it.
func TestFocusPreviewTogglesWithTabAndShiftTab(t *testing.T) {
	m, _ := readyModel(t)
	if m.previewFocused {
		t.Fatal("preview should not start focused")
	}

	m = step(t, m, tea.KeyPressMsg{Text: "tab"})
	if !m.previewFocused {
		t.Fatal("tab should move focus to the preview column")
	}

	m = step(t, m, tea.KeyPressMsg{Text: "tab"})
	if m.previewFocused {
		t.Fatal("a second tab should move focus back to the list column")
	}

	m = step(t, m, tea.KeyPressMsg{Text: "shift+tab"})
	if !m.previewFocused {
		t.Fatal("shift+tab should also toggle focus to the preview column")
	}
}

// TestNavigateScrollsPreviewInsteadOfSelectionWhenFocused proves that once
// tab has moved focus to the preview column, up/down/j/k scroll its task
// list rather than moving the archived-list selection — the two columns
// scroll independently.
func TestNavigateScrollsPreviewInsteadOfSelectionWhenFocused(t *testing.T) {
	m, _ := readyModel(t)
	sel, ok := m.selectedEntry()
	if !ok {
		t.Fatal("nothing selected after refresh")
	}

	viewport := m.previewViewportRows()
	rows := make([]apptypes.Row, 0, viewport+5)
	for i := range viewport + 5 {
		rows = append(rows, apptypes.Row{Task: apptypes.Task{Title: fmt.Sprintf("task %d", i)}})
	}
	m = step(t, m, cmds.RefreshArchivedListPreviewMsg{ListID: sel.List.ID, Rows: rows})

	m = step(t, m, tea.KeyPressMsg{Text: "tab"})
	m = step(t, m, tea.KeyPressMsg{Text: "down"})

	if m.selectedIdx != 0 {
		t.Errorf("selectedIdx = %d after down with preview focused, want unchanged (0)", m.selectedIdx)
	}
	if m.previewScroll != 1 {
		t.Errorf("previewScroll = %d after one down press with preview focused, want 1", m.previewScroll)
	}

	m = step(t, m, tea.KeyPressMsg{Text: "j"})
	if m.previewScroll != 2 {
		t.Errorf("previewScroll = %d after a second down press, want 2", m.previewScroll)
	}
}

// TestPreviewGoToEndAndStartClampToBounds proves G/end and g/home jump the
// preview scroll to its last and first valid offsets rather than past the
// end of the row list.
func TestPreviewGoToEndAndStartClampToBounds(t *testing.T) {
	m, _ := readyModel(t)
	sel, ok := m.selectedEntry()
	if !ok {
		t.Fatal("nothing selected after refresh")
	}

	viewport := m.previewViewportRows()
	n := viewport + 7
	rows := make([]apptypes.Row, 0, n)
	for i := range n {
		rows = append(rows, apptypes.Row{Task: apptypes.Task{Title: fmt.Sprintf("task %d", i)}})
	}
	m = step(t, m, cmds.RefreshArchivedListPreviewMsg{ListID: sel.List.ID, Rows: rows})
	m = step(t, m, tea.KeyPressMsg{Text: "tab"})

	m = step(t, m, tea.KeyPressMsg{Text: "end"})
	if want := n - viewport; m.previewScroll != want {
		t.Errorf("previewScroll after end = %d, want %d", m.previewScroll, want)
	}

	m = step(t, m, tea.KeyPressMsg{Text: "home"})
	if m.previewScroll != 0 {
		t.Errorf("previewScroll after home = %d, want 0", m.previewScroll)
	}
}

// TestListScrollFollowsSelectionPastViewport proves the archived-list column
// scrolls to keep the selection visible once it moves past the first
// screenful — without this, a selection driven below the viewport would
// simply vanish off the bottom of a column that never scrolls itself.
func TestListScrollFollowsSelectionPastViewport(t *testing.T) {
	m, _ := manyArchivedListsModel(t, 40)
	viewport := m.listViewportRows()
	if viewport <= 0 || viewport >= 40 {
		t.Fatalf("test needs a viewport smaller than the entry count, got %d rows for 40 entries", viewport)
	}

	for range viewport + 3 {
		m = step(t, m, tea.KeyPressMsg{Text: "down"})
	}

	if m.selectedIdx < m.listScroll || m.selectedIdx >= m.listScroll+viewport {
		t.Errorf("selection %d fell outside the scrolled window [%d, %d)", m.selectedIdx, m.listScroll, m.listScroll+viewport)
	}
	if m.listScroll == 0 {
		t.Error("listScroll should have advanced once the selection moved past the first viewport")
	}
}

// TestFilterKeyReturnsFocusToListColumn proves pressing / while the preview
// column has focus brings focus back to the list column, since the filter
// row it opens lives there.
func TestFilterKeyReturnsFocusToListColumn(t *testing.T) {
	m, _ := readyModel(t)
	m = step(t, m, tea.KeyPressMsg{Text: "tab"})
	if !m.previewFocused {
		t.Fatal("precondition: tab should have focused the preview column")
	}

	m = step(t, m, tea.KeyPressMsg{Text: "/"})
	if m.previewFocused {
		t.Error("/ should return keyboard focus to the list column")
	}
}

// TestUnarchiveActsOnListSelectionEvenWhenPreviewFocused proves Unarchive
// (and, by the same code path, Delete) still reads the archived-list
// column's own selection regardless of which column currently has keyboard
// focus for scrolling.
func TestUnarchiveActsOnListSelectionEvenWhenPreviewFocused(t *testing.T) {
	m, _ := readyModel(t)
	sel, ok := m.selectedEntry()
	if !ok {
		t.Fatal("nothing selected after refresh")
	}

	m = step(t, m, tea.KeyPressMsg{Text: "tab"})

	_, cmd := m.Update(tea.KeyPressMsg{Text: "u"})
	if cmd == nil {
		t.Fatal("u produced no command")
	}
	msg, ok := cmd().(cmds.UnarchiveArchivedListMsg)
	if !ok {
		t.Fatalf("u command produced %T, want cmds.UnarchiveArchivedListMsg", cmd())
	}
	if msg.ListID != sel.List.ID {
		t.Errorf("ListID = %q, want %q", msg.ListID, sel.List.ID)
	}
}

// TestOpenArchivePageMsgResetsFocusAndScroll extends
// TestOpenArchivePageMsgResetsStaleState to the focus/scroll state this task
// added: reopening the page must not resume a stale preview focus or scroll
// position any more than it resumes a stale filter.
func TestOpenArchivePageMsgResetsFocusAndScroll(t *testing.T) {
	m, _ := readyModel(t)
	m = step(t, m, tea.KeyPressMsg{Text: "tab"})
	m = step(t, m, tea.KeyPressMsg{Text: "down"})

	m = step(t, m, cmds.OpenArchivePageMsg{})

	if m.previewFocused {
		t.Error("OpenArchivePageMsg did not reset preview focus")
	}
	if m.listScroll != 0 || m.previewScroll != 0 {
		t.Errorf("OpenArchivePageMsg did not reset scroll state: listScroll=%d previewScroll=%d", m.listScroll, m.previewScroll)
	}
}

var _ tea.Model = Model{}
