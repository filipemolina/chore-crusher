package keys

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/filipemolina/chore-crusher/src/constants"
)

// The fixed global bindings are part of docs/DESIGN.md §5 — pin them so a
// refactor cannot silently move a key the docs promise.
func TestGlobalBindingsAreFixed(t *testing.T) {
	cases := []struct {
		name string
		b    key.Binding
		want string
	}{
		{"NextPanel", Global.NextPanel, "tab"},
		{"PrevPanel", Global.PrevPanel, "shift+tab"},
		{"ToggleLists", Global.ToggleListsPanel, "L"},
		{"Help", Global.Help, "?"},
		{"Quit", Global.Quit, "q"},
		{"ForceQuit", Global.ForceQuit, "ctrl+c"},
		{"Theme", Global.Theme, "T"},
		{"Filter", Global.Filter, "/"},
		{"Picker", Global.Picker, "F"},
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

func TestActiveReturnsBindingsForEveryFocusableZone(t *testing.T) {
	for _, focused := range []int{constants.COMPONENT_LISTS_PANEL, constants.COMPONENT_TASK_TREE} {
		ctx := Context{Focused: focused, ListsPanelVisible: true, HasActiveList: true}
		if len(Active(ctx)) == 0 {
			t.Errorf("Active for zone %d is empty", focused)
		}
	}
}

func TestActiveReturnsCreateBindingsWhenCreating(t *testing.T) {
	ctx := Context{
		Focused:           constants.COMPONENT_TASK_TREE,
		HasActiveList:     true,
		Creating:          true,
		TaskTreeEmpty:     false,
		ListsPanelVisible: true,
	}
	bindings := Active(ctx)
	if len(bindings) != 4 {
		t.Fatalf("expected 4 create bindings, got %d: %v", len(bindings), bindings)
	}
	for _, b := range bindings {
		found := false
		for _, expected := range []key.Binding{Create.Submit, Create.Cancel, Create.Outdent, Create.Indent} {
			if sameBinding(b, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected binding in create context: %s", b.Help().Key)
		}
	}
}

func TestActiveReturnsFilterBindingsWhenFiltering(t *testing.T) {
	ctx := Context{
		Focused:           constants.COMPONENT_TASK_TREE,
		HasActiveList:     true,
		Filtering:         true,
		TaskTreeEmpty:     false,
		ListsPanelVisible: true,
	}
	bindings := Active(ctx)
	if len(bindings) != 2 {
		t.Fatalf("expected 2 filter bindings, got %d: %v", len(bindings), bindings)
	}
	for _, b := range bindings {
		found := false
		for _, expected := range []key.Binding{Overlay.Submit, Overlay.Cancel} {
			if sameBinding(b, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected binding in filter context: %s", b.Help().Key)
		}
	}
}

func TestActiveReturnsEmptyWhenModalOwnsKeyboard(t *testing.T) {
	ctx := Context{
		Focused:           constants.COMPONENT_TASK_TREE,
		HasActiveList:     true,
		HasModal:          true,
		TaskTreeEmpty:     false,
		ListsPanelVisible: true,
	}
	if len(Active(ctx)) != 0 {
		t.Errorf("expected no bindings when modal is open, got %d", len(Active(ctx)))
	}
}

func TestDeleteBindingIsD(t *testing.T) {
	keys := Tree.Delete.Keys()
	if len(keys) != 1 || keys[0] != "d" {
		t.Errorf("Tree.Delete binds %v, want d", keys)
	}
	if len(Lists.Delete.Keys()) != 1 || Lists.Delete.Keys()[0] != "d" {
		t.Errorf("Lists.Delete binds %v, want d", Lists.Delete.Keys())
	}
}

func TestCreateBindingsUseBrackets(t *testing.T) {
	if len(Create.Outdent.Keys()) != 1 || Create.Outdent.Keys()[0] != "[" {
		t.Errorf("Create.Outdent binds %v, want [", Create.Outdent.Keys())
	}
	if len(Create.Indent.Keys()) != 1 || Create.Indent.Keys()[0] != "]" {
		t.Errorf("Create.Indent binds %v, want ]", Create.Indent.Keys())
	}
}

func TestCreateBindingsDoNotUseTab(t *testing.T) {
	for _, b := range []key.Binding{Create.Outdent, Create.Indent, Create.Submit, Create.Cancel} {
		for _, k := range b.Keys() {
			if k == "tab" || k == "shift+tab" {
				t.Errorf("Create.%s uses %q, expected bracket keys", b.Help().Key, k)
			}
		}
	}
}
