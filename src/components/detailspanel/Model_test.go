package detailspanel

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/appstyles"
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
	m = typeRune(t, m, 'x') // draft is now dirty

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
	if m.NotesValue() != "x" {
		t.Fatalf("n dropped the draft: NotesValue = %q", m.NotesValue())
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

	// Type a character so the draft is dirty and distinct.
	m = typeRune(t, m, 'Z')

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
	if !strings.Contains(got.Notes, "Z") {
		t.Fatalf("save did not persist the edit: stored notes = %q", got.Notes)
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
	m = typeRune(t, m, 'Q')
	if err := s.SetNotes(taskID, "clobbered"); err != nil {
		t.Fatalf("set notes: %v", err)
	}
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	if m.NotesValue() != "externalQ" {
		t.Fatalf("dirty refresh overwrote the draft: NotesValue = %q", m.NotesValue())
	}
}

func TestLongTitleTruncatedWithinWidth(t *testing.T) {
	m, _, _ := loaded(t, "")
	m.title = strings.Repeat("verylongtitle ", 40)

	const width, height = 30, 24
	m, _ = updateModel(m, cmds.SetBodyLayout(height, 0, width, 0, width)())

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
	m, _ = updateModel(m, cmds.SetBodyLayout(height, 0, width, 0, width)())

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

// focusComments moves keyboard focus into the comment compose zone by
// tab-cycling from notes past progress.
func focusCommentZone(t *testing.T, m *Model) *Model {
	t.Helper()
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "tab"})
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "tab"})
	return m
}

// TestCommentsAppearAfterRefresh verifies RefreshDetails hydrates the task's
// comment thread into the panel. Ordering by created_at is the store's
// responsibility (TestAddCommentAndListComments pins it); here we assert the
// panel surfaces every comment the store returns, with the author and note
// intact (docs/plan/task-comments.md §6, Commit 5).
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
// the compose input appends it to the live thread without a poll round-trip
// (docs/plan/task-comments.md §6, Commit 5).
func TestPostCommentAppearsImmediately(t *testing.T) {
	m, s, taskID := loaded(t, "")
	m = focusCommentZone(t, m)
	for _, r := range "hello world" {
		m = typeComment(t, m, r)
	}
	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "ctrl+enter", Mod: tea.ModCtrl})
	// Posting a comment produces no top-level command: it mutates the panel
	// in place (the comment is appended, the input cleared).
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

// TestPostCommentDoesNotTriggerCtrlSSave verifies the compose input and the
// notes/progress save path are independent: ctrl+s (save) is never bound to
// posting a comment, even when focus is on the compose input (docs/plan/task-comments.md
// §6 — "NOT ctrl+s"). When focused on comments, ctrl+s saves notes/progress
// (closing the panel here, since notes are clean) and leaves the unsent draft
// in the compose input — it never posts it.
func TestPostCommentDoesNotTriggerCtrlSSave(t *testing.T) {
	m, _, _ := loaded(t, "")
	m = focusCommentZone(t, m)
	for _, r := range "unsent draft" {
		m = typeComment(t, m, r)
	}

	// ctrl+s while focused on comments saves notes/progress (a clean save
	// closes the panel) and must NOT consume the keystroke as a comment post.
	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "ctrl+s", Mod: tea.ModCtrl, Code: 's'})
	if _, ok := runCmd(cmd).(cmds.CloseDetailsSideMsg); !ok {
		t.Fatalf("ctrl+s while on comments: got %T, want CloseDetailsSideMsg (save-then-close)", runCmd(cmd))
	}
	// The comment draft was NOT posted — it never became a comment.
	if len(m.Comments()) != 0 {
		t.Errorf("ctrl+s posted a comment unexpectedly: %d comments", len(m.Comments()))
	}
}

// TestPostCommentRefusedWhenDisabled verifies a comment on a task in a
// comments-disabled list surfaces the store error as an in-panel message.
func TestPostCommentRefusedWhenDisabled(t *testing.T) {
	m, s, listID, _ := loadedWithList(t, "")
	m = focusCommentZone(t, m)
	for _, r := range "blocked" {
		m = typeComment(t, m, r)
	}
	if err := s.SetCommentsDisabled(listID, true); err != nil {
		t.Fatalf("disable comments: %v", err)
	}
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "ctrl+enter", Mod: tea.ModCtrl})
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
	m = focusCommentZone(t, m)
	// Whitespace-only drafts are a no-op.
	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "ctrl+enter", Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatalf("empty post produced a command %T, want nil", runCmd(cmd))
	}
	if len(m.Comments()) != 0 {
		t.Errorf("empty post added %d comments, want 0", len(m.Comments()))
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

	const width, height = 40, 24
	m, _ = updateModel(m, cmds.SetBodyLayout(height, 0, width, 0, width)())
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
