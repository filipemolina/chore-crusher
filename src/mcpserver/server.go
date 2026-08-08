package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/config"
	"github.com/filipemolina/chore-crusher/src/constants"
	"github.com/filipemolina/chore-crusher/src/store"
)

// ownerTagPattern is the human-readable form of createdByRE, used in error
// messages (docs/plan/list-ownership-enforcement.md §4.C).
const ownerTagPattern = "^[A-Za-z0-9_-]{1,32}$"

// createdByRE validates an explicit created_by tag an agent may pass to
// add_list (docs/plan/list-ownership-enforcement.md §4.C). The store does not
// re-validate the tag format, so the MCP layer is the only place this check
// lives.
var createdByRE = regexp.MustCompile(ownerTagPattern)

// progressJSON is the derived-progress shape shared by task rows.
type progressJSON struct {
	Kind            string `json:"kind"`
	Percent         *int   `json:"percent"`
	DisplayAsSimple bool   `json:"display_as_simple"`
}

// taskRowJSON is one row of a task tree/list result, matching the CLI's
// --json shape for tasks (docs/DESIGN.md §9).
type taskRowJSON struct {
	ID             string        `json:"id"`
	ParentID       *string       `json:"parent_id"`
	Title          string        `json:"title"`
	Status         string        `json:"status"`
	Progress       *progressJSON `json:"progress,omitempty"`
	Depth          int           `json:"depth"`
	ListOwner      string        `json:"list_owner"`
	HasNotes       bool          `json:"has_notes"`
	NotesLen       int           `json:"notes_len"`
	Notes          string        `json:"notes,omitempty"`           // populated only when include=notes; see step B
	NotesTruncated bool          `json:"notes_truncated,omitempty"` // true when inlined notes were trimmed to notesTruncationLimit; see step B
	Assignee       string        `json:"assignee"`                  // "" when unassigned
	AssignedAt     *int64        `json:"assigned_at"`               // null when unassigned
	AssigneeLive   bool          `json:"assignee_live"`             // live presence claim by the assignee
	Priority       string        `json:"priority"`                  // none|low|medium|high
	ContextOnly    bool          `json:"context_only,omitempty"`    // ancestor skeleton row
	Comments       []commentJSON `json:"comments,omitempty"`        // descendant rows in show_task; include=['comments'] in list_tasks
}

// taskDetailsJSON is the payload for the show_task tool.
type taskDetailsJSON struct {
	ID           string        `json:"id"`
	ListID       string        `json:"list_id"`
	ListOwner    string        `json:"list_owner"`
	Title        string        `json:"title"`
	Notes        string        `json:"notes"`
	Status       string        `json:"status"`
	Progress     progressJSON  `json:"progress"`
	CreatedAt    int64         `json:"created_at"`
	UpdatedAt    int64         `json:"updated_at"`
	CompletedAt  *int64        `json:"completed_at"`
	Assignee     string        `json:"assignee"`      // "" when unassigned
	AssignedAt   *int64        `json:"assigned_at"`   // null when unassigned
	AssigneeLive bool          `json:"assignee_live"` // live presence claim by the assignee
	Priority     string        `json:"priority"`      // none|low|medium|high
	Children     []taskRowJSON `json:"children"`
	Comments     []commentJSON `json:"comments"`
}

