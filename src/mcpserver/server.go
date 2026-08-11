package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/config"
	"github.com/filipemolina/farol/src/constants"
	"github.com/filipemolina/farol/src/store"
)

// ownerTagPattern is the human-readable form of createdByRE, used in error
// messages.
const ownerTagPattern = "^[A-Za-z0-9_-]{1,32}$"

// createdByRE validates an explicit created_by tag an agent may pass to
// add_list. The store does not re-validate the tag format, so the MCP
// layer is the only place this check lives.
var createdByRE = regexp.MustCompile(ownerTagPattern)

// serverIdentity returns the agent tag this server acts as. FAROL_AGENT wins
// verbatim when set, so a human who wants a stable tag across sessions keeps
// one. When it is unset the tag is unique to this PROCESS — "agent-" plus six
// random hex digits — rather than the constant it used to be.
//
// The constant was the bug: identity is what every cross-agent guard compares
// on, so two unconfigured clients sharing one tag compared equal and wrote
// over each other with no refusal, no force and no takeover comment. That was
// the DEFAULT configuration, not an edge case.
//
// A per-process tag is only coherent because an assignment no longer outlives
// the session that made it (decision 3) — Run releases this identity's claims
// and assignments on the way out. The result must satisfy ownerTagPattern,
// because it is written to list.created_by and validated there; "agent-7f3a2c"
// does.
//
// WARNING: when FAROL_AGENT is unset this returns a DIFFERENT value on every
// call. Call it once per process and thread the result — Run does. Two calls
// would leave the server releasing claims under a tag it never wrote.
func serverIdentity() string {
	if identity := os.Getenv("FAROL_AGENT"); identity != "" {
		return identity
	}
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail in practice; the pid keeps the tag
		// unique among concurrent processes, which is the only property
		// that actually matters here.
		return fmt.Sprintf("agent-%d", os.Getpid())
	}
	return fmt.Sprintf("agent-%x", b)
}

// progressJSON is the derived-progress shape shared by task rows.
type progressJSON struct {
	Kind            string `json:"kind"`
	Percent         *int   `json:"percent"`
	DisplayAsSimple bool   `json:"display_as_simple"`
}

// taskRowJSON is one row of a task tree/list result, matching the CLI's
// --json shape for tasks (docs/DESIGN.md §9).
type taskRowJSON struct {
	ID           string        `json:"id"`
	ParentID     *string       `json:"parent_id"`
	Title        string        `json:"title"`
	Status       string        `json:"status"`
	Progress     *progressJSON `json:"progress,omitempty"`
	Depth        int           `json:"depth"`
	ListOwner    string        `json:"list_owner"`
	HasNotes     bool          `json:"has_notes"`
	NotesLen     int           `json:"notes_len"`
	Notes        string        `json:"notes,omitempty"`        // populated only when include=notes; never cut mid-text
	Assignee     string        `json:"assignee"`               // "" when unassigned
	AssignedAt   *int64        `json:"assigned_at"`            // null when unassigned
	AssigneeLive bool          `json:"assignee_live"`          // live presence claim by the assignee
	Priority     string        `json:"priority"`               // none|low|medium|high
	ContextOnly  bool          `json:"context_only,omitempty"` // ancestor skeleton row
	Comments     []commentJSON `json:"comments,omitempty"`     // descendant rows in show_task; include=['comments'] in list_tasks
}

// listTasksResult is the list_tasks return envelope: the tasks array plus
// the ids of the rows the body budget dropped (so the caller can
// re-fetch them with show_task) and whether the budget was exceeded at all
// — true exactly when elided is non-empty.
type listTasksResult struct {
	Tasks          []taskRowJSON `json:"tasks"`
	Elided         []string      `json:"elided"`
	BudgetExceeded bool          `json:"budget_exceeded"`
}

// taskDetailsJSON is the payload for the show_task tool.
type taskDetailsJSON struct {
	ID           string           `json:"id"`
	ListID       string           `json:"list_id"`
	ListOwner    string           `json:"list_owner"`
	Title        string           `json:"title"`
	Notes        string           `json:"notes"`
	Status       string           `json:"status"`
	Progress     progressJSON     `json:"progress"`
	CreatedAt    int64            `json:"created_at"`
	UpdatedAt    int64            `json:"updated_at"`
	CompletedAt  *int64           `json:"completed_at"`
	Assignee     string           `json:"assignee"`      // "" when unassigned
	AssignedAt   *int64           `json:"assigned_at"`   // null when unassigned
	AssigneeLive bool             `json:"assignee_live"` // live presence claim by the assignee
	Priority     string           `json:"priority"`      // none|low|medium|high
	Children     []taskRowJSON    `json:"children"`
	Comments     []commentJSON    `json:"comments"`
	Attachments  []attachmentJSON `json:"attachments"`
}

// commentJSON mirrors store.Comment for the MCP surface. It appears in
// taskDetailsJSON (show_task). Author is the OS username (CLI/TUI path) or
// the server identity (agent path).
type commentJSON struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"created_at"`
}

// attachmentJSON mirrors store.Attachment for the MCP surface. It appears in
// taskDetailsJSON (show_task).
type attachmentJSON struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	CreatedAt int64  `json:"created_at"`
}

// searchResultJSON is one row of the search_tasks result.
type searchResultJSON struct {
	ID           string       `json:"id"`
	ListID       string       `json:"list_id"`
	ListName     string       `json:"list_name"`
	ListOwner    string       `json:"list_owner"`
	Title        string       `json:"title"`
	Status       string       `json:"status"`
	Progress     progressJSON `json:"progress"`
	Assignee     string       `json:"assignee"`      // "" when unassigned
	AssignedAt   *int64       `json:"assigned_at"`   // null when unassigned
	AssigneeLive bool         `json:"assignee_live"` // live presence claim by the assignee
	Priority     string       `json:"priority"`      // none|low|medium|high
}

// NewServer opens the default store, registers every MCP tool, and returns
// the configured server. The caller owns the store and must close it.
//
// The identity is resolved here and is not reported back. Run needs the same
// value to clean up under on the way out, and with an unset FAROL_AGENT a
// second serverIdentity() call would produce a different tag — so Run resolves
// it once and uses newServer directly rather than calling this.
func NewServer() (*mcp.Server, *store.Store, error) {
	return newServer(serverIdentity())
}

// newServer is NewServer with the identity supplied, so a caller that needs to
// know which tag the server acts as can resolve it once and pass it in.
func newServer(identity string) (*mcp.Server, *store.Store, error) {
	s, err := store.Open(config.DBPath())
	if err != nil {
		return nil, nil, err
	}

	// The Instructions doc is delivered to clients in the initialize result and
	// is how an agent discovers this API without trial-and-error (query it with
	// `mcp({ instructions: "farol" })`). Keep it in sync with the tool
	// list below.
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "farol",
		Version: constants.Version(),
	}, &mcp.ServerOptions{Instructions: `Farol is the todo store this work lives in; the TUI is how the human watches it. Read your tasks from here at the start of every session and keep their status current as you work — on your own, without being asked.

