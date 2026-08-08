package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/filipemolina/chore-crusher/src/config"
	"github.com/filipemolina/chore-crusher/src/store"
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
	mustCLI(t, data, parent)
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
	mustCLI(t, data, tid)
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
	mustCLI(t, data, c1)
	if out := mustCLI(t, data, "tasks", lid); !strings.Contains(out, "[~] parent (50%)") {
		t.Errorf("tasks after one child complete: %q, want parent at 50%%", out)
	}
	// Both children complete: the store auto-promotes the parent (§3) and the
	// whole tree moves to Complete.
	mustCLI(t, data, c2)
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

// TestCLIShowIncludesChildren pins hardening §6 assertion 4 (H4): `crush
// show --json` must emit non-empty children with depth relative to the shown
// task (child at 1, grandchild at 2) — the old code ran the descendant set
// through apptypes.Flatten, which only emits ParentID==nil rows, so children
// was always empty and the CLI diverged from the MCP's show_task.
func TestCLIShowIncludesChildren(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	parent := strings.TrimSpace(mustCLI(t, data, "add", lid, "Buy paint"))
	child := strings.TrimSpace(mustCLI(t, data, "add", lid, "Choose color", "--parent", parent))
	grand := strings.TrimSpace(mustCLI(t, data, "add", lid, "Hex code", "--parent", child))

	var payload showJSON
	mustJSONCLI(t, data, &payload, "show", parent, "--json")

	if len(payload.Children) != 2 {
		t.Fatalf("show children = %d, want 2 (child + grandchild): %+v", len(payload.Children), payload.Children)
	}
	if payload.Children[0].ID != child || payload.Children[0].Depth != 1 {
		t.Errorf("first child = %+v, want %q at depth 1", payload.Children[0], child)
	}
	if payload.Children[1].ID != grand || payload.Children[1].Depth != 2 {
		t.Errorf("second child = %+v, want %q at depth 2", payload.Children[1], grand)
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

// TestTasksJSONCarriesListOwner pins §10.5: the list_owner field (the
// parent list's created_by) appears on task rows and show output. An owner
// tag is set via `crush lists add --owner`, which is the CLI analogue of
// the MCP add_list(created_by=...) path.
func TestTasksJSONCarriesListOwner(t *testing.T) {
	data := t.TempDir()
	owned := strings.TrimSpace(mustCLI(t, data, "lists", "add", "pi: Sprint", "--owner", "pi"))
	mustCLI(t, data, "add", owned, "Write tests")

	// crush tasks --json carries list_owner on every row.
	var rows []taskRowJSON
	mustJSONCLI(t, data, &rows, "tasks", owned, "--json")
	if len(rows) != 1 || rows[0].ListOwner != "pi" {
		t.Errorf("tasks --json list_owner = %+v, want pi", rows)
	}

	// crush show --json carries list_owner on the task and its children.
	tid := strings.TrimSpace(mustCLI(t, data, "add", owned, "Child task", "--parent", rows[0].ID))
	var details showJSON
	mustJSONCLI(t, data, &details, "show", tid, "--json")
	if details.ListOwner != "pi" {
		t.Errorf("show --json list_owner = %q, want pi", details.ListOwner)
	}
}

// TestCommentRoundTrip pins the CLI comment surface (docs/plan/task-comments.md
// §5): `crush tasks comment` writes a comment attributed to the OS username,
// and `crush show --json` includes it in the comments array, oldest first.
func TestCommentRoundTrip(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	cid1 := strings.TrimSpace(mustCLI(t, data, "comment", tid, "first"))
	cid2 := strings.TrimSpace(mustCLI(t, data, "comment", tid, "second"))
	if cid1 == "" || cid2 == "" || cid1 == cid2 {
		t.Fatalf("expected two distinct comment ids, got %q and %q", cid1, cid2)
	}

	var details showJSON
	mustJSONCLI(t, data, &details, "show", tid, "--json")
	if len(details.Comments) != 2 {
		t.Fatalf("show --json comments = %d, want 2", len(details.Comments))
	}
	// Oldest first (ORDER BY created_at ASC).
	if details.Comments[0].Note != "first" || details.Comments[0].Author == "" {
		t.Errorf("first comment = %+v", details.Comments[0])
	}
	if details.Comments[0].ID != cid1 {
		t.Errorf("first comment id = %q, want %q", details.Comments[0].ID, cid1)
	}
	if details.Comments[1].Note != "second" || details.Comments[1].ID != cid2 {
		t.Errorf("second comment = %+v", details.Comments[1])
	}

	// Human mode shows the comments section.
	out := mustCLI(t, data, "show", tid)
	if !strings.Contains(out, "Comments (2):") || !strings.Contains(out, "first") {
		t.Errorf("show human output missing comments: %q", out)
	}
}

// TestCommentAuthorIsOSUsername pins that the CLI attribution is the OS user,
// not an empty string or a hardcoded placeholder.
func TestCommentAuthorIsOSUsername(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	mustCLI(t, data, "comment", tid, "note")

	var details showJSON
	mustJSONCLI(t, data, &details, "show", tid, "--json")
	if len(details.Comments) != 1 {
		t.Fatalf("want 1 comment, got %d", len(details.Comments))
	}
	author := details.Comments[0].Author
	if author == "" {
		t.Error("comment author must not be empty")
	}
	if author != osUser() {
		t.Errorf("comment author = %q, want osUser() = %q", author, osUser())
	}
}

// TestCommentRefusedOnDisabledList pins (docs/plan/task-comments.md §1): a
// task whose list has comments_disabled refuses new comments with a domain
// error (exit 1).
func TestCommentRefusedOnDisabledList(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	// No CLI toggle exists yet (deferred per the plan); disable via the
	// store method so the CLI enforcement path is exercised end to end.
	t.Setenv("XDG_DATA_HOME", data)
	s, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := s.SetCommentsDisabled(lid, true); err != nil {
		s.Close()
		t.Fatalf("SetCommentsDisabled: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("s.Close: %v", err)
	}

	code, _, errOut := runCLI(t, data, "comment", tid, "note")
	if code != 1 || !strings.Contains(errOut, "disabled") {
		t.Errorf("comment on disabled list: exit %d stderr %q, want exit 1 mentioning disabled", code, errOut)
	}
}

// TestCommentRefusedOnMissingTask pins the existence check at the CLI level:
// commenting on a nonexistent task id resolves to none and surfaces the
// store's not-found error as a domain failure (exit 1).
func TestCommentRefusedOnMissingTask(t *testing.T) {
	data := t.TempDir()
	code, _, errOut := runCLI(t, data, "comment", "01ARZ", "note")
	if code != 1 || !strings.Contains(errOut, "not found") {
		t.Errorf("comment on nonexistent task: exit %d stderr %q, want exit 1 mentioning not found", code, errOut)
	}
}

// TestCommentRmRequiresForce mirrors TestRmRequiresForce: no store call at
// all without --force, and the comment survives.
func TestCommentRmRequiresForce(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))
	cid := strings.TrimSpace(mustCLI(t, data, "comment", tid, "note"))

	code, _, errOut := runCLI(t, data, "comment", "rm", cid)
	if code != 1 || !strings.Contains(errOut, "--force") {
		t.Errorf("comment rm without --force: exit %d stderr %q", code, errOut)
	}

	var details showJSON
	mustJSONCLI(t, data, &details, "show", tid, "--json")
	if len(details.Comments) != 1 {
		t.Errorf("comment deleted despite missing --force: %+v", details.Comments)
	}
}

// TestCommentRmForce pins the --json contract on both success and failure
// (exactly one JSON value on stdout either way, docs/DESIGN.md §9): rm
// --force emits {"ok":true} and removes the comment; rm on a nonexistent id
// emits {"error":"..."}.
func TestCommentRmForce(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))
	cid := strings.TrimSpace(mustCLI(t, data, "comment", tid, "note"))

	var ok okPayload
	mustJSONCLI(t, data, &ok, "comment", "rm", cid, "--force", "--json")
	if !ok.OK {
		t.Errorf("comment rm --force --json = %+v, want ok:true", ok)
	}

	var details showJSON
	mustJSONCLI(t, data, &details, "show", tid, "--json")
	if len(details.Comments) != 0 {
		t.Errorf("comment still present after rm --force: %+v", details.Comments)
	}

	code, stdout, _ := runCLI(t, data, "comment", "rm", "no-such-comment", "--force", "--json")
	if code != 1 {
		t.Errorf("comment rm nonexistent: exit %d, want 1", code)
	}
	var errPayload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &errPayload); err != nil {
		t.Fatalf("stdout %q is not one JSON value: %v", stdout, err)
	}
	if !strings.Contains(errPayload.Error, "not found") {
		t.Errorf("comment rm nonexistent error = %q, want it to mention 'not found'", errPayload.Error)
	}
}

