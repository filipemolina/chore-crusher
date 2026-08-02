package cli

import (
	"strings"
	"testing"
)

func TestTaskTreeAndCascade(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home"))
	parent := strings.TrimSpace(mustCLI(t, data, "add", lid, "Buy paint"))
	child := strings.TrimSpace(mustCLI(t, data, "add", lid, "Choose color", "--parent", parent))

	// Tree: parent under Pending, child nested one level (docs/DESIGN.md §12
	// row layout: glyph column, checkbox, then the title).
	out := mustCLI(t, data, "tasks", lid)
	if !strings.Contains(out, "Pending (2)") {
		t.Errorf("tasks: %q, want a Pending (2) header", out)
	}
	if !strings.Contains(out, "▾ [ ] Buy paint") || !strings.Contains(out, "  [ ] Choose color") {
		t.Errorf("tasks: %q, want parent with a nested child row", out)
	}

	// Completing the parent cascades to the child.
	mustCLI(t, data, "complete", parent)
	var payload []taskRowJSON
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	if payload[0].Status != "complete" || payload[1].Status != "complete" || payload[1].ID != child {
		t.Errorf("after cascade: %+v", payload)
	}

	// Reopening the parent does not cascade.
	mustCLI(t, data, "reopen", parent)
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	if payload[0].Status != "pending" || payload[1].Status != "complete" {
		t.Errorf("after reopen: %+v", payload)
	}

	// Toggle flips whichever applies: complete parent back to pending.
	mustCLI(t, data, "toggle", parent)
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	if payload[0].Status != "complete" {
		t.Errorf("after toggle: %+v", payload)
	}

	// §6: the whole tree renders under Complete once its root is complete.
	out = mustCLI(t, data, "tasks", lid)
	if !strings.Contains(out, "Complete (2)") || strings.Contains(out, "Pending") {
		t.Errorf("tasks after toggle: %q, want a Complete (2) header and no Pending", out)
	}

	// --flat: id, status, title per line, no headers.
	out = mustCLI(t, data, "tasks", lid, "--flat")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], parent+"\tcomplete\t") {
		t.Errorf("tasks --flat: %q", out)
	}
}

func TestProgressValidationAndDisplay(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	// The mode/percent combination rules are store's validation
	// (docs/DESIGN.md §3), surfaced as domain errors — exit 1, store's message.
	code, _, errOut := runCLI(t, data, "progress", tid, "--mode", "percentage")
	if code != 1 || !strings.Contains(errOut, "requires a percent") {
		t.Errorf("percentage without --percent: exit %d stderr %q", code, errOut)
	}
	code, _, errOut = runCLI(t, data, "progress", tid, "--mode", "simple", "--percent", "50")
	if code != 1 || !strings.Contains(errOut, "only valid") {
		t.Errorf("simple with --percent: exit %d stderr %q", code, errOut)
	}
	code, _, errOut = runCLI(t, data, "progress", tid, "--mode", "percentage", "--percent", "150")
	if code != 1 || !strings.Contains(errOut, "out of range") {
		t.Errorf("percent 150: exit %d stderr %q", code, errOut)
	}

	// A valid percentage starts the task (pending -> in_progress) and shows
	// the trailing suffix.
	mustCLI(t, data, "progress", tid, "--mode", "percentage", "--percent", "60")
	var payload []taskRowJSON
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	if payload[0].Status != "in_progress" || payload[0].Progress.Percent == nil || *payload[0].Progress.Percent != 60 {
		t.Errorf("after progress: %+v", payload[0])
	}
	out := mustCLI(t, data, "tasks", lid)
	if !strings.Contains(out, "[~] task (60%)") {
		t.Errorf("tasks: %q, want an in-progress row with a 60%% suffix", out)
	}

	// Setting progress on a complete task is a domain error, not a reopen.
	mustCLI(t, data, "complete", tid)
	code, _, errOut = runCLI(t, data, "progress", tid, "--mode", "percentage", "--percent", "50")
	if code != 1 || !strings.Contains(errOut, "reopen") {
		t.Errorf("progress on complete: exit %d stderr %q", code, errOut)
	}
}

func TestSubtasksDerivationThroughCLI(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	parent := strings.TrimSpace(mustCLI(t, data, "add", lid, "parent"))
	c1 := strings.TrimSpace(mustCLI(t, data, "add", lid, "child 1", "--parent", parent))
	c2 := strings.TrimSpace(mustCLI(t, data, "add", lid, "child 2", "--parent", parent))
	mustCLI(t, data, "progress", parent, "--mode", "subtasks")

	// Zero children done renders 0%, not the simple fallback (it has children).
	if out := mustCLI(t, data, "tasks", lid); !strings.Contains(out, "[~] parent (0%)") {
		t.Errorf("tasks after subtasks mode: %q, want parent at 0%%", out)
	}
	// One of two children complete: 50%.
	mustCLI(t, data, "complete", c1)
	if out := mustCLI(t, data, "tasks", lid); !strings.Contains(out, "[~] parent (50%)") {
		t.Errorf("tasks after one child complete: %q, want parent at 50%%", out)
	}
	// Both children complete: the store auto-promotes the parent (§3) and the
	// whole tree moves to Complete.
	mustCLI(t, data, "complete", c2)
	out := mustCLI(t, data, "tasks", lid)
	if !strings.Contains(out, "Complete (3)") || strings.Contains(out, "Pending") {
		t.Errorf("tasks after both children complete: %q, want the whole tree under Complete", out)
	}
}

