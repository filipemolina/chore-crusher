// Package keys is the single source of truth for every keybinding in the
// app. A key is declared here exactly once; components match against these
// bindings, and any help overlay renders from them (docs/DESIGN.md §5 and
// CONTRIBUTING.md's rule against inventing a keybinding elsewhere).
//
// The Global group is complete from phase 3 on — its bindings are fixed by
// docs/DESIGN.md §5. The TaskTree, AddInput, and ListsPanel groups are
// declared now and filled in by phases 4-6, so later phases extend one file
// instead of each inventing their own dispatch.
package keys

import (
	"charm.land/bubbles/v2/key"
	"github.com/filipemolina/chore-completer/src/constants"
)

// GlobalKeys work anywhere that no overlay owns the keyboard.
type GlobalKeys struct {
	// NextPanel/PrevPanel are tab/shift+tab, cycling only through the zones
	// currently visible (docs/DESIGN.md §5).
	NextPanel key.Binding
	PrevPanel key.Binding
	// ToggleListsPanel is L (shift+l). Lowercase l is the task tree's
	// expand key (phase 4), so the toggle takes the shifted form — the same
	// collision-avoidance as Global.Theme (T) vs the details panel's t.
	ToggleListsPanel key.Binding
	// Quit is q, and it is the one global key that yields: a modal needs the
	// letter for typing. ForceQuit is separate from it precisely so that
	// ctrl+c yields to nothing.
	Quit      key.Binding
	ForceQuit key.Binding
	// Back is esc away from everything that has a stronger claim on it: a
	// modal closes itself first, the add input with text in it claims it
	// next, an applied filter after that; what is left is "back to the task
	// tree from wherever else" (docs/DESIGN.md §5's esc ladder).
	Back key.Binding
	// Help opens the help overlay. The overlay renders from this package, so
	// what it says is what the handlers do.
	Help key.Binding
	// Theme opens the theme picker: a list of registered themes with live
	// preview on cursor movement and persist-on-confirm. T (shift+t) so it
	// does not collide with any lowercase t.
	Theme key.Binding
	// Filter enters (/) the task tree's local fuzzy filter: it narrows the
	// current list's rows in place, showing each match's ancestor chain.
	// F, by contrast, opens the cross-list picker.
	Filter key.Binding
	// Picker opens (F) the cross-list search picker: type a query, pick a
	// task from any list, enter jumps to it and switches the active list.
	Picker key.Binding
}

// TaskTreeKeys act on the task tree: navigation, expand/collapse, toggling
// complete, opening the details screen. Filled in by phase 4
// (docs/plans/phase-4-task-tree.md); the group is declared here so the
// package's Context/Active pattern is in place before any handler exists.
type TaskTreeKeys struct {
	Navigate    key.Binding
	Expand      key.Binding
	Collapse    key.Binding
	Toggle      key.Binding
	OpenDetails key.Binding
}

// AddInputKeys act inside the add-input zone: editing the draft, changing
// its level, submitting. Filled in by phase 5 (docs/plans/phase-5-add-input.md).
type AddInputKeys struct {
	Level  key.Binding
	Submit key.Binding
}

// ListsPanelKeys act on the lists panel: navigating lists, creating,
// renaming, deleting. Filled in by phase 6 (docs/plans/phase-6-lists-panel.md).
type ListsPanelKeys struct {
	Navigate key.Binding
	New      key.Binding
	Rename   key.Binding
	Delete   key.Binding
}

// DetailsKeys act inside the details screen modal: saving, cycling between
// notes and progress editor zones, cycling progress modes, and entering
// percentages. Phase 7 (docs/plans/phase-7-details-screen.md).
type DetailsKeys struct {
	Save          key.Binding
	NextField     key.Binding
	CycleMode     key.Binding
	CycleModeBack key.Binding
}

// OverlayKeys are the keys every modal answers to. Cancel is one binding for
// every overlay in the app, so "esc backs out" needs no exceptions.
type OverlayKeys struct {
	Submit key.Binding
	Cancel key.Binding
	Yes    key.Binding
	No     key.Binding
	// Navigation is the help-only binding for the list navigation arrows a
	// modal's bubbles list handles itself — declared here so a modal's hint
	// line reads from the package instead of hardcoding "↑/↓".
	Navigation key.Binding
}

var Global = GlobalKeys{
	NextPanel: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next")),
	PrevPanel: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev")),
	// Uppercase, per docs/DESIGN.md §5: the task tree owns lowercase l.
	ToggleListsPanel: key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "lists")),
	Quit:             key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	// Not advertised in the help overlay's headline group: ctrl+c is the
	// escape hatch every terminal program has, and the overlay says q. It
	// is still declared here so handlers match on the same binding as
	// everything else.
	ForceQuit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "force quit")),
	Back:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Theme:     key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "theme")),
	Filter:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Picker:    key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "search")),
}

var Tree = TaskTreeKeys{
	Navigate:    key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑/↓", "navigate")),
	Expand:      key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "expand")),
	Collapse:    key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "collapse")),
	Toggle:      key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
	OpenDetails: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
}

var Lists = ListsPanelKeys{
	Navigate: key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑/↓", "navigate")),
	New:      key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new list")),
	Rename:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "rename list")),
	Delete:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete list")),
}