// TestShowIncludesCommentsArray pins that `crush show --json` emits the
// comments field (even when empty, as an empty array) so callers always
// get the key — additive per docs/DESIGN.md §9.
func TestShowIncludesCommentsArray(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	var details showJSON
	mustJSONCLI(t, data, &details, "show", tid, "--json")
	if details.Comments == nil {
		t.Error("show --json should always include comments (even when empty)")
	}
	if len(details.Comments) != 0 {
		t.Errorf("new task should have 0 comments, got %d", len(details.Comments))
	}
}

// TestAssignJSONShapes pins the step-5 acceptance criterion for `crush
// assign` (docs/DESIGN.md §9): exactly one JSON value on stdout, success or
// failure. Success echoes the caller's own tag; a conflict without --force
// is the §9 error shape naming the holder; --force takes the task and
// echoes the new holder.
func TestAssignJSONShapes(t *testing.T) {
	data := t.TempDir()
	t.Setenv("CRUSH_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	var res assignResultJSON
	mustJSONCLI(t, data, &res, "assign", tid, "--json")
	if !res.OK || res.Assignee != "pi" {
		t.Fatalf("assign --json = %+v, want ok:true assignee:pi", res)
	}

	// The assignment landed: show carries the assignee and a non-null
	// assigned_at.
	var details showJSON
	mustJSONCLI(t, data, &details, "show", tid, "--json")
	if details.Assignee != "pi" || details.AssignedAt == nil {
		t.Errorf("show after assign: assignee %q assigned_at %v", details.Assignee, details.AssignedAt)
	}

	// A second agent's assign without --force fails with exactly one JSON
	// error value naming the holder.
	t.Setenv("CRUSH_AGENT", "claude")
	code, out, errOut := runCLI(t, data, "assign", tid, "--json")
	if code != 1 || errOut != "" {
		t.Fatalf("conflicting assign: exit %d stderr %q, want exit 1 with empty stderr", code, errOut)
	}
	var errPayload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &errPayload); err != nil {
		t.Fatalf("stdout %q is not one JSON value: %v", out, err)
	}
	if !strings.Contains(errPayload.Error, "pi") {
		t.Errorf("conflict error %q should name the holder", errPayload.Error)
	}

	// --force takes it, echoing the new holder.
	mustJSONCLI(t, data, &res, "assign", tid, "--force", "--json")
	if !res.OK || res.Assignee != "claude" {
		t.Fatalf("assign --force --json = %+v, want ok:true assignee:claude", res)
	}
}

