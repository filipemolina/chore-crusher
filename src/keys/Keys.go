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
	// Quit is q, the ordinary way out: it is a printable character, so it
	// yields to anything typing one — a modal, an inline create row, a
	// filter being typed — and quits only from the task tree or the lists
	// panel with none of those active (docs/DESIGN.md §5).
	Quit key.Binding
	// ForceQuit is ctrl+c, the escape hatch that yields to nothing, so it
	// quits from a modal or a text input alike where q cannot.
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
// complete, opening the details screen, delete, and restructuring the
// selected task (outdent/indent, move up/down).
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
	// Outdent/Indent change the selected task's depth. The same two keys pick
	// the new task's level while the inline input is active (Create scope,
	// docs/DESIGN.md §4) — one declaration, two contexts.
	Outdent key.Binding
	Indent  key.Binding
	// MoveUp/MoveDown reorder the selected task within its own Pending or
	// Complete section (docs/DESIGN.md §6). Alt is the modifier vim-move and
	// VS Code converge on for moving a line.
	MoveUp   key.Binding
	MoveDown key.Binding
	// GoToStart/GoToEnd jump the cursor to the first or last visible row.
	// PageUp/PageDown move it one viewport height up/down, clamped to the
	// row bounds (docs/DESIGN.md §5). Key choices match ListKeyMap() so the
	// two panels agree; left/right are deliberately not borrowed here because
	// the tree reserves them for expand/collapse.
	GoToStart key.Binding
	GoToEnd   key.Binding
	PageUp    key.Binding
	PageDown  key.Binding
}

// CreateKeys act inside the inline create row: editing the draft, changing
// its level, submitting, and cancelling. The level keys themselves live on
// Tree (Outdent/Indent) — the same [ / ] do double duty on the selected task.
type CreateKeys struct {
	Submit key.Binding
	Cancel key.Binding
}

// ListsPanelKeys act on the lists panel: navigating lists, creating,
// renaming, deleting.
type ListsPanelKeys struct {
	Navigate key.Binding
	New      key.Binding
	Rename   key.Binding
	Delete   key.Binding
}

// DetailsKeys act inside the Details modal: saving, cycling between the
// title/notes/progress/comments zones, cycling progress modes, entering
// percentages, copying the task id, and the comment thread's own actions
// (add, submit, copy id).
type DetailsKeys struct {
	Save          key.Binding
	NextField     key.Binding
	CycleMode     key.Binding
	CycleModeBack key.Binding
	PercentNudge  key.Binding
	PercentType   key.Binding
	DiscardPrompt key.Binding
	CopyTaskID    key.Binding
	CommentNew    key.Binding
	CommentSubmit key.Binding
	CopyCommentID key.Binding
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
	ForceQuit:        key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
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
	New:       key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
	Delete:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
	Outdent:   key.NewBinding(key.WithKeys("["), key.WithHelp("[", "outdent")),
	Indent:    key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "indent")),
	MoveUp:    key.NewBinding(key.WithKeys("alt+up", "alt+k"), key.WithHelp("alt+↑/alt+k", "move up")),
	MoveDown:  key.NewBinding(key.WithKeys("alt+down", "alt+j"), key.WithHelp("alt+↓/alt+j", "move down")),
	GoToStart: key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "first")),
	GoToEnd:   key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "last")),
	PageUp:    key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
	PageDown:  key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdown", "page down")),
}

var Create = CreateKeys{
	Submit: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "create")),
	Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
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
// claims keys this app spends elsewhere: / was the task tree's filter (now
// contextual — see src/model/Update.go), esc is normally AppModel's, and
// ctrl+c is the app's quit key. Keys the app owns outright are left with no
// keystrokes rather than removed, because list.Model reads every field: an
// empty binding matches nothing, which is the intent, whereas a missing one
// would be a nil-safe accident.
//
// The filter keys are bound so the list's own handling works once the
// filter is open, but / itself stays unbound: AppModel intercepts the
// global / and routes it contextually (ActivateListFilterMsg when the lists
// panel is focused), so the list must not also claim it. esc reaches the
// list only because the panel's KeepsEsc claims it while a filter is open
// or applied; inside the list, esc cancels (CancelWhileFiltering while
// typing, ClearFilter while browsing an applied filter) and enter accepts.
func ListKeyMap() list.KeyMap {
	unbound := key.NewBinding()

	return list.KeyMap{
		CursorUp:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		CursorDown: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PrevPage:   key.NewBinding(key.WithKeys("left", "pgup"), key.WithHelp("←/pgup", "prev page")),
		NextPage:   key.NewBinding(key.WithKeys("right", "pgdown"), key.WithHelp("→/pgdn", "next page")),
		GoToStart:  key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "first row")),
		GoToEnd:    key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "last row")),

		Filter:               unbound,
		ClearFilter:          key.NewBinding(key.WithKeys("esc")),
		CancelWhileFiltering: key.NewBinding(key.WithKeys("esc")),
		AcceptWhileFiltering: key.NewBinding(key.WithKeys("enter")),
		ShowFullHelp:         unbound,
		CloseFullHelp:        unbound,
		Quit:                 unbound,
		ForceQuit:            unbound,
	}
}

