package importexportmodal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/store"
)

func TestImportSubmitWithEmptyPathWritesNothing(t *testing.T) {
	s := openTestStore(t)
	m := NewImport(s, testTermWidth).(ImportModel)
	_, cmd := stepImport(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})
	if cmd != nil {
		t.Error("submit with empty path should produce no command")
	}
}

func TestImportCancelReturnsCloseModalMsg(t *testing.T) {
	s := openTestStore(t)
	m := NewImport(s, testTermWidth).(ImportModel)
	_, cmd := stepImport(t, m, tea.KeyPressMsg{Text: "esc", Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should produce a CloseModal command")
	}
	if _, ok := cmd().(cmds.CloseModalMsg); !ok {
		t.Fatalf("esc produced %T, want cmds.CloseModalMsg", cmd())
	}
}

// TestImportBadPathRoutesErrorThroughLastError drives the follow command with
// a non-existent file; the result must be a RefreshListsMsg carrying Err.
func TestImportBadPathRoutesErrorThroughLastError(t *testing.T) {
	s := openTestStore(t)
	m := NewImport(s, testTermWidth).(ImportModel)
	msg := runImportFollow(t, m.importFollowCmd(filepath.Join(t.TempDir(), "nope.json")))
	rlm, ok := msg.(cmds.RefreshListsMsg)
	if !ok {
		t.Fatalf("follow returned %T, want cmds.RefreshListsMsg", msg)
	}
	if rlm.Err == nil {
		t.Error("RefreshListsMsg.Err is nil, want the read failure")
	}
}

// TestImportRecreatesLists drives the full submit flow against a file the
// export modal (or store.Export) produced, asserting the lists reappear with
// their task structure intact.
func TestImportRecreatesLists(t *testing.T) {
	s := openTestStore(t)
	srcID, err := s.CreateList("Groceries", "pi")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	parent, err := s.CreateTask(srcID, "Produce", nil, "buy fresh")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := s.CreateTask(srcID, "Apples", &parent, "red ones"); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Export the list to a file, then import it into the same store to
	// verify additive recreation (the list appears twice, fresh ids).
	doc, err := s.Export(&srcID)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	expPath := filepath.Join(t.TempDir(), "exp.json")
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(expPath, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	before, _ := s.ListLists()
	m := NewImport(s, testTermWidth).(ImportModel)
	m = typeImportPath(t, m, expPath)
	_, cmd := stepImport(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})
	runSubmitImport(t, cmd)

	after, _ := s.ListLists()
	if len(after) != len(before)+1 {
		t.Fatalf("lists after import = %d, want %d", len(after), len(before)+1)
	}
	// The imported list carries the same name; ids differ (additive).
	var found bool
	for _, l := range after {
		if l.Name == "Groceries" && l.ID != srcID {
			found = true
			tasks, err := s.ListTasks(l.ID)
			if err != nil {
				t.Fatalf("ListTasks: %v", err)
			}
			if len(tasks) != 2 {
				t.Errorf("imported list has %d tasks, want 2", len(tasks))
			}
		}
	}
	if !found {
		t.Error("no imported list with fresh id and name \"Groceries\" found")
	}
}

// TestImportVersionMismatchRoutesError feeds a doc with the wrong version and
// asserts the follow command rejects it via lastError.
func TestImportVersionMismatchRoutesError(t *testing.T) {
	s := openTestStore(t)
	path := filepath.Join(t.TempDir(), "badver.json")
	doc := store.ExportDocument{Version: store.ExportVersion + 1, Lists: nil}
	raw, _ := json.MarshalIndent(doc, "", "  ")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	m := NewImport(s, testTermWidth).(ImportModel)
	msg := runImportFollow(t, m.importFollowCmd(path))
	rlm, ok := msg.(cmds.RefreshListsMsg)
	if !ok {
		t.Fatalf("follow returned %T, want cmds.RefreshListsMsg", msg)
	}
	if rlm.Err == nil {
		t.Error("RefreshListsMsg.Err is nil, want the version-mismatch failure")
	}
}

func TestImportViewContainsTitle(t *testing.T) {
	s := openTestStore(t)
	m := NewImport(s, testTermWidth).(ImportModel)
	v := m.View().Content
	if !containsImport(v, "Import") {
		t.Error("View should contain the modal title \"Import\"")
	}
}

// --- import test helpers (kept separate from the export ones to avoid a
// single helper name collision across the package's two test files) ---

func stepImport(t *testing.T, m ImportModel, msg tea.Msg) (ImportModel, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	out, ok := updated.(ImportModel)
	if !ok {
		t.Fatalf("Update returned %T, want ImportModel", updated)
	}
	return out, cmd
}

func typeImportPath(t *testing.T, m ImportModel, path string) ImportModel {
	t.Helper()
	for _, r := range path {
		m, _ = stepImport(t, m, tea.KeyPressMsg{Text: string(r), Code: rune(r)})
	}
	return m
}

func runSubmitImport(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("submit produced no command")
	}
	closeMsg, ok := cmd().(cmds.CloseModalMsg)
	if !ok {
		t.Fatalf("submit produced %T, want cmds.CloseModalMsg", cmd())
	}
	if closeMsg.Follow == nil {
		t.Fatal("CloseModalMsg.Follow is nil")
	}
	return closeMsg.Follow()
}

func runImportFollow(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("follow command is nil")
	}
	return cmd()
}

func containsImport(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// TestImportViewShowsFullPlaceholder verifies the path input's placeholder
// renders in full. The bubbles v2 textinput's placeholderView truncates to
// the first character when Width is 0, leaking a stray 'p' (first rune of
// "path/to/file.json") as the cursor char. Setting Width from terminalWidth
// in View fixes this — the full placeholder must appear in the rendered output.
func TestImportViewShowsFullPlaceholder(t *testing.T) {
	s := openTestStore(t)
	m := NewImport(s, testTermWidth).(ImportModel)
	v := ansi.Strip(m.View().Content)
	if !containsImport(v, "path/to/file.json") {
		t.Error("View should contain the full placeholder \"path/to/file.json\", not just 'p'")
	}
}
