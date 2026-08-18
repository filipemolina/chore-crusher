package cli

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/filipemolina/farol/src/config"
	"github.com/filipemolina/farol/src/store"
)

func TestTaskTreeAndCascade(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home", "--owner", "pi"))
	parent := strings.TrimSpace(mustCLI(t, data, "add", lid, "Buy paint"))
	// Set parent to percentage mode to prevent our auto-switch to subtasks when adding a child.
	// This way the test can verify the original behavior.
	mustCLI(t, data, "progress", parent, "--mode", "percentage", "--percent", "0")
	child := strings.TrimSpace(mustCLI(t, data, "add", lid, "Choose color", "--parent", parent))

	// Tree: parent under Pending, child nested one level (docs/DESIGN.md §12
	// row layout: glyph column, checkbox, then the title).
	out := mustCLI(t, data, "tasks", lid)
	if !strings.Contains(out, "Pending (2)") {
		t.Errorf("tasks: %q, want a Pending (2) header", out)
	}
	if !strings.Contains(out, "▾ [~] Buy paint (0%)") || !strings.Contains(out, "  [ ] Choose color") {
		t.Errorf("tasks: %q, want parent with a nested child row", out)
	}

	// Completing the parent cascades to the child.
	mustCLI(t, data, parent)
	var payload listTasksResult
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	rows := payload.Tasks
	if rows[0].Status != "complete" || rows[1].Status != "complete" || rows[1].ID != child {
		t.Errorf("after cascade: %+v", rows)
	}

	// Reopening the parent does not cascade.
	mustCLI(t, data, "reopen", parent)
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	rows = payload.Tasks
	if rows[0].Status != "pending" || rows[1].Status != "complete" {
		t.Errorf("after reopen: %+v", rows)
	}

	// Toggle flips whichever applies: complete parent back to pending.
	mustCLI(t, data, "toggle", parent)
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	rows = payload.Tasks
	if rows[0].Status != "complete" {
		t.Errorf("after toggle: %+v", rows)
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
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
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
	var payload listTasksResult
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	rows := payload.Tasks
	if rows[0].Status != "in_progress" || rows[0].Progress.Percent == nil || *rows[0].Progress.Percent != 60 {
		t.Errorf("after progress: %+v", rows[0])
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
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
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
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
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
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
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

	var payloadList []showJSON
	mustJSONCLI(t, data, &payloadList, "show", tid, "--json")
	payload := payloadList[0]
	if payload.Title != "Buy paint" || payload.Notes != "two\nlines" ||
		payload.Progress.Kind != "subtasks" || payload.Progress.DisplayAsSimple != true {
		t.Errorf("show --json: %+v", payload)
	}
}

// TestShowMentionsJSON pins the mention metadata in `farol show --json`:
// title_mentions and notes_mentions arrays with id, title, start, end, and
// deleted flag per the task-mentions plan Commit 2.
func TestShowMentionsJSON(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))

	// Create a task to be mentioned.
	mentioned := strings.TrimSpace(mustCLI(t, data, "add", lid, "Login validation"))

	// Create a task that mentions the first task in title and notes.
	title := "Fix bug in @" + mentioned
	notes := "Related to @" + mentioned + " and @01ARZ9Y6Z7A8B9C0D1E2F3G4H5"
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, title, "--notes", notes))

	var payloadList []showJSON
	mustJSONCLI(t, data, &payloadList, "show", tid, "--json")
	payload := payloadList[0]

	// Title mentions: one mention in the title.
	if len(payload.TitleMentions) != 1 {
		t.Fatalf("title_mentions = %d, want 1: %+v", len(payload.TitleMentions), payload.TitleMentions)
	}
	tm := payload.TitleMentions[0]
	if tm.ID != mentioned {
		t.Errorf("title_mentions[0].ID = %q, want %q", tm.ID, mentioned)
	}
	if tm.Title == nil || *tm.Title != "Login validation" {
		t.Errorf("title_mentions[0].Title = %v, want \"Login validation\"", tm.Title)
	}
	// "Fix bug in @" = 11 chars (indices 0-10), @ at 11, ULID 26 chars (12-37), end = 38
	if tm.Start != 11 || tm.End != 38 {
		t.Errorf("title_mentions[0] Start/End = %d/%d, want 11/38", tm.Start, tm.End)
	}
	if tm.Deleted {
		t.Errorf("title_mentions[0].Deleted = true, want false")
	}

	// Notes mentions: two mentions, one valid, one deleted.
	if len(payload.NotesMentions) != 2 {
		t.Fatalf("notes_mentions = %d, want 2: %+v", len(payload.NotesMentions), payload.NotesMentions)
	}
	nm1 := payload.NotesMentions[0]
	if nm1.ID != mentioned {
		t.Errorf("notes_mentions[0].ID = %q, want %q", nm1.ID, mentioned)
	}
	if nm1.Title == nil || *nm1.Title != "Login validation" {
		t.Errorf("notes_mentions[0].Title = %v, want \"Login validation\"", nm1.Title)
	}
	// "Related to @" = 11 chars (indices 0-10), @ at 11, ULID 26 chars (12-37), end = 38
	if nm1.Start != 11 || nm1.End != 38 {
		t.Errorf("notes_mentions[0] Start/End = %d/%d, want 11/38", nm1.Start, nm1.End)
	}
	if nm1.Deleted {
		t.Errorf("notes_mentions[0].Deleted = true, want false")
	}

	nm2 := payload.NotesMentions[1]
	if nm2.ID != "01ARZ9Y6Z7A8B9C0D1E2F3G4H5" {
		t.Errorf("notes_mentions[1].ID = %q, want deleted task ID", nm2.ID)
	}
	if nm2.Title != nil {
		t.Errorf("notes_mentions[1].Title = %v, want nil", nm2.Title)
	}
	if !nm2.Deleted {
		t.Errorf("notes_mentions[1].Deleted = false, want true")
	}
	// "Related to @<ULID> and @" = 11 + 27 + 5 = 43, second @ at 43, ULID 26 chars (44-69), end = 70
	if nm2.Start != 43 || nm2.End != 70 {
		t.Errorf("notes_mentions[1] Start/End = %d/%d, want 43/70", nm2.Start, nm2.End)
	}
}