// commentJSON mirrors store.Comment for the MCP surface (docs/plan/task-comments.md
// §4). It appears in taskDetailsJSON (show_task). Author is the OS username
// (CLI/TUI path) or the server identity (agent path).
type commentJSON struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Note      string `json:"note"`
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
func NewServer() (*mcp.Server, *store.Store, error) {
	s, err := store.Open(config.DBPath())
	if err != nil {
		return nil, nil, err
	}

	// identity is the agent tag this server acts as: status/progress writes
	// refresh a live claim held by this identity (a presence heartbeat,
	// docs/plan/agent-presence-heartbeat.md §3.3). Ownership enforcement
	// (docs/plan/list-ownership-enforcement.md) will reuse the same read.
	identity := os.Getenv("CRUSH_AGENT")
	if identity == "" {
		identity = "agent"
	}

	// The Instructions doc is delivered to clients in the initialize result and
	// is how an agent discovers this API without trial-and-error (query it with
	// `mcp({ instructions: "chore-crusher" })`). Keep it in sync with the tool
	// list below.
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "chore-crusher",
		Version: constants.Version(),
	}, &mcp.ServerOptions{Instructions: `Chore Crusher is the todo store this work lives in; the TUI is how the human watches it. It IS your todo list: read your tasks from here at the start of every session and keep their status current as you work — on your own, without being asked. Do NOT use the host's built-in todo tool.

IDENTITY & OWNERSHIP. You act under the tag CRUSH_AGENT (here: "` + identity + `"). Track your own work in a list named "` + identity + `: ..." — chore_crusher_my_list get-or-creates it. Each list has an owner (created_by); a list is yours only when created_by == your tag. The server ENFORCES this: structural edits (add_task, edit_task, delete_task, add_list) on a list you do NOT own are refused. But on ANY list you may read everything and change status/progress (set_progress, complete_task, reopen_task) and comment. Untagged lists (human-made) are owned by nobody and are foreign to you — UNLESS a human has explicitly marked it collaborative (a per-list opt-in flag, off by default, set from the TUI's list-rename modal): a collaborative list accepts structural edits from any agent regardless of created_by. Check the collaborative field on my_list's foreign_lists before assuming a foreign list is read-only. Comments have their own, narrower ownership rule: delete_comment only removes a comment whose author is your own tag, regardless of who owns the list it's on.

IDs: every id parameter accepts a short unambiguous prefix. Lists are addressed by id, never by name. Tools whose parameter is 'ids' accept 1..50 in one call.

TOOLS (chore_crusher_<name>):
- my_list() — session opener: {mine, foreign_lists} with pending/complete counts
- list_tasks(list_id, status?, include?) — one list's task tree; status=all|pending|in_progress|complete; include=['notes'] inlines note bodies
- list_changes(list_id, since, include?) — tasks changed after unix 'since'; call between tasks instead of re-reading the list
- show_task(ids) — full details + children + comments for 1..50 tasks
- search_tasks(query, list_id?) — fuzzy over titles and notes
- add_task(list_id, title, parent?, notes?)
- edit_task(id, title?, notes?, parent?, to_root?) — change any field; notes replaces the whole body; parent re-parents; to_root moves to the list root
- delete_task(id, force=true)
- set_progress(ids, mode, percent?) — mode=simple|subtasks|percentage; percent only for percentage; flips the task to in_progress
- complete_task(ids) — cascades to descendants, auto-completes ancestors
- reopen_task(ids) — back to pending; does not cascade
- add_comment(task_id, note) — attributed to your tag
- delete_comment(id, force=true) — only your own comments (author == your tag), regardless of list ownership
- add_list(name, created_by?) — owned by you
- claim_work(entity_type, entity_id, kind?, release?) — light/stop the TUI spinner; writes auto-claim for you, so you only need this to reserve a task BEFORE writing

KEEP THE BOARD LIVE (do this yourself, unasked):
- Start a task = set_progress on it (flips it to in_progress and lights the spinner). Advance the percentage as you go, not only at the end — the human watches the TUI live.
- On a list you do not own: read the whole list plus the task's notes and comments first; leave add_comment at decision points; never edit its content.
- Finish = complete_task. A percentage of 100 does NOT auto-complete.

For the full working loop and a one-read session opener, use the crush_inbox prompt or read the crush:///inbox resource.

GOTCHAS: set_progress on a complete task errors — reopen first. set_progress(mode=subtasks) derives from children — on a shared task use percentage. Your own auto-created "` + identity + `: ..." Inbox is deleted at session end if it is empty or all-complete, so don't rely on it as long-term storage.`})

	addListTools(server, s, identity)
	addTaskTools(server, s, identity)
	addWorkTools(server, s, identity)
	addWorkResource(server, s)
	addResources(server, s, identity)
	addPrompts(server, s)

	return server, s, nil
}