IDENTITY & OWNERSHIP. You act under the tag "` + identity + `" — FAROL_AGENT when it is set, otherwise a tag unique to this session. Either way it is yours alone: no other running agent shares it, and anything you still hold is released when this session ends. Track your own work in a list named "` + identity + `: ..." — chore_crusher_my_list get-or-creates it. Each list has an owner (created_by); a list is yours only when created_by == your tag. The server ENFORCES this: structural edits (add_task, edit_task, delete_task, add_list) on a list you do NOT own are refused. But on ANY list you may read everything, grab tasks (assign_task, next_task), change status/progress (set_status) and comment. Untagged lists (human-made) are owned by nobody and are foreign to you — UNLESS a human has explicitly marked it collaborative (a per-list opt-in flag, off by default, set from the TUI's list-rename modal): a collaborative list accepts structural edits from any agent regardless of created_by. Check the collaborative field on my_list's foreign_lists before assuming a foreign list is read-only. Comments have their own, narrower ownership rule: only the comment tool's delete mode (comment(id=..., delete=true, force=true)) removes a comment, and only one whose author is your own tag, regardless of who owns the list it's on.

ASSIGNMENT — grab a task before you research it. Three separate axes, do not confuse them: status is what the work IS; the TUI spinner is presence, a 120-second claim any write refreshes and the end of your session clears; assignee is ownership with NO TTL and no sweeper — it changes when someone assigns, releases or completes, and it is released for you when THIS session ends. Grabbing first is the point: next_task(list_id) or assign_task(ids=[...]) makes the task yours before you spend tokens on it, so a second agent does not research the same thing in parallel. Every task row you read carries assignee, assigned_at, assignee_live and priority. An assignee with assignee_live false is abandoned work, not free work: it means a session died before it could release, which is the only way an assignment outlives its owner. A write to a task another agent holds (set_status, edit_task, delete_task) is REFUSED; force=true performs it, reassigns the task to you and records a takeover comment. Commenting is never refused — leaving a note on another agent's task is how coordination works. Completing a task auto-unassigns it, every descendant the cascade completes, and every ancestor it promotes. Assignment reserves the subtree: a task whose ancestor or descendant is held by someone else is refused EVEN with force — release the blocker with assign_task(ids=[blocker], release=true, force=true), or ask the human to release the whole list from the TUI.

PRIORITY. Four values, ranked high > medium > low > none (the default). next_task picks by priority first and tree order second, and every task row you read carries the field. Set it with add_task(priority=...) or edit_task(id, priority=...) — a structural edit, so only on a list you own; the human sets it from the TUI or the CLI (farol priority) on theirs. Omitting priority on edit_task leaves the current value alone, so a rename never silently clears a high someone set. Priority does not re-order the tree: it steers what to pick up next, nothing else.

IDs: every id parameter accepts a short unambiguous prefix. Lists are addressed by id, never by name. Tools whose parameter is 'ids' accept 1..50 in one call.

TOOLS (chore_crusher_<name>):
- my_list() — session opener: {mine, foreign_lists} with pending/complete counts
- list_tasks(list_id, status?, since?, include?) — one list's task tree as preorder rows with ancestor skeletons; status defaults to 'open' (pending + in_progress), also pending|in_progress|complete|all; include=['notes','comments'] inlines whole bodies, a byte budget caps the response and over-budget rows are named in the 'elided' field of the {tasks, elided, budget_exceeded} result, never cut mid-text; since=<unix> returns only tasks changed after that time and widens the default status to 'all' so completions show (the old list_changes tool, folded into this parameter)
- show_task(ids) — full details + children + comments for 1..50 tasks
- search_tasks(query, list_id?) — fuzzy over titles and notes
- add_task(list_id, title, parent?, notes?, priority?) — priority is none|low|medium|high, default none
- edit_task(id, title?, notes?, parent?, to_root?, priority?, force?) — change any field; notes replaces the whole body; parent re-parents; to_root moves to the list root; priority re-ranks it (omit it and the current value is left alone); force=true takes over a task another agent holds
- delete_task(id, force=true) — task and descendants; the same force also takes over another agent's task
- set_status(ids, status?, progress?, percent?, comment?, force?) — the one status/progress write; status=pending|in_progress|complete (complete cascades and auto-unassigns), progress=simple|subtasks|percentage flips the task to in_progress (percent only with percentage), comment lands after the state change; a complete task is reopened first, so progress never errors; force=true performs the write on a task assigned to another agent and records a takeover comment
- assign_task(ids, release?, force?) — the durable grab: assign 1..50 tasks to yourself (this server's identity) and get their full show_task payloads; release=true unassigns (silently succeeds if you did not hold it); force=true takes a task from its holder and records a takeover comment — but a task blocked by an ancestor/descendant assigned to another agent is refused EVEN with force; release the blocker first
- next_task(list_id) — atomically grab and read the top eligible task for you: highest priority (high > medium > low > none), then tree order; nothing eligible returns {ok:false, reason:'no eligible task in this list'} (not an error)
- comment(task_id, note) — add a comment on any task, never blocked by assignment (coordination between agents); attributed to your tag. comment(id=..., delete=true, force=true) — delete only your own comments (author == your tag), regardless of list ownership
- add_list(name, created_by?) — owned by you
- add_attachment(task_id, path) — attach a file to a task; path is any string (typically a file path)
- list_attachments(task_id) — list all attachments for a task
- delete_attachment(attachment_id) — remove an attachment

RESOURCES — two, and that is the whole resource surface:
- farol:///inbox — your list, every foreign list, and each one's top 20 pending tasks with notes inlined: a whole session opener in one read
- farol://work — the live presence claims, i.e. who is at the keyboard right now. Presence, NOT assignment: who OWNS a task is the assignee field on the task row.

KEEP THE BOARD LIVE (do this yourself, unasked):
- Grab before you research: next_task(list_id) hands you the top eligible task and everything about it in one call; assign_task(ids=[...]) grabs a specific one.
- Start a task = set_status(ids, progress=...) on it (flips it to in_progress and lights the spinner). Advance the percentage as you go, not only at the end — the human watches the TUI live.
- On a list you do not own: read the whole list plus the task's notes and comments first; leave comments at decision points; never edit its content.
- Finish = set_status(ids, status='complete'). A percentage of 100 does NOT auto-complete.
- Stopping without finishing = assign_task(ids=[...], release=true). Your session end releases it too, but release explicitly: it frees the task the moment you stop, not whenever your process happens to exit.
- Between tasks: list_tasks(list_id, since=<your last call>) — priorities and comments move while you work.

For the full working loop and a one-read session opener, use the farol_inbox prompt or read the farol:///inbox resource.

GOTCHAS: set_status(progress='subtasks') derives from children — on a shared task use percentage. Your assignments outlive your session (only the presence claims are cleared when it ends), so release what you did not finish. Your own auto-created "` + identity + `: ..." Inbox is deleted at session end if it is empty or all-complete, so don't rely on it as long-term storage.`})

	addListTools(server, s, identity)
	addTaskTools(server, s, identity)
	addWorkResource(server, s)
	addResources(server, s, identity)
	addPrompts(server, s)

	return server, s, nil
}

// Run starts the MCP server on the stdio transport. It blocks until the
// client disconnects or the context is cancelled.
func Run(ctx context.Context) error {
	// Resolved ONCE, here, and threaded into the server: with FAROL_AGENT
	// unset serverIdentity returns a fresh tag per call, so asking twice would
	// build the server under one identity and clean up under another, leaving
	// this session's claims and assignments behind forever.
	identity := serverIdentity()

	server, s, err := newServer(identity)
	if err != nil {
		return err
	}
	defer s.Close()

	err = server.Run(ctx, &mcp.StdioTransport{})

	// Session-end cleanup, in order. Every step is best-effort and reports to
	// stderr: the session is already over, and failing the shutdown path would
	// turn a tidy-up problem into a visible crash.
	//
	// H13: release this identity's presence claims so the TUI stops drawing a
	// spinner for a process that is gone. Other agents' claims are untouched.
	if _, rErr := s.ReleaseAgentClaims(identity); rErr != nil {
		fmt.Fprintf(os.Stderr, "releasing claims on session end: %v\n", rErr)
	}
	// Then the assignments. A per-session identity never returns, so work it
	// still holds would be stranded — no other session can take it without
	// force, and no future session answers to this tag.
	if _, rErr := s.UnassignAgent(identity); rErr != nil {
		fmt.Fprintf(os.Stderr, "releasing assignments on session end: %v\n", rErr)
	}
	// Finally the auto-created Inbox, and only when empty. A unique identity
	// per session means my_list mints a new "<identity>: Inbox" every run, so
	// without this the lists panel fills with abandoned empties. An Inbox with
	// tasks in it is left alone — the agent did work worth keeping, and
	// deciding what to do with it is not the shutdown path's business
	// (decision 5).
	if _, rErr := s.DeleteEmptyAgentInbox(identity); rErr != nil {
		fmt.Fprintf(os.Stderr, "removing empty inbox on session end: %v\n", rErr)
	}
	return err
}

// addListTools registers the list-oriented MCP tools.
func addListTools(server *mcp.Server, s *store.Store, identity string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_list",
		Description: "Create a new task list and return its id. Example: add_list(name='Shopping'). Owned by created_by (an agent tag like 'pi'), which defaults to this server's identity; only the owner may add/edit/delete tasks and rename/delete the list — other agents may read it and update task status/progress only. Warning: setting created_by to a tag other than your own provisions a list you cannot write — the server refuses structural edits on it — so only do that for a list you intend another agent to own.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		Name      string `json:"name" jsonschema:"name of the new list"`
		CreatedBy string `json:"created_by,omitempty" jsonschema:"owning agent tag (e.g. pi); defaults to this server's identity"`
	}) (*mcp.CallToolResult, any, error) {
		owner := in.CreatedBy
		if owner == "" {
			owner = identity
		} else if !createdByRE.MatchString(owner) {
			return errorResult(fmt.Errorf("created_by must match %s, got %q", ownerTagPattern, owner)), nil, nil
		}
		id, err := s.CreateList(in.Name, owner)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]string{"id": id})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "my_list",
		Description: "Get or create your own list (named after the FAROL_AGENT tag) for tracking your own work, plus a summary of every other (foreign) list, so you can start a session in one call. Example: my_list(). Returns {mine: {id,name,pending,complete}, foreign_lists: [{id,name,pending,complete,created_by,collaborative}]}. collaborative=true means structural edits (add_task, edit_task, delete_task) are allowed on that list despite not owning it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		id, err := s.GetOrCreateAgentList(identity)
		if err != nil {
			return errorResult(err), nil, nil
		}
		lists, err := s.ListLists()
		if err != nil {
			return errorResult(err), nil, nil
		}
		type mine struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Pending  int    `json:"pending"`
			Complete int    `json:"complete"`
		}
		type foreign struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			Pending       int    `json:"pending"`
			Complete      int    `json:"complete"`
			CreatedBy     string `json:"created_by"`
			Collaborative bool   `json:"collaborative"`
		}
		var m mine
		others := make([]foreign, 0)
		for _, l := range lists {
			if l.ID == id {
				m = mine{ID: l.ID, Name: l.Name, Pending: l.PendingCount, Complete: l.CompleteCount}
				continue
			}
			others = append(others, foreign{
				ID: l.ID, Name: l.Name,
				Pending: l.PendingCount, Complete: l.CompleteCount, CreatedBy: l.CreatedBy,
				Collaborative: l.Collaborative,
			})
		}
		return jsonResult(map[string]any{
			"mine":          m,
			"foreign_lists": others,
		})
	})
}