// TestShowMentionsHuman pins the human-readable mention rendering in
// `farol show`: @Task Title for resolved, [deleted task] for missing.
func TestShowMentionsHuman(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))

	// Create a task to be mentioned.
	mentioned := strings.TrimSpace(mustCLI(t, data, "add", lid, "Login validation"))

	// Create a task that mentions the first task in title and notes.
	title := "Fix bug in @" + mentioned
	notes := "Related to @" + mentioned + " and @01ARZ9Y6Z7A8B9C0D1E2F3G4H5"
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, title, "--notes", notes))

	out := mustCLI(t, data, "show", tid)

	// Title should show resolved mention.
	if !strings.Contains(out, "Title: Fix bug in @Login validation") {
		t.Errorf("show title missing resolved mention:\n%s", out)
	}

	// Notes should show resolved mention and [deleted task].
	if !strings.Contains(out, "Related to @Login validation and [deleted task]") {
		t.Errorf("show notes missing resolved/deleted mentions:\n%s", out)
	}
}

// TestShowMentionsDeletedTask pins that a mention to a task that is later
// deleted renders as [deleted task] in both JSON and human output.
// Note: creating a task with a mention to a deleted task is now rejected
// by the store (mirroring SetNotes validation), so this test verifies
// the show output for a task that was created before the mentioned task
// was deleted (simulated by directly manipulating the store).
func TestShowMentionsDeletedTask(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))

	// Create a task to be mentioned.
	mentioned := strings.TrimSpace(mustCLI(t, data, "add", lid, "To be deleted"))

	// Create a task that mentions the first task.
	title := "See @" + mentioned
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, title))

	// Now delete the mentioned task.
	mustCLI(t, data, "rm", mentioned, "--force")

	// JSON: title_mentions has deleted=true, title=null.
	var payloadList []showJSON
	mustJSONCLI(t, data, &payloadList, "show", tid, "--json")
	payload := payloadList[0]
	if len(payload.TitleMentions) != 1 {
		t.Fatalf("title_mentions = %d, want 1", len(payload.TitleMentions))
	}
	tm := payload.TitleMentions[0]
	if tm.ID != mentioned {
		t.Errorf("ID = %q, want %q", tm.ID, mentioned)
	}
	if tm.Title != nil {
		t.Errorf("Title = %v, want nil", tm.Title)
	}
	if !tm.Deleted {
		t.Errorf("Deleted = false, want true")
	}

	// Human: shows [deleted task].
	out := mustCLI(t, data, "show", tid)
	if !strings.Contains(out, "Title: See [deleted task]") {
		t.Errorf("show human output missing [deleted task]:\n%s", out)
	}
}