// Run starts the MCP server on the stdio transport. It blocks until the
// client disconnects or the context is cancelled.
func Run(ctx context.Context) error {
	server, s, err := NewServer()
	if err != nil {
		return err
	}
	defer s.Close()

	err = server.Run(ctx, &mcp.StdioTransport{})
	// H13: when the MCP session ends (client disconnected or context
	// cancelled), release every claim so the TUI does not show stale
	// spinners for a disconnected agent. The enhancement plan
	// (docs/plan/mcp-server-enhancement.md §3.1) promised this on session
	// end but it was never wired; the 120s TTL covered the gap.
	if _, rErr := s.ReleaseAllClaims(); rErr != nil {
		fmt.Fprintf(os.Stderr, "releasing claims on session end: %v\n", rErr)
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
		Description: "Get or create your own list (named after the CRUSH_AGENT tag) for tracking your own work, plus a summary of every other (foreign) list, so you can start a session in one call. Example: my_list(). Returns {mine: {id,name,pending,complete}, foreign_lists: [{id,name,pending,complete,created_by,collaborative}]}. collaborative=true means structural edits (add_task, edit_task, delete_task) are allowed on that list despite not owning it.",
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
		Description: "List a list's tasks as a depth-annotated tree. Example: list_tasks(list_id='01ABC...', status='pending', include=['notes']). status defaults to 'all'; one of all, pending, in_progress, complete. include is an optional set of extra fields to inline per row; supported values: 'notes' (inline the full notes body, truncated at 2000 chars with notes_truncated=true). Prefer this over N show_task calls.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ListID  string   `json:"list_id" jsonschema:"list id or unambiguous prefix"`
		Status  string   `json:"status,omitempty" jsonschema:"all, pending, in_progress, or complete"`
		Include []string `json:"include,omitempty" jsonschema:"extra per-row fields to inline; currently supports 'notes'"`
	}) (*mcp.CallToolResult, any, error) {
		if in.Status == "" {
			in.Status = "all"
		}
		if !validStatusFilter(in.Status) {
			return errorResult(fmt.Errorf("invalid status %q: want all, pending, in_progress, or complete", in.Status)), nil, nil
		}
		includeNotes := false
		for _, k := range in.Include {
			switch k {
			case "notes":
				includeNotes = true
			default:
				return errorResult(fmt.Errorf("unknown include %q: supported values: notes", k)), nil, nil
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
		if includeNotes {
			inlineNotes(rows, tasks)
		}
		return jsonResult(rows)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_changes",
		Description: "List the tasks in one list whose activity changed strictly " +
			"after a unix timestamp — created, status/progress, renamed, re-noted, " +
			"re-parented, or newly commented. Use it between tasks to see whether the " +
			"list moved since you last looked, instead of re-reading the whole list. " +
			"Rows use the same shape as list_tasks (has_notes/notes_len; add " +
			"include=['notes'] to inline bodies). Deletions are not reported. " +
			"Tip: pass the time you made your PREVIOUS call as `since`. " +
			"Example: list_changes(list_id='01ABC...', since=1786000000).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ListID  string   `json:"list_id" jsonschema:"list id or unambiguous prefix"`
		Since   int64    `json:"since" jsonschema:"unix seconds; return tasks changed strictly after this"`
		Include []string `json:"include,omitempty" jsonschema:"extra per-row fields to inline; supports 'notes'"`
	}) (*mcp.CallToolResult, any, error) {
		includeNotes := false
		for _, k := range in.Include {
			switch k {
			case "notes":
				includeNotes = true
			default:
				return errorResult(fmt.Errorf("unknown include %q: supported values: notes", k)), nil, nil
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
		// Full-list rows first, so depth/parent are correct (sectionRows calls
		// apptypes.Flatten which needs the whole list to resolve parent chains).
		all, err := s.ListTasks(listID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		live, err := liveAgents(s)
		if err != nil {
			return errorResult(err), nil, nil
		}
		rows, err := sectionRows(s, all, "all", list.CreatedBy, live)
		if err != nil {
			return errorResult(err), nil, nil
		}
		// Then keep only the ones that changed since `since`.
		changed, err := s.TasksChangedSince(listID, in.Since)
		if err != nil {
			return errorResult(err), nil, nil
		}
		changedSet := make(map[string]bool, len(changed))
		for _, t := range changed {
			changedSet[t.ID] = true
		}
		out := make([]taskRowJSON, 0, len(changed))
		for _, r := range rows {
			if changedSet[r.ID] {
				out = append(out, r)
			}
		}
		if includeNotes {
			inlineNotes(out, all)
		}
		return jsonResult(out)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_task",
		Description: "Add a task to a list, optionally nested under a parent. Example: add_task(list_id='01ABC...', title='Buy milk', parent='01DEF...', notes='whole milk').",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ListID string `json:"list_id" jsonschema:"list id or unambiguous prefix"`
		Title  string `json:"title" jsonschema:"task title"`
		Parent string `json:"parent,omitempty" jsonschema:"optional parent task id or prefix"`
		Notes  string `json:"notes,omitempty" jsonschema:"optional notes"`
	}) (*mcp.CallToolResult, any, error) {
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
		// joins against this map instead of querying per row
		// (docs/plan/mcp-assignment-and-priorities.md §8).
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
			t, err := s.GetTask(id)
			if err != nil {
				out = append(out, errRow{ID: raw, Error: err.Error()})
				continue
			}
			prog, err := taskProgressJSON(s, id)
			if err != nil {
				out = append(out, errRow{ID: raw, Error: err.Error()})
				continue
			}
			all, err := s.ListTasks(t.ListID)
			if err != nil {
				out = append(out, errRow{ID: raw, Error: err.Error()})
				continue
			}
			l, err := s.GetList(t.ListID)
			if err != nil {
				out = append(out, errRow{ID: raw, Error: err.Error()})
				continue
			}
			children, err := descendantRows(s, all, id, l.CreatedBy, live)
			if err != nil {
				out = append(out, errRow{ID: raw, Error: err.Error()})
				continue
			}
			comments, err := s.ListComments(id)
			if err != nil {
				out = append(out, errRow{ID: raw, Error: err.Error()})
				continue
			}
			out = append(out, taskDetailsJSON{
				ID: t.ID, ListID: t.ListID, ListOwner: l.CreatedBy, Title: t.Title, Notes: t.Notes,
				Status: string(t.Status), Progress: prog, CreatedAt: t.CreatedAt,
				UpdatedAt: t.UpdatedAt, CompletedAt: t.CompletedAt,
				Assignee: t.Assignee, AssignedAt: t.AssignedAt,
				AssigneeLive: assigneeLive(live, t.Assignee), Priority: string(t.Priority),
				Children: children, Comments: commentsJSON(comments),
			})
		}
		return jsonResult(out)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_comment",
		Description: "Add a comment to a task. Anyone may comment on any task regardless of ownership, unless the list has comments disabled. Example: add_comment(task_id='01ABC...', note='checking in'). author is not accepted — comments are attributed to this server's identity (CRUSH_AGENT).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		TaskID string `json:"task_id" jsonschema:"task id or unambiguous prefix"`
		Note   string `json:"note" jsonschema:"the comment text"`
		Author string `json:"author,omitempty" jsonschema:"rejected if set — comments are always attributed to the server identity"`
	}) (*mcp.CallToolResult, any, error) {
		if in.Author != "" {
			return errorResult(fmt.Errorf("author is not a supported parameter: comments are attributed to this server's identity (%q)", identity)), nil, nil
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
		// agent, same as a status/progress write (docs/plan/mcp-presence-on-all-writes.md).
		autoClaim(s, "task", id, identity)
		return jsonResult(map[string]string{"id": cid})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_comment",
		Description: "Delete a comment. Requires force=true. Only your own comments (author == this server's identity) may be deleted, regardless of who owns the comment's list — a foreign comment is refused. Example: delete_comment(id='01ABC...', force=true).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID    string `json:"id" jsonschema:"comment id"`
		Force bool   `json:"force" jsonschema:"must be true to confirm deletion"`
	}) (*mcp.CallToolResult, any, error) {
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
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "complete_task",
		Description: "Mark 1..50 tasks complete; each cascades to all descendants. Example: complete_task(ids=['01ABC...','01DEF...']). Returns one result row per id in input order: {id,ok:true} or {id,error}; a bad id does not stop the rest.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		IDs []string `json:"ids" jsonschema:"task ids or unambiguous prefixes; 1..50"`
	}) (*mcp.CallToolResult, any, error) {
		return batchApply(s, identity, in.IDs, func(id string) error { return s.Complete(id) })
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reopen_task",
		Description: "Reopen 1..50 tasks to pending; does not cascade to children. Example: reopen_task(ids=['01ABC...','01DEF...']). Returns one result row per id in input order: {id,ok:true} or {id,error}; a bad id does not stop the rest.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		IDs []string `json:"ids" jsonschema:"task ids or unambiguous prefixes; 1..50"`
	}) (*mcp.CallToolResult, any, error) {
		return batchApply(s, identity, in.IDs, func(id string) error { return s.Reopen(id) })
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_task",
		Description: "Delete a task and its descendants. Requires force=true. Example: delete_task(id='01ABC...', force=true).",
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
		if err := requireWritableTask(s, identity, id); err != nil {
			return errorResult(err), nil, nil
		}
		if err := s.DeleteTask(id); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "edit_task",
		Description: "Edit a task's fields in one call. Any omitted field is left unchanged. title renames it; notes REPLACES the whole notes body (pass '' to clear); parent re-parents it under that task; to_root=true moves it to the list root. Example: edit_task(id='01ABC', title='New name', notes='updated'). Structural edit — refused on a list you do not own.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID     string  `json:"id" jsonschema:"task id or unambiguous prefix"`
		Title  *string `json:"title,omitempty" jsonschema:"new title; omit to leave unchanged"`
		Notes  *string `json:"notes,omitempty" jsonschema:"replacement notes (whole body; '' clears); omit to leave unchanged"`
		Parent *string `json:"parent,omitempty" jsonschema:"new parent task id or prefix; omit to leave unchanged"`
		ToRoot bool    `json:"to_root,omitempty" jsonschema:"true moves the task to the list root"`
	}) (*mcp.CallToolResult, any, error) {
		if in.Title == nil && in.Notes == nil && in.Parent == nil && !in.ToRoot {
			return errorResult(fmt.Errorf("edit_task needs at least one of title, notes, parent, to_root")), nil, nil
		}
		if in.Parent != nil && in.ToRoot {
			return errorResult(fmt.Errorf("pass either parent or to_root, not both")), nil, nil
		}
		id, err := s.ResolveID("task", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		// Title/notes edits touch the task's own content, so gate on its list.
		if in.Title != nil || in.Notes != nil {
			if err := requireWritableTask(s, identity, id); err != nil {
				return errorResult(err), nil, nil
			}
		}
		// Re-parenting needs the TARGET list writable: the parent's list for a
		// cross-list move, or the task's own list when moving to root. This
		// preserves move_task's rule (docs/plan/list-ownership-enforcement.md
		// §4.D, §7) — a task must never be half-moved into a list the
		// requester does not own.
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
		autoClaim(s, "task", id, identity)
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_progress",
		Description: "Set a task's progress mode (simple, subtasks, or percentage) for 1..50 tasks in one call. Example: set_progress(ids=['01ABC...'], mode='percentage', percent=50). percent is required only when mode='percentage'. Returns one result row per id in input order: {id,ok:true} or {id,error}; a bad id does not stop the rest.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		IDs     []string `json:"ids" jsonschema:"task ids or unambiguous prefixes; 1..50"`
		Mode    string   `json:"mode" jsonschema:"simple, percentage, or subtasks"`
		Percent *int     `json:"percent,omitempty" jsonschema:"percent 0-100, required when mode=percentage"`
	}) (*mcp.CallToolResult, any, error) {
		if len(in.IDs) == 0 {
			return errorResult(fmt.Errorf("set_progress requires at least one id")), nil, nil
		}
		if len(in.IDs) > 50 {
			return errorResult(fmt.Errorf("set_progress capped at 50 ids per call, got %d", len(in.IDs))), nil, nil
		}
		// Validate mode/percent once up front so a bad request fails before any
		// write (mirrors the old update_tasks behaviour).
		switch in.Mode {
		case "simple", "subtasks":
			if in.Percent != nil {
				return errorResult(fmt.Errorf("percent is only valid with mode=percentage")), nil, nil
			}
		case "percentage":
			if in.Percent == nil {
				return errorResult(fmt.Errorf("mode=percentage requires percent")), nil, nil
			}
		default:
			return errorResult(fmt.Errorf("invalid mode %q: want simple, subtasks, or percentage", in.Mode)), nil, nil
		}
		kind := store.ProgressKind(in.Mode)
		percent := in.Percent
		return batchApply(s, identity, in.IDs, func(id string) error { return s.SetProgress(id, kind, percent) })
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
// write already committed and presence tracking is not a write guarantee
// (docs/plan/agent-presence-heartbeat.md §7). If another agent already
// holds the entity, ClaimWork returns an error which is silently dropped
// here — we do not steal their spinner; the write itself is allowed today
// because the write path does not gate on claims; this behaviour is
// unchanged.
func autoClaim(s *store.Store, entityType, entityID, agentID string) {
	if err := s.TouchWork(entityType, entityID, agentID); err == nil {
		_, _ = s.ClaimWork(entityType, entityID, agentID, store.ActivityWorking)
	}
}

// requireWritable rejects a structural write to a list the requester does
// not own (docs/plan/list-ownership-enforcement.md §3.8), UNLESS the list's
// Collaborative flag is set — an explicit human opt-in that lets any agent
// make structural edits regardless of created_by (docs/DESIGN.md §9, "Tag a
// list as collaborative"). An untagged, non-collaborative list is owned by
// nobody and is therefore foreign to every agent: a human manages it via the
// CLI/TUI, which are deliberately unenforced. The check runs after
// ResolveID, so listID is the suffix-free id; the error names that id so the
// agent knows which list refused it. Step D wires this into the structural
// tools; it is defined here (Step C) so the identity read and the helper
// land together.
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
// to (docs/plan/list-ownership-enforcement.md §4.D). It resolves the task,
// reads its ListID, and applies requireWritable — so rename_task/set_notes/
// delete_task defer to the same owner check as the list tools. move_task is
// handled inline because its target list is the *parent's* list, not the
// task's own.
func requireWritableTask(s *store.Store, identity, taskID string) error {
	t, err := s.GetTask(taskID)
	if err != nil {
		return err
	}
	return requireWritable(s, identity, t.ListID)
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

// sectionRows returns the preorder task rows for a status filter, mirroring
// the CLI's sectionRows logic (docs/DESIGN.md §6, §9).
//
// live is read once per request by the caller and passed in, exactly like
// descendantRows: the inbox resource calls this once per list, so reading
// presence in here would run one ListWork query per list rather than one per
// request (docs/plan/mcp-assignment-and-priorities.md §8).
func sectionRows(s *store.Store, tasks []store.Task, status string, listOwner string, live map[string]bool) ([]taskRowJSON, error) {
	converted := apptypes.FromStoreTasks(tasks)
	rows := apptypes.Flatten(converted)
	byID := make(map[string]apptypes.Task, len(converted))
	for _, t := range converted {
		byID[t.ID] = t
	}

	var out []taskRowJSON
	for _, r := range rows {
		root := r.Task
		for root.ParentID != nil {
			root = byID[*root.ParentID]
		}
		switch status {
		case "all":
		case "pending":
			if root.Status != apptypes.StatusPending {
				continue
			}
		case "in_progress":
			if root.Status != apptypes.StatusInProgress {
				continue
			}
		case "complete":
			if root.Status != apptypes.StatusComplete {
				continue
			}
		}
		prog, err := taskProgressJSON(s, r.Task.ID)
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
			Assignee:     r.Task.Assignee,
			AssignedAt:   r.Task.AssignedAt,
			AssigneeLive: assigneeLive(live, r.Task.Assignee),
			Priority:     string(r.Task.Priority),
		}
		if !(prog.Kind == "none" && prog.Percent == nil && prog.DisplayAsSimple) {
			p := prog
			row.Progress = &p
		}
		out = append(out, row)
	}
	return out, nil
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

// descendantRows returns every descendant of rootID (the task itself
// excluded) as depth-annotated rows with derived progress per row, using the
// shared apptypes.DescendantsOf so `crush show` and show_task cannot drift
// (docs/plan/mcp-agent-todo-hardening.md §4.4). Depth is relative to rootID:
// direct children at depth 1. The previous code ran a store-level walk
// through apptypes.Flatten, but Flatten only emits ParentID==nil rows, so a
// pure-descendant set (no list root) flattened to nothing and "children"
// was always empty.
//
// Every row carries its full notes and comments, uncapped: a task's subtree
// is bounded, so one show_task call must be self-contained
// (docs/plan/mcp-assignment-and-priorities.md §4, decision 8). live maps
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
	case "all", "pending", "in_progress", "complete":
		return true
	}
	return false
}