// addTaskTools registers the task-oriented MCP tools. identity is the agent
// tag whose live claims status/progress writes refresh (a presence heartbeat).
func addTaskTools(server *mcp.Server, s *store.Store, identity string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List a list's tasks as a depth-annotated tree. Example: list_tasks(list_id='01ABC...', status='pending', include=['notes']). status defaults to 'open' (pending + in_progress); one of open, pending, in_progress, complete, all. Rows are filtered per task, not per tree root, and an included row's non-matching ancestors come back as skeleton rows with context_only=true. since (unix seconds) returns only tasks whose activity changed strictly after it — the folded list_changes; passing it widens the default status to 'all' so a just-completed task still shows up, and an explicit status still wins. include is an optional set of extra fields to inline per row; supported values: 'notes' (the full notes body), 'comments' (comment bodies). Inlined bodies are never cut mid-text: a byte budget caps the response and an over-budget row is dropped whole, its id reported in the 'elided' array of the {tasks, elided, budget_exceeded} return object — fetch it with show_task. Prefer this over N show_task calls.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ListID  string   `json:"list_id" jsonschema:"list id or unambiguous prefix"`
		Status  string   `json:"status,omitempty" jsonschema:"open (default), pending, in_progress, complete, or all"`
		Since   int64    `json:"since,omitempty" jsonschema:"unix seconds; return tasks changed strictly after this"`
		Include []string `json:"include,omitempty" jsonschema:"extra per-row fields to inline; supports 'notes', 'comments'"`
	}) (*mcp.CallToolResult, any, error) {
		if in.Status == "" {
			// `since` absorbed list_changes, which had no status filter at all.
			// Composing it with the `open` default would make the call
			// docs/DESIGN.md §9 writes for change detection —
			// list_tasks(list_id, since=<unix>) — blind to the most common
			// change of all, a task being completed. An explicit status wins.
			if in.Since > 0 {
				in.Status = "all"
			} else {
				in.Status = "open"
			}
		}
		if !validStatusFilter(in.Status) {
			return errorResult(fmt.Errorf("invalid status %q: want open, pending, in_progress, complete, or all", in.Status)), nil, nil
		}
		includeNotes, includeComments := false, false
		for _, k := range in.Include {
			switch k {
			case "notes":
				includeNotes = true
			case "comments":
				includeComments = true
			default:
				return errorResult(fmt.Errorf("unknown include %q: supported values: notes, comments", k)), nil, nil
			}
		}
		listID, err := s.ResolveID("list", in.ListID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		list, err := s.GetList(listID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		tasks, err := s.ListTasks(listID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		live, err := liveAgents(s)
		if err != nil {
			return errorResult(err), nil, nil
		}
		rows, err := sectionRows(s, tasks, in.Status, list.CreatedBy, live)
		if err != nil {
			return errorResult(err), nil, nil
		}
		// The folded list_changes: keep only rows whose activity is strictly
		// after `since`, so the response doubles as "what moved since my last
		// call". Ancestor skeletons that did not change are dropped with the
		// rest — list_changes never promised a connected tree either.
		if in.Since > 0 {
			changed, err := s.TasksChangedSince(listID, in.Since)
			if err != nil {
				return errorResult(err), nil, nil
			}
			changedSet := make(map[string]bool, len(changed))
			for _, t := range changed {
				changedSet[t.ID] = true
			}
			kept := rows[:0]
			for _, r := range rows {
				if changedSet[r.ID] {
					kept = append(kept, r)
				}
			}
			rows = kept
		}
		var (
			elided         []string
			budgetExceeded bool
		)
		if includeNotes || includeComments {
			elided, budgetExceeded, err = inlineBodyBudget(s, listID, rows, tasks, includeNotes, includeComments)
			if err != nil {
				return errorResult(err), nil, nil
			}
		}
		return jsonResult(listTasksResult{Tasks: rows, Elided: elided, BudgetExceeded: budgetExceeded})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_task",
		Description: "Add a task to a list, optionally nested under a parent. Example: add_task(list_id='01ABC...', title='Buy milk', parent='01DEF...', notes='whole milk', priority='high'). priority is one of none|low|medium|high and defaults to none; it is what next_task sorts by.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ListID   string `json:"list_id" jsonschema:"list id or unambiguous prefix"`
		Title    string `json:"title" jsonschema:"task title"`
		Parent   string `json:"parent,omitempty" jsonschema:"optional parent task id or prefix"`
		Notes    string `json:"notes,omitempty" jsonschema:"optional notes"`
		Priority string `json:"priority,omitempty" jsonschema:"none, low, medium or high; omit to leave it at none"`
	}) (*mcp.CallToolResult, any, error) {
		// An omitted priority leaves the column at its 'none' default rather
		// than calling SetPriority("") — the zero value is not
		// PriorityNone, it is invalid.
		if in.Priority != "" {
			if err := checkPriority(in.Priority); err != nil {
				return errorResult(err), nil, nil
			}
		}
		listID, err := s.ResolveID("list", in.ListID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if err := requireWritable(s, identity, listID); err != nil {
			return errorResult(err), nil, nil
		}
		var parentID *string
		if strings.TrimSpace(in.Parent) != "" {
			pid, err := s.ResolveID("task", in.Parent)
			if err != nil {
				return errorResult(err), nil, nil
			}
			parentID = &pid
		}
		id, err := s.CreateTask(listID, in.Title, parentID, in.Notes)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if in.Priority != "" {
			if err := s.SetPriority(id, store.Priority(in.Priority)); err != nil {
				return errorResult(err), nil, nil
			}
		}
		// Light the spinner on the task just created.
		autoClaim(s, "task", id, identity)
		return jsonResult(map[string]string{"id": id})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "show_task",
		Description: "Show full details for 1..50 tasks: notes, comments, and the entire subtree — every descendant row carries its own full notes and comments, uncapped, so one call replaces N. Example: show_task(ids=['01ABC','01DEF']). Returns an array in the same order as ids; an id that cannot be resolved comes back as {id,error}.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		IDs []string `json:"ids" jsonschema:"task ids or unambiguous prefixes; 1..50"`
	}) (*mcp.CallToolResult, any, error) {
		if len(in.IDs) == 0 {
			return errorResult(fmt.Errorf("show_task requires at least one id")), nil, nil
		}
		if len(in.IDs) > 50 {
			return errorResult(fmt.Errorf("show_task capped at 50 ids per call, got %d", len(in.IDs))), nil, nil
		}
		type errRow struct {
			ID    string `json:"id"`
			Error string `json:"error"`
		}
		out := make([]any, 0, len(in.IDs))
		// One presence read for the whole batch: assignee_live on every row
		// joins against this map instead of querying per row.
		live, err := liveAgents(s)
		if err != nil {
			return errorResult(err), nil, nil
		}
		for _, raw := range in.IDs {
			id, err := s.ResolveID("task", raw)
			if err != nil {
				out = append(out, errRow{ID: raw, Error: err.Error()})
				continue
			}
			details, err := taskDetailsJSONFor(s, id, live)
			if err != nil {
				out = append(out, errRow{ID: id, Error: err.Error()})
				continue
			}
			out = append(out, details)
		}
		return jsonResult(out)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "comment",
		Description: "Add a comment on a task, or delete one of your own. Add: comment(task_id='01ABC...', note='checking in') — anyone may comment on any task regardless of ownership or assignment (posting is never blocked by the assignment guard: leaving a note on another agent's task is how coordination works), unless the list has comments disabled; author is not accepted — comments are attributed to this server's identity (FAROL_AGENT). Delete: comment(id='01XYZ...', delete=true, force=true) — deletion requires force=true and removes only a comment whose author is this server's identity, regardless of who owns the list it's on; a foreign comment is refused.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		TaskID string `json:"task_id,omitempty" jsonschema:"task id or unambiguous prefix (add mode)"`
		Note   string `json:"note,omitempty" jsonschema:"the comment text (add mode)"`
		ID     string `json:"id,omitempty" jsonschema:"comment id (delete mode)"`
		Delete bool   `json:"delete,omitempty" jsonschema:"true deletes the comment instead of adding (delete mode)"`
		Force  bool   `json:"force,omitempty" jsonschema:"must be true to confirm deletion (delete mode)"`
		Author string `json:"author,omitempty" jsonschema:"rejected if set — comments are always attributed to the server identity"`
	}) (*mcp.CallToolResult, any, error) {
		// author is rejected in BOTH modes, before the mode split. The two
		// tools this merged: delete_comment had no author field at all, so a
		// caller passing one got a schema error; the merged input struct has
		// to carry author for add mode, which would otherwise make it a
		// silently ignored parameter on the delete path — and an identity
		// that thinks it can name the author of a deletion has the ownership
		// rule exactly backwards.
		if in.Author != "" {
			return errorResult(fmt.Errorf("author is not a supported parameter: comments are attributed to this server's identity (%q)", identity)), nil, nil
		}
		if in.Delete {
			if !in.Force {
				return errorResult(fmt.Errorf("deleting a comment requires force=true")), nil, nil
			}
			if _, err := requireOwnComment(s, identity, in.ID); err != nil {
				return errorResult(err), nil, nil
			}
			if err := s.DeleteComment(in.ID); err != nil {
				return errorResult(err), nil, nil
			}
			return jsonResult(map[string]bool{"ok": true})
		}
		if strings.TrimSpace(in.Note) == "" {
			return errorResult(fmt.Errorf("note must not be empty")), nil, nil
		}
		id, err := s.ResolveID("task", in.TaskID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		cid, err := s.AddComment(id, identity, in.Note)
		if err != nil {
			return errorResult(err), nil, nil
		}
		// A comment is activity on the task: paint the spinner under this
		// agent, same as a status/progress write.
		autoClaim(s, "task", id, identity)
		return jsonResult(map[string]string{"id": cid})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_attachment",
		Description: "Attach a file to a task. Example: add_attachment(task_id='01ABC...', path='/path/to/file.png'). Returns the attachment id.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		TaskID string `json:"task_id" jsonschema:"task id or unambiguous prefix"`
		Path   string `json:"path" jsonschema:"file path to attach"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("task", in.TaskID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		attID, err := s.AddAttachment(id, in.Path)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]string{"id": attID})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_attachments",
		Description: "List all attachments for a task. Example: list_attachments(task_id='01ABC...'). Returns an array of attachments.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		TaskID string `json:"task_id" jsonschema:"task id or unambiguous prefix"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("task", in.TaskID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		attachments, err := s.ListAttachments(id)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(attachments)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_attachment",
		Description: "Delete an attachment. Example: delete_attachment(attachment_id='01XYZ...').",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		AttachmentID string `json:"attachment_id" jsonschema:"attachment id or unambiguous prefix"`
	}) (*mcp.CallToolResult, any, error) {
		if err := s.DeleteAttachment(in.AttachmentID); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_status",
		Description: "The one status/progress write: mark 1..50 tasks complete, reopen them, or set progress in one call. status=pending|in_progress|complete, progress=simple|subtasks|percentage, percent=0..100 (only valid with progress=percentage), comment=<text> added after the state change lands so it records the final state. Per id, applied in this order: assignment guard (a task assigned to another agent is refused unless force=true, which takes it over and records a takeover comment), then reopen if the task is complete and the write needs it, then progress, then status, then the comment. status='complete' cascades to descendants and auto-unassigns; status='pending' reopens without cascading. At least one of status, progress, comment is required. Example: set_status(ids=['01ABC...'], status='in_progress', progress='percentage', percent=50). Returns one result row per id in input order: {id,ok:true} or {id,error}; a bad id does not stop the rest.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		IDs      []string `json:"ids" jsonschema:"task ids or unambiguous prefixes; 1..50"`
		Status   string   `json:"status,omitempty" jsonschema:"pending, in_progress, or complete"`
		Progress string   `json:"progress,omitempty" jsonschema:"simple, subtasks, or percentage"`
		Percent  *int     `json:"percent,omitempty" jsonschema:"percent 0-100, only valid when progress=percentage"`
		Comment  string   `json:"comment,omitempty" jsonschema:"comment added to each id after the state change lands, recording the final state"`
		Force    bool     `json:"force,omitempty" jsonschema:"true overrides another agent's assignment of the task (records a takeover comment)"`
	}) (*mcp.CallToolResult, any, error) {
		if len(in.IDs) == 0 {
			return errorResult(fmt.Errorf("set_status requires at least one id")), nil, nil
		}
		if len(in.IDs) > 50 {
			return errorResult(fmt.Errorf("set_status capped at 50 ids per call, got %d", len(in.IDs))), nil, nil
		}
		if in.Status == "" && in.Progress == "" && in.Comment == "" {
			return errorResult(fmt.Errorf("set_status needs at least one of status, progress, comment")), nil, nil
		}
		// Validate the options once up front so a bad request fails before any
		// write; the percent range and the per-id state transitions are the
		// store's job, surfaced per row (mirrors the old set_progress shape).
		switch in.Status {
		case "", "pending", "in_progress", "complete":
		default:
			return errorResult(fmt.Errorf("invalid status %q: want pending, in_progress, or complete", in.Status)), nil, nil
		}
		switch in.Progress {
		case "", "simple", "subtasks", "percentage":
		default:
			return errorResult(fmt.Errorf("invalid progress %q: want simple, subtasks, or percentage", in.Progress)), nil, nil
		}
		if in.Progress != "percentage" && in.Percent != nil {
			return errorResult(fmt.Errorf("percent is only valid when progress=percentage")), nil, nil
		}
		if in.Progress == "percentage" && in.Percent == nil {
			return errorResult(fmt.Errorf("progress=percentage requires percent")), nil, nil
		}
		// One presence read for the whole batch: the assignment guard's
		// conflict text needs to know whether the holder is at the keyboard.
		live, err := liveAgents(s)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return batchApply(s, identity, in.IDs, func(id string) error {
			return applySetStatus(s, identity, id, in.Status, in.Progress, in.Percent, in.Comment, in.Force, live)
		})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "assign_task",
		Description: "The durable grab: assign 1..50 tasks to yourself (this server's identity) and get their full show_task payloads back — grabbing and reading a task is one call. release=true unassigns instead, succeeding silently if you did not hold the task. force=true takes a task another agent holds, reassigning it and writing a takeover comment that records who took it from whom and how stale it was. An explicit agent_id is rejected: an agent may only assign work to itself; assigning work TO another agent is a human action taken from the TUI. Assignment reserves the subtree: a task whose ancestor or descendant is held by a different agent is refused EVEN with force — release the blocker first. Example: assign_task(ids=['01ABC...']).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		IDs     []string `json:"ids" jsonschema:"task ids or unambiguous prefixes; 1..50"`
		Release bool     `json:"release,omitempty" jsonschema:"true unassigns instead of assigning; a no-op when the task is not held"`
		Force   bool     `json:"force,omitempty" jsonschema:"true takes a task held by another agent and writes a takeover comment; does not override a subtree reservation"`
		AgentID string   `json:"agent_id,omitempty" jsonschema:"rejected if set — tasks are always assigned to the server identity"`
	}) (*mcp.CallToolResult, any, error) {
		if in.AgentID != "" {
			return errorResult(fmt.Errorf("agent_id is not a supported parameter: tasks are assigned to this server's identity (%q)", identity)), nil, nil
		}
		if len(in.IDs) == 0 {
			return errorResult(fmt.Errorf("assign_task requires at least one id")), nil, nil
		}
		if len(in.IDs) > 50 {
			return errorResult(fmt.Errorf("assign_task capped at 50 ids per call, got %d", len(in.IDs))), nil, nil
		}
		// One presence read for the whole batch: the conflict text and
		// the takeover comment both need to know whether the holder is live.
		live, err := liveAgents(s)
		if err != nil {
			return errorResult(err), nil, nil
		}
		type errRow struct {
			ID    string `json:"id"`
			Error string `json:"error"`
		}
		type okRow struct {
			ID string `json:"id"`
			OK bool   `json:"ok"`
		}
		out := make([]any, 0, len(in.IDs))
		for _, raw := range in.IDs {
			id, err := s.ResolveID("task", raw)
			if err != nil {
				out = append(out, errRow{ID: raw, Error: err.Error()})
				continue
			}
			if in.Release {
				// Releasing a task nobody holds is a silent no-op; force
				// releases another agent's task without any subtree check —
				// the escape hatch force exists to be.
				if err := s.UnassignTask(id, identity, in.Force); err != nil {
					out = append(out, errRow{ID: id, Error: err.Error()})
					continue
				}
				out = append(out, okRow{ID: id, OK: true})
				continue
			}
			// The takeover comment names the previous holder, so read it
			// before the guarded update swaps it away.
			var prev string
			var prevAt *int64
			if in.Force {
				t, err := s.GetTask(id)
				if err != nil {
					out = append(out, errRow{ID: id, Error: err.Error()})
					continue
				}
				prev, prevAt = t.Assignee, t.AssignedAt
			}
			if err := s.AssignTask(id, identity, in.Force); err != nil {
				out = append(out, errRow{ID: id, Error: assignmentConflict(s, id, err, live).Error()})
				continue
			}
			// A grab is a write, so it refreshes presence like every other
			// write. Without this the agent that just grabbed the task reads
			// back assignee_live:false, which docs/DESIGN.md §3 defines as
			// abandoned — the stale tier would light up on work nobody has
			// let go of. `live` was snapshotted before the grab (it has to
			// be: the takeover comment reports the PREVIOUS holder's
			// liveness), so record our own claim in it rather than paying a
			// second ListWork — we just made it, so it is live by
			// construction, and one-presence-read-per-request holds.
			autoClaim(s, "task", id, identity)
			live[identity] = true
			if prev != "" && prev != identity {
				if _, err := s.AddComment(id, identity, takeoverComment(identity, prev, prevAt, live)); err != nil {
					out = append(out, errRow{ID: id, Error: err.Error()})
					continue
				}
			}
			details, err := taskDetailsJSONFor(s, id, live)
			if err != nil {
				out = append(out, errRow{ID: id, Error: err.Error()})
				continue
			}
			out = append(out, details)
		}
		return jsonResult(out)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "next_task",
		Description: "Grab and read the top eligible task in one call: atomically assigns the highest-priority eligible task (high > medium > low > none, then tree order) to you and returns its full show_task payload. Eligible means not complete, unassigned, and no ancestor or descendant assigned to a different agent. Nothing eligible is NOT an error: returns {ok:false, reason:'no eligible task in this list'}. Example: next_task(list_id='01ABC...').",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ListID string `json:"list_id" jsonschema:"list id or unambiguous prefix"`
	}) (*mcp.CallToolResult, any, error) {
		listID, err := s.ResolveID("list", in.ListID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		// One presence read per request, for the returned payload's
		// assignee_live fields.
		live, err := liveAgents(s)
		if err != nil {
			return errorResult(err), nil, nil
		}
		t, err := s.NextAssignable(listID, identity)
		if err != nil {
			if errors.Is(err, store.ErrNoAssignable) {
				// An empty board is a normal state, not a failure.
				return jsonResult(map[string]any{"ok": false, "reason": "no eligible task in this list"})
			}
			return errorResult(err), nil, nil
		}
		// Same as assign_task: the grab is a write, so it claims presence,
		// and the payload this call returns must already show it.
		autoClaim(s, "task", t.ID, identity)
		live[identity] = true
		details, err := taskDetailsJSONFor(s, t.ID, live)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(details)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_task",
		Description: "Delete a task and its descendants. Requires force=true. Assignment guard: a task assigned to another agent is only deletable with that same force, which records a takeover comment first — and a task blocked by a subtree reservation stays refused. Example: delete_task(id='01ABC...', force=true).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID    string `json:"id" jsonschema:"task id or unambiguous prefix"`
		Force bool   `json:"force" jsonschema:"must be true to confirm deletion"`
	}) (*mcp.CallToolResult, any, error) {
		if !in.Force {
			return errorResult(fmt.Errorf("deleting a task requires force=true")), nil, nil
		}
		id, err := s.ResolveID("task", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		// List ownership is checked BEFORE the assignment guard: the guard's
		// force branch is itself a write (it reassigns and comments), so
		// running it ahead of a check that can still refuse would leave the
		// task stolen by a delete that never happened. Same rule as the
		// re-parent gate below — a refused write never half-happens.
		if err := requireWritableTask(s, identity, id); err != nil {
			return errorResult(err), nil, nil
		}
		// One presence read per request, shared by the guard below.
		live, err := liveAgents(s)
		if err != nil {
			return errorResult(err), nil, nil
		}
		// The assignment guard. delete_task already requires force (the
		// confirmation), so the guard's force branch always runs here: a
		// forced delete of a held task records the takeover comment, and a
		// subtree reservation still refuses the delete — the reservation
		// survives force on every write path (decision 4).
		if err := requireAssignable(s, identity, id, in.Force, live); err != nil {
			return errorResult(err), nil, nil
		}
		if err := s.DeleteTask(id); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "edit_task",
		Description: "Edit a task's fields in one call. Any omitted field is left unchanged. title renames it; notes REPLACES the whole notes body (pass '' to clear); parent re-parents it under that task; to_root=true moves it to the list root; priority sets none|low|medium|high (omit it and the current priority is left alone — it is never reset as a side effect of a rename). Example: edit_task(id='01ABC', title='New name', priority='high'). Structural edit — refused on a list you do not own, and on a task assigned to another agent unless force=true (which takes the task over and records a takeover comment).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID       string  `json:"id" jsonschema:"task id or unambiguous prefix"`
		Title    *string `json:"title,omitempty" jsonschema:"new title; omit to leave unchanged"`
		Notes    *string `json:"notes,omitempty" jsonschema:"replacement notes (whole body; '' clears); omit to leave unchanged"`
		Parent   *string `json:"parent,omitempty" jsonschema:"new parent task id or prefix; omit to leave unchanged"`
		ToRoot   bool    `json:"to_root,omitempty" jsonschema:"true moves the task to the list root"`
		Priority *string `json:"priority,omitempty" jsonschema:"none, low, medium or high; omit to leave the current priority unchanged"`
		Force    bool    `json:"force,omitempty" jsonschema:"true overrides another agent's assignment of the task (records a takeover comment)"`
	}) (*mcp.CallToolResult, any, error) {
		if in.Title == nil && in.Notes == nil && in.Parent == nil && in.Priority == nil && !in.ToRoot {
			return errorResult(fmt.Errorf("edit_task needs at least one of title, notes, parent, to_root, priority")), nil, nil
		}
		if in.Parent != nil && in.ToRoot {
			return errorResult(fmt.Errorf("pass either parent or to_root, not both")), nil, nil
		}
		// Presence of the parameter, not its emptiness, is what means "set
		// it": an omitted priority must leave a high someone set alone, so
		// the pointer is checked for nil and "" is a rejected value
		// rather than a silent none. Validated up front, before the renames
		// below write anything.
		if in.Priority != nil {
			if err := checkPriority(*in.Priority); err != nil {
				return errorResult(err), nil, nil
			}
		}
		id, err := s.ResolveID("task", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		// Title/notes/priority edits touch the task's own content, so gate on
		// its list. Priority belongs on this side of the line, not with
		// status/progress: it is the human's steer about what an agent should
		// pick up next, so re-ranking someone else's list is exactly the
		// structural edit ownership refuses.
		if in.Title != nil || in.Notes != nil || in.Priority != nil {
			if err := requireWritableTask(s, identity, id); err != nil {
				return errorResult(err), nil, nil
			}
		}
		// Re-parenting needs the TARGET list writable: the parent's list for a
		// cross-list move, or the task's own list when moving to root. This
		// preserves move_task's rule — a task must never be half-moved into a
		// list the requester does not own.
		var parentID *string
		if in.Parent != nil || in.ToRoot {
			if in.Parent != nil && strings.TrimSpace(*in.Parent) != "" {
				pid, err := s.ResolveID("task", *in.Parent)
				if err != nil {
					return errorResult(err), nil, nil
				}
				parentID = &pid
				p, err := s.GetTask(pid)
				if err != nil {
					return errorResult(err), nil, nil
				}
				if err := requireWritable(s, identity, p.ListID); err != nil {
					return errorResult(err), nil, nil
				}
			} else { // to_root
				if err := requireWritableTask(s, identity, id); err != nil {
					return errorResult(err), nil, nil
				}
			}
		}

		// The assignment guard runs LAST of the checks and first of the
		// writes: its force branch reassigns the task and records a takeover
		// comment, so anything that can still refuse the edit — every
		// ownership gate above, including the re-parent target's — has to
		// have passed already. Otherwise a refused edit would leave the task
		// taken over, which is the same half-happened write the re-parent
		// gate exists to prevent.
		// One presence read per request, for the guard's conflict text.
		live, err := liveAgents(s)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if err := requireAssignable(s, identity, id, in.Force, live); err != nil {
			return errorResult(err), nil, nil
		}

		if in.Title != nil {
			if err := s.RenameTask(id, *in.Title); err != nil {
				return errorResult(err), nil, nil
			}
		}
		if in.Notes != nil {
			if err := s.SetNotes(id, *in.Notes); err != nil {
				return errorResult(err), nil, nil
			}
		}
		if in.Parent != nil || in.ToRoot {
			if err := s.Reparent(id, parentID); err != nil {
				return errorResult(err), nil, nil
			}
		}
		if in.Priority != nil {
			if err := s.SetPriority(id, store.Priority(*in.Priority)); err != nil {
				return errorResult(err), nil, nil
			}
		}
		autoClaim(s, "task", id, identity)
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_tasks",
		Description: "Fuzzy search task titles and notes across all lists, or within one list if list_id is given; title matches rank first. Example: search_tasks(query='budget').",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		Query  string `json:"query" jsonschema:"search query"`
		ListID string `json:"list_id,omitempty" jsonschema:"optional list id or prefix to limit the search"`
	}) (*mcp.CallToolResult, any, error) {
		var listID *string
		if strings.TrimSpace(in.ListID) != "" {
			lid, err := s.ResolveID("list", in.ListID)
			if err != nil {
				return errorResult(err), nil, nil
			}
			listID = &lid
		}

		tasks, err := s.SearchTasks(in.Query, listID)
		if err != nil {
			return errorResult(err), nil, nil
		}

		lists, err := s.ListLists()
		if err != nil {
			return errorResult(err), nil, nil
		}
		listNames := make(map[string]string, len(lists))
		listOwners := make(map[string]string, len(lists))
		for _, l := range lists {
			listNames[l.List.ID] = l.List.Name
			listOwners[l.List.ID] = l.List.CreatedBy
		}

		live, err := liveAgents(s)
		if err != nil {
			return errorResult(err), nil, nil
		}

		out := make([]searchResultJSON, len(tasks))
		for i, t := range tasks {
			prog, err := taskProgressJSON(s, t.ID)
			if err != nil {
				return errorResult(err), nil, nil
			}
			out[i] = searchResultJSON{
				ID:           t.ID,
				ListID:       t.ListID,
				ListName:     listNames[t.ListID],
				ListOwner:    listOwners[t.ListID],
				Title:        t.Title,
				Status:       string(t.Status),
				Progress:     prog,
				Assignee:     t.Assignee,
				AssignedAt:   t.AssignedAt,
				AssigneeLive: assigneeLive(live, t.Assignee),
				Priority:     string(t.Priority),
			}
		}
		return jsonResult(out)
	})
}

