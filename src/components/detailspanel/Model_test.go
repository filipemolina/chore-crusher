package detailspanel

import (
	"image/color"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/constants"
	"github.com/filipemolina/chore-crusher/src/store"
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

// loaded builds a panel hydrated with one task in a fresh store, plus the
// store and task id so save-path tests can inspect the writes.
func loaded(t *testing.T, notes string) (*Model, *store.Store, string) {
	t.Helper()
	s := openTestStore(t)
	listID, err := s.CreateList("Chores", "")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	taskID, err := s.CreateTask(listID, "Water plants", nil, notes)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	m := New(s).(*Model)
	m, _ = updateModel(m, cmds.SetFocus(constants.COMPONENT_DETAILS_PANEL)())
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	return m, s, taskID
}

// loadedWithList is like loaded but also returns the list id, for tests that
// need to flip list-level flags (e.g. comments_disabled) via the store.
func loadedWithList(t *testing.T, notes string) (*Model, *store.Store, string, string) {
	t.Helper()
	m, s, taskID := loaded(t, notes)
	row, err := s.GetTask(taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	return m, s, row.ListID, taskID
}

func updateModel(m *Model, msg tea.Msg) (*Model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(*Model), cmd
}

// runCmd resolves a cmd to its message (nil-safe).
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func typeRune(t *testing.T, m *Model, r rune) *Model {
	t.Helper()
	m, _ = updateModel(m, tea.KeyPressMsg{Text: string(r), Code: r})
	return m
}

func TestCleanEscClosesPanel(t *testing.T) {
	m, _, _ := loaded(t, "")

	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "esc"})
	if _, ok := runCmd(cmd).(cmds.CloseDetailsSideMsg); !ok {
		t.Fatalf("clean esc: got %T, want CloseDetailsSideMsg", runCmd(cmd))
	}
}

func TestDirtyEscPromptsThenKeepsOrDiscards(t *testing.T) {
	m, _, _ := loaded(t, "")
	// Dirty the draft by typing into the entry Title field.
	m = typeRune(t, m, '!') // types into Title

	// Esc alone must not close: it opens the discard prompt.
	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "esc"})
	if runCmd(cmd) != nil {
		t.Fatalf("dirty esc closed early: %T", runCmd(cmd))
	}
	if !m.confirmingDiscard {
		t.Fatal("dirty esc did not open the discard prompt")
	}

	// n keeps the draft.
	m, cmd = updateModel(m, tea.KeyPressMsg{Text: "n", Code: 'n'})
	if runCmd(cmd) != nil {
		t.Fatalf("n closed the panel: %T", runCmd(cmd))
	}
	if m.confirmingDiscard {
		t.Fatal("n did not dismiss the discard prompt")
	}
	if m.TitleValue() != "Water plants!" {
		t.Fatalf("n dropped the draft: TitleValue = %q", m.TitleValue())
	}

	// Esc again, then y discards and closes.
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "esc"})
	_, cmd = updateModel(m, tea.KeyPressMsg{Text: "y", Code: 'y'})
	if _, ok := runCmd(cmd).(cmds.CloseDetailsSideMsg); !ok {
		t.Fatalf("y after dirty esc: got %T, want CloseDetailsSideMsg", runCmd(cmd))
	}
}