var Details = DetailsKeys{
	Save:          key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),
	NextField:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
	CycleMode:     key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next mode")),
	CycleModeBack: key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev mode")),
	// Percentage-mode progress only: ↑/↓ step the value by 5, clamped to
	// 0–100. Live only while the Progress field has focus and the mode is
	// percentage, so the modal advertises it only there.
	PercentNudge: key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "±5")),
	// Help-only, like Overlay.Navigation: digits type straight into the
	// percentage field, so there is no one keystroke to bind — but without a
	// hint nothing on screen says the field takes typing at all.
	PercentType: key.NewBinding(key.WithHelp("0-9", "type a number")),
	// Help-only. Esc on a dirty modal raises "Discard changes? (y/n)", and
	// that prompt answers to y and n alone. enter is deliberately not bound:
	// unlike the confirm modal — where enter acts on a yes/no button the user
	// can see is selected — this prompt has no visible default, so binding
	// enter would make a single stray keystroke throw away unsaved edits.
	// The Overlays scope's "enter confirm" is true of the modals that do have
	// that selection; this entry is what keeps it from over-promising here.
	DiscardPrompt: key.NewBinding(key.WithHelp("y/n", "discard or keep edits")),
	// ctrl+y copies the open task's id; it is bound to no input widget, so it
	// works from every zone including a focused text field.
	CopyTaskID: key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("ctrl+y", "copy task id")),
	// c opens the inline compose card from the comments zone (mirroring the task
	// tree's inline create), and enter posts it — distinct from ctrl+s (which
	// saves notes/progress), so the two write paths never collide. A terminal
	// cannot reliably distinguish ctrl+enter from enter, so enter is the submit
	// key (docs/plan/task-comments.md §6, Commit 5).
	CommentNew:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "add comment")),
	CommentSubmit: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "post comment")),
	CopyCommentID: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy comment id")),
}

var Overlay = OverlayKeys{
	Submit: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
	Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Yes:    key.NewBinding(key.WithKeys("y", "Y"), key.WithHelp("y", "yes")),
	No:     key.NewBinding(key.WithKeys("n", "N"), key.WithHelp("n", "no")),
	// The arrows every overlay moves within itself with. Where a modal wraps
	// a bubbles list, the list handles the keystrokes itself and this binding
	// is only the hint; the help overlay, which has no list, matches against
	// it directly to scroll its catalog. One declaration, both uses.
	Navigation: key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑/↓", "navigate")),
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
	// DetailsPanelVisible reports whether the Details side panel is open. While
	// it is, Details owns the keyboard (docs/DESIGN.md §5): only its own
	// bindings plus Esc are live, and no global or task-tree key acts.
	DetailsPanelVisible bool
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

	// The Details side panel owns the keyboard while open: only its own
	// bindings plus Esc are live — no task-tree, Lists, tab, search, theme, or
	// panel-toggle key acts (docs/DESIGN.md §5). ctrl+c stays the unadvertised
	// emergency exit.
	if ctx.DetailsPanelVisible {
		// Cancel sits ahead of the comment/copy extras: the footer bar sheds
		// hints from the tail when the terminal is narrow, and the core
		// save/next/mode/cancel keys must survive that shedding.
		return []key.Binding{
			Details.Save, Details.NextField, Details.CycleMode,
			Details.CycleModeBack, Overlay.Cancel, Details.CopyTaskID,
			Details.CommentNew, Details.CommentSubmit, Details.CopyCommentID,
		}
	}

	// While the inline create input is active, only create keys are live.
	if ctx.Creating && ctx.Focused == constants.COMPONENT_TASK_TREE {
		return []key.Binding{
			Create.Submit, Create.Cancel, Tree.Outdent, Tree.Indent,
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
				Global.NextPanel, Global.Quit,
			}
		}
	case constants.COMPONENT_TASK_TREE:
		if ctx.HasActiveList && !ctx.TaskTreeEmpty {
			return []key.Binding{
				Tree.Navigate, Tree.Toggle, Tree.OpenDetails,
				Tree.Expand, Tree.Collapse, Tree.Delete, Tree.New,
				Tree.Outdent, Tree.Indent, Tree.MoveUp, Tree.MoveDown,
				Tree.GoToStart, Tree.GoToEnd, Tree.PageUp, Tree.PageDown,
				Global.NextPanel, Global.Quit,
			}
		}
		if ctx.HasActiveList {
			// Empty tree: n (new) is the only task action there is — the
			// inline input is the empty state's way in
			// (docs/plan/task-row-cards-and-status.md).
			return []key.Binding{Tree.New, Global.NextPanel, Global.Quit}
		}
	}

	return []key.Binding{Global.NextPanel, Global.Quit}
}

