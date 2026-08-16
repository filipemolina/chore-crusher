package cli

import (
	"strings"
	"testing"

	"github.com/filipemolina/farol/src/config"
	"github.com/filipemolina/farol/src/store"
)

// TestInboxMyForeignSplit pins the mine/foreign_lists partition: the list
// owned by FAROL_AGENT lands in mine (with created_by dropped), every other
// list lands in foreign_lists. The agent tag is the only thing that decides
// which side a list is on — the CLI equivalent of the MCP farol:///inbox
// resource's split.
func TestInboxMyForeignSplit(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")

	mineID := strings.TrimSpace(mustCLI(t, data, "lists", "add", "pi: Board", "--owner", "pi"))
	foreignID := strings.TrimSpace(mustCLI(t, data, "lists", "add", "claude: Board", "--owner", "claude"))
	mustCLI(t, data, "add", mineID, "mine task")
	mustCLI(t, data, "add", foreignID, "foreign task", "--force")

	var inbox inboxJSON
	mustJSONCLI(t, data, &inbox, "inbox", "--json")

	if inbox.Mine.ID != mineID || inbox.Mine.Name != "pi: Board" {
		t.Fatalf("mine = %+v, want id %q / name pi: Board", inbox.Mine, mineID)
	}
	// The resource drops the reader's own tag from mine.
	if inbox.Mine.CreatedBy != "" {
		t.Errorf("mine.created_by = %q, want empty (the agent's own tag is implicit)", inbox.Mine.CreatedBy)
	}
	if len(inbox.ForeignLists) != 1 {
		t.Fatalf("foreign_lists len = %d, want 1", len(inbox.ForeignLists))
	}
	if inbox.ForeignLists[0].ID != foreignID || inbox.ForeignLists[0].CreatedBy != "claude" {
		t.Errorf("foreign[0] = %+v, want id %q / created_by claude", inbox.ForeignLists[0], foreignID)
	}
	// The mine task must be findable, not lost to foreign.
	if len(inbox.Mine.Tasks) != 1 || inbox.Mine.Tasks[0].Title != "mine task" {
		t.Errorf("mine tasks = %+v, want one 'mine task'", inbox.Mine.Tasks)
	}
}

// TestInboxPendingPerTaskFilter pins the per-task filter: only tasks whose
// own status is pending or in_progress appear. Completing a task cascades to
// its descendants, so the reachable mixed state is a complete child under a
// still-pending root — and the per-task rule drops that complete child while
// keeping the pending root (distinct from `farol tasks`'s root-based section,
// which would keep the whole root subtree).
func TestInboxPendingPerTaskFilter(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")

	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	parent := strings.TrimSpace(mustCLI(t, data, "add", lid, "parent"))
	// Set parent to percentage mode to prevent our auto-switch to subtasks when adding a child.
	// This way the test can verify the original behavior: completing a child does not affect the parent's pending status.
	mustCLI(t, data, "progress", parent, "--mode", "percentage", "--percent", "0")
	child := strings.TrimSpace(mustCLI(t, data, "add", lid, "child", "--parent", parent))
	done := strings.TrimSpace(mustCLI(t, data, "add", lid, "done"))

	// Complete only the child (cascades to its own descendants, none) and the
	// standalone "done" task. parent stays pending (because it's in percentage mode, not subtasks), so the inbox should keep
	// parent and drop both complete tasks.
	mustCLI(t, data, child)
	mustCLI(t, data, done)

	var inbox inboxJSON
	mustJSONCLI(t, data, &inbox, "inbox", "--json")

	titles := map[string]bool{}
	for _, r := range inbox.Mine.Tasks {
		titles[r.Title] = true
	}
	if !titles["parent"] {
		t.Errorf("pending root must appear; tasks = %+v", inbox.Mine.Tasks)
	}
	if titles["child"] {
		t.Errorf("complete child must be filtered out; tasks = %+v", inbox.Mine.Tasks)
	}
	if titles["done"] {
		t.Errorf("complete standalone must be filtered out; tasks = %+v", inbox.Mine.Tasks)
	}
}