// notesTruncationLimit caps inlined notes per row so a very long note on
// one task cannot balloon the whole list_tasks payload. Keep in sync with
// the description of list_tasks(include=['notes']).
const notesTruncationLimit = 2000

// inlineNotes fills the Notes and NotesTruncated fields on rows from the
// matching store tasks. Rows without a match (should not happen in
// practice) are left as-is.
func inlineNotes(rows []taskRowJSON, tasks []store.Task) {
	byID := make(map[string]string, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t.Notes
	}
	for i := range rows {
		body, ok := byID[rows[i].ID]
		if !ok {
			continue
		}
		if len(body) > notesTruncationLimit {
			rows[i].Notes = body[:notesTruncationLimit]
			rows[i].NotesTruncated = true
		} else {
			rows[i].Notes = body
		}
	}
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

// addWorkTools registers the agent-presence tool claim_work
// (docs/plan/mcp-server-enhancement.md §3.8). It absorbs the former
// release_work via release=true; the former list_work is served by the
// crush://work resource (docs/plan/mcp-tool-consolidation.md §4.5).
// identity is the server's CRUSH_AGENT tag: an omitted agent_id defaults to
// it, so a claim made without one matches the write-heartbeat in
// TouchWork (docs/plan/mcp-agent-todo-hardening.md §4.2).
func addWorkTools(server *mcp.Server, s *store.Store, identity string) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "claim_work",
		Description: "Show or stop the TUI spinner for an agent on a task or list. " +
			"entity_type is \"task\" or \"list\". kind is \"working\" (default) or " +
			"\"inspecting\". release=true stops the spinner (no-op if not claimed). " +
			"Re-claiming by the same agent refreshes the timer (heartbeat); a " +
			"different agent holding the entity returns an error. agent_id defaults " +
			"to this server's identity (CRUSH_AGENT) — omit it unless you need a " +
			"different label, since a claim under another agent_id is not refreshed " +
			"by your writes. NOTE: any task write already auto-claims for you, so " +
			"you only need this to reserve a task BEFORE you start writing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		EntityType string `json:"entity_type"`        // "task" or "list"
		EntityID   string `json:"entity_id"`          // task or list id, or unambiguous prefix
		AgentID    string `json:"agent_id,omitempty"` // short label; default: this server's identity
		Kind       string `json:"kind,omitempty"`     // "working" or "inspecting"; default "working"
		Release    bool   `json:"release,omitempty"`  // true stops the spinner instead of starting it
	}) (*mcp.CallToolResult, any, error) {
		if in.EntityType != "task" && in.EntityType != "list" {
			return errorResult(fmt.Errorf("entity_type must be \"task\" or \"list\", got %q", in.EntityType)), nil, nil
		}
		if in.AgentID == "" {
			in.AgentID = identity
		}
		if in.Kind == "" {
			in.Kind = "working"
		}
		if in.Kind != "working" && in.Kind != "inspecting" {
			return errorResult(fmt.Errorf("kind must be \"working\" or \"inspecting\", got %q", in.Kind)), nil, nil
		}
		id, err := s.ResolveID(in.EntityType, in.EntityID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if in.Release {
			if err := s.ReleaseWork(in.EntityType, id, in.AgentID); err != nil {
				return errorResult(err), nil, nil
			}
			return jsonResult(map[string]bool{"ok": true})
		}
		activityID, err := s.ClaimWork(in.EntityType, id, in.AgentID, store.ActivityKind(in.Kind))
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]string{"id": activityID})
	})
}

