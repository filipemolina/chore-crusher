package helpoverlay

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/keys"
)

// bindingsOf reflects over one of the keymap structs in src/keys and returns
// every exported key.Binding field, with the field's name for reporting.
//
// Reflection rather than a hand-written slice is the whole point: a slice
// would have to be kept in step with the struct by the same discipline that
// already failed once (n, the task tree's own create key, was bound and
// handled but appeared nowhere in the overlay). Adding a field to a keymap
// struct without documenting it has to fail this test on its own.
func bindingsOf(t *testing.T, group string, keymap any) map[string]key.Binding {
	t.Helper()
	v := reflect.ValueOf(keymap)
	out := make(map[string]key.Binding, v.NumField())
	for i := range v.NumField() {
		f := v.Type().Field(i)
		if !f.IsExported() || f.Type != reflect.TypeOf(key.Binding{}) {
			continue
		}
		out[group+"."+f.Name] = v.Field(i).Interface().(key.Binding)
	}
	if len(out) == 0 {
		t.Fatalf("%s: reflected no bindings; the struct shape changed", group)
	}
	return out
}

// allBindings is every declared binding in the app, across every keymap the
// overlay is responsible for documenting.
func allBindings(t *testing.T) map[string]key.Binding {
	t.Helper()
	all := map[string]key.Binding{}
	for group, keymap := range map[string]any{
		"Global":  keys.Global,
		"Tree":    keys.Tree,
		"Lists":   keys.Lists,
		"Create":  keys.Create,
		"Details": keys.Details,
		"Overlay": keys.Overlay,
	} {
		for name, b := range bindingsOf(t, group, keymap) {
			all[name] = b
		}
	}
	return all
}

// THE guard this package exists for: every key declared in src/keys appears
// in the rendered overlay, from ONE screen — the overlay is a reference for
// the whole app, not a description of the corner the user happened to open it
// from. A key that works but is not advertised is a key nobody can discover
// (CONTRIBUTING.md, "Do not invent a keybinding").
//
// This is what would have caught the missing n: it was declared as
// keys.Tree.New, matched by the tree's handler, advertised by the footer and
// named by the empty state, but the overlay's hand-written Task Tree list
// simply did not mention it — so help taught a reader how to create a list
// and not how to create a task.
func TestOverlayDocumentsEveryBinding(t *testing.T) {
	// The emptiest possible screen: nothing focused, no list, no panel. Even
	// here the overlay documents the whole app; only availability changes.
	// Tall enough that nothing is windowed out — TestCatalogFitsASmallTerminal
	// covers what a short terminal does with the same catalog.
	view := ansi.Strip(New(keys.Context{}, 100, 200).View().Content)

	for name, b := range allBindings(t) {
		help := b.Help()
		if help.Key == "" && help.Desc == "" {
			t.Errorf("%s has no help text, so the overlay cannot advertise it", name)
			continue
		}
		if !strings.Contains(view, help.Key) {
			t.Errorf("%s (%q %q) is bound but does not appear in the help overlay:\n%s",
				name, help.Key, help.Desc, view)
		}
	}
}

// Every scope carries entries in every context. Scopes used to appear and
// disappear with the screen — Lists only while the lists panel was visible,
// Task Tree only with an active list — so the overlay could not be used to
// find a key for a surface the user had not already reached.
func TestEveryScopeIsAlwaysPresent(t *testing.T) {
	titles := func(ctx keys.Context) []string {
		var out []string
		for _, s := range keys.Catalog(ctx) {
			if len(s.Entries) == 0 {
				t.Errorf("scope %q is empty", s.Title)
			}
			out = append(out, s.Title)
		}
		return out
	}

	base := titles(keys.Context{})
	for name, ctx := range map[string]keys.Context{
		"lists visible":   {ListsPanelVisible: true},
		"active list":     {HasActiveList: true},
		"creating":        {HasActiveList: true, Creating: true},
		"filtering":       {HasActiveList: true, Filtering: true},
		"details open":    {DetailsPanelVisible: true},
		"modal owns keys": {HasModal: true},
	} {
		if got := titles(ctx); !reflect.DeepEqual(got, base) {
			t.Errorf("%s: scopes = %v, want the same set as every other screen %v", name, got, base)
		}
	}
}

// The catalog is longer than a terminal, so the overlay must fit the terminal
// it is drawn on and scroll rather than run off the bottom. An overlay whose
// tail is unreachable documents those keys no better than omitting them.
func TestCatalogFitsASmallTerminalAndScrolls(t *testing.T) {
	const w, h = 80, 24
	m := New(keys.Context{}, w, h).(Model)

	if got := lipgloss.Height(m.View().Content); got > h {
		t.Fatalf("overlay is %d rows on a %d-row terminal:\n%s", got, h, ansi.Strip(m.View().Content))
	}
	if m.maxOffset() == 0 {
		t.Fatal("precondition: the catalog should not fit an 80x24 terminal")
	}

	first := ansi.Strip(m.View().Content)
	if !strings.Contains(first, "below") {
		t.Errorf("a windowed catalog must say how much is hidden:\n%s", first)
	}

	// ↓ scrolls, and the last line of the catalog is reachable.
	last := m.contentLines()[len(m.contentLines())-1]
	for range m.maxOffset() {
		u, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = u.(Model)
	}
	end := ansi.Strip(m.View().Content)
	if !strings.Contains(end, ansi.Strip(last)) {
		t.Errorf("the end of the catalog is not reachable by scrolling:\n%s", end)
	}
	if strings.Contains(end, "below") {
		t.Errorf("still reports hidden lines below at the bottom:\n%s", end)
	}

	// And it stops there rather than scrolling past the content.
	u, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := u.(Model).offset; got != m.maxOffset() {
		t.Errorf("scrolled past the end: offset = %d, max = %d", got, m.maxOffset())
	}
}