func TestSaveWritesAndClosesWithRefresh(t *testing.T) {
	m, s, taskID := loaded(t, "old notes")

	// Type a character into the entry Title field so the draft is dirty.
	m = typeRune(t, m, '!')

	_, cmd := updateModel(m, tea.KeyPressMsg{Text: "ctrl+s", Mod: tea.ModCtrl, Code: 's'})
	msg := runCmd(cmd)
	closeMsg, ok := msg.(cmds.CloseDetailsSideMsg)
	if !ok {
		t.Fatalf("ctrl+s: got %T, want CloseDetailsSideMsg", msg)
	}
	if closeMsg.Follow == nil {
		t.Fatal("save close carried no follow refresh command")
	}
	if _, ok := runCmd(closeMsg.Follow).(cmds.RefreshTasksMsg); !ok {
		t.Fatalf("save follow: got %T, want RefreshTasksMsg", runCmd(closeMsg.Follow))
	}

	got, err := s.GetTask(taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !strings.Contains(got.Title, "!") {
		t.Fatalf("save did not persist the edit: stored title = %q", got.Title)
	}
}

func TestRefreshUpdatesCleanButNotDirty(t *testing.T) {
	m, s, taskID := loaded(t, "first")

	// External change lands on a clean panel: it should show through.
	if err := s.SetNotes(taskID, "external"); err != nil {
		t.Fatalf("set notes: %v", err)
	}
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	if m.NotesValue() != "external" {
		t.Fatalf("clean refresh: NotesValue = %q, want %q", m.NotesValue(), "external")
	}

	// Now dirty the draft and push another external change: the draft wins.
	// Move to Notes (Title is the entry field, so tab once) and type into it.
	// A hydrated note parks the cursor on line 1 (see
	// TestMultilineNotesOpenOnTheFirstLine), so the rune lands at the front.
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "tab"})
	m = typeRune(t, m, 'Q')
	if err := s.SetNotes(taskID, "clobbered"); err != nil {
		t.Fatalf("set notes: %v", err)
	}
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	if m.NotesValue() != "Qexternal" {
		t.Fatalf("dirty refresh overwrote the draft: NotesValue = %q", m.NotesValue())
	}
}

func TestLongTitleTruncatedWithinWidth(t *testing.T) {
	m, _, _ := loaded(t, "")
	m.titleInput.SetValue(strings.Repeat("verylongtitle ", 40))

	const width, height = 30, 24
	m, _ = updateModel(m, cmds.SetDetailsLayout(width, height)())

	view := m.View().Content
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("line exceeds panel width %d: %d (%q)", width, w, line)
		}
	}
}

func TestNarrowViewFitsAndSeals(t *testing.T) {
	m, _, _ := loaded(t, "some notes\nover two lines")

	const width, height = 24, 18
	m, _ = updateModel(m, cmds.SetDetailsLayout(width, height)())

	view := m.View().Content
	if w := lipgloss.Width(view); w > width {
		t.Fatalf("view width %d exceeds supplied %d", w, width)
	}
	if h := lipgloss.Height(view); h > height {
		t.Fatalf("view height %d exceeds supplied %d", h, height)
	}
	if appstyles.HasBackgroundBleed(view) {
		t.Fatal("view has background bleed")
	}
}

// typeComment rune-types a character into the comment compose input while it
// is focused, mirroring typeRune for the notes editor.
func typeComment(t *testing.T, m *Model, r rune) *Model {
	t.Helper()
	m, _ = updateModel(m, tea.KeyPressMsg{Text: string(r), Code: r})
	return m
}

// focusCommentZone moves keyboard focus into the comments zone by tab-cycling
// from Title (the entry field) through Notes and Progress.
func focusCommentZone(t *testing.T, m *Model) *Model {
	t.Helper()
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "tab"}) // notes
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "tab"}) // progress
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "tab"}) // priority
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "tab"}) // comments
	return m
}

// beginCompose focuses the comments zone and opens the inline compose card with
// the c shortcut, leaving the compose input focused and ready to type into.
func beginCompose(t *testing.T, m *Model) *Model {
	t.Helper()
	m = focusCommentZone(t, m)
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "c", Code: 'c'})
	if !m.Composing() {
		t.Fatal("c in the comments zone did not open the compose card")
	}
	return m
}

// TestCommentsAppearAfterRefresh verifies RefreshDetails hydrates the task's
// comment thread into the panel. Ordering by created_at is the store's
// responsibility (TestAddCommentAndListComments pins it); here we assert the
// panel surfaces every comment the store returns, with the author and note
// intact (§6, Commit 5).
func TestCommentsAppearAfterRefresh(t *testing.T) {
	m, s, taskID := loaded(t, "notes")
	if _, err := s.AddComment(taskID, "alice", "second"); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	if _, err := s.AddComment(taskID, "bob", "first"); err != nil {
		t.Fatalf("add comment: %v", err)
	}

	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())

	got := m.Comments()
	if len(got) != 2 {
		t.Fatalf("want 2 comments, got %d", len(got))
	}
	want := map[string]bool{"alice": false, "bob": false}
	for _, c := range got {
		if _, ok := want[c.Author]; ok && (c.Author == "alice" && c.Note == "second") || (c.Author == "bob" && c.Note == "first") {
			want[c.Author] = true
		}
	}
	for author, seen := range want {
		if !seen {
			t.Errorf("comment by %q not surfaced after refresh", author)
		}
	}
}