// applySetStatus runs the per-id order — the assignment guard first (step
// 9's requireAssignable), then reopen-if-needed, progress, status, comment.
// The reopen step is the fix for the documented gotcha where set_progress
// on a complete task used to error: one call now reopens and sets percentage
// on a complete task because the reopen happens first. status='in_progress'
// has no direct store write of its own — SetProgress is the only transition
// that flips pending → in_progress — so when the call carries no progress of
// its own it is re-applied with whatever progress the task already carries,
// which is why marking a task started never clobbers its percentage (a task
// just reopened above carries 'none', which SetProgress accepts).
func applySetStatus(s *store.Store, identity, id, status, progress string, percent *int, comment string, force bool, live map[string]bool) error {
	// The guard is the first step of the per-id order: a task held by
	// another agent is refused here; force takes it over first.
	if err := requireAssignable(s, identity, id, force, live); err != nil {
		return err
	}

	t, err := s.GetTask(id)
	if err != nil {
		return err
	}

	if t.Status == store.StatusComplete && (progress != "" || status == "in_progress") {
		if err := s.Reopen(id); err != nil {
			return err
		}
		t, err = s.GetTask(id)
		if err != nil {
			return err
		}
	}

	if progress != "" {
		if err := s.SetProgress(id, store.ProgressKind(progress), percent); err != nil {
			return err
		}
	}

	switch status {
	case "complete":
		if err := s.Complete(id); err != nil {
			return err
		}
	case "pending":
		if err := s.Reopen(id); err != nil {
			return err
		}
	case "in_progress":
		if progress != "" {
			// The progress write above already flipped it to in_progress;
			// re-applying the same kind here would only be a second write of
			// values the store just stored.
			break
		}
		if err := s.SetProgress(id, t.ProgressKind, t.ProgressPct); err != nil {
			return err
		}
	}

	if comment != "" {
		if _, err := s.AddComment(id, identity, comment); err != nil {
			return err
		}
	}
	return nil
}

