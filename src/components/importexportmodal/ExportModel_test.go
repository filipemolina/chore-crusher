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

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	out, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	return out, cmd
}

// runSubmit resolves a submit keypress's command chain fully: the outer cmd
// is cmds.CloseModal's, whose result is a CloseModalMsg carrying the actual
// export work as Follow — running that is what performs the file write.
func runSubmit(t *testing.T, cmd tea.Cmd) tea.Msg {
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

// typePath feeds the rune sequence into a fresh export modal's text input.
func typePath(t *testing.T, m Model, path string) Model {
	t.Helper()
	for _, r := range path {
		m, _ = step(t, m, tea.KeyPressMsg{Text: string(r), Code: rune(r)})
	}
	return m
}

// testTermWidth is the terminal width used in tests: wide enough that the
// input width (termWidth - 6, capped at 50) lands at 50, a deterministic
// value the placeholder test asserts against.
const testTermWidth = 120

func TestNewExportSeedsWholeStoreWhenNoList(t *testing.T) {
	s := openTestStore(t)
	m := NewExport(s, nil, testTermWidth).(Model)
	if !m.wholeStore {
		t.Error("wholeStore = false, want true when no list is highlighted")
	}
}

func TestNewExportSeedsThisListWhenListGiven(t *testing.T) {
	s := openTestStore(t)
	id, err := s.CreateList("Work", "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	m := NewExport(s, &id, testTermWidth).(Model)
	if m.wholeStore {
		t.Error("wholeStore = true, want false when a list is highlighted")
	}
	if m.listID == nil || *m.listID != id {
		t.Error("listID not wired through to the modal")
	}
}

func TestExportSubmitWithEmptyPathWritesNothing(t *testing.T) {
	s := openTestStore(t)
	m := NewExport(s, nil, testTermWidth).(Model)
	_, cmd := step(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})
	if cmd != nil {
		t.Error("submit with empty path should produce no command")
	}
}

func TestExportCancelReturnsCloseModalMsg(t *testing.T) {
	s := openTestStore(t)
	m := NewExport(s, nil, testTermWidth).(Model)
	_, cmd := step(t, m, tea.KeyPressMsg{Text: "esc", Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should produce a CloseModal command")
	}
	if _, ok := cmd().(cmds.CloseModalMsg); !ok {
		t.Fatalf("esc produced %T, want cmds.CloseModalMsg", cmd())
	}
}

// TestExportSubmitWritesWholeStoreFile drives the full input->enter flow and
// asserts the follow command wrote an ExportDocument with every list in it.
func TestExportSubmitWritesWholeStoreFile(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.CreateList("Groceries", "pi"); err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if _, err := s.CreateList("Work", "pi"); err != nil {
		t.Fatalf("CreateList: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "exp.json")
	m := NewExport(s, nil, testTermWidth).(Model)
	m = typePath(t, m, outPath)
	_, cmd := step(t, m, tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})
	runSubmit(t, cmd)

	doc := readExportDoc(t, outPath)
	if len(doc.Lists) != 2 {
		t.Fatalf("whole-store export wrote %d lists, want 2", len(doc.Lists))
	}
}

// TestExportFollowSingleList writes only the highlighted list.
func TestExportFollowSingleList(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.CreateList("Groceries", "pi"); err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if _, err := s.CreateList("Work", "pi"); err != nil {
		t.Fatalf("CreateList: %v", err)
	}

	lists, _ := s.ListLists()
	var groceriesID string
	for _, l := range lists {
		if l.Name == "Groceries" {
			groceriesID = l.ID
		}
	}
	onePath := filepath.Join(t.TempDir(), "one.json")
	m := NewExport(s, &groceriesID, testTermWidth).(Model)
	runSubmit(t, cmds.CloseModal(m.exportFollowCmd(onePath)))

	doc := readExportDoc(t, onePath)
	if len(doc.Lists) != 1 || doc.Lists[0].Name != "Groceries" {
		t.Fatalf("single-list export wrote %+v, want just Groceries", doc.Lists)
	}
}

func TestExportTabTogglesFocusAndSpaceFlipsWholeStore(t *testing.T) {
	s := openTestStore(t)
	m := NewExport(s, nil, testTermWidth).(Model)
	if !m.wholeStore {
		t.Fatal("precondition: wholeStore should start true with no list")
	}

	m, _ = step(t, m, tea.KeyPressMsg{Text: "tab", Code: tea.KeyTab})
	if !m.toggleFocused {
		t.Fatal("tab should have moved focus onto the toggle")
	}
	if m.input.Focused() {
		t.Error("the path input should be blurred while the toggle has focus")
	}

	// space flips wholeStore off (from true)
	m, _ = step(t, m, tea.KeyPressMsg{Text: " ", Code: ' '})
	if m.wholeStore {
		t.Error("space should have flipped wholeStore to false")
	}

	// a second tab returns focus to the path field.
	m, _ = step(t, m, tea.KeyPressMsg{Text: "tab", Code: tea.KeyTab})
	if m.toggleFocused {
		t.Error("a second tab should have returned focus to the path field")
	}
	if !m.input.Focused() {
		t.Error("the path input should be focused again")
	}
	m, _ = step(t, m, tea.KeyPressMsg{Text: " ", Code: ' '})
	if m.input.Value() != " " {
		t.Errorf("input value = %q, want %q (space typed, not intercepted)", m.input.Value(), " ")
	}
}

func TestExportViewContainsTitleAndToggleGlyph(t *testing.T) {
	s := openTestStore(t)
	m := NewExport(s, nil, testTermWidth).(Model)
	v := m.View().Content
	if !strings.Contains(v, "Export") {
		t.Error("View should contain the modal title \"Export\"")
	}
	if !strings.Contains(v, "◻") && !strings.Contains(v, "◼") {
		t.Error("View should contain the whole-store toggle glyph ◻/◼")
	}
}

// TestExportViewShowsFullPlaceholder verifies the path input's placeholder
// renders in full. The bubbles v2 textinput's placeholderView truncates to
// the first character when Width is 0, leaking a stray 'p' (first rune of
// "path/to/file.json") as the cursor char. Setting Width from terminalWidth
// in View fixes this — the full placeholder must appear in the rendered output.
func TestExportViewShowsFullPlaceholder(t *testing.T) {
	s := openTestStore(t)
	m := NewExport(s, nil, testTermWidth).(Model)
	v := ansi.Strip(m.View().Content)
	if !strings.Contains(v, "path/to/file.json") {
		t.Error("View should contain the full placeholder \"path/to/file.json\", not just 'p'")
	}
}

func readExportDoc(t *testing.T, path string) store.ExportDocument {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc store.ExportDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return doc
}