var Details = DetailsKeys{
	Save:          key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),
	NextField:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
	CycleMode:     key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next mode")),
	CycleModeBack: key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev mode")),
}

var Overlay = OverlayKeys{
	Submit: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
	Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Yes:    key.NewBinding(key.WithKeys("y", "Y"), key.WithHelp("y", "yes")),
	No:     key.NewBinding(key.WithKeys("n", "N"), key.WithHelp("n", "no")),
	// Help-only: the bubbles list owns the arrow keystrokes.
	Navigation: key.NewBinding(key.WithHelp("↑/↓", "navigate")),
}

// Context is what the help overlay knows about the screen: enough to decide
// which bindings are live, and nothing more. Filled in by phases 4-6 as the
// zones gain state (an empty tree, a list without items); phase 3 only needs
// the focus and visibility facts.
type Context struct {
	// Focused is the currently focused zone id — one of
	// constants.COMPONENT_LISTS_PANEL / COMPONENT_TASK_TREE /
	// COMPONENT_ADD_INPUT.
	Focused int
	// ListsPanelVisible reports whether the lists panel is in the layout
	// (and therefore in the focus cycle) right now.
	ListsPanelVisible bool
	// TaskTreeEmpty reports whether the active list has no visible rows.
	TaskTreeEmpty bool
	// HasActiveList reports whether a list is selected at all.
	HasActiveList bool
}

// Active returns the bindings the user can press right now, in the order they
// should be shown.
func Active(ctx Context) []key.Binding {
	bindings := []key.Binding{Global.NextPanel}

	if ctx.ListsPanelVisible && ctx.Focused == constants.COMPONENT_LISTS_PANEL {
		bindings = append(bindings, Lists.Navigate, Lists.New, Lists.Rename, Lists.Delete)
	}

	if ctx.Focused == constants.COMPONENT_TASK_TREE {
		bindings = append(bindings, Tree.Navigate, Tree.Expand, Tree.Collapse, Tree.Toggle, Tree.OpenDetails)
	}

	return bindings
}

// Globals are the always-available keys, pinned away from the
// context-dependent ones.
func Globals() []key.Binding {
	return []key.Binding{Global.NextPanel, Global.Help, Global.Quit}
}

// Scope is one group of related keys in the help overlay.
type Scope struct {
	Title   string
	Entries []Entry
}

// Entry is one row of the help overlay: the binding to render, and whether
// the user can press it right now. Rows that cannot be pressed are dimmed.
type Entry struct {
	Binding   key.Binding
	Available bool
}

// Catalog returns every key in the app, grouped by scope, with availability
// resolved against ctx. It reads the same bindings the handlers match
// against - that is the point of the overlay: it cannot drift from the
// handlers. Phases 4-6 add their groups to the catalog as they bind keys.
func Catalog(ctx Context) []Scope {
	live := pressableNow(ctx)

	entries := func(bindings ...key.Binding) []Entry {
		out := make([]Entry, 0, len(bindings))
		for _, b := range bindings {
			out = append(out, Entry{Binding: b, Available: containsBinding(live, b)})
		}
		return out
	}

	return []Scope{
		{
			Title: "Global",
			Entries: entries(
				Global.NextPanel, Global.PrevPanel, Global.ToggleListsPanel,
				Global.Back, Global.Quit, Global.ForceQuit, Global.Help,
				Global.Theme, Global.Filter, Global.Picker,
			),
		},
		{
			Title: "Lists",
			Entries: entries(
				Lists.Navigate, Lists.New, Lists.Rename, Lists.Delete,
			),
		},
		{
			Title: "Task Tree",
			Entries: entries(
				Tree.Navigate, Tree.Expand, Tree.Collapse, Tree.Toggle, Tree.OpenDetails,
			),
		},
		{
			Title: "Details",
			Entries: entries(
				Details.Save, Details.NextField, Details.CycleMode, Details.CycleModeBack,
			),
		},
	}
}

// pressableNow is the set of bindings the user can actually press in ctx: the
// contextual ones Active returns, plus the globals that are always live
// whether or not the help overlay has room to advertise them.
func pressableNow(ctx Context) []key.Binding {
	live := append(Active(ctx), Globals()...)
	live = append(live, Global.ForceQuit, Global.Theme, Global.ToggleListsPanel)
	live = append(live, Global.Filter, Global.Picker)

	// shift+tab is tab's twin: live wherever tab is.
	if containsBinding(live, Global.NextPanel) {
		live = append(live, Global.PrevPanel)
	}

	return live
}

// containsBinding reports whether haystack contains a binding with the same
// keystrokes and help text as needle. key.Binding is not comparable (it
// carries a slice), so the comparison is by content.
func containsBinding(haystack []key.Binding, needle key.Binding) bool {
	for _, b := range haystack {
		if sameBinding(b, needle) {
			return true
		}
	}
	return false
}

// sameBinding compares two bindings by their keystrokes and help text.
func sameBinding(a, b key.Binding) bool {
	ka, kb := a.Keys(), b.Keys()
	if len(ka) != len(kb) {
		return false
	}
	for i := range ka {
		if ka[i] != kb[i] {
			return false
		}
	}
	return a.Help().Key == b.Help().Key && a.Help().Desc == b.Help().Desc
}