// batchApply runs fn over up to 50 task ids, auto-claiming each successful
// write under identity, and returns one result row per id in input order. A
// bad id does not stop the rest. It reproduces the resolve → op → autoClaim
// loop the old update_tasks used for each status/progress op.
func batchApply(s *store.Store, identity string, ids []string, fn func(id string) error) (*mcp.CallToolResult, any, error) {
	if len(ids) == 0 {
		return errorResult(fmt.Errorf("requires at least one id")), nil, nil
	}
	if len(ids) > 50 {
		return errorResult(fmt.Errorf("capped at 50 ids per call, got %d", len(ids))), nil, nil
	}
	type row struct {
		ID    string `json:"id"`
		OK    bool   `json:"ok,omitempty"`
		Error string `json:"error,omitempty"`
	}
	out := make([]row, 0, len(ids))
	for _, raw := range ids {
		id, err := s.ResolveID("task", raw)
		if err != nil {
			out = append(out, row{ID: raw, Error: err.Error()})
			continue
		}
		if err := fn(id); err != nil {
			out = append(out, row{ID: raw, Error: err.Error()})
			continue
		}
		autoClaim(s, "task", id, identity)
		out = append(out, row{ID: id, OK: true})
	}
	return jsonResult(out)
}

// autoClaim renews the writing agent's live claim on entityID, or opens
// one if none exists. Best-effort: any error is swallowed because the
// write already committed and presence tracking is not a write guarantee.
// If another agent already holds the entity, ClaimWork returns an error which
// is silently dropped here — we do not steal their spinner; the write itself
// is allowed today because the write path does not gate on claims; this
// behaviour is unchanged.
func autoClaim(s *store.Store, entityType, entityID, agentID string) {
	if err := s.TouchWork(entityType, entityID, agentID); err == nil {
		_, _ = s.ClaimWork(entityType, entityID, agentID, store.ActivityWorking)
	}
}

