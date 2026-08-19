package keys

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/filipemolina/farol/src/constants"
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
		{"ForceQuit", Global.ForceQuit, "ctrl+c"},
		{"Theme", Global.Theme, "T"},
		{"Filter", Global.Filter, "/"},
		{"Picker", Global.Picker, "F"},
		{"ArchivePage", Global.ArchivePage, "A"},
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
		Global.Back, Global.ForceQuit, Global.Help, Global.Theme,
		Global.Filter, Global.Picker, Global.ArchivePage,
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
		for _, expected := range []key.Binding{Create.Submit, Create.Cancel, Tree.Outdent, Tree.Indent} {
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

func TestActiveReturnsDetailsBindingsWhenDetailsVisible(t *testing.T) {
	ctx := Context{
		Focused:             constants.COMPONENT_DETAILS_PANEL,
		DetailsPanelVisible: true,
		HasActiveList:       true,
		ListsPanelVisible:   false,
	}
	bindings := Active(ctx)

	want := []key.Binding{
		Details.Save, Details.NextField, Details.CycleMode,
		Details.CycleModeBack, Details.CyclePriority, Details.CopyTaskID,
		Details.CommentNew,
		Details.CommentSubmit, Details.CopyCommentID, Details.CommentDelete,
		Overlay.Cancel,
	}
	if len(bindings) != len(want) {
		t.Fatalf("expected %d Details bindings, got %d: %v", len(want), len(bindings), bindings)
	}
	for _, b := range bindings {
		if !containsBinding(want, b) {
			t.Errorf("unexpected binding in Details context: %s", b.Help().Key)
		}
	}

	// The task-tree, Lists, and normal global keys must not be live.
	for _, banned := range []key.Binding{
		Tree.Navigate, Tree.OpenDetails, Lists.Navigate,
		Global.NextPanel, Global.Picker, Global.Theme, Global.ToggleListsPanel,
	} {
		if containsBinding(bindings, banned) {
			t.Errorf("Details context wrongly advertises %q", banned.Help().Key)
		}
	}
}

// TestActiveReturnsArchivePageBindingsWhenVisible proves the Archive page
// owns the keyboard exactly like Details does (docs/DESIGN.md §5): only its
// own bindings plus Esc are live, and no task-tree, Lists, or normal global
// key acts.
func TestActiveReturnsArchivePageBindingsWhenVisible(t *testing.T) {
	ctx := Context{
		Focused:            constants.COMPONENT_ARCHIVE_PAGE,
		ArchivePageVisible: true,
		HasActiveList:      true,
		ListsPanelVisible:  false,
	}
	bindings := Active(ctx)

	want := []key.Binding{
		ArchivePage.Navigate, ArchivePage.GoToStart, ArchivePage.GoToEnd,
		ArchivePage.Filter, ArchivePage.Unarchive, Global.Back,
	}
	if len(bindings) != len(want) {
		t.Fatalf("expected %d Archive page bindings, got %d: %v", len(want), len(bindings), bindings)
	}
	for _, b := range bindings {
		if !containsBinding(want, b) {
			t.Errorf("unexpected binding in Archive page context: %s", b.Help().Key)
		}
	}

	// Tree.Navigate and Lists.Navigate are deliberately not in this list:
	// their keystrokes and help text are identical to ArchivePage.Navigate's
	// (all three are "↑/↓ navigate"), and sameBinding compares by content,
	// not identity — they are legitimately indistinguishable that way, so it
	// would be a false positive here, not a real leak.
	for _, banned := range []key.Binding{
		Tree.OpenDetails, Tree.Delete, Lists.New, Lists.Delete,
		Global.NextPanel, Global.Picker, Global.Theme, Global.ToggleListsPanel,
		Global.ArchivePage,
	} {
		if containsBinding(bindings, banned) {
			t.Errorf("Archive page context wrongly advertises %q", banned.Help().Key)
		}
	}
}

// TestPressableNowArchivePageOmitsGlobals proves the Archive page's
// keyboard-ownership extends past Active into pressableNow (and therefore the
// help overlay's dimming): only Back and the emergency ForceQuit are
// pressable, matching Details' contract.
func TestPressableNowArchivePageOmitsGlobals(t *testing.T) {
	ctx := Context{
		Focused:            constants.COMPONENT_ARCHIVE_PAGE,
		ArchivePageVisible: true,
		HasActiveList:      true,
	}
	live := pressableNow(ctx)

	if !containsBinding(live, Global.Back) || !containsBinding(live, Global.ForceQuit) {
		t.Fatalf("pressableNow for the Archive page = %v, want Back and ForceQuit", live)
	}
	if containsBinding(live, Global.ArchivePage) || containsBinding(live, Global.ToggleListsPanel) {
		t.Errorf("pressableNow wrongly advertises a global while the Archive page owns the keyboard: %v", live)
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

// The bracket keys are the tree's outdent/indent on a selected task; the
// inline create row reuses the same two bindings for its level selector, so
// there is exactly one declaration (docs/DESIGN.md §4, §5).
func TestBracketBindingsAreOutdentIndent(t *testing.T) {
	if len(Tree.Outdent.Keys()) != 1 || Tree.Outdent.Keys()[0] != "[" {
		t.Errorf("Tree.Outdent binds %v, want [", Tree.Outdent.Keys())
	}
	if len(Tree.Indent.Keys()) != 1 || Tree.Indent.Keys()[0] != "]" {
		t.Errorf("Tree.Indent binds %v, want ]", Tree.Indent.Keys())
	}
}

// Move up/down follow the vim-move / VS Code convention: alt plus the
// existing navigation keys (alt+↑/alt+k, alt+↓/alt+j).
func TestMoveBindingsUseAlt(t *testing.T) {
	for _, tc := range []struct {
		b    key.Binding
		want []string
	}{
		{Tree.MoveUp, []string{"alt+up", "alt+k"}},
		{Tree.MoveDown, []string{"alt+down", "alt+j"}},
	} {
		if len(tc.b.Keys()) != len(tc.want) {
			t.Errorf("%v binds %v, want %v", tc.want, tc.b.Keys(), tc.want)
			continue
		}
		for i, k := range tc.b.Keys() {
			if k != tc.want[i] {
				t.Errorf("%v binds %v, want %v", tc.want, tc.b.Keys(), tc.want)
			}
		}
	}
}

func TestCreateBindingsDoNotUseTab(t *testing.T) {
	for _, b := range []key.Binding{Tree.Outdent, Tree.Indent, Create.Submit, Create.Cancel} {
		for _, k := range b.Keys() {
			if k == "tab" || k == "shift+tab" {
				t.Errorf("binding %q uses %q, expected bracket keys", b.Help().Key, k)
			}
		}
	}
}

// With no side panel open the focus cycle is a single zone, so tab/shift+tab
// are dead keys: neither Active nor GlobalsFor may advertise them. The lists
// panel is the one side panel that shares the cycle, so its visibility is
// what restores the hints.
func TestPanelKeysHiddenWithoutSidePanel(t *testing.T) {
	base := Context{Focused: constants.COMPONENT_TASK_TREE, HasActiveList: true}

	if containsBinding(Active(base), Global.NextPanel) {
		t.Errorf("Active advertises tab with no side panel open")
	}
	for _, b := range GlobalsFor(base) {
		if sameBinding(b, Global.NextPanel) || sameBinding(b, Global.PrevPanel) {
			t.Errorf("GlobalsFor advertises %q with no side panel open", b.Help().Key)
		}
	}
	if !containsBinding(GlobalsFor(base), Global.Help) {
		t.Errorf("GlobalsFor must keep ? when it drops tab/shift+tab")
	}

	withPanel := base
	withPanel.ListsPanelVisible = true
	if !containsBinding(Active(withPanel), Global.NextPanel) {
		t.Errorf("Active must advertise tab when the lists panel is visible")
	}
	if !containsBinding(GlobalsFor(withPanel), Global.NextPanel) ||
		!containsBinding(GlobalsFor(withPanel), Global.PrevPanel) {
		t.Errorf("GlobalsFor must keep tab/shift+tab when the lists panel is visible")
	}
}

// The task-tree Active list carries tab only while the lists panel is open;
// with it closed the tree is the only zone and the hint would be dead.
func TestActiveTaskTreeOmitsTabWithoutListsPanel(t *testing.T) {
	ctx := Context{
		Focused:       constants.COMPONENT_TASK_TREE,
		HasActiveList: true,
		TaskTreeEmpty: false,
	}
	if containsBinding(Active(ctx), Global.NextPanel) {
		t.Errorf("Active advertises tab with no side panel open")
	}

	ctx.ListsPanelVisible = true
	if !containsBinding(Active(ctx), Global.NextPanel) {
		t.Errorf("Active must advertise tab while the lists panel is visible")
	}
}
