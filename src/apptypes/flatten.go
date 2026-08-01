package apptypes

import "github.com/filipemolina/chore-completer/src/store"

// Row is one row of a flattened task tree: the task itself, its depth
// (0 = a root task), and whether it has children — so a tree renderer knows
// whether to draw an expand glyph without a second pass.
type Row struct {
	Task        store.Task
	Depth       int
	HasChildren bool
}

// Flatten walks the flat rows store.ListTasks returns into depth-first
// preorder rows with depth annotations: parents before their children,
// siblings in the order given (which ListTasks already orders by position).
// This is the single ordering both the CLI's text tree and the TUI's tree
// component (phase 4) render from — one implementation, per docs/DESIGN.md
// §10's "neither front end is secondary".
//
// It assumes the input is a complete list's rows, so every parent_id in it
// resolves to another row in the input — store.ListTasks and the foreign
// keys in 0001_init.sql guarantee this, so Flatten does not defend against
// an orphaned row it cannot place.
func Flatten(tasks []store.Task) []Row {
	children := make(map[string][]store.Task)
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

func flattenInto(t store.Task, depth int, children map[string][]store.Task, out *[]Row) {
	kids := children[t.ID]
	*out = append(*out, Row{Task: t, Depth: depth, HasChildren: len(kids) > 0})
	for _, k := range kids {
		flattenInto(k, depth+1, children, out)
	}
}