// addWorkResource registers the crush://work static resource — a read-only
// mirror of list_work so any MCP host that auto-reads resources surfaces it
// (docs/plan/mcp-server-enhancement.md §3.8).
func addWorkResource(server *mcp.Server, s *store.Store) {
	server.AddResource(&mcp.Resource{
		URI:         "crush://work",
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

// addResources registers crush:///inbox, the one read-only resource that is
// not a duplicate of a tool (docs/plan/mcp-server-enhancement.md §4.1).
//
// crush:///lists, crush:///lists/{id}, crush:///lists/{id}/tasks,
// crush:///tasks/{id} and crush:///search/{query} used to live here and were
// deleted (docs/plan/mcp-assignment-and-priorities.md §8): each was a
// row-for-row duplicate of my_list / list_tasks / show_task / search_tasks,
// and docs/DESIGN.md §9 pins resource rows as a superset of the CLI's --json
// shapes — so every field added to a task had to be added in three places or
// the surfaces drifted. Hosts do not auto-read resources, so they cost
// maintenance and bought nothing at runtime. Do not re-add them; add the
// field to the tool instead.
func addResources(server *mcp.Server, s *store.Store, identity string) {
	// Static: crush:///inbox — one-shot start-of-session context:
	// your list plus every foreign list, each with up to 20 pending tasks
	// and their notes inlined, so a session can open in one read instead
	// of my_list + list_tasks + show_task fan-out.
	server.AddResource(&mcp.Resource{
		URI:         "crush:///inbox",
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
		// one per list (docs/plan/mcp-assignment-and-priorities.md §8).
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
// ready-made message (docs/plan/mcp-server-enhancement.md §4.2).
func addPrompts(server *mcp.Server, s *store.Store) {
	// crush_inbox is the canonical one-shot opener (registered below). The old
	// crush_daily_agenda prompt overlapped it and embedded a second, heavier
	// copy of app state, so it was dropped (docs/plan/mcp-tool-consolidation.md
	// §8) — one opener prompt is enough.
	server.AddPrompt(&mcp.Prompt{
		Name:        "crush_inbox",
		Description: "One-shot start-of-session triage: read the crush:///inbox resource and pick the next task. Carries the full working loop so the agent does not need the heavy blob every session.",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		msg := "Read the resource crush:///inbox for your list, every foreign list, and their top 20 pending tasks with notes inlined. This store IS your todo list — keep its status current as you work, on your own, without being asked. Do NOT use the host's built-in todo tool.\n\n" +
			"WORKING LOOP (run it automatically, without being told):\n" +
			"1. Open the session in one read: crush:///inbox (or my_list + list_tasks with include=['notes']). Skip show_task where has_notes is false.\n" +
			"2. Get your tasks from Chore Crusher at the start of every session and refresh them as you go; read from it rather than working from memory.\n" +
			"3. Before working a task on a list you do not own, read the WHOLE list first (related / prerequisite / converging tasks), and read that task's notes AND comments (show_task returns both).\n" +
			"4. Starting a task = set_progress on it: that flips it to in_progress and auto-claims it (the spinner shows), so you do not need a separate claim_work unless reserving it before you write.\n" +
			"5. Set a percentage scaled to the task: mode='percentage' with percent ~= fraction of steps done for multi-step work; mode='subtasks' when it has children; mode='simple' only for atomic tasks. A flat \"in progress\" with no percentage is not enough.\n" +
			"6. Advance the percentage as you go, not only at the end — the human watches the TUI live. Leave add_comment notes at decision points on tasks you do not own.\n" +
			"7. After finishing: re-read the task's comments, then complete_task.\n" +
			"8. Before the next task: check what changed since you last looked (list_changes(list_id, since)) — priorities or comments may have moved.\n\n" +
			"Pick one pending task (prefer a foreign list), claim it with claim_work, and start working. Do not fan out to show_task for tasks whose has_notes is false."
		return &mcp.GetPromptResult{
			Messages: []*mcp.PromptMessage{{
				Role:    "user",
				Content: &mcp.TextContent{Text: msg},
			}},
		}, nil
	})

	server.AddPrompt(&mcp.Prompt{
		Name:        "crush_breakdown",
		Description: "Break a task into subtasks. Give the task's id (prefix ok) and the agent walks the task and asks for sub-bullets.",
		Arguments: []*mcp.PromptArgument{{
			Name:        "task_id",
			Description: "task id or prefix",
			Required:    true,
		}},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		arg := req.Params.Arguments["task_id"]
		if strings.TrimSpace(arg) == "" {
			return nil, fmt.Errorf("crush_breakdown requires a task_id argument")
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