// TestCLIShowIncludesChildren pins H4: `farol show --json` must emit
// non-empty children with depth relative to the shown
// task (child at 1, grandchild at 2) — the old code ran the descendant set
// through apptypes.Flatten, which only emits ParentID==nil rows, so children
// was always empty and the CLI diverged from the MCP's show_task.
func TestCLIShowIncludesChildren(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	parent := strings.TrimSpace(mustCLI(t, data, "add", lid, "Buy paint"))
	child := strings.TrimSpace(mustCLI(t, data, "add", lid, "Choose color", "--parent", parent))
	grand := strings.TrimSpace(mustCLI(t, data, "add", lid, "Hex code", "--parent", child))

	var payloadList []showJSON
	mustJSONCLI(t, data, &payloadList, "show", parent, "--json")
	payload := payloadList[0]

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
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
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
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home", "--owner", "pi"))
	root := strings.TrimSpace(mustCLI(t, data, "add", lid, "Buy paint"))
	child := strings.TrimSpace(mustCLI(t, data, "add", lid, "Choose color", "--parent", root))
	other := strings.TrimSpace(mustCLI(t, data, "add", lid, "Clean gutters"))

	// `complete mv` is a deliberate restructure: move a root task under
	// another task's subtree, with no ±1-level add-flow restriction.
	mustCLI(t, data, "mv", other, "--parent", root)
	var payload listTasksResult
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	rows := payload.Tasks
	if len(rows) != 3 || rows[0].ID != root || rows[1].ID != child || rows[2].ID != other {
		t.Fatalf("after mv preorder = %s,%s,%s; want root, child, other", rows[0].ID, rows[1].ID, rows[2].ID)
	}
	for _, p := range rows[1:] {
		if p.ParentID == nil || *p.ParentID != root {
			t.Errorf("row %s parent = %v after mv, want root", p.ID, p.ParentID)
		}
	}

	// An empty --parent (the flag default, i.e. omitting it) moves to root.
	mustCLI(t, data, "mv", child, "--parent", "")
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	rows = payload.Tasks
	for _, p := range rows {
		if p.ID == child && p.ParentID != nil {
			t.Errorf("child parent = %v after mv to root, want nil", p.ParentID)
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

// TestTasksJSONCarriesListOwner pins the list_owner field (the
// parent list's created_by) appears on task rows and show output. An owner
// tag is set via `farol lists add --owner`, which is the CLI analogue of
// the MCP add_list(created_by=...) path.
func TestTasksJSONCarriesListOwner(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	owned := strings.TrimSpace(mustCLI(t, data, "lists", "add", "pi: Sprint", "--owner", "pi"))
	mustCLI(t, data, "add", owned, "Write tests")

	// farol tasks --json carries list_owner on every row.
	var res listTasksResult
	mustJSONCLI(t, data, &res, "tasks", owned, "--json")
	rows := res.Tasks
	if len(rows) != 1 || rows[0].ListOwner != "pi" {
		t.Errorf("tasks --json list_owner = %+v, want pi", rows)
	}

	// farol show --json carries list_owner on the task and its children.
	tid := strings.TrimSpace(mustCLI(t, data, "add", owned, "Child task", "--parent", rows[0].ID))
	var detailsList []showJSON
	mustJSONCLI(t, data, &detailsList, "show", tid, "--json")
	details := detailsList[0]
	if details.ListOwner != "pi" {
		t.Errorf("show --json list_owner = %q, want pi", details.ListOwner)
	}
}

// TestCommentRoundTrip pins the CLI comment surface: `farol tasks
// comment` writes a comment attributed to the OS username, and
// `farol show --json` includes it in the comments array, oldest first.
func TestCommentRoundTrip(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	cid1 := strings.TrimSpace(mustCLI(t, data, "comment", tid, "first"))
	cid2 := strings.TrimSpace(mustCLI(t, data, "comment", tid, "second"))
	if cid1 == "" || cid2 == "" || cid1 == cid2 {
		t.Fatalf("expected two distinct comment ids, got %q and %q", cid1, cid2)
	}

	var detailsList []showJSON
	mustJSONCLI(t, data, &detailsList, "show", tid, "--json")
	details := detailsList[0]
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
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	mustCLI(t, data, "comment", tid, "note")

	var detailsList []showJSON
	mustJSONCLI(t, data, &detailsList, "show", tid, "--json")
	details := detailsList[0]
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

// TestCommentRefusedOnDisabledList pins the list-level disable flag: a task
// whose list has comments_disabled refuses new comments with a domain error
// (exit 1).
func TestCommentRefusedOnDisabledList(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home", "--owner", "pi"))
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
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))
	cid := strings.TrimSpace(mustCLI(t, data, "comment", tid, "note"))

	code, _, errOut := runCLI(t, data, "comment", "rm", cid)
	if code != 1 || !strings.Contains(errOut, "--force") {
		t.Errorf("comment rm without --force: exit %d stderr %q", code, errOut)
	}

	var detailsList []showJSON
	mustJSONCLI(t, data, &detailsList, "show", tid, "--json")
	details := detailsList[0]
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
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))
	cid := strings.TrimSpace(mustCLI(t, data, "comment", tid, "note"))

	var ok okPayload
	mustJSONCLI(t, data, &ok, "comment", "rm", cid, "--force", "--json")
	if !ok.OK {
		t.Errorf("comment rm --force --json = %+v, want ok:true", ok)
	}

	var detailsList []showJSON
	mustJSONCLI(t, data, &detailsList, "show", tid, "--json")
	details := detailsList[0]
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

// TestShowIncludesCommentsArray pins that `farol show --json` emits the
// comments field (even when empty, as an empty array) so callers always
// get the key — additive per docs/DESIGN.md §9.
func TestShowIncludesCommentsArray(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	var detailsList []showJSON
	mustJSONCLI(t, data, &detailsList, "show", tid, "--json")
	details := detailsList[0]
	if details.Comments == nil {
		t.Error("show --json should always include comments (even when empty)")
	}
	if len(details.Comments) != 0 {
		t.Errorf("new task should have 0 comments, got %d", len(details.Comments))
	}
}

// TestAssignJSONShapes pins the step-5 acceptance criterion for `farol
// assign` (docs/DESIGN.md §9): exactly one JSON value on stdout, success or
// failure. Success echoes the caller's own tag; a conflict without --force
// is the §9 error shape naming the holder; --force takes the task and
// echoes the new holder.
func TestAssignJSONShapes(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	var res assignResultJSON
	mustJSONCLI(t, data, &res, "assign", tid, "--json")
	if !res.OK || res.Assignee != "pi" {
		t.Fatalf("assign --json = %+v, want ok:true assignee:pi", res)
	}

	// The assignment landed: show carries the assignee and a non-null
	// assigned_at.
	var detailsList []showJSON
	mustJSONCLI(t, data, &detailsList, "show", tid, "--json")
	details := detailsList[0]
	if details.Assignee != "pi" || details.AssignedAt == nil {
		t.Errorf("show after assign: assignee %q assigned_at %v", details.Assignee, details.AssignedAt)
	}

	// A second agent's assign without --force fails with exactly one JSON
	// error value naming the holder.
	t.Setenv("FAROL_AGENT", "claude")
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

// TestUnassignJSONShapes pins `farol unassign <task-id>`: the holder's
// release emits {"ok":true,"assignee":""} and clears assigned_at; releasing
// another agent's task is the §9 error shape.
func TestUnassignJSONShapes(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))
	mustCLI(t, data, "assign", tid)

	// Another agent cannot release it.
	t.Setenv("FAROL_AGENT", "claude")
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
	t.Setenv("FAROL_AGENT", "pi")
	var res assignResultJSON
	mustJSONCLI(t, data, &res, "unassign", tid, "--json")
	if !res.OK || res.Assignee != "" {
		t.Fatalf("unassign --json = %+v, want ok:true assignee:\"\"", res)
	}
	var detailsList []showJSON
	mustJSONCLI(t, data, &detailsList, "show", tid, "--json")
	details := detailsList[0]
	if details.Assignee != "" || details.AssignedAt != nil {
		t.Errorf("show after unassign: assignee %q assigned_at %v", details.Assignee, details.AssignedAt)
	}

	// No task id and no --list is a usage error (exit 2).
	if code, _, _ := runCLI(t, data, "unassign"); code != 2 {
		t.Errorf("unassign with no target: exit %d, want 2", code)
	}
}

