package archivepage

import (
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
	if sel.List.Name == "Groceries" {
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
	if len(visible) != 1 || visible[0].List.Name != "Groceries" {
		t.Fatalf("visibleEntries after filtering \"Groc\" = %v, want just Groceries", visible)
	}

	out := ansi.Strip(m.View().Content)
	if strings.Contains(out, "Chores") {
		t.Errorf("filtered-out list still rendered:\n%s", out)
	}
}

// TestEscClearsFilterBeforeClosingPage proves the esc-ladder precedence: a
// first esc (after committing out of typing) clears a non-empty filter, and
// only a second esc — with nothing left to clear — closes the page. This
// mirrors the tree and Lists panel's own established esc-ladder idiom
// (docs/DESIGN.md §5).
func TestEscClearsFilterBeforeClosingPage(t *testing.T) {
	m, _ := readyModel(t)

	m = step(t, m, tea.KeyPressMsg{Text: "/"})
	m = step(t, m, tea.KeyPressMsg{Text: "z"}) // matches nothing
	m = step(t, m, tea.KeyPressMsg{Text: "esc"})
	if m.filtering {
		t.Fatal("esc should have stopped typing, not still be in filtering mode")
	}
	if m.filterInput.Value() != "z" {
		t.Fatalf("first esc should only commit the query, not clear it; got %q", m.filterInput.Value())
	}

	// Second esc: nothing selected (the filter matched no one) but the query
	// is still non-empty, so this esc must clear it, not close the page.
	updated, cmd := m.Update(tea.KeyPressMsg{Text: "esc"})
	m = updated.(Model)
	if m.filterInput.Value() != "" {
		t.Fatalf("second esc should clear the filter; value = %q", m.filterInput.Value())
	}
	if cmd != nil {
		if _, ok := cmd().(cmds.CloseArchivePageMsg); ok {
			t.Fatal("second esc closed the page instead of clearing the filter first")
		}
	}

	// Third esc, with an empty query and nothing left to clear, closes the page.
	_, cmd = m.Update(tea.KeyPressMsg{Text: "esc"})
	if cmd == nil {
		t.Fatal("esc with no filter and no other claim should close the page")
	}
	if _, ok := cmd().(cmds.CloseArchivePageMsg); !ok {
		t.Errorf("esc command = %T, want cmds.CloseArchivePageMsg", cmd())
	}
}

// TestUnarchiveRemovesSelectedEntryAndSelectsNext proves u restores the
// selected list to normal discovery (it must show up again in
// store.ListLists) and that the page's own list narrows to reflect it,
// landing selection on whatever now occupies the vacated slot — the same
// "select what's now there" outcome clampSelection already gives for free
// after any set-shrinking refresh.
func TestUnarchiveRemovesSelectedEntryAndSelectsNext(t *testing.T) {
	m, s := readyModel(t)
	sel, ok := m.selectedEntry()
	if !ok {
		t.Fatal("nothing selected after refresh")
	}
	unarchivedID := sel.List.ID

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "u"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("u produced no command")
	}
	msg, ok := cmd().(cmds.RefreshArchivedListsMsg)
	if !ok {
		t.Fatalf("u command produced %T, want cmds.RefreshArchivedListsMsg", cmd())
	}
	m = step(t, m, msg)

	for _, e := range m.entries {
		if e.List.ID == unarchivedID {
			t.Fatalf("unarchived list %q is still in the Archive page's entries", e.List.Name)
		}
	}
	lists, err := s.ListLists()
	if err != nil {
		t.Fatalf("list lists: %v", err)
	}
	found := false
	for _, l := range lists {
		if l.List.ID == unarchivedID {
			found = true
		}
	}
	if !found {
		t.Error("unarchived list did not reappear in normal list discovery (store.ListLists)")
	}
	if len(m.entries) > 0 {
		if _, ok := m.selectedEntry(); !ok {
			t.Error("selection is out of bounds after the set shrank")
		}
	}
}

// TestUnarchiveFailureSurfacesActionErrWithoutLosingTheList proves a failed
// write (here: the list vanished from under the page, simulating a race with
// another agent) shows an inline message rather than blowing away the
// already-loaded list with the full-page error state.
func TestUnarchiveFailureSurfacesActionErrWithoutLosingTheList(t *testing.T) {
	m, s := readyModel(t)
	sel, ok := m.selectedEntry()
	if !ok {
		t.Fatal("nothing selected after refresh")
	}
	if err := s.DeleteList(sel.List.ID); err != nil {
		t.Fatalf("delete list out from under the page: %v", err)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "u"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("u produced no command")
	}
	msg, ok := cmd().(archiveActionErrMsg)
	if !ok {
		t.Fatalf("u command produced %T, want archiveActionErrMsg", cmd())
	}
	m = step(t, m, msg)

	if m.actionErr == "" {
		t.Error("failed unarchive did not set actionErr")
	}
	if len(m.entries) == 0 {
		t.Error("a failed unarchive should not clear the already-loaded entries")
	}
	out := ansi.Strip(m.View().Content)
	if !strings.Contains(out, sel.List.Name) {
		t.Errorf("failed unarchive's view lost the list it was acting on:\n%s", out)
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

// TestDeleteKeyDoesNotItselfWriteToTheStore proves d, unlike u, performs no
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

var _ tea.Model = Model{}