// TestInboxNotesInline pins that `--include notes` inlines each row's notes
// body, matching the MCP resource (which always inlines notes). Without the
// flag, only has_notes/notes_len are reported and Notes stays empty.
func TestInboxNotesInline(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")

	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	mustCLI(t, data, "add", lid, "noted", "--notes", "remember this")

	var noInline inboxJSON
	mustJSONCLI(t, data, &noInline, "inbox", "--json")
	if noInline.Mine.Tasks[0].Notes != "" {
		t.Errorf("without --include notes, Notes = %q, want empty", noInline.Mine.Tasks[0].Notes)
	}
	if !noInline.Mine.Tasks[0].HasNotes || noInline.Mine.Tasks[0].NotesLen != len("remember this") {
		t.Errorf("has_notes/notes_len wrong: %+v", noInline.Mine.Tasks[0])
	}

	var withInline inboxJSON
	mustJSONCLI(t, data, &withInline, "inbox", "--include", "notes", "--json")
	if withInline.Mine.Tasks[0].Notes != "remember this" {
		t.Errorf("with --include notes, Notes = %q, want 'remember this'", withInline.Mine.Tasks[0].Notes)
	}
}

// TestInboxCapPinsTop20 pins the inbox cap: a list with more than 20 pending
// tasks reports exactly 20, matching the MCP resource's cap.
func TestInboxCapPinsTop20(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")

	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	for i := 0; i < 25; i++ {
		mustCLI(t, data, "add", lid, "task")
	}

	var inbox inboxJSON
	mustJSONCLI(t, data, &inbox, "inbox", "--json")
	if len(inbox.Mine.Tasks) != inboxCap {
		t.Errorf("inbox reported %d tasks, want cap %d", len(inbox.Mine.Tasks), inboxCap)
	}
}

// TestInboxAssigneeLive pins assignee_live: a pending task assigned to a live
// agent reports true, a task assigned to an agent with no presence claim
// reports false, and an unassigned task reports false. One ListWork read
// serves the whole request (docs/DESIGN.md §3).
func TestInboxAssigneeLive(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")

	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	live := strings.TrimSpace(mustCLI(t, data, "add", lid, "held by live"))
	stale := strings.TrimSpace(mustCLI(t, data, "add", lid, "held by dead"))
	// "nobody" is created but not captured: the unassigned-row check looks it
	// up by title in the inbox payload, so no variable is needed here.
	mustCLI(t, data, "add", lid, "nobody")

	db, err := openTestStore(t, data)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AssignTask(live, "pi", false); err != nil {
		t.Fatalf("assign live: %v", err)
	}
	if err := db.AssignTask(stale, "ghost", false); err != nil {
		t.Fatalf("assign stale: %v", err)
	}
	if _, err := db.ClaimWork("task", live, "pi", store.ActivityWorking); err != nil {
		t.Fatalf("claim: %v", err)
	}
	db.Close()

	var inbox inboxJSON
	mustJSONCLI(t, data, &inbox, "inbox", "--json")

	byTitle := map[string]taskRowJSON{}
	for _, r := range inbox.Mine.Tasks {
		byTitle[r.Title] = r
	}
	if got := byTitle["held by live"]; got.Assignee != "pi" || !got.AssigneeLive {
		t.Errorf("live row = %+v, want pi/true", got)
	}
	if got := byTitle["held by dead"]; got.Assignee != "ghost" || got.AssigneeLive {
		t.Errorf("stale row = %+v, want ghost/false", got)
	}
	if got := byTitle["nobody"]; got.Assignee != "" || got.AssigneeLive {
		t.Errorf("free row = %+v, want empty/false", got)
	}
}

// TestInboxHumanRenders pins the human-readable opener: the agent's own list
// header and rows render in the shared tree layout, and an empty opener
// prints nothing (docs/DESIGN.md §9 no-output rule).
func TestInboxHumanRenders(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")

	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "pi: Board", "--owner", "pi"))
	mustCLI(t, data, "add", lid, "first pending")

	out := mustCLI(t, data, "inbox")
	if !strings.Contains(out, "Your list: pi: Board (1 pending, 0 complete)") {
		t.Errorf("human inbox: %q, want a 'Your list' header with counts", out)
	}
	if !strings.Contains(out, "[ ] first pending") {
		t.Errorf("human inbox: %q, want a pending row in the shared chrome", out)
	}

	// Empty: an agent owning no list and no foreign lists prints nothing.
	empty := t.TempDir()
	t.Setenv("FAROL_AGENT", "solo")
	if out := mustCLI(t, empty, "inbox"); out != "" {
		t.Errorf("empty inbox printed %q, want nothing", out)
	}
}

// openTestStore opens the store at dataDir for tests that need raw store
// writes the CLI does not expose (assignment + presence). It mirrors the
// open used by the rest of the suite's setup helpers.
func openTestStore(t *testing.T, dataDir string) (*store.Store, error) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", dataDir)
	return store.Open(config.DBPath())
}