// TestUnassignListJSONShape pins `farol unassign --list`: one JSON value
// reporting how many assignments were cleared — 0 on a list holding none is
// a success, not an error (docs/DESIGN.md §9).
func TestUnassignListJSONShape(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
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

	var res listTasksResult
	mustJSONCLI(t, data, &res, "tasks", lid, "--json")
	rows := res.Tasks
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

// TestPriorityJSONShapes pins `farol priority`: the echoed level on success,
// the §9 error shape for an invalid level, and — the trap — an omitted
// --level failing as an error rather than defaulting to none.
func TestPriorityJSONShapes(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	var res priorityResultJSON
	mustJSONCLI(t, data, &res, "priority", tid, "--level", "high", "--json")
	if !res.OK || res.Priority != "high" {
		t.Fatalf("priority --json = %+v, want ok:true priority:high", res)
	}

	// The level landed where both read surfaces report it.
	var detailsList []showJSON
	mustJSONCLI(t, data, &detailsList, "show", tid, "--json")
	details := detailsList[0]
	if details.Priority != "high" {
		t.Errorf("show priority = %q, want high", details.Priority)
	}
	var tres listTasksResult
	mustJSONCLI(t, data, &tres, "tasks", lid, "--json")
	rows := tres.Tasks
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
	// to none.
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
	mustJSONCLI(t, data, &detailsList, "show", tid, "--json")
	details = detailsList[0]
	if details.Priority != "high" {
		t.Errorf("priority after refused writes = %q, want high unchanged", details.Priority)
	}
}

// TestTasksRowsCarryAssignment pins the new fields on `farol tasks --json`
// rows: assignee is "" when unassigned and the holder's tag once assigned.
func TestTasksRowsCarryAssignment(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	var res listTasksResult
	mustJSONCLI(t, data, &res, "tasks", lid, "--json")
	rows := res.Tasks
	if len(rows) != 1 || rows[0].Assignee != "" || rows[0].Priority != "none" {
		t.Fatalf("new task rows = %+v, want assignee \"\" and priority none", rows)
	}

	mustCLI(t, data, "assign", tid)
	mustJSONCLI(t, data, &res, "tasks", lid, "--json")
	rows = res.Tasks
	if rows[0].Assignee != "pi" {
		t.Errorf("assigned row = %+v, want assignee pi", rows[0])
	}
}

// TestTasksRowSupersetAndInclude pins the parity 1.3/1.4 surface:
// has_notes/notes_len on every row, --include notes inlines bodies under the
// byte budget with elided/budget_exceeded, and --since returns only rows
// changed after the cutoff.
func TestTasksRowSupersetAndInclude(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	withNotes := strings.TrimSpace(mustCLI(t, data, "add", lid, "has notes"))
	mustCLI(t, data, "notes", withNotes, "some body text here")
	plain := strings.TrimSpace(mustCLI(t, data, "add", lid, "plain"))

	// has_notes / notes_len present on every row.
	var res listTasksResult
	mustJSONCLI(t, data, &res, "tasks", lid, "--json")
	if len(res.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(res.Tasks))
	}
	for _, r := range res.Tasks {
		switch r.ID {
		case withNotes:
			if !r.HasNotes || r.NotesLen != len("some body text here") {
				t.Errorf("withNotes row %+v: want has_notes true and notes_len", r)
			}
		case plain:
			if r.HasNotes || r.NotesLen != 0 {
				t.Errorf("plain row %+v: want has_notes false, notes_len 0", r)
			}
		}
		if res.Elided != nil {
			t.Errorf("no --include: elided should be nil, got %v", res.Elided)
		}
	}

	// --include notes inlines the body.
	mustJSONCLI(t, data, &res, "tasks", lid, "--include", "notes", "--json")
	var inlined *taskRowJSON
	for i := range res.Tasks {
		if res.Tasks[i].ID == withNotes {
			inlined = &res.Tasks[i]
		}
	}
	if inlined == nil || inlined.Notes != "some body text here" {
		t.Fatalf("--include notes: body not inlined on %s: %+v", withNotes, res.Tasks)
	}
	if res.BudgetExceeded {
		t.Errorf("small bodies should not exceed budget")
	}
}

