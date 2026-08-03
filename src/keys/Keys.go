// Package keys is the single source of truth for every keybinding in the
// app. A key is declared here exactly once; components match against these
// bindings, and any help overlay renders from them (docs/DESIGN.md §5 and
// CONTRIBUTING.md's rule against inventing a keybinding elsewhere).
//
// The Global group is complete from phase 3 on — its bindings are fixed by
// docs/DESIGN.md §5. The TaskTree, Create, and ListsPanel groups are
// declared here and filled in by the remaining phases.
package keys

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"github.com/filipemolina/chore-crusher/src/constants"
)

// GlobalKeys work anywhere that no overlay owns the keyboard.
type GlobalKeys struct {
	// NextPanel/PrevPanel are tab / shift+tab, cycling only through the zones
	// currently visible (docs/DESIGN.md §5).
	NextPanel key.Binding
	PrevPanel key.Binding
	// ToggleListsPanel is L (shift+l). Lowercase l is the task tree's
	// expand key, so the toggle takes the shifted form.
	ToggleListsPanel key.Binding
	// Quit is q. ForceQuit is separate from it precisely so that
	// ctrl+c yields to nothing.
	Quit      key.Binding
	ForceQuit key.Binding
	// Back is esc away from everything that has a stronger claim on it: a
	// modal closes itself first, the add input with text in it claims it
	// next, an applied filter after that; what is left is "back to the task
	// tree from wherever else" (docs/DESIGN.md §5's esc ladder).
	Back key.Binding
	// Help opens the help overlay.
	Help key.Binding
	// Theme opens the theme picker.
	Theme key.Binding
	// Filter enters (/) the task tree's local fuzzy filter.
	Filter key.Binding
	// Picker opens (F) the cross-list search picker.
	Picker key.Binding
}

// TaskTreeKeys act on the task tree: navigation, expand/collapse, toggling
// complete, opening the details screen, and delete.
type TaskTreeKeys struct {
	Navigate    key.Binding
	Expand      key.Binding
	Collapse    key.Binding
	Toggle      key.Binding
	OpenDetails key.Binding
	// New is declared here for stability; the handler lives in AppModel
	// (context = focused panel), so it is not advertised in Active or Catalog.
	New    key.Binding
	Delete key.Binding
}

// CreateKeys act inside the inline create row: editing the draft, changing
// its level, submitting, and cancelling.
type CreateKeys struct {
	Indent  key.Binding
	Outdent key.Binding
	Submit  key.Binding
	Cancel  key.Binding
}

// ListsPanelKeys act on the lists panel: navigating lists, creating,
// renaming, deleting.
type ListsPanelKeys struct {
	Navigate key.Binding
	New      key.Binding
	Rename   key.Binding
	Delete   key.Binding
}

// DetailsKeys act inside the details screen modal: saving, cycling between
// notes and progress editor zones, cycling progress modes, and entering
// percentages.
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
	// Navigation is the help-only binding for the arrow keystrokes a
	// modal's bubbles list handles itself.
	Navigation key.Binding
}

var Global = GlobalKeys{
	NextPanel:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next")),
	PrevPanel:        key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev")),
	ToggleListsPanel: key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "lists")),
	Quit:             key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	ForceQuit:        key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "force quit")),
	Back:             key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Help:             key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Theme:            key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "theme")),
	Filter:           key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Picker:           key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "search")),
}

var Tree = TaskTreeKeys{
	Navigate:    key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑/↓", "navigate")),
	Expand:      key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "expand")),
	Collapse:    key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "collapse")),
	Toggle:      key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
	OpenDetails: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details")),
	// New is handled at AppModel level (context = focused panel); kept here
	// so the tree's handler can match it until the wiring moves in step 2.
	New:    key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
	Delete: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
}

var Create = CreateKeys{
	Outdent: key.NewBinding(key.WithKeys("["), key.WithHelp("[", "outdent")),
	Indent:  key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "indent")),
	Submit:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "create")),
	Cancel:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
}

var Lists = ListsPanelKeys{
	Navigate: key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑/↓", "navigate")),
	New:      key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new list")),
	Rename:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "rename list")),
	Delete:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
}

// ListKeyMap is the keymap the lists panel installs on its inner bubbles
// list, replacing list.DefaultKeyMap.
//
// The default map is written for a list that is the whole program, so it
// claims keys this app spends elsewhere: / is the task tree's filter, esc
// and ? are handled by AppModel, and q / ctrl+c are the app's quit keys.
// The results were visible - pressing / both opened the filter and did nothing
// useful in the lists panel - so the list has to be told which keys are not
// its own.
//
// What stays is what only the list can answer: where its cursor is. Filtering
// is worth keeping with many lists, but the lists panel currently has no
// filter input, so it is unbound here.
//
// Keys the app owns are left with no keystrokes rather than removed, because
// list.Model reads every field: an empty binding matches nothing, which is the
// intent, whereas a missing one would be a nil-safe accident.
func ListKeyMap() list.KeyMap {
	unbound := key.NewBinding()

	return list.KeyMap{
		CursorUp:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		CursorDown: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PrevPage:   key.NewBinding(key.WithKeys("left", "pgup"), key.WithHelp("←/pgup", "prev page")),
		NextPage:   key.NewBinding(key.WithKeys("right", "pgdown"), key.WithHelp("→/pgdn", "next page")),
		GoToStart:  key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "first row")),
		GoToEnd:    key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "last row")),

		Filter:            unbound,
		ClearFilter:       unbound,
		CancelWhileFiltering: unbound,
		AcceptWhileFiltering: unbound,
		ShowFullHelp:      unbound,
		CloseFullHelp:     unbound,
		Quit:              unbound,
		ForceQuit:         unbound,
	}
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