// TestPostCommentAppearsImmediately verifies that posting a comment through
// the inline compose card appends it to the live thread without a poll
// round-trip, and that enter is the submit key (§6, Commit 5).
func TestPostCommentAppearsImmediately(t *testing.T) {
	m, s, taskID := loaded(t, "")
	m = beginCompose(t, m)
	for _, r := range "hello world" {
		m = typeComment(t, m, r)
	}
	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "enter"})
	// Posting a comment produces no top-level command: it mutates the panel
	// in place (the comment is appended, the card closed).
	if cmd != nil {
		t.Fatalf("postComment produced a command %T, want nil", runCmd(cmd))
	}

	got := m.Comments()
	if len(got) != 1 {
		t.Fatalf("want 1 comment after post, got %d", len(got))
	}
	if got[0].Note != "hello world" {
		t.Errorf("posted comment note = %q, want %q", got[0].Note, "hello world")
	}
	if m.Composing() {
		t.Error("compose card still open after posting")
	}
	if m.CommentInputValue() != "" {
		t.Errorf("compose input not cleared after post: %q", m.CommentInputValue())
	}

	// The comment is persisted in the store, too.
	stored, err := s.ListComments(taskID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(stored) != 1 || stored[0].Note != "hello world" {
		t.Errorf("store has %v, want one comment %q", stored, "hello world")
	}
}

// TestCtrlSWhileComposingDoesNotPost verifies the compose card and the
// notes/progress save path are independent: ctrl+s is never bound to posting a
// comment, even while the compose card owns the keyboard (§6 — "NOT
// ctrl+s"). Posting is enter; ctrl+s while composing is a no-op that keeps
// the draft in place.
func TestCtrlSWhileComposingDoesNotPost(t *testing.T) {
	m, _, _ := loaded(t, "")
	m = beginCompose(t, m)
	for _, r := range "unsent draft" {
		m = typeComment(t, m, r)
	}

	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "ctrl+s", Mod: tea.ModCtrl, Code: 's'})
	if _, ok := runCmd(cmd).(cmds.CloseDetailsSideMsg); ok {
		t.Fatal("ctrl+s while composing closed the panel, want it swallowed")
	}
	// The comment draft was NOT posted — it never became a comment.
	if len(m.Comments()) != 0 {
		t.Errorf("ctrl+s posted a comment unexpectedly: %d comments", len(m.Comments()))
	}
	if !m.Composing() {
		t.Error("ctrl+s closed the compose card, want it kept open")
	}
	if m.CommentInputValue() != "unsent draft" {
		t.Errorf("ctrl+s dropped the draft: %q", m.CommentInputValue())
	}
}

// TestComposeDraftSurvivesPollRefresh pins the fix for the "comment input goes
// blank after a few characters" bug: a poll RefreshDetails landing mid-compose
// must not wipe the half-typed draft (the input is cleared only when the
// compose card is closed).
func TestComposeDraftSurvivesPollRefresh(t *testing.T) {
	m, s, taskID := loaded(t, "notes")
	m = beginCompose(t, m)
	for _, r := range "half typed" {
		m = typeComment(t, m, r)
	}

	// A background poll refresh for the same task arrives while composing.
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())

	if !m.Composing() {
		t.Fatal("poll refresh closed the compose card")
	}
	if m.CommentInputValue() != "half typed" {
		t.Errorf("poll refresh wiped the compose draft: %q", m.CommentInputValue())
	}
}

// TestPostCommentRefusedWhenDisabled verifies a comment on a task in a
// comments-disabled list surfaces the store error as an in-panel message.
func TestPostCommentRefusedWhenDisabled(t *testing.T) {
	m, s, listID, _ := loadedWithList(t, "")
	m = beginCompose(t, m)
	for _, r := range "blocked" {
		m = typeComment(t, m, r)
	}
	if err := s.SetCommentsDisabled(listID, true); err != nil {
		t.Fatalf("disable comments: %v", err)
	}
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "enter"})
	if m.errMsg == "" {
		t.Fatal("expected an error message for posting on a disabled list")
	}
	if len(m.Comments()) != 0 {
		t.Errorf("comment posted on a disabled list: %d comments", len(m.Comments()))
	}
}

