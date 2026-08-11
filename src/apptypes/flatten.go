package apptypes

// Row is one row of a flattened task tree: the task itself, its depth
// (0 = a root task), whether it has children — so a tree renderer knows
// whether to draw an expand glyph without a second pass — and whether the
// task has any comments, so the tasktree row can draw the comments glyph
// without another lookup per row.
type Row struct {
	Task        Task
	Depth       int
	HasChildren bool
	HasComments bool
}

// Flatten walks the flat rows store.ListTasks returns (converted via
// FromStoreTasks) into depth-first preorder rows with depth annotations:
// parents before their children, siblings in the order given (which
// ListTasks already orders by position). This is the single ordering both
// the CLI's text tree and the TUI's tree component (phase 4) render from —
// one implementation, per docs/DESIGN.md §10's "neither front end is
// secondary".
//
// It assumes the input is a complete list's rows, so every parent_id in it
// resolves to another row in the input — store.ListTasks and the foreign
// keys in 0001_init.sql guarantee this, so Flatten does not defend against
// an orphaned row it cannot place.
func Flatten(tasks []Task) []Row {
	children := make(map[string][]Task)
	for _, t := range tasks {
		if t.ParentID != nil {
			children[*t.ParentID] = append(children[*t.ParentID], t)
		}
	}

	var out []Row
	for _, t := range tasks {
		if t.ParentID != nil {
			continue
		}
		flattenInto(t, 0, children, &out)
	}
	return out
}

func flattenInto(t Task, depth int, children map[string][]Task, out *[]Row) {
	kids := children[t.ID]
	*out = append(*out, Row{Task: t, Depth: depth, HasChildren: len(kids) > 0})
	for _, k := range kids {
		flattenInto(k, depth+1, children, out)
	}
}

// DescendantsOf returns every descendant of rootID (the root itself
// excluded) as depth-annotated rows, with depth relative to rootID: direct
// children at depth 1. Flatten cannot produce this shape — it only emits
// ParentID==nil rows, and a pure-descendant set has no roots — so this is
// the one helper both `farol show` (CLI) and show_task (MCP) use for their
// children rows, and the two surfaces cannot drift again.
func DescendantsOf(tasks []Task, rootID string) []Row {
	children := make(map[string][]Task)
	for _, t := range tasks {
		if t.ParentID != nil {
			children[*t.ParentID] = append(children[*t.ParentID], t)
		}
	}

	var out []Row
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		for _, c := range children[id] {
			kids := children[c.ID]
			out = append(out, Row{Task: c, Depth: depth, HasChildren: len(kids) > 0})
			walk(c.ID, depth+1)
		}
	}
	walk(rootID, 1)
	return out
}