// TestUnassignJSONShapes pins `crush unassign <task-id>`: the holder's
// release emits {"ok":true,"assignee":""} and clears assigned_at; releasing
// another agent's task is the §9 error shape.
func TestUnassignJSONShapes(t *testing.T) {
	data := t.TempDir()
	t.Setenv("CRUSH_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))
	mustCLI(t, data, "assign", tid)

	// Another agent cannot release it.
	t.Setenv("CRUSH_AGENT", "claude")
	code, out, _ := runCLI(t, data, "unassign", tid, "--json")
	if code != 1 {
		t.Fatalf("foreign unassign: exit %d, want 1", code)
	}
	var errPayload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &errPayload); err != nil {
		t.Fatalf("stdout %q is not one JSON value: %v", out, err)
	}

	// The holder's release succeeds and clears both fields.
	t.Setenv("CRUSH_AGENT", "pi")
	var res assignResultJSON
	mustJSONCLI(t, data, &res, "unassign", tid, "--json")
	if !res.OK || res.Assignee != "" {
		t.Fatalf("unassign --json = %+v, want ok:true assignee:\"\"", res)
	}
	var details showJSON
	mustJSONCLI(t, data, &details, "show", tid, "--json")
	if details.Assignee != "" || details.AssignedAt != nil {
		t.Errorf("show after unassign: assignee %q assigned_at %v", details.Assignee, details.AssignedAt)
	}

	// No task id and no --list is a usage error (exit 2).
	if code, _, _ := runCLI(t, data, "unassign"); code != 2 {
		t.Errorf("unassign with no target: exit %d, want 2", code)
	}
}

