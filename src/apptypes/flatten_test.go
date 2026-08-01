package apptypes

import "testing"

func strptr(s string) *string { return &s }

// TestFlattenPreorderWithDepth exercises the ordering contract both the
// CLI's tree and the TUI's tree render from: depth-first preorder, parents
// before children, depth and has-children annotated per row. The top-level
// input order is deliberately scrambled to prove ordering comes from the
// walk, not from input order; within a sibling set the input order is
// preserved (that is ListTasks's position order for real data).
func TestFlattenPreorderWithDepth(t *testing.T) {
	rootA := Task{ID: "a", Title: "root a"}
	child := Task{ID: "a1", Title: "child", ParentID: strptr("a")}
	grand := Task{ID: "a1a", Title: "grand", ParentID: strptr("a1")}
	leaf := Task{ID: "a2", Title: "leaf", ParentID: strptr("a")}
	rootB := Task{ID: "b", Title: "root b"}

	rows := Flatten([]Task{rootB, child, rootA, grand, leaf})

	want := []struct {
		id          string
		depth       int
		hasChildren bool
	}{
		{"b", 0, false},
		{"a", 0, true},
		{"a1", 1, true},
		{"a1a", 2, false},
		{"a2", 1, false},
	}
	if len(rows) != len(want) {
		t.Fatalf("Flatten returned %d rows, want %d", len(rows), len(want))
	}
	for i, w := range want {
		got := rows[i]
		if got.Task.ID != w.id || got.Depth != w.depth || got.HasChildren != w.hasChildren {
			t.Errorf("row %d: got (id=%s depth=%d hasChildren=%v), want (id=%s depth=%d hasChildren=%v)",
				i, got.Task.ID, got.Depth, got.HasChildren, w.id, w.depth, w.hasChildren)
		}
	}
}

func TestFlattenEmpty(t *testing.T) {
	if rows := Flatten(nil); len(rows) != 0 {
		t.Errorf("Flatten(nil): got %d rows, want 0", len(rows))
	}
}