// requireWritable rejects a structural write to a list the requester does
// not own, UNLESS the list's Collaborative flag is set — an explicit human
// opt-in that lets any agent make structural edits regardless of
// created_by (docs/DESIGN.md §9, "Tag a list as collaborative"). An untagged,
// non-collaborative list is owned by nobody and is therefore foreign to
// every agent: a human manages it via the CLI/TUI, which are deliberately
// unenforced. The check runs after ResolveID, so listID is the suffix-free
// id; the error names that id so the agent knows which list refused it. Step
// D wires this into the structural tools; it is defined here (Step C) so the
// identity read and the helper land together.
func requireWritable(s *store.Store, identity, listID string) error {
	l, err := s.GetList(listID)
	if err != nil {
		return err
	}
	if l.CreatedBy == identity || l.Collaborative {
		return nil
	}
	owner := l.CreatedBy
	if owner == "" {
		owner = "no one (untagged)"
	}
	return fmt.Errorf("list %s is owned by %s — you may read it and update task status/progress only", l.ID, owner)
}

// requireWritableTask rejects a structural write to the list a task belongs
// to. It resolves the task, reads its ListID, and applies requireWritable —
// so rename_task/set_notes/delete_task defer to the same owner check as the
// list tools. move_task is handled inline because its target list is the
// *parent's* list, not the task's own.
func requireWritableTask(s *store.Store, identity, taskID string) error {
	t, err := s.GetTask(taskID)
	if err != nil {
		return err
	}
	return requireWritable(s, identity, t.ListID)
}

// checkPriority validates an optional priority parameter BEFORE any write
// runs. store.SetPriority does its own validation, but both callers write
// something else first — add_task creates the task, edit_task may already
// have renamed it — so a value rejected at write time would leave the
// half-happened write the re-parent gate exists to prevent. The empty string
// is invalid here on purpose: Priority("") is not PriorityNone,
// and "omitted" is signalled by not passing the parameter at all, never by
// passing "".
func checkPriority(p string) error {
	switch store.Priority(p) {
	case store.PriorityNone, store.PriorityLow, store.PriorityMedium, store.PriorityHigh:
		return nil
	}
	return fmt.Errorf("%w: %q (want none, low, medium, or high)", store.ErrInvalidPriority, p)
}

// requireOwnComment rejects deleting a comment this identity did not write.
// Unlike requireWritable it keys off the individual comment's author, not
// the list it lives on — an agent may always delete its own comment, even
// on a list it does not own. The error shape mirrors requireWritable's.
func requireOwnComment(s *store.Store, identity, commentID string) (store.Comment, error) {
	c, err := s.GetComment(commentID)
	if err != nil {
		return store.Comment{}, err
	}
	if c.Author != identity {
		return store.Comment{}, fmt.Errorf("comment %s is owned by %s — you may only delete your own comments", c.ID, c.Author)
	}
	return c, nil
}

// assignmentBlockerRE extracts the blocker named by a store subtree conflict
// — "ancestor task X is held by Y" (src/store/assignment.go's
// subtreeReserved phrasing) — so the hint can name the exact task to
// release. When the pattern does not match, the caller falls back to the raw
// error rather than guessing.
var assignmentBlockerRE = regexp.MustCompile(`(ancestor|descendant) task "([^"]+)" is held by "([^"]+)"`)

// requireAssignable refuses a write to a task held by a different agent
// — the refuse-with-override rule applied to set_status, edit_task and
// delete_task. When force is set it performs the takeover — reassigns the
// task to identity and records a takeover comment — and the caller's write
// then applies to a task the caller now holds. force does NOT override the
// subtree reservation (plan decision 4): AssignTask enforces that under
// force too, and the conflict is returned as-is.
//
// live is the caller's one-per-request presence read, passed in because the
// guard runs once per id inside a batch (the per-request rule) and the
// conflict text names whether the holder is actually at the keyboard.
func requireAssignable(s *store.Store, identity, taskID string, force bool, live map[string]bool) error {
	t, err := s.GetTask(taskID)
	if err != nil {
		return err
	}
	if t.Assignee == "" || t.Assignee == identity {
		return nil
	}
	if !force {
		return assignmentConflict(s, taskID, fmt.Errorf("%w: task %q is held by %q", store.ErrAssigned, taskID, t.Assignee), live)
	}
	if err := s.AssignTask(taskID, identity, true); err != nil {
		return assignmentConflict(s, taskID, err, live)
	}
	if _, err := s.AddComment(taskID, identity, takeoverComment(identity, t.Assignee, t.AssignedAt, live)); err != nil {
		return err
	}
	return nil
}

// assignmentConflict renders the conflict error for a refused AssignTask
// or a guarded write. It names the holder and the age of the assignment, and
// keys the remediation hint off the conflict class: force CAN take a task
// from its holder, so the hint says so; force CANNOT override the subtree
// reservation (decision 4), so for a subtree conflict the hint names the
// actual escape hatch instead — release the blocker with
// assign_task(release=true, force=true), or release the whole list from the
// TUI. Telling an agent to retry with force on the second class sends it
// into a loop it cannot exit (a dead agent holding a parent and its child
// blocks both from either direction).
func assignmentConflict(s *store.Store, taskID string, err error, live map[string]bool) error {
	if errors.Is(err, store.ErrSubtreeAssigned) {
		m := assignmentBlockerRE.FindStringSubmatch(err.Error())
		if m == nil {
			// Unknown phrasing: keep the raw error, but never emit the
			// force hint for a conflict this class cannot resolve by
			// force.
			return fmt.Errorf("%v — force will not override a subtree reservation; release the blocking task first (assign_task(release=true, force=true)) or release the whole list from the TUI", err)
		}
		var assignedAt *int64
		if b, gerr := s.GetTask(m[2]); gerr == nil {
			assignedAt = b.AssignedAt
		}
		return fmt.Errorf("task %s is blocked by its %s %s, assigned to %q (%s ago, %s) — force will not override a subtree reservation; release that task first (assign_task(release=true, force=true)) or release the whole list from the TUI",
			taskID, m[1], m[2], m[3], durationAgo(assignedAt), liveState(live, m[3]))
	}
	if errors.Is(err, store.ErrAssigned) {
		t, gerr := s.GetTask(taskID)
		if gerr != nil {
			return err
		}
		return fmt.Errorf("task %s is assigned to %q (%s ago, %s) — pass force=true to take it",
			taskID, t.Assignee, durationAgo(t.AssignedAt), liveState(live, t.Assignee))
	}
	return err
}

// takeoverComment is the audit line recorded when an agent force-takes a
// task: who took it, from whom, and how stale the handover was. The
// same line is written by assign_task(force=true) and by requireAssignable's
// force path, so a takeover leaves the same trail everywhere.
func takeoverComment(identity, previous string, assignedAt *int64, live map[string]bool) string {
	return fmt.Sprintf("%s took this task from %s (assigned %s ago, %s)",
		identity, previous, durationAgo(assignedAt), liveState(live, previous))
}

// durationAgo renders an assignment's age for conflict text and takeover
// comments: minutes precision above an hour, seconds below ("2h14m ago").
// assignedAt is nil only in impossible states (assign always sets it
// alongside assignee), so nil degrades to "unknown" rather than panicking.
func durationAgo(unix *int64) string {
	if unix == nil {
		return "unknown"
	}
	d := time.Since(time.Unix(*unix, 0))
	if d < time.Minute {
		d = d.Round(time.Second)
		if d < time.Second {
			return "<1s"
		}
		return d.String()
	}
	d = d.Round(time.Minute)
	if d < time.Minute {
		return "<1m"
	}
	return strings.TrimSuffix(d.String(), "0s")
}

// liveState words whether a task's holder is currently at the keyboard, for
// the conflict text and the takeover comment. The stale holder — assigned
// and not live — is the whole reason the board has a release path, so the
// wording is part of the contract an agent acts on.
func liveState(live map[string]bool, holder string) string {
	if live[holder] {
		return "live session"
	}
	return "no live session"
}

// sectionRows returns the preorder task rows for a PER-TASK status filter
// (docs/DESIGN.md §9): a row is included when its own status matches, not
// its root ancestor's. The old root-based walk was a "section" filter
// mirroring the TUI's Pending/Complete split and dropped whole subtrees when
// the root was in a different state — that is the behaviour this replaces.
// The CLI's identically named function (src/cli/tasks.go) KEEPS the
// root-based semantics for the human-facing Pending/Complete sections; do
// not merge them back.
//
// A matching row's non-matching ancestors are emitted as skeleton rows
// (context_only=true) so parent_id chains and depth stay meaningful; a
// skeleton never receives an inlined body even under include, and a
// row that matches in its own right is a full row even when it is also
// someone's ancestor. Emitted rows keep the original preorder.
//
// live is read once per request by the caller and passed in, exactly like
// descendantRows: the inbox resource calls this once per list, so reading
// presence in here would run one ListWork query per list rather than one per
// request.
func sectionRows(s *store.Store, tasks []store.Task, status string, listOwner string, live map[string]bool) ([]taskRowJSON, error) {
	converted := apptypes.FromStoreTasks(tasks)
	rows := apptypes.Flatten(converted)
	byID := make(map[string]apptypes.Task, len(converted))
	for _, t := range converted {
		byID[t.ID] = t
	}

	matched := make(map[string]bool, len(converted))
	for _, t := range converted {
		if matchesStatus(t, status) {
			matched[t.ID] = true
		}
	}

	// The ancestor skeleton: a match's non-matching ancestors come back
	// as context rows, so a pending child of a complete parent still arrives
	// with its parent chain intact.
	skeletons := make(map[string]bool)
	for _, t := range converted {
		if !matched[t.ID] {
			continue
		}
		for par := t.ParentID; par != nil; {
			p, ok := byID[*par]
			if !ok {
				break
			}
			if !matched[p.ID] {
				skeletons[p.ID] = true
			}
			par = p.ParentID
		}
	}

	var out []taskRowJSON
	for _, r := range rows {
		id := r.Task.ID
		if !matched[id] && !skeletons[id] {
			continue
		}
		prog, err := taskProgressJSON(s, id)
		if err != nil {
			return nil, err
		}
		notes := r.Task.Notes
		row := taskRowJSON{
			ID:           id,
			ParentID:     r.Task.ParentID,
			Title:        r.Task.Title,
			Status:       string(r.Task.Status),
			Depth:        r.Depth,
			ListOwner:    listOwner,
			HasNotes:     len(notes) > 0,
			NotesLen:     len(notes),
			Assignee:     r.Task.Assignee,
			AssignedAt:   r.Task.AssignedAt,
			AssigneeLive: assigneeLive(live, r.Task.Assignee),
			Priority:     string(r.Task.Priority),
			ContextOnly:  skeletons[id],
		}
		if !(prog.Kind == "none" && prog.Percent == nil && prog.DisplayAsSimple) {
			p := prog
			row.Progress = &p
		}
		out = append(out, row)
	}
	return out, nil
}