// TestUnassignListJSONShape pins `crush unassign --list`: one JSON value
// reporting how many assignments were cleared — 0 on a list holding none is
// a success, not an error (docs/DESIGN.md §9).
func TestUnassignListJSONShape(t *testing.T) {
	data := t.TempDir()
	t.Setenv("CRUSH_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	t1 := strings.TrimSpace(mustCLI(t, data, "add", lid, "one"))
	t2 := strings.TrimSpace(mustCLI(t, data, "add", lid, "two"))
	mustCLI(t, data, "add", lid, "unassigned")
	mustCLI(t, data, "assign", t1)
	mustCLI(t, data, "assign", t2)

	var rel releasedJSON
	mustJSONCLI(t, data, &rel, "unassign", "--list", lid, "--json")
	if !rel.OK || rel.Released != 2 {
		t.Fatalf("unassign --list --json = %+v, want ok:true released:2", rel)
	}

	var rows []taskRowJSON
	mustJSONCLI(t, data, &rows, "tasks", lid, "--json")
	for _, r := range rows {
		if r.Assignee != "" {
			t.Errorf("row %s still assigned after unassign --list: %+v", r.ID, r)
		}
	}

	// A list with nothing assigned releases 0 — success, not an error.
	mustJSONCLI(t, data, &rel, "unassign", "--list", lid, "--json")
	if !rel.OK || rel.Released != 0 {
		t.Fatalf("second unassign --list --json = %+v, want ok:true released:0", rel)
	}
}

// TestPriorityJSONShapes pins `crush priority`: the echoed level on success,
// the §9 error shape for an invalid level, and — the §6.5 trap — an omitted
// --level failing as an error rather than defaulting to none.
func TestPriorityJSONShapes(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	var res priorityResultJSON
	mustJSONCLI(t, data, &res, "priority", tid, "--level", "high", "--json")
	if !res.OK || res.Priority != "high" {
		t.Fatalf("priority --json = %+v, want ok:true priority:high", res)
	}

	// The level landed where both read surfaces report it.
	var details showJSON
	mustJSONCLI(t, data, &details, "show", tid, "--json")
	if details.Priority != "high" {
		t.Errorf("show priority = %q, want high", details.Priority)
	}
	var rows []taskRowJSON
	mustJSONCLI(t, data, &rows, "tasks", lid, "--json")
	if len(rows) != 1 || rows[0].Priority != "high" {
		t.Errorf("tasks rows = %+v, want priority high", rows)
	}

	// An invalid level is the §9 error shape, exit 1.
	code, out, _ := runCLI(t, data, "priority", tid, "--level", "urgent", "--json")
	if code != 1 {
		t.Fatalf("invalid level: exit %d, want 1", code)
	}
	var errPayload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &errPayload); err != nil {
		t.Fatalf("stdout %q is not one JSON value: %v", out, err)
	}
	if !strings.Contains(errPayload.Error, "invalid priority") {
		t.Errorf("invalid-level error = %q, want store's invalid-priority message", errPayload.Error)
	}

	// An omitted --level fails with the §9 error shape and does not default
	// to none (docs/plan/mcp-assignment-and-priorities.md §6.5).
	code, out, _ = runCLI(t, data, "priority", tid, "--json")
	if code != 1 {
		t.Fatalf("missing --level: exit %d, want 1", code)
	}
	if err := json.Unmarshal([]byte(out), &errPayload); err != nil {
		t.Fatalf("stdout %q is not one JSON value: %v", out, err)
	}
	if !strings.Contains(errPayload.Error, "--level") {
		t.Errorf("missing-level error = %q, want it to name --level", errPayload.Error)
	}
	mustJSONCLI(t, data, &details, "show", tid, "--json")
	if details.Priority != "high" {
		t.Errorf("priority after refused writes = %q, want high unchanged", details.Priority)
	}
}

// TestTasksRowsCarryAssignment pins the new fields on `crush tasks --json`
// rows: assignee is "" when unassigned and the holder's tag once assigned.
func TestTasksRowsCarryAssignment(t *testing.T) {
	data := t.TempDir()
	t.Setenv("CRUSH_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	var rows []taskRowJSON
	mustJSONCLI(t, data, &rows, "tasks", lid, "--json")
	if len(rows) != 1 || rows[0].Assignee != "" || rows[0].Priority != "none" {
		t.Fatalf("new task rows = %+v, want assignee \"\" and priority none", rows)
	}

	mustCLI(t, data, "assign", tid)
	mustJSONCLI(t, data, &rows, "tasks", lid, "--json")
	if rows[0].Assignee != "pi" {
		t.Errorf("assigned row = %+v, want assignee pi", rows[0])
	}
}
