package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/constants"
	"github.com/filipemolina/farol/src/store"
)

// TestFullFrameDrawsNoDefaultForegroundOnCrushDay pins the whole-app
// foreground invariant against the light theme that exposed the bug class:
// every visible glyph in a rendered frame must carry an explicit foreground
// tier, because a glyph with none draws in the terminal's own default
// color — light on nearly every terminal, invisible on crush-day's warm
// off-white panels. The original report was pending task titles rendering
// white on white; the fix styled the rows and sealed every text input, and
// this test guards all of it at once by asserting
// appstyles.HasDefaultForeground over the entire composed frame.
func TestFullFrameDrawsNoDefaultForegroundOnCrushDay(t *testing.T) {
	if !appstyles.SetTheme("crush-day") {
		t.Fatal("crush-day theme not registered")
	}
	defer appstyles.SetTheme(appstyles.DefaultTheme)

	m := newTestModel(t, t.TempDir())
	listID, err := m.store.CreateList("Errands", "")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	// Seed one of every status the row renderer knows, plus a parent whose
	// expand marker must be styled: pending, in progress (percentage mode),
	// complete, and a parent with a child.
	pendingID, err := m.store.CreateTask(listID, "Buy milk", nil, "")
	if err != nil {
		t.Fatalf("create pending task: %v", err)
	}
	if _, err := m.store.CreateTask(listID, "Half done", nil, ""); err != nil {
		t.Fatalf("create in-progress task: %v", err)
	}
	doneID, err := m.store.CreateTask(listID, "Done thing", nil, "")
	if err != nil {
		t.Fatalf("create complete task: %v", err)
	}
	if err := m.store.Complete(doneID); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	parentID, err := m.store.CreateTask(listID, "Project", nil, "")
	if err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	if _, err := m.store.CreateTask(listID, "Child task", &parentID, ""); err != nil {
		t.Fatalf("create child task: %v", err)
	}

	m = refresh(t, m, cmds.RefreshLists(m.store)())
	m = refresh(t, m, cmds.RefreshTasks(m.store, listID, apptypes.SortManual)())
	// Same startup-focus broadcast seedOneList does: without it the tree
	// renders unfocused, which is a weaker frame than the one users see.
	m = refresh(t, m, cmds.SetFocus(constants.COMPONENT_TASK_TREE)())
	// The keybinding bar and panels only learn their widths from the layout
	// broadcast; re-send the model's own computed layout (the layout_test.go
	// startup trick) so the frame is the full-screen one.
	m = refresh(t, m, m.bodyLayout)

	// Push one task into percentage mode so the IN PROGRESS label and its
	// percentage column render in the frame too.
	pct := 40
	if err := m.store.SetProgress(pendingID, store.ProgressPercentage, &pct); err != nil {
		t.Fatalf("set progress: %v", err)
	}
	m = refresh(t, m, cmds.RefreshTasks(m.store, listID, apptypes.SortManual)())

	out := m.View().Content
	// Precondition: the frame actually shows the seeded rows — a window or a
	// layout bug that drops them would make the assertion below vacuous.
	stripped := ansi.Strip(out)
	for _, want := range []string{"Buy milk", "Half done", "Done thing", "Project", "Child task"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("frame does not show seeded row %q:\n%s", want, stripped)
		}
	}
	if appstyles.HasDefaultForeground(out) {
		t.Errorf("crush-day frame draws glyphs in the terminal default foreground:\n%s", stripped)
	}
}

// TestFullFrameDrawsNoDefaultForegroundOnDefaultTheme runs the same
// assertion under the shipped default theme, so the invariant is pinned on
// both sides of the light/dark split rather than only where the bug was
// first visible.
func TestFullFrameDrawsNoDefaultForegroundOnDefaultTheme(t *testing.T) {
	if !appstyles.SetTheme(appstyles.DefaultTheme) {
		t.Fatalf("default theme %q not registered", appstyles.DefaultTheme)
	}

	m := seedOneList(t)
	m = refresh(t, m, m.bodyLayout)

	out := m.View().Content
	if appstyles.HasDefaultForeground(out) {
		t.Errorf("default-theme frame draws glyphs in the terminal default foreground:\n%s", ansi.Strip(out))
	}
}

// TestListsFilterBarDrawsNoDefaultForegroundOnCrushDay covers the one input
// the frame tests above never draw: the lists panel's filter bar, which is
// the bubbles list's own widget sealed by chrome.SealListFilter and only
// renders while the panel is in its Filtering state. Open the panel,
// activate the filter, type a character, and assert the bar and the panel
// beneath it are clean. Same setup as quit_test.go's lists-filter case.
func TestListsFilterBarDrawsNoDefaultForegroundOnCrushDay(t *testing.T) {
	if !appstyles.SetTheme("crush-day") {
		t.Fatal("crush-day theme not registered")
	}
	defer appstyles.SetTheme(appstyles.DefaultTheme)

	m := showLists(t, seedOneList(t))
	m = refresh(t, m, m.bodyLayout)

	// "/" routes to the lists panel's filter while it has focus. refresh is
	// safe here — the activation cmd terminates — but the typed character
	// must go through press: executing a keypress's cmd would run the filter
	// input's cursor blink chain forever.
	m = refresh(t, m, tea.KeyPressMsg{Text: "/", Code: '/'})
	if !listsFilterActive(t, m) {
		t.Fatal("/ did not open the lists filter")
	}
	m, _ = press(t, m, tea.KeyPressMsg{Text: "e", Code: 'e'})

	out := m.View().Content
	stripped := ansi.Strip(out)
	if !strings.Contains(stripped, "Filter:") {
		t.Fatalf("frame does not show the lists filter bar:\n%s", stripped)
	}
	if appstyles.HasDefaultForeground(out) {
		t.Errorf("crush-day lists filter frame draws glyphs in the terminal default foreground:\n%s", stripped)
	}
}