// Globals are the always-available keys the footer pins to its right-hand side,
// away from the context-dependent ones.
func Globals() []key.Binding {
	return []key.Binding{
		Global.NextPanel,
		Global.PrevPanel,
		Global.Help,
	}
}

// Scope is one group of related keys in the help overlay.
type Scope struct {
	Title   string
	Entries []Entry
	// Note is one line of prose under the scope's keys, for behaviour a
	// key/description pair cannot carry on its own — what a key does BESIDES
	// what its help text says, or which of a scope's keys only apply to some
	// of the surfaces the scope covers. Where a scope is context-dependent,
	// this is how it says so; omitting the key instead is what made the
	// overlay incomplete in the first place.
	Note string
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

	// Every scope is present on every screen. They used to come and go with
	// the context — Lists only while the lists panel was visible, Task Tree
	// only with an active list, Creating/Filter/Task Tree as three exclusive
	// branches — so the overlay described the corner the user opened it from
	// rather than the app. A key you can only read about once you have already
	// found the surface it belongs to is not documentation. Availability is
	// carried by Entry.Available, which the overlay renders as dimming.
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
			Title: "Task Tree",
			Entries: entries(
				Tree.Navigate, Tree.GoToStart, Tree.GoToEnd,
				Tree.PageUp, Tree.PageDown, Tree.Expand, Tree.Collapse,
				Tree.Toggle, Tree.OpenDetails, Tree.New, Tree.Delete,
				Tree.Outdent, Tree.Indent, Tree.MoveUp, Tree.MoveDown,
			),
		},
		{
			Title:   "Creating a task",
			Entries: entries(Create.Submit, Create.Cancel, Tree.Outdent, Tree.Indent),
			Note:    "[ and ] set the new task's level while the input is open.",
		},
		{
			Title:   "Filtering",
			Entries: entries(Global.Filter, Overlay.Submit, Overlay.Cancel),
			Note:    "/ filters the focused panel; F searches across every list.",
		},
		{
			Title:   "Lists",
			Entries: entries(Lists.Navigate, Lists.New, Lists.Rename, Lists.Delete),
			Note:    "L shows the lists panel and moves focus into it; tab moves focus back.",
		},
		{
			Title:   "Details",
			Entries: entries(Details.Save, Details.NextField, Details.CycleMode, Details.CycleModeBack, Details.PercentNudge, Details.PercentType, Details.DiscardPrompt, Details.CopyTaskID, Details.CommentNew, Details.CommentSubmit, Details.CopyCommentID),
		},
		{
			Title:   "Overlays",
			Entries: entries(Overlay.Submit, Overlay.Cancel, Overlay.Yes, Overlay.No),
			Note:    "Details' \"Discard changes?\" prompt answers to y and n only — it has no visible default for enter to act on.",
		},
	}
}

// pressableNow is the set of bindings the user can actually press in ctx: the
// contextual ones Active returns, plus the globals that are always live
// whether or not the footer has room to advertise them.
func pressableNow(ctx Context) []key.Binding {
	// While Details owns the keyboard, only its own bindings (Active returns
	// them plus Esc) and the emergency ForceQuit are live — no globals.
	if ctx.DetailsPanelVisible {
		return append(Active(ctx), Global.ForceQuit)
	}

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