// TestPostCommentEmptyIsNoOp verifies an empty/whitespace-only comment input
// does nothing on ctrl+enter (no blank comment row).
func TestPostCommentEmptyIsNoOp(t *testing.T) {
	m, _, _ := loaded(t, "")
	m = beginCompose(t, m)
	// Enter with an empty draft is a no-op — no blank comment is posted.
	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "enter"})
	if cmd != nil {
		t.Fatalf("empty post produced a command %T, want nil", runCmd(cmd))
	}
	if len(m.Comments()) != 0 {
		t.Errorf("empty post added %d comments, want 0", len(m.Comments()))
	}
}

// TestTitleEditableSavesRename verifies the title field is editable and a
// dirty title is persisted through the store on save (Title is the entry field,
// the first in the cycle, so no tabbing is needed to reach it).
func TestTitleEditableSavesRename(t *testing.T) {
	m, s, taskID := loaded(t, "")

	// Title is the entry field and already focused after hydration.
	if m.focus != focusTitle {
		t.Fatalf("focus = %d, want focusTitle after hydration", m.focus)
	}

	m = typeRune(t, m, '!')
	if !m.hasDirtyFields() {
		t.Fatal("editing the title did not mark the draft dirty")
	}

	_, cmd := updateModel(m, tea.KeyPressMsg{Text: "ctrl+s", Mod: tea.ModCtrl, Code: 's'})
	if _, ok := runCmd(cmd).(cmds.CloseDetailsSideMsg); !ok {
		t.Fatalf("ctrl+s after title edit: got %T, want CloseDetailsSideMsg", runCmd(cmd))
	}

	got, err := s.GetTask(taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Title != "Water plants!" {
		t.Fatalf("title not persisted: stored title = %q", got.Title)
	}
}

// TestEmptyTitleRefusedOnSave verifies a title cleared to whitespace is refused
// in place (the store forbids an empty title) rather than closing the modal.
func TestEmptyTitleRefusedOnSave(t *testing.T) {
	m, _, _ := loaded(t, "")
	// Title is the entry field and already focused after hydration.
	m.titleInput.SetValue("")

	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "ctrl+s", Mod: tea.ModCtrl, Code: 's'})
	if runCmd(cmd) != nil {
		t.Fatalf("ctrl+s with empty title: got %T, want nil (refused in place)", runCmd(cmd))
	}
	if m.errMsg == "" {
		t.Error("expected an error message for an empty title")
	}
}

// TestCopyTaskIDYanksID verifies ctrl+y copies the task id from anywhere in the
// modal and flashes a confirmation naming it.
func TestCopyTaskIDYanksID(t *testing.T) {
	m, _, taskID := loaded(t, "")
	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "ctrl+y", Mod: tea.ModCtrl, Code: 'y'})
	if cmd == nil {
		t.Fatal("ctrl+y produced no command — expected a clipboard write")
	}
	if !strings.Contains(m.flash, taskID) {
		t.Fatalf("flash = %q, want it to name the task id %q", m.flash, taskID)
	}
}

// TestCommentCopyYanksSelectedID verifies that in the comments zone ↑/↓ move
// the highlight and y copies the highlighted comment's id: it emits a command
// (tea.SetClipboard, OSC-52) and flashes a confirmation naming the id.
func TestCommentCopyYanksSelectedID(t *testing.T) {
	m, s, taskID := loaded(t, "")
	if _, err := s.AddComment(taskID, "a", "first"); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	if _, err := s.AddComment(taskID, "b", "second"); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	m, _ = updateModel(m, cmds.SetDetailsLayout(80, 24)())

	// Tab title → notes → progress → priority → comments (the thread is
	// non-empty, so the comments zone is in the cycle).
	m = focusCommentZone(t, m)
	if m.focus != focusComments {
		t.Fatalf("focus = %d, want focusComments after four tabs with comments present", m.focus)
	}

	// Move the highlight to the second card, then copy it.
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "down"})
	if m.SelectedCommentIndex() != 1 {
		t.Fatalf("selected = %d, want 1 after down", m.SelectedCommentIndex())
	}
	wantID := m.Comments()[1].ID
	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "y", Code: 'y'})
	if cmd == nil {
		t.Fatal("y produced no command — expected a clipboard write")
	}
	if !strings.Contains(m.flash, wantID) {
		t.Fatalf("flash = %q, want it to name copied id %q", m.flash, wantID)
	}

	// The flash clears on the next keystroke (a plain navigation move).
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "up"})
	if m.flash != "" {
		t.Fatalf("flash = %q, want cleared after the next key", m.flash)
	}
}