// Context is what the help overlay and keybinding bar know about the screen:
// enough to decide which bindings are live, and nothing more.
type Context struct {
	// Focused is the currently focused zone id — one of
	// constants.COMPONENT_LISTS_PANEL / COMPONENT_TASK_TREE.
	Focused int
	// ListsPanelVisible reports whether the lists panel is in the layout
	// (and therefore in the focus cycle) right now.
	ListsPanelVisible bool
	// TaskTreeEmpty reports whether the active list has no visible rows.
	TaskTreeEmpty bool
	// HasActiveList reports whether a list is selected at all.
	HasActiveList bool
	// Creating reports whether the task tree's inline input is active.
	Creating bool
	// Filtering reports whether the task tree's /-filter is open or applied.
	Filtering bool
	// HasModal reports whether a modal or overlay owns the keyboard.
	HasModal bool
}

// Active returns the bindings the user can press right now, in the order they
// should be shown.
func Active(ctx Context) []key.Binding {
	// A modal or overlay owns the keyboard exclusively while open.
	if ctx.HasModal {
		return nil
	}

	// While the inline create input is active, only create keys are live.
	if ctx.Creating && ctx.Focused == constants.COMPONENT_TASK_TREE {
		return []key.Binding{
			Create.Submit, Create.Cancel, Create.Outdent, Create.Indent,
		}
	}

	// While a /-filter is being typed or applied, only filter keys are live.
	if ctx.Filtering && ctx.Focused == constants.COMPONENT_TASK_TREE {
		return []key.Binding{Overlay.Submit, Overlay.Cancel}
	}

	switch ctx.Focused {
	case constants.COMPONENT_LISTS_PANEL:
		if ctx.ListsPanelVisible {
			return []key.Binding{
				Lists.Navigate, Lists.New, Lists.Rename, Lists.Delete,
				Global.NextPanel,
			}
		}
	case constants.COMPONENT_TASK_TREE:
		if ctx.HasActiveList && !ctx.TaskTreeEmpty {
			return []key.Binding{
				Tree.Navigate, Tree.Expand, Tree.Collapse,
				Tree.Toggle, Tree.OpenDetails, Tree.Delete,
				Global.NextPanel,
			}
		}
	}

	return []key.Binding{Global.NextPanel}
}

// Globals are the always-available keys the footer pins to its right-hand side,
// away from the context-dependent ones.
func Globals() []key.Binding {
	return []key.Binding{
		Global.NextPanel,
		Global.PrevPanel,
		Global.Help,
		Global.Quit,
	}
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
// handlers.
func Catalog(ctx Context) []Scope {
	live := pressableNow(ctx)

	entries := func(bindings ...key.Binding) []Entry {
		out := make([]Entry, 0, len(bindings))
		for _, b := range bindings {
			out = append(out, Entry{Binding: b, Available: containsBinding(live, b)})
		}
		return out
	}

	scopes := []Scope{
		{
			Title: "Global",
			Entries: entries(
				Global.NextPanel, Global.PrevPanel, Global.ToggleListsPanel,
				Global.Back, Global.Quit, Global.ForceQuit, Global.Help,
				Global.Theme, Global.Filter, Global.Picker,
			),
		},
		{
			Title:   "Overlays",
			Entries: entries(Overlay.Submit, Overlay.Cancel, Overlay.Yes, Overlay.No),
		},
	}

	if ctx.ListsPanelVisible {
		scopes = append(scopes, Scope{
			Title:   "Lists",
			Entries: entries(Lists.Navigate, Lists.New, Lists.Rename, Lists.Delete),
		})
	}

	if ctx.HasActiveList {
		switch {
		case ctx.Creating:
			scopes = append(scopes, Scope{
				Title:   "Creating",
				Entries: entries(Create.Submit, Create.Cancel, Create.Outdent, Create.Indent),
			})
		case ctx.Filtering:
			scopes = append(scopes, Scope{
				Title:   "Filter",
				Entries: entries(Overlay.Submit, Overlay.Cancel),
			})
		default:
			scopes = append(scopes, Scope{
				Title:   "Task Tree",
				Entries: entries(
					Tree.Navigate, Tree.Expand, Tree.Collapse,
					Tree.Toggle, Tree.OpenDetails, Tree.Delete,
				),
			})
		}
	}

	scopes = append(scopes, Scope{
		Title:   "Details",
		Entries: entries(Details.Save, Details.NextField, Details.CycleMode, Details.CycleModeBack),
	})

	return scopes
}

// pressableNow is the set of bindings the user can actually press in ctx: the
// contextual ones Active returns, plus the globals that are always live
// whether or not the footer has room to advertise them.
func pressableNow(ctx Context) []key.Binding {
	live := append(Active(ctx), Globals()...)
	live = append(live, Global.ForceQuit)

	// When a modal owns the keyboard, or the user is typing a create or
	// filter input, only the always-available keys remain pressable.
	if !ctx.HasModal && !ctx.Creating && !ctx.Filtering {
		live = append(live, Global.Back, Global.Theme, Global.ToggleListsPanel, Global.Filter, Global.Picker)
	}

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