// matchesStatus reports whether a task's own status satisfies the list_tasks
// filter; "open" means pending + in_progress. Unknown statuses match
// nothing — the handler validates the value before calling.
func matchesStatus(t apptypes.Task, status string) bool {
	switch status {
	case "all":
		return true
	case "open":
		return t.Status == apptypes.StatusPending || t.Status == apptypes.StatusInProgress
	case "pending":
		return t.Status == apptypes.StatusPending
	case "in_progress":
		return t.Status == apptypes.StatusInProgress
	case "complete":
		return t.Status == apptypes.StatusComplete
	}
	return false
}

// taskProgressJSON returns the derived progress representation for one task.
func taskProgressJSON(s *store.Store, id string) (progressJSON, error) {
	kind, pct, simple, err := s.DerivedProgress(id)
	if err != nil {
		return progressJSON{}, err
	}
	p := progressJSON{Kind: string(kind), DisplayAsSimple: simple}
	if (kind == store.ProgressPercentage || kind == store.ProgressSubtasks) && !simple {
		p.Percent = &pct
	}
	return p, nil
}

// taskDetailsJSONFor renders the full show_task payload for one task: its
// own fields, its entire subtree with every descendant's full notes and
// comments (decision 8), and the task's own comments. It is also the payload
// assign_task and next_task return after a grab — "grabbing and reading a
// task is one call" — so the three tool surfaces share one definition
// and cannot drift. live is the caller's one-per-request presence read; do
// not query presence in here.
func taskDetailsJSONFor(s *store.Store, id string, live map[string]bool) (taskDetailsJSON, error) {
	t, err := s.GetTask(id)
	if err != nil {
		return taskDetailsJSON{}, err
	}
	prog, err := taskProgressJSON(s, id)
	if err != nil {
		return taskDetailsJSON{}, err
	}
	all, err := s.ListTasks(t.ListID)
	if err != nil {
		return taskDetailsJSON{}, err
	}
	l, err := s.GetList(t.ListID)
	if err != nil {
		return taskDetailsJSON{}, err
	}
	children, err := descendantRows(s, all, id, l.CreatedBy, live)
	if err != nil {
		return taskDetailsJSON{}, err
	}
	comments, err := s.ListComments(id)
	if err != nil {
		return taskDetailsJSON{}, err
	}
	attachments, err := s.ListAttachments(id)
	if err != nil {
		return taskDetailsJSON{}, err
	}
	return taskDetailsJSON{
		ID: t.ID, ListID: t.ListID, ListOwner: l.CreatedBy, Title: t.Title, Notes: t.Notes,
		Status: string(t.Status), Progress: prog, CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt, CompletedAt: t.CompletedAt,
		Assignee: t.Assignee, AssignedAt: t.AssignedAt,
		AssigneeLive: assigneeLive(live, t.Assignee), Priority: string(t.Priority),
		Children: children, Comments: commentsJSON(comments), Attachments: attachmentsJSON(attachments),
	}, nil
}

// descendantRows reports every descendant of rootID (the task itself
// excluded) as depth-annotated rows with derived progress per row, using the
// shared apptypes.DescendantsOf so `farol show` and show_task cannot drift.
// Depth is relative to rootID: direct children at depth 1. The previous code
// ran a store-level walk through apptypes.Flatten, but Flatten only emits
// ParentID==nil rows, so a pure-descendant set (no list root) flattened to
// nothing and "children" was always empty.
//
// Every row carries its full notes and comments, uncapped: a task's subtree
// is bounded, so one show_task call must be self-contained. live maps
// agent tags to a live presence claim and is read once per request by the
// caller — do not query presence per row here.
func descendantRows(s *store.Store, tasks []store.Task, rootID string, listOwner string, live map[string]bool) ([]taskRowJSON, error) {
	rows := apptypes.DescendantsOf(apptypes.FromStoreTasks(tasks), rootID)
	out := make([]taskRowJSON, 0, len(rows))
	for _, r := range rows {
		prog, err := taskProgressJSON(s, r.Task.ID)
		if err != nil {
			return nil, err
		}
		comments, err := s.ListComments(r.Task.ID)
		if err != nil {
			return nil, err
		}
		notes := r.Task.Notes
		row := taskRowJSON{
			ID:           r.Task.ID,
			ParentID:     r.Task.ParentID,
			Title:        r.Task.Title,
			Status:       string(r.Task.Status),
			Depth:        r.Depth,
			ListOwner:    listOwner,
			HasNotes:     len(notes) > 0,
			NotesLen:     len(notes),
			Notes:        notes,
			Assignee:     r.Task.Assignee,
			AssignedAt:   r.Task.AssignedAt,
			AssigneeLive: assigneeLive(live, r.Task.Assignee),
			Priority:     string(r.Task.Priority),
			Comments:     commentsJSON(comments),
		}
		if !(prog.Kind == "none" && prog.Percent == nil && prog.DisplayAsSimple) {
			p := prog
			row.Progress = &p
		}
		out = append(out, row)
	}
	return out, nil
}

// commentsJSON converts store comments to the MCP shape. Used for both the
// root task and every descendant row in show_task so the two cannot drift.
func commentsJSON(comments []store.Comment) []commentJSON {
	out := make([]commentJSON, 0, len(comments))
	for _, c := range comments {
		out = append(out, commentJSON{
			ID:        c.ID,
			Author:    c.Author,
			Note:      c.Note,
			CreatedAt: c.CreatedAt,
		})
	}
	return out
}

func attachmentsJSON(attachments []store.Attachment) []attachmentJSON {
	out := make([]attachmentJSON, 0, len(attachments))
	for _, a := range attachments {
		out = append(out, attachmentJSON{
			ID:        a.ID,
			Path:      a.Path,
			CreatedAt: a.CreatedAt,
		})
	}
	return out
}

// liveAgents returns the set of agent tags holding a live presence claim.
// store.ListWork already filters to claims within WorkTTL, so membership is
// exactly "at the keyboard right now" (docs/DESIGN.md §3). Read once per
// request and joined against row assignees to compute assignee_live — the
// stale-assignment tier is assignee != "" && !assignee_live.
// assigneeLive reports whether an assigned task's holder is at the keyboard.
// An unassigned row is never live: without the empty-string guard a stray
// AgentActivity row with an empty agent_id would light up assignee_live on
// every unassigned task in the response, and the TUI's stale tier reads this
// field as "assignee != ” && !assignee_live" (docs/DESIGN.md §3).
func assigneeLive(live map[string]bool, assignee string) bool {
	return assignee != "" && live[assignee]
}

func liveAgents(s *store.Store) (map[string]bool, error) {
	work, err := s.ListWork()
	if err != nil {
		return nil, err
	}
	live := make(map[string]bool, len(work))
	for _, w := range work {
		live[w.AgentID] = true
	}
	return live, nil
}

func validStatusFilter(status string) bool {
	switch status {
	case "open", "all", "pending", "in_progress", "complete":
		return true
	}
	return false
}

// notesBudget caps the total inlined body bytes in one list_tasks response
// A row's body is never cut mid-text: a row that would push the
// response past the budget keeps its (has_notes, notes_len) flags but its
// body stays out, and its id is reported in `elided` — it comes back whole
// or not at all.
const notesBudget = 40000

// inlineNotes fills the Notes fields on rows from the matching store tasks.
// Rows without a match (should not happen in practice) are left as-is. This
// is the unbudgeted form used by farol://inbox, which caps rows at 20 of its
// own and is not a list_tasks response.
//
// Skeleton rows are skipped: the inbox filters per task like list_tasks, so it
// emits context_only ancestors too, and a skeleton is tree scaffolding rather
// than content — bodies stay off it on every surface, not just
// list_tasks.
func inlineNotes(rows []taskRowJSON, tasks []store.Task) {
	byID := make(map[string]string, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t.Notes
	}
	for i := range rows {
		if rows[i].ContextOnly {
			continue
		}
		body, ok := byID[rows[i].ID]
		if !ok {
			continue
		}
		rows[i].Notes = body
	}
}

// inlineBodyBudget inlines notes and comments into rows under the byte
// budget, walking rows in preorder and accumulating len(notes) +
// sum(len(comment.note)). Once a row's body would push the running total
// past notesBudget, that row and every later row keep has_notes/notes_len
// but get no inlined body, and their ids are returned in elided — never cut
// mid-text. Skeleton rows (context_only) never take from the budget: their
// bodies are never inlined. budgetExceed reports whether the budget
// was hit at all, which is exactly len(elided) > 0.
//
// Only rows that actually have a body are charged to the budget or named in
// elided. elided exists so the agent can re-fetch the dropped bodies with
// show_task, so listing a row with no notes and no comments would buy it a
// round-trip that returns nothing — a cost worth removing.
//
// Comment presence comes from ONE store.TaskIDsWithComments query for the
// whole list, not a ListComments per row: that helper exists for this exact
// N+1 — a per-request read stays per-request. Only rows the set says are
// commented are read.
func inlineBodyBudget(s *store.Store, listID string, rows []taskRowJSON, tasks []store.Task, includeNotes, includeComments bool) (elided []string, budgetExceeded bool, err error) {
	byID := make(map[string]string, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t.Notes
	}
	commented := map[string]bool{}
	if includeComments {
		commented, err = s.TaskIDsWithComments(listID)
		if err != nil {
			return nil, false, err
		}
	}
	// hasBody is what makes a row eligible to be inlined — and therefore to be
	// elided when the budget runs out.
	hasBody := func(r *taskRowJSON) bool {
		if r.ContextOnly {
			return false
		}
		return (includeNotes && len(byID[r.ID]) > 0) || (includeComments && commented[r.ID])
	}
	used := 0
	for i := range rows {
		row := &rows[i]
		if !hasBody(row) {
			continue
		}
		cost := 0
		if includeNotes {
			cost += len(byID[row.ID])
		}
		var comments []store.Comment
		if includeComments && commented[row.ID] {
			comments, err = s.ListComments(row.ID)
			if err != nil {
				return nil, false, err
			}
			for _, c := range comments {
				cost += len(c.Note)
			}
		}
		if used+cost > notesBudget {
			// This row and every later one keep has_notes/notes_len but get no
			// body; rows with nothing to inline are not "dropped" and so are
			// not named (skeletons among them — they never had a body).
			for j := i; j < len(rows); j++ {
				if !hasBody(&rows[j]) {
					continue
				}
				elided = append(elided, rows[j].ID)
			}
			budgetExceeded = true
			break
		}
		if includeNotes {
			row.Notes = byID[row.ID]
		}
		if len(comments) > 0 {
			row.Comments = commentsJSON(comments)
		}
		used += cost
	}
	return elided, budgetExceeded, nil
}