// TestComposeCardRendersAuthorAndDraft verifies the inline compose card renders
// without overflowing its box, showing the OS author and the in-progress draft
// alongside the existing thread (the "fake card" the c shortcut opens).
func TestComposeCardRendersAuthorAndDraft(t *testing.T) {
	m, s, taskID := loaded(t, "a note")
	if _, err := s.AddComment(taskID, "alice", "existing comment"); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())

	const width, height = 72, 34
	m, _ = updateModel(m, cmds.SetDetailsLayout(width, height)())
	m = beginCompose(t, m)
	for _, r := range "my draft" {
		m = typeComment(t, m, r)
	}

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "my draft") {
		t.Errorf("compose card did not render the draft:\n%s", view)
	}
	if !strings.Contains(view, osUser()) {
		t.Errorf("compose card did not render the author %q:\n%s", osUser(), view)
	}
	if !strings.Contains(view, "existing comment") {
		t.Errorf("compose card hid the existing thread:\n%s", view)
	}
	for _, line := range strings.Split(m.View().Content, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("composing line exceeds width %d: %d (%q)", width, w, line)
		}
	}
	if appstyles.HasBackgroundBleed(m.View().Content) {
		t.Fatal("composing view has background bleed")
	}
}

// TestNotesCapKeepsCommentsVisible verifies a large note does not swallow the
// modal: the notes textarea is capped so at least reservedCommentCards worth of
// comment cards stay visible (the "notes shouldn't occupy so much space" ask).
func TestNotesCapKeepsCommentsVisible(t *testing.T) {
	m, s, taskID := loaded(t, strings.Repeat("a line\n", 50))
	for i := 0; i < 5; i++ {
		if _, err := s.AddComment(taskID, "a", "c"); err != nil {
			t.Fatalf("add comment: %v", err)
		}
	}
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	m, _ = updateModel(m, cmds.SetDetailsLayout(80, 40)())

	flex := m.flexRows()
	reserved := m.commentsReservedRows()
	notes := m.notesRows()
	if notes > flex-reserved {
		t.Errorf("notes rows %d exceed the cap %d (flex %d - reserved %d)", notes, flex-reserved, flex, reserved)
	}
	if got := flex - notes; got < reserved {
		t.Errorf("only %d rows left for comments, want at least %d", got, reserved)
	}
}

// TestCommentsRenderOldestFirst verifies the rendered view emits comments in
// created_at ascending order (docs/DESIGN.md §12). AddComment stamps
// created_at at second resolution, so the two comments are written a second
// apart to guarantee a deterministic ORDER BY — without the gap they would tie
// and the sort order would be unspecified, which would make this assertion
// flaky.
func TestCommentsRenderOldestFirst(t *testing.T) {
	m, s, taskID := loaded(t, "")
	if _, err := s.AddComment(taskID, "b", "beta"); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := s.AddComment(taskID, "a", "alpha"); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())

	// A tall, wide box so both spaced/wrapped comment cards fit in the window.
	const width, height = 60, 40
	m, _ = updateModel(m, cmds.SetDetailsLayout(width, height)())
	view := ansi.Strip(m.View().Content)

	// "beta" (added first, older created_at) must appear before "alpha".
	ib := strings.Index(view, "beta")
	ia := strings.Index(view, "alpha")
	if ib < 0 || ia < 0 {
		t.Fatalf("expected both comment notes in %q", view)
	}
	if ib >= ia {
		t.Errorf("beta (older) at %d must come before alpha (newer) at %d", ib, ia)
	}
	if appstyles.HasBackgroundBleed(m.View().Content) {
		t.Fatal("view has background bleed")
	}
}