// TestTasksSinceFilter pins the folded list_changes: --since returns only
// rows whose activity is strictly after the cutoff.
func TestTasksSinceFilter(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	old := strings.TrimSpace(mustCLI(t, data, "add", lid, "old task"))
	cutoff := time.Now().Unix()
	// Sleep long enough that the next write is strictly after cutoff.
	time.Sleep(1100 * time.Millisecond)
	recent := strings.TrimSpace(mustCLI(t, data, "add", lid, "recent task"))

	var res listTasksResult
	mustJSONCLI(t, data, &res, "tasks", lid, "--since", strconv.FormatInt(cutoff, 10), "--json")
	ids := map[string]bool{}
	for _, r := range res.Tasks {
		ids[r.ID] = true
	}
	if ids[old] {
		t.Errorf("--since returned pre-cutoff task %s", old)
	}
	if !ids[recent] {
		t.Errorf("--since dropped post-cutoff task %s", recent)
	}
	if len(res.Tasks) != 1 {
		t.Fatalf("--since returned %d tasks, want 1", len(res.Tasks))
	}
}

// TestTasksIncludeInvalidRejected pins that an unknown --include value is a
// §9 error, not a silent no-op.
func TestTasksIncludeInvalidRejected(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	mustCLI(t, data, "add", lid, "task")

	code, out, _ := runCLI(t, data, "tasks", lid, "--include", "bogus", "--json")
	if code != 1 {
		t.Fatalf("invalid --include: exit %d, want 1", code)
	}
	if !strings.Contains(out, "unknown --include") {
		t.Errorf("invalid --include error = %q, want it to name --include", out)
	}
}