// jsonResult returns a tool result whose single text content is a JSON
// encoding of v.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil, nil
}

// addWorkResource registers the farol://work static resource — a read-only
// mirror of list_work so any MCP host that auto-reads resources surfaces it.
// The claim_work tool that used to write this set is gone: presence claims
// now come only from task writes (autoClaim).
func addWorkResource(server *mcp.Server, s *store.Store) {
	server.AddResource(&mcp.Resource{
		URI:         "farol://work",
		Name:        "Agent activity",
		Description: "Which tasks/lists an agent is currently working on, as seen by the TUI.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		work, err := s.ListWork()
		if err != nil {
			return nil, err
		}
		out := make([]struct {
			ID         string `json:"id"`
			EntityType string `json:"entity_type"`
			EntityID   string `json:"entity_id"`
			AgentID    string `json:"agent_id"`
			Kind       string `json:"kind"`
			AcquiredAt int64  `json:"acquired_at"`
		}, len(work))
		for i, w := range work {
			out[i] = struct {
				ID         string `json:"id"`
				EntityType string `json:"entity_type"`
				EntityID   string `json:"entity_id"`
				AgentID    string `json:"agent_id"`
				Kind       string `json:"kind"`
				AcquiredAt int64  `json:"acquired_at"`
			}{
				ID: w.ID, EntityType: w.EntityType, EntityID: w.EntityID,
				AgentID: w.AgentID, Kind: string(w.Kind), AcquiredAt: w.AcquiredAt,
			}
		}
		b, err := json.Marshal(out)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(b),
			}},
		}, nil
	})
}

// addResources registers farol:///inbox, the one read-only resource that is
// not a duplicate of a tool.
//
// crush:///lists, crush:///lists/{id}, crush:///lists/{id}/tasks,
// crush:///tasks/{id} and crush:///search/{query} used to live here and were
// deleted: each was a row-for-row duplicate of my_list / list_tasks /
// show_task / search_tasks, and docs/DESIGN.md §9 pins resource rows as a
// superset of the CLI's --json shapes — so every field added to a task had
// to be added in three places or the surfaces drifted. Hosts do not
// auto-read resources, so they cost maintenance and bought nothing at
// runtime. Do not re-add them; add the field to the tool instead.
func addResources(server *mcp.Server, s *store.Store, identity string) {
	// Static: farol:///inbox — one-shot start-of-session context:
	// your list plus every foreign list, each with up to 20 pending tasks
	// and their notes inlined, so a session can open in one read instead
	// of my_list + list_tasks + show_task fan-out.
	server.AddResource(&mcp.Resource{
		URI:         "farol:///inbox",
		Name:        "Inbox",
		Description: "Start-of-session context: your list plus every foreign list with the top 20 pending tasks and their notes.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		lists, err := s.ListLists()
		if err != nil {
			return nil, err
		}
		type block struct {
			ID            string        `json:"id"`
			Name          string        `json:"name"`
			Pending       int           `json:"pending"`
			Complete      int           `json:"complete"`
			CreatedBy     string        `json:"created_by,omitempty"`
			Collaborative bool          `json:"collaborative,omitempty"`
			Tasks         []taskRowJSON `json:"tasks"`
		}
		// Hoisted out of the loop: one presence read for the whole inbox, not
		// one per list.
		live, err := liveAgents(s)
		if err != nil {
			return nil, err
		}
		var mine block
		foreign := make([]block, 0)
		for _, l := range lists {
			tasks, err := s.ListTasks(l.List.ID)
			if err != nil {
				return nil, err
			}
			rows, err := sectionRows(s, tasks, "pending", l.CreatedBy, live)
			if err != nil {
				return nil, err
			}
			if len(rows) > 20 {
				rows = rows[:20]
			}
			inlineNotes(rows, tasks)
			b := block{
				ID: l.List.ID, Name: l.List.Name,
				Pending: l.PendingCount, Complete: l.CompleteCount,
				CreatedBy: l.CreatedBy, Collaborative: l.Collaborative, Tasks: rows,
			}
			if l.CreatedBy == identity {
				b.CreatedBy = ""
				mine = b
			} else {
				foreign = append(foreign, b)
			}
		}
		return marshalResource(req.Params.URI, map[string]any{
			"mine":          mine,
			"foreign_lists": foreign,
		})
	})
}

// addPrompts registers canned agent workflows that embed the current app
// state, so an agent that reads prompts/list can pick one and get a
// ready-made message.
func addPrompts(server *mcp.Server, s *store.Store) {
	// farol_inbox is the canonical one-shot opener (registered below). The old
	// farol_daily_agenda prompt overlapped it and embedded a second, heavier
	// copy of app state, so it was dropped — one opener prompt is enough.
	server.AddPrompt(&mcp.Prompt{
		Name:        "farol_inbox",
		Description: "One-shot start-of-session triage: read the farol:///inbox resource and pick the next task. Carries the full working loop so the agent does not need the heavy blob every session.",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		msg := "Read the resource farol:///inbox for your list, every foreign list, and their top 20 pending tasks with notes inlined. Keep their status current as you work, on your own, without being asked.\n\n" +
			"WORKING LOOP (run it automatically, without being told):\n" +
			"1. Open the session in one read: farol:///inbox (or my_list + list_tasks with include=['notes']). Skip show_task where has_notes is false.\n" +
			"2. Get your tasks from Farol at the start of every session and refresh them as you go; read from it rather than working from memory.\n" +
			"3. Grab the task BEFORE you research it: next_task(list_id) atomically assigns you the top eligible task — highest priority (high > medium > low > none), then tree order — and returns its whole subtree, notes and comments in one call; assign_task(ids=[...]) grabs a specific one the same way. Owning it first is what stops a second agent burning tokens on the same work.\n" +
			"4. Before working a task on a list you do not own, read the WHOLE list first (related / prerequisite / converging tasks), and read that task's notes AND comments (show_task returns both).\n" +
			"5. Starting a task: set_status(ids, progress=...) flips it to in_progress and auto-claims it (the spinner shows). Set a percentage scaled to the task: progress='percentage' with percent ~= fraction of steps done for multi-step work; progress='subtasks' when it has children; progress='simple' only for atomic tasks. A flat \"in progress\" with no percentage is not enough.\n" +
			"6. Advance the percentage as you go, not only at the end — the human watches the TUI live. Leave comment notes at decision points on tasks you do not own.\n" +
			"7. After finishing: re-read the task's comments, then set_status(ids, status='complete') — completing auto-unassigns the task. If you stop without finishing, release it with assign_task(ids=[...], release=true): an assignment has no TTL, and although your session end releases it too, an explicit release frees the task the moment you stop rather than whenever your process exits.\n" +
			"8. Before the next task: check what changed since you last looked (list_tasks(list_id, since=<time of your last call>)) — priorities or comments may have moved.\n\n" +
			"A task whose assignee is set but whose assignee_live is false is abandoned work, not free work: take it over with force=true (which records a takeover comment). Force never overrides a subtree reservation — when an ancestor or descendant is the blocker, release that task first (assign_task(ids=[blocker], release=true, force=true)) or ask the human to release the whole list from the TUI.\n\n" +
			"Pick one pending task (prefer a foreign list), grab it with next_task (or assign_task for a specific one), and start working. Do not fan out to show_task for tasks whose has_notes is false."
		return &mcp.GetPromptResult{
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: msg},
			}},
		}, nil
	})

	server.AddPrompt(&mcp.Prompt{
		Name:        "farol_breakdown",
		Description: "Break a task into subtasks. Give the task's id (prefix ok) and the agent walks the task and asks for sub-bullets.",
		Arguments: []*mcp.PromptArgument{{
			Name:        "task_id",
			Description: "task id or prefix",
			Required:    true,
		}},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		arg := req.Params.Arguments["task_id"]
		if strings.TrimSpace(arg) == "" {
			return nil, fmt.Errorf("farol_breakdown requires a task_id argument")
		}
		id, err := s.ResolveID("task", arg)
		if err != nil {
			return nil, err
		}
		t, err := s.GetTask(id)
		if err != nil {
			return nil, err
		}

		msg := fmt.Sprintf(
			"Break this task into concrete, shippable subtasks.\n\n"+
				"Task: %s\nStatus: %s\nNotes: %s\n\n"+
				"Return a numbered list of subtasks, each a single actionable unit. "+
				"Capture each one with add_task, passing this task's id as parent.",
			t.Title, t.Status, t.Notes,
		)

		return &mcp.GetPromptResult{
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: msg},
			}},
		}, nil
	})
}

// marshalResource marshals v and wraps it in a ReadResourceResult.
func marshalResource(uri string, v any) (*mcp.ReadResourceResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(b),
		}},
	}, nil
}

// errorResult turns a domain error into a tool error the client can see.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}