func TestTasksStatusFilter(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	mustCLI(t, data, "add", lid, "pending task")
	started := strings.TrimSpace(mustCLI(t, data, "add", lid, "started task"))
	mustCLI(t, data, "progress", started, "--mode", "simple")

	out := mustCLI(t, data, "tasks", lid, "--status", "pending")
	if !strings.Contains(out, "pending task") || strings.Contains(out, "started task") {
		t.Errorf("--status pending: %q", out)
	}
	out = mustCLI(t, data, "tasks", lid, "--status", "in_progress")
	if !strings.Contains(out, "started task") || strings.Contains(out, "pending task") {
		t.Errorf("--status in_progress: %q", out)
	}

	// A bad enum value is a validation failure, exit 1 (§9), not a silent
	// "show everything".
	code, _, errOut := runCLI(t, data, "tasks", lid, "--status", "bogus")
	if code != 1 || !strings.Contains(errOut, "--status") {
		t.Errorf("--status bogus: exit %d stderr %q", code, errOut)
	}
}

func TestShow(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "Buy paint"))
	mustCLI(t, data, "notes", tid, "two\nlines")
	mustCLI(t, data, "progress", tid, "--mode", "subtasks")

	// Setting progress starts the task, and a subtasks task with no children
	// displays as simple (§3) — spelled out, not a misleading 0%.
	out := mustCLI(t, data, "show", tid)
	for _, want := range []string{"Title: Buy paint", "Status: in_progress",
		"Progress: subtasks (simple)", "Notes:", "  two", "  lines"} {
		if !strings.Contains(out, want) {
			t.Errorf("show missing %q in:\n%s", want, out)
		}
	}

	var payload showJSON
	mustJSONCLI(t, data, &payload, "show", tid, "--json")
	if payload.Title != "Buy paint" || payload.Notes != "two\nlines" ||
		payload.Progress.Kind != "subtasks" || payload.Progress.DisplayAsSimple != true {
		t.Errorf("show --json: %+v", payload)
	}
}

func TestRmRequiresForce(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	code, _, errOut := runCLI(t, data, "rm", tid)
	if code != 1 || !strings.Contains(errOut, "--force") {
		t.Errorf("rm without --force: exit %d stderr %q", code, errOut)
	}
	// The task survives a refused rm.
	if out := mustCLI(t, data, "tasks", lid); !strings.Contains(out, "[ ] task") {
		t.Errorf("tasks after refused rm: %q", out)
	}
	mustCLI(t, data, "rm", tid, "--force")
	if out := mustCLI(t, data, "tasks", lid); out != "" {
		t.Errorf("tasks after rm --force: %q, want empty", out)
	}
}

func TestMoveReparents(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home"))
	root := strings.TrimSpace(mustCLI(t, data, "add", lid, "Buy paint"))
	child := strings.TrimSpace(mustCLI(t, data, "add", lid, "Choose color", "--parent", root))
	other := strings.TrimSpace(mustCLI(t, data, "add", lid, "Clean gutters"))

	// `complete mv` is a deliberate restructure: move a root task under
	// another task's subtree, with no ±1-level add-flow restriction.
	mustCLI(t, data, "mv", other, "--parent", root)
	var payload []taskRowJSON
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	if len(payload) != 3 || payload[0].ID != root || payload[1].ID != child || payload[2].ID != other {
		t.Fatalf("after mv preorder = %s,%s,%s; want root, child, other", payload[0].ID, payload[1].ID, payload[2].ID)
	}
	for _, p := range payload[1:] {
		if p.ParentID == nil || *p.ParentID != root {
			t.Errorf("row %s parent = %v after mv, want root", p.ID, p.ParentID)
		}
	}

	// An empty --parent (the flag default, i.e. omitting it) moves to root.
	mustCLI(t, data, "mv", child, "--parent", "")
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	for _, p := range payload {
		if p.ID == child && p.ParentID != nil {
			t.Errorf("child parent = %v after moving to root, want nil", p.ParentID)
		}
	}

	// Cycle and missing-parent failures are domain errors (exit 1).
	code, _, errOut := runCLI(t, data, "mv", root, "--parent", other)
	if code != 1 || !strings.Contains(errOut, "cycle") {
		t.Errorf("moving a task under its own descendant: exit %d stderr %q", code, errOut)
	}
	code, _, _ = runCLI(t, data, "mv", root, "--parent", "01ARZ")
	if code != 1 {
		t.Errorf("mv to a missing parent: exit %d, want 1", code)
	}
}