// TestShowBatchPins the MCP show_task parity: `farol show <id>...` accepts
// up to 50 ids and returns an array of full subtrees; an unresolvable id is
// a per-element {id,error} row, not a whole-call failure.
func TestShowBatch(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	t1 := strings.TrimSpace(mustCLI(t, data, "add", lid, "one"))
	t2 := strings.TrimSpace(mustCLI(t, data, "add", lid, "two"))

	var details []showJSON
	mustJSONCLI(t, data, &details, "show", t1, t2, "--json")
	if len(details) != 2 {
		t.Fatalf("show batch = %d tasks, want 2", len(details))
	}
	if details[0].ID != t1 || details[1].ID != t2 {
		t.Errorf("batch order wrong: %s, %s", details[0].ID, details[1].ID)
	}

	// A bad id reports {id,error} inline and the call still succeeds (exit 0),
	// like MCP show_task — the per-row error is the failure signal, not a
	// whole-call error that would emit a second JSON value on stdout.
	code, out, _ := runCLI(t, data, "show", t1, "does-not-exist", "--json")
	if code != 0 {
		t.Fatalf("show with bad id: exit %d, want 0 (per-row error carries it)", code)
	}
	var rows []any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("stdout %q is not one JSON array: %v", out, err)
	}
	if len(rows) != 2 {
		t.Fatalf("show mixed = %d rows, want 2 (one ok, one error)", len(rows))
	}
}

// eligible task (highest priority, then tree order) to FAROL_AGENT and returns
// its full show payload; an empty/exhausted list is {ok:false,reason:...} in
// --json, not an error. Pulled from the MCP next_task contract.
func TestNextGrabsAndShows(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tLow := strings.TrimSpace(mustCLI(t, data, "add", lid, "low prio"))
	tHigh := strings.TrimSpace(mustCLI(t, data, "add", lid, "high prio"))
	mustCLI(t, data, "priority", tLow, "--level", "low")
	mustCLI(t, data, "priority", tHigh, "--level", "high")

	// next picks the high-priority task, and assigns it.
	var got showJSON
	mustJSONCLI(t, data, &got, "next", lid, "--json")
	if got.ID != tHigh {
		t.Fatalf("next = %s, want the high-priority task %s", got.ID, tHigh)
	}
	if got.Assignee != "pi" {
		t.Errorf("next did not assign to pi: assignee=%q", got.Assignee)
	}

	// next again grabs the remaining low-priority task.
	mustJSONCLI(t, data, &got, "next", lid, "--json")
	if got.ID != tLow {
		t.Fatalf("second next = %s, want the low-priority task %s", got.ID, tLow)
	}

	// Exhausted list: {ok:false}, not an error.
	var empty nextEmptyJSON
	code, out, _ := runCLI(t, data, "next", lid, "--json")
	if code != 0 {
		t.Fatalf("exhausted next: exit %d, want 0", code)
	}
	if err := json.Unmarshal([]byte(out), &empty); err != nil {
		t.Fatalf("exhausted next stdout %q is not one JSON value: %v", out, err)
	}
	if empty.OK {
		t.Fatalf("exhausted next: ok=%v, want false", empty.OK)
	}
}