// TestNotesFieldHasNoLineNumberGutter pins the removal of the notes field's
// line-number gutter. It was a code-editor affordance on a to-do note: these
// notes are a few lines of prose, never something a reader cites by line, so
// the column only spent width and made the field read as an editor pane.
func TestNotesFieldHasNoLineNumberGutter(t *testing.T) {
	m, s, taskID := loaded(t, "line one\nline two\nline three")
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	m, _ = updateModel(m, cmds.SetDetailsLayout(80, 40)())

	view := ansi.Strip(m.View().Content)
	for _, gutter := range []string{"┃  1", "┃ 1 ", "┃   1"} {
		if strings.Contains(view, gutter) {
			t.Errorf("notes field still renders a line-number gutter (%q):\n%s", gutter, view)
		}
	}
	// The prompt bar itself stays — it is the field's left edge — and the text
	// now sits one space after it.
	if !strings.Contains(view, "┃ line one") {
		t.Errorf("notes text must sit directly after the prompt bar, got:\n%s", view)
	}
}

// TestNotesCursorLineIsLifted keeps the surviving half of the notes theming
// work: the line the cursor is on renders on BackgroundElevated, the same
// "this is where the keyboard is" lift the Details modal's percentage field
// and its highlighted comment card use (docs/DESIGN.md §12). Only the gutter's
// own colours went away with the gutter.
func TestNotesCursorLineIsLifted(t *testing.T) {
	m, s, taskID := loaded(t, "line one\nline two\nline three")
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	m, _ = updateModel(m, cmds.SetDetailsLayout(80, 40)())

	view := m.View().Content

	probe := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextPrimary).
		Background(appstyles.Active.BackgroundElevated).
		Render("x")
	sgr := probe[:strings.Index(probe, "x")]
	if !strings.Contains(view, sgr) {
		t.Errorf("the notes cursor line must render on BackgroundElevated:\n%s", ansi.Strip(view))
	}
}

// TestFocusedFieldLiftsToElevated pins the focus signal for the Title and
// Notes fields. Neither has a frame or a border of its own, and until now the
// only thing marking one as active was its section label turning bold — easy
// to miss on a modal with four zones. Both now lift to BackgroundElevated
// while they hold the keyboard, the app's one way of showing focus
// (docs/DESIGN.md §12), and drop back flush with the modal when they do not.
func TestFocusedFieldLiftsToElevated(t *testing.T) {
	// Probe with the same foreground/background pair the fields render, since
	// lipgloss emits one combined SGR rather than a bare background.
	lifted := func(m *Model) bool {
		probe := lipgloss.NewStyle().
			Foreground(appstyles.Active.TextPrimary).
			Background(appstyles.Active.BackgroundElevated).
			Render("x")
		sgr := probe[:strings.Index(probe, "x")]
		return strings.Contains(m.View().Content, sgr)
	}

	for _, zone := range []struct {
		focus int
		name  string
	}{{focusTitle, "Title"}, {focusNotes, "Notes"}} {
		m, s, taskID := loaded(t, "line one\nline two")
		m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
		m, _ = updateModel(m, cmds.SetDetailsLayout(80, 40)())
		m = zoneFor(t, m, zone.focus)
		m, _ = updateModel(m, cmds.SetDetailsLayout(80, 40)())

		if !lifted(m) {
			t.Errorf("%s must lift to BackgroundElevated while focused", zone.name)
		}
	}

	// With focus parked on Progress in a mode that has no editable value,
	// nothing in the modal claims the lift.
	m, s, taskID := loaded(t, "line one\nline two")
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	m, _ = updateModel(m, cmds.SetDetailsLayout(80, 40)())
	m = zoneFor(t, m, focusProgress)
	m.progressKind = apptypes.ProgressSimple
	m, _ = updateModel(m, cmds.SetDetailsLayout(80, 40)())
	if lifted(m) {
		t.Error("no field should read as focused when the keyboard is on a valueless Progress mode")
	}
}

// longNote is taller than the notes textarea's row cap, so the textarea has to
// scroll and it matters which end it scrolls to.
const longNote = "first line\nsecond\nthird\nfourth\nfifth\nsixth\nseventh\neighth\nninth\nlast line"

// A note taller than the cap opens on its first line and stays there across a
// poll refresh. SetValue leaves the cursor at the end of the buffer and the
// textarea repositions onto the cursor on every SetHeight, so the note used to
// jump to its last line one poll tick after the modal opened.
func TestMultilineNotesOpenOnTheFirstLine(t *testing.T) {
	m, s, taskID := loaded(t, longNote)
	m, _ = updateModel(m, cmds.SetDetailsLayout(60, 24)())

	assertFirstLine := func(when string) {
		t.Helper()
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "first line") {
			t.Fatalf("%s: notes do not show their first line:\n%s", when, view)
		}
		if strings.Contains(view, "last line") {
			t.Fatalf("%s: notes are scrolled to their last line:\n%s", when, view)
		}
	}

	assertFirstLine("on open")

	// The poll loop re-hydrates the open modal. Rendering first matters: the
	// textarea only scrolls once its viewport has content, which is what made
	// this surface a tick after opening rather than immediately.
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	assertFirstLine("after a poll refresh")
}

