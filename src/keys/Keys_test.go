package keys

import (
	"testing"

	"charm.land/bubbles/v2/key"
)

// The fixed global bindings are part of docs/DESIGN.md §5 — pin them so a
// refactor cannot silently move a key the docs promise.
func TestGlobalBindingsAreFixed(t *testing.T) {
	cases := []struct {
		name string
		b    key.Binding
		want string
	}{
		{"TogglePanels", Global.ToggleListsPanel, "L"},
		{"Help", Global.Help, "?"},
		{"Quit", Global.Quit, "q"},
		{"ForceQuit", Global.ForceQuit, "ctrl+c"},
		{"Theme", Global.Theme, "T"},
		{"Filter", Global.Filter, "/"},
		{"Picker", Global.Picker, "F"},
		{"NextPanel", Global.NextPanel, "ctrl+right"},
		{"PrevPanel", Global.PrevPanel, "ctrl+left"},
	}

	for _, tc := range cases {
		keys := tc.b.Keys()
		if len(keys) != 1 || keys[0] != tc.want {
			t.Errorf("%s binds %v, want %q", tc.name, keys, tc.want)
		}
	}
}

// The esc ladder (docs/DESIGN.md §5) is one binding shared by every claim on
// esc — a modal's cancel and the app's back must be the same key.
func TestEscIsOneBinding(t *testing.T) {
	if len(Global.Back.Keys()) != 1 || Global.Back.Keys()[0] != "esc" {
		t.Errorf("Global.Back binds %v, want esc", Global.Back.Keys())
	}
	if len(Overlay.Cancel.Keys()) != 1 || Overlay.Cancel.Keys()[0] != "esc" {
		t.Errorf("Overlay.Cancel binds %v, want esc", Overlay.Cancel.Keys())
	}
}

// The help overlay renders every live binding; a binding the handlers match
// on must appear in the catalog so the overlay cannot drift from the code.
func TestCatalogContainsEveryGlobalBinding(t *testing.T) {
	scopes := Catalog(Context{})

	var bindings []key.Binding
	for _, scope := range scopes {
		for _, e := range scope.Entries {
			bindings = append(bindings, e.Binding)
		}
	}

	for _, g := range []key.Binding{
		Global.NextPanel, Global.PrevPanel, Global.ToggleListsPanel,
		Global.Back, Global.Quit, Global.ForceQuit, Global.Help, Global.Theme,
		Global.Filter, Global.Picker,
	} {
		if !containsBinding(bindings, g) {
			t.Errorf("catalog is missing %q", g.Help().Key)
		}
	}
}

func TestActiveReturnsMovementForEveryZone(t *testing.T) {
	for _, focused := range []int{0, 1, 2} {
		ctx := Context{Focused: focused, ListsPanelVisible: true}
		if len(Active(ctx)) == 0 {
			t.Errorf("Active for zone %d is empty", focused)
		}
	}
}