// TestNextHumanEmptyPrintsNothing pins the §9 human-mode contract: an
// exhausted list prints nothing and exits 0.
func TestNextHumanEmptyPrintsNothing(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	code, out, _ := runCLI(t, data, "next", lid)
	if code != 0 {
		t.Fatalf("exhausted next: exit %d, want 0", code)
	}
	if out != "" {
		t.Fatalf("exhausted next human output = %q, want empty", out)
	}
}

// TestAddBatch pins the multi-title add: `farol add <list> <title>...`
// creates every task under the same resolved parent (or the list root when
// no --parent is given), prints one id per line in human mode, and returns
// {"ids": [...]} in --json mode — the plural of the single-add {"id": ...}.
func TestAddBatch(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	parent := strings.TrimSpace(mustCLI(t, data, "add", lid, "parent"))

	// Human mode: one id per line, in input order.
	out := mustCLI(t, data, "add", lid, "one", "two", "three", "--parent", parent)
	ids := strings.Fields(out)
	if len(ids) != 3 {
		t.Fatalf("add batch human output = %q, want 3 ids", out)
	}
	for _, id := range ids {
		if id == "" {
			t.Errorf("add batch returned an empty id")
		}
	}

	// JSON mode: {"ids": [...]} in input order.
	var res struct {
		IDs []string `json:"ids"`
	}
	mustJSONCLI(t, data, &res, "add", lid, "four", "five", "--json")
	if len(res.IDs) != 2 {
		t.Fatalf("add batch --json ids = %v, want 2", res.IDs)
	}

	// Every batch task landed, children under the shared parent and the
	// --parent-less batch at the list root.
	var payload listTasksResult
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	byTitle := map[string]taskRowJSON{}
	for _, r := range payload.Tasks {
		byTitle[r.Title] = r
	}
	for _, title := range []string{"one", "two", "three"} {
		r, ok := byTitle[title]
		if !ok {
			t.Fatalf("task %q missing after batch add", title)
		}
		if r.ParentID == nil || *r.ParentID != parent {
			t.Errorf("task %q parent = %v, want the shared parent %s", title, r.ParentID, parent)
		}
	}
	for _, title := range []string{"four", "five"} {
		r, ok := byTitle[title]
		if !ok {
			t.Fatalf("task %q missing after batch add", title)
		}
		if r.ParentID != nil {
			t.Errorf("root batch task %q parent = %v, want nil", title, r.ParentID)
		}
	}
}

// TestCompleteReopenBatch pins the batch complete/reopen contract: a single
// id keeps the legacy {"ok":true} shape; 2+ ids return an array of
// {id, ok:true} rows in input order, and the statuses all land.
func TestCompleteReopenBatch(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	t1 := strings.TrimSpace(mustCLI(t, data, "add", lid, "one"))
	t2 := strings.TrimSpace(mustCLI(t, data, "add", lid, "two"))
	t3 := strings.TrimSpace(mustCLI(t, data, "add", lid, "three"))

	// Single id: the original shape.
	var single okPayload
	mustJSONCLI(t, data, &single, "complete", t3, "--json")
	if !single.OK {
		t.Errorf("single complete --json = %+v, want ok:true", single)
	}

	// Batch complete: an array of per-id ok rows in input order.
	var rows []taskBatchResult
	mustJSONCLI(t, data, &rows, "complete", t1, t2, "--json")
	if len(rows) != 2 || !rows[0].OK || !rows[1].OK {
		t.Fatalf("complete batch = %+v, want two ok rows", rows)
	}
	if rows[0].ID != t1 || rows[1].ID != t2 {
		t.Errorf("complete batch order wrong: %+v", rows)
	}
	var payload listTasksResult
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	for _, r := range payload.Tasks {
		if (r.ID == t1 || r.ID == t2 || r.ID == t3) && r.Status != "complete" {
			t.Errorf("task %s status = %q after complete, want complete", r.ID, r.Status)
		}
	}

	// Batch reopen returns the two to pending; the single-completed t3
	// stays complete (reopen does not cascade, §3).
	mustJSONCLI(t, data, &rows, "reopen", t1, t2, "--json")
	if len(rows) != 2 || !rows[0].OK || !rows[1].OK {
		t.Fatalf("reopen batch = %+v, want two ok rows", rows)
	}
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	status := map[string]string{}
	for _, r := range payload.Tasks {
		status[r.ID] = r.Status
	}
	if status[t1] != "pending" || status[t2] != "pending" {
		t.Errorf("after reopen: t1=%s t2=%s, want both pending", status[t1], status[t2])
	}
	if status[t3] != "complete" {
		t.Errorf("after reopen: t3=%s, want complete (reopen does not cascade)", status[t3])
	}
}

