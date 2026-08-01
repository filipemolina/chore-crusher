package apptypes

// ThemeItem is a list.Item for the theme picker modal. Active marks the
// theme that was in effect when the modal opened, so the user can see
// which one they started from and Esc can restore it.
type ThemeItem struct {
	Name   string
	Active bool
}

func (t ThemeItem) Title() string {
	if t.Active {
		return t.Name + "  (active)"
	}
	return t.Name
}

func (t ThemeItem) FilterValue() string { return t.Name }

// ListListItem is the bubbles/list.Item wrapper for one list row in the
// lists panel (phase 6). The panel renders through a plain bubbles list, so
// its items need the list's Title/FilterValue face; the underlying data
// stays apptypes.List.
type ListListItem struct {
	List
}

func (i ListListItem) Title() string       { return i.Name }
func (i ListListItem) FilterValue() string { return i.Name }

// TaskListItem is the bubbles/list.Item wrapper for a flat, already-flattened
// task row, for the pieces that still render tasks through a plain bubbles
// list rather than the custom tree (the search modal, phase 8). The task tree
// itself (phase 4) is custom-rendered, not a bubbles list — see
// docs/plans/phase-3-tui-shell.md step 3.
type TaskListItem struct {
	Task
}

func (i TaskListItem) Title() string       { return i.Task.Title }
func (i TaskListItem) FilterValue() string { return i.Task.Title }