// The row budget is derived from the note's line count, so it has to be
// computed after the note is written — otherwise a long note opens at the
// height the previous (empty) task needed and grows a tick later.
//
// The box is 30 rows rather than 24: the Priority zone added two fixed rows
// (detailsFixedRows), and at 24 the flexible budget is exactly minNotesRows,
// so the note could not grow past the floor no matter how the budget was
// computed — which would make this test pass for the wrong reason.
func TestNotesSizedToTheNoteOnOpen(t *testing.T) {
	m, _, _ := loaded(t, longNote)
	m, _ = updateModel(m, cmds.SetDetailsLayout(60, 30)())

	view := ansi.Strip(m.View().Content)
	rows := strings.Count(view, m.notes.Prompt)
	if want := m.notesRows(); rows != want {
		t.Fatalf("notes rendered %d rows, want the budgeted %d:\n%s", rows, want, view)
	}
	if rows <= minNotesRows {
		t.Fatalf("a %d-line note rendered at %d rows, no taller than the minimum", strings.Count(longNote, "\n")+1, rows)
	}
}

// The compose input renders inside the compose card, so every state it can be
// in has to paint on the card's tier. Sealing it onto the modal background
// instead cut a modal-coloured stripe through the middle of the card, which
// read as the card being broken. The card fill and the input used to name
// their tier independently and agreed only by coincidence; this pins them
// together in both the empty (placeholder) and drafted states.
func TestComposeInputPaintsOnTheCardBackground(t *testing.T) {
	// Probe with the foreground/background pair the input actually renders,
	// since lipgloss emits one combined SGR rather than a bare background.
	sgrFor := func(fg, bg color.Color) string {
		probe := lipgloss.NewStyle().Foreground(fg).Background(bg).Render("x")
		return probe[:strings.Index(probe, "x")]
	}
	onModal := func(fg color.Color) string { return sgrFor(fg, appstyles.Active.ModalBg) }
	onCard := func(fg color.Color) string { return sgrFor(fg, composeCardBg()) }

	open := func(t *testing.T) *Model {
		t.Helper()
		m, _, _ := loaded(t, "a note")
		m, _ = updateModel(m, cmds.SetDetailsLayout(60, 24)())
		m = zoneFor(t, m, focusComments)
		m, _ = updateModel(m, tea.KeyPressMsg{Text: "c", Code: 'c'})
		if !m.Composing() {
			t.Fatal("c did not open the compose card")
		}
		return m
	}

	t.Run("placeholder", func(t *testing.T) {
		view := open(t).View().Content
		if strings.Contains(view, onModal(appstyles.Active.TextDim)) {
			t.Errorf("the compose placeholder paints on the modal background, not the card:\n%s", ansi.Strip(view))
		}
		if !strings.Contains(view, onCard(appstyles.Active.TextDim)) {
			t.Errorf("the compose placeholder does not paint on the card background:\n%s", ansi.Strip(view))
		}
	})

	t.Run("draft", func(t *testing.T) {
		m := open(t)
		m = typeRune(t, m, 'h')
		m = typeRune(t, m, 'i')
		view := m.View().Content
		if !strings.Contains(view, onCard(appstyles.Active.TextPrimary)+"hi") {
			t.Errorf("the compose draft does not paint on the card background:\n%s", ansi.Strip(view))
		}
	})

	// The panel can lose the keyboard with the card still open — a theme
	// picker over the modal, say. The card is still on screen, so the input
	// still has to match it.
	t.Run("blurred", func(t *testing.T) {
		m := open(t)
		m = typeRune(t, m, 'h')
		m.commentInput.Blur()
		view := m.View().Content
		if strings.Contains(view, onModal(appstyles.Active.TextPrimary)+"h") {
			t.Errorf("the blurred compose draft paints on the modal background:\n%s", ansi.Strip(view))
		}
	})
}