// TestRmBatchRequiresForce pins the batch rm gate: without --force a batch
// is refused before any store call and every task survives; with --force all
// ids are deleted and --json returns per-id ok rows.
func TestRmBatchRequiresForce(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	t1 := strings.TrimSpace(mustCLI(t, data, "add", lid, "one"))
	t2 := strings.TrimSpace(mustCLI(t, data, "add", lid, "two"))

	code, _, errOut := runCLI(t, data, "rm", t1, t2)
	if code != 1 || !strings.Contains(errOut, "--force") {
		t.Errorf("rm batch without --force: exit %d stderr %q, want exit 1 mentioning --force", code, errOut)
	}
	out := mustCLI(t, data, "tasks", lid)
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Errorf("tasks after refused rm batch: %q, want both tasks intact", out)
	}

	// --force deletes both, one ok row per id.
	var rows []taskBatchResult
	mustJSONCLI(t, data, &rows, "rm", t1, t2, "--force", "--json")
	if len(rows) != 2 || !rows[0].OK || !rows[1].OK {
		t.Fatalf("rm batch --force --json = %+v, want two ok rows", rows)
	}
	if out := mustCLI(t, data, "tasks", lid); out != "" {
		t.Errorf("tasks after rm batch --force: %q, want empty", out)
	}
}

// TestBatchMutatorPerIDErrors pins the per-id error contract of the batch
// mutators: in --json mode a bad id becomes a {id, error} row and the call
// still succeeds (the valid ids were processed) — matching `farol show`'s
// batch and MCP set_status; in human mode per-id errors go to stderr and the
// call exits 1.
func TestBatchMutatorPerIDErrors(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	t1 := strings.TrimSpace(mustCLI(t, data, "add", lid, "one"))

	code, out, errOut := runCLI(t, data, "complete", t1, "does-not-exist", "--json")
	if code != 0 || errOut != "" {
		t.Fatalf("batch complete with bad id: exit %d stderr %q, want exit 0 with empty stderr", code, errOut)
	}
	var rows []taskBatchResult
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("stdout %q is not one JSON array: %v", out, err)
	}
	if len(rows) != 2 || !rows[0].OK || rows[0].ID != t1 {
		t.Fatalf("rows = %+v, want an ok row for %s first", rows, t1)
	}
	if rows[1].OK || rows[1].Err == "" || rows[1].ID != "does-not-exist" {
		t.Errorf("bad-id row = %+v, want {id, error}", rows[1])
	}
	if !strings.Contains(rows[1].Err, "not found") {
		t.Errorf("bad-id error = %q, want it to name the not-found cause", rows[1].Err)
	}

	// The valid id was completed despite the bad one.
	var payload listTasksResult
	mustJSONCLI(t, data, &payload, "tasks", lid, "--json")
	for _, r := range payload.Tasks {
		if r.ID == t1 && r.Status != "complete" {
			t.Errorf("task %s status = %q, want complete", r.ID, r.Status)
		}
	}

	// Human mode: stdout stays empty, per-id errors go to stderr, exit 1.
	code, out, errOut = runCLI(t, data, "complete", t1, "does-not-exist")
	if code != 1 {
		t.Fatalf("human batch complete with bad id: exit %d, want 1", code)
	}
	if out != "" {
		t.Errorf("human mode stdout = %q, want empty", out)
	}
	if !strings.Contains(errOut, "does-not-exist") {
		t.Errorf("stderr %q should name the bad id", errOut)
	}
}
