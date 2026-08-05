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
	ID       string       `json:"id"`
	ParentID *string      `json:"parent_id"`
	Title    string       `json:"title"`
	Status   string       `json:"status"`
	Progress progressJSON `json:"progress"`
	Depth    int          `json:"depth"`
}

// taskDetailsJSON is the payload for the show_task tool.
type taskDetailsJSON struct {
	ID          string        `json:"id"`
	ListID      string        `json:"list_id"`
	Title       string        `json:"title"`
	Notes       string        `json:"notes"`
	Status      string        `json:"status"`
	Progress    progressJSON  `json:"progress"`
	CreatedAt   int64         `json:"created_at"`
	UpdatedAt   int64         `json:"updated_at"`
	CompletedAt *int64        `json:"completed_at"`
	Children    []taskRowJSON `json:"children"`
}

// searchResultJSON is one row of the search_tasks result.
type searchResultJSON struct {
	ID       string       `json:"id"`
	ListID   string       `json:"list_id"`
	ListName string       `json:"list_name"`
	Title    string       `json:"title"`
	Status   string       `json:"status"`
	Progress progressJSON `json:"progress"`
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
	}, &mcp.ServerOptions{Instructions: `Chore Crusher is the todo store this work lives in; the TUI is how the human watches it. For tracking your OWN work, use a list named "` + identity + `: ..." (create with chore_crusher_add_list, or call chore_crusher_my_list to get/create it). Do NOT use the host's built-in todo tool — Chore Crusher is the todo store. Act under the tag CRUSH_AGENT (default "agent"; here it is "` + identity + `"). Every list has an owner tag, reported as created_by by chore_crusher_list_lists; a list is yours only when its created_by equals your tag. The server ENFORCES this — it is not just a convention: structure/content edits (chore_crusher_rename_list, chore_crusher_delete_list, chore_crusher_add_task, chore_crusher_rename_task, chore_crusher_delete_task, chore_crusher_set_notes, chore_crusher_move_task) on a list you do not own are refused with "list <id> is owned by <owner> — you may read it and update task status/progress only". An untagged list (created_by empty — typically one the human made in the CLI or TUI) belongs to nobody and is foreign to every agent, so those edits are refused there too. On any list, owned or not, you may read everything and change status and progress (chore_crusher_complete_task, chore_crusher_reopen_task, chore_crusher_toggle_task, chore_crusher_set_progress) and chore_crusher_claim_work / chore_crusher_release_work. chore_crusher_add_list and chore_crusher_my_list always create a list owned by you.

Tools are exposed to MCP hosts as chore_crusher_<name> (shown verbatim below). Every id-bearing parameter accepts a short unambiguous prefix of the full id. Lists are addressed by id prefix, never by name.

QUICK REFERENCE (tool: parameters)
- chore_crusher_list_lists()                      every list with pending/complete counts
- chore_crusher_list_tasks(list_id, status)       one list's tasks as a depth-annotated tree; status = all|pending|in_progress|complete (default all)
- chore_crusher_show_task(task_id)                one task's details plus its children
- chore_crusher_search_tasks(query, list_id)      fuzzy search across titles and notes (title matches rank first); list_id narrows to one list
- chore_crusher_set_progress(id, mode, percent)   mode = simple|subtasks|percentage; percent required only for percentage
- chore_crusher_complete_task(id)                 complete, cascades to descendants and auto-completes ancestors once all their children are done
- chore_crusher_reopen_task(id)                   reopen to pending; does NOT cascade to children
- chore_crusher_toggle_task(id)                   toggle complete <-> pending
- chore_crusher_add_task(list_id, title, parent, notes)   add a task, optionally nested and annotated
- chore_crusher_rename_task(id, title)            rename a task
- chore_crusher_set_notes(id, notes)              replace a task's notes (whole text; empty clears)
- chore_crusher_move_task(id, parent)             re-parent; omit parent to move to the list root
- chore_crusher_delete_task(id, force=true) / chore_crusher_delete_list(id, force=true)   deletes require force=true
- chore_crusher_add_list(name, created_by)       created_by is optional and defaults to your own tag
- chore_crusher_rename_list(id, name)            rename a list you own
- chore_crusher_my_list()                       get or create your own "` + identity + `:" list (where to track your own work)
- chore_crusher_claim_work(entity_type, entity_id, agent_id, kind)   entity_type = task|list, kind = working|inspecting
- chore_crusher_release_work(entity_type, entity_id, agent_id)       release a claim; no-op if not claimed
- chore_crusher_list_work()                       active agent claims (the TUI spinners)

BEHAVIOUR & GOTCHAS
- chore_crusher_set_progress(mode="subtasks") derives from the task's subtree; on a shared task prefer mode="percentage".
- A percentage of 100 does NOT auto-complete. To finish a parent, complete its final child (which auto-completes it) or call chore_crusher_complete_task on the parent.
- chore_crusher_set_progress on an already-complete task errors ("reopen it first"): set progress before completing the last child.
- chore_crusher_claim_work is a presence heartbeat: status/progress writes by the claiming agent refresh it; a live claim by another agent on that entity blocks writes — take another task or work your own list.
- chore_crusher_claim_work's agent_id defaults to your identity: omit it, or set it equal to your tag, or your writes will not refresh the spinner.
- Reclaim after an idle pause of ~2 minutes; chore_crusher_release_work when you finish.`})

	addListTools(server, s, identity)
	addTaskTools(server, s, identity)
	addWorkTools(server, s, identity)
	addWorkResource(server, s)
	addResources(server, s)
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

	return server.Run(ctx, &mcp.StdioTransport{})
}

// addListTools registers the list-oriented MCP tools.
func addListTools(server *mcp.Server, s *store.Store, identity string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_lists",
		Description: "List every task list with pending and complete counts. Example: list_lists().",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		lists, err := s.ListLists()
		if err != nil {
			return errorResult(err), nil, nil
		}

		type row struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Pending   int    `json:"pending"`
			Complete  int    `json:"complete"`
			CreatedBy string `json:"created_by"`
			CreatedAt int64  `json:"created_at"`
			Position  int    `json:"position"`
		}
		out := make([]row, len(lists))
		for i, l := range lists {
			out[i] = row{
				ID:        l.List.ID,
				Name:      l.List.Name,
				Pending:   l.PendingCount,
				Complete:  l.CompleteCount,
				CreatedBy: l.CreatedBy,
				CreatedAt: l.List.CreatedAt,
				Position:  l.List.Position,
			}
		}
		return jsonResult(out)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_list",
		Description: "Create a new task list and return its id. Example: add_list(name='Shopping'). Owned by created_by (an agent tag like 'pi'), which defaults to this server's identity; only the owner may add/edit/delete tasks and rename/delete the list — other agents may read it and update task status/progress only.",
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
		Name:        "rename_list",
		Description: "Rename an existing task list. Example: rename_list(id='01ABC...', name='Groceries'). id is a full id or unambiguous prefix.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID   string `json:"id" jsonschema:"list id or unambiguous prefix"`
		Name string `json:"name" jsonschema:"new name"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("list", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if err := requireWritable(s, identity, id); err != nil {
			return errorResult(err), nil, nil
		}
		if err := s.RenameList(id, in.Name); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_list",
		Description: "Delete a task list and every task in it. Requires force=true. Example: delete_list(id='01ABC...', force=true).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID    string `json:"id" jsonschema:"list id or unambiguous prefix"`
		Force bool   `json:"force" jsonschema:"must be true to confirm deletion"`
	}) (*mcp.CallToolResult, any, error) {
		if !in.Force {
			return errorResult(fmt.Errorf("deleting a list requires force=true")), nil, nil
		}
		id, err := s.ResolveID("list", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if err := requireWritable(s, identity, id); err != nil {
			return errorResult(err), nil, nil
		}
		if err := s.DeleteList(id); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "my_list",
		Description: "Get or create your own list (named after the CRUSH_AGENT tag) for tracking your own work. Returns the list id, name, and task counts. Example: my_list().",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		id, err := s.GetOrCreateAgentList(identity)
		if err != nil {
			return errorResult(err), nil, nil
		}
		lists, err := s.ListLists()
		if err != nil {
			return errorResult(err), nil, nil
		}
		for _, l := range lists {
			if l.ID == id {
				return jsonResult(map[string]any{
					"id":       l.ID,
					"name":     l.Name,
					"pending":  l.PendingCount,
					"complete": l.CompleteCount,
				})
			}
		}
		return jsonResult(map[string]string{"id": id})
	})
}

// addTaskTools registers the task-oriented MCP tools. identity is the agent
// tag whose live claims status/progress writes refresh (a presence heartbeat).
func addTaskTools(server *mcp.Server, s *store.Store, identity string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List a list's tasks as a depth-annotated tree. Example: list_tasks(list_id='01ABC...', status='pending'). status defaults to 'all'; one of all, pending, in_progress, complete.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ListID string `json:"list_id" jsonschema:"list id or unambiguous prefix"`
		Status string `json:"status,omitempty" jsonschema:"all, pending, in_progress, or complete"`
	}) (*mcp.CallToolResult, any, error) {
		if in.Status == "" {
			in.Status = "all"
		}
		if !validStatusFilter(in.Status) {
			return errorResult(fmt.Errorf("invalid status %q: want all, pending, in_progress, or complete", in.Status)), nil, nil
		}

		listID, err := s.ResolveID("list", in.ListID)
		if err != nil {
			return errorResult(err), nil, nil
		}

		tasks, err := s.ListTasks(listID)
		if err != nil {
			return errorResult(err), nil, nil
		}

		rows, err := sectionRows(s, tasks, in.Status)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(rows)
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
		return jsonResult(map[string]string{"id": id})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "show_task",
		Description: "Show one task's details and its children as depth-annotated rows. Example: show_task(task_id='01ABC...').",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		TaskID string `json:"task_id" jsonschema:"task id or unambiguous prefix"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("task", in.TaskID)
		if err != nil {
			return errorResult(err), nil, nil
		}

		t, err := s.GetTask(id)
		if err != nil {
			return errorResult(err), nil, nil
		}
		prog, err := taskProgressJSON(s, id)
		if err != nil {
			return errorResult(err), nil, nil
		}

		all, err := s.ListTasks(t.ListID)
		if err != nil {
			return errorResult(err), nil, nil
		}

		children, err := descendantRows(s, all, id)
		if err != nil {
			return errorResult(err), nil, nil
		}

		return jsonResult(taskDetailsJSON{
			ID:          t.ID,
			ListID:      t.ListID,
			Title:       t.Title,
			Notes:       t.Notes,
			Status:      string(t.Status),
			Progress:    prog,
			CreatedAt:   t.CreatedAt,
			UpdatedAt:   t.UpdatedAt,
			CompletedAt: t.CompletedAt,
			Children:    children,
		})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "complete_task",
		Description: "Mark a task complete; cascades to all descendants. Example: complete_task(id='01ABC...').",
	}, taskMutator(s, identity, func(id string) error { return s.Complete(id) }))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reopen_task",
		Description: "Reopen a task to pending; does not cascade to children. Example: reopen_task(id='01ABC...').",
	}, taskMutator(s, identity, func(id string) error { return s.Reopen(id) }))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "toggle_task",
		Description: "Toggle a task between complete and pending. Example: toggle_task(id='01ABC...').",
	}, taskMutator(s, identity, func(id string) error { return s.Toggle(id) }))

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
		Name:        "rename_task",
		Description: "Rename a task. Example: rename_task(id='01ABC...', title='New name').",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID    string `json:"id" jsonschema:"task id or unambiguous prefix"`
		Title string `json:"title" jsonschema:"new title"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("task", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if err := requireWritableTask(s, identity, id); err != nil {
			return errorResult(err), nil, nil
		}
		if err := s.RenameTask(id, in.Title); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_notes",
		Description: "Replace a task's notes (whole text, not append); empty string clears them. Example: set_notes(id='01ABC...', notes='...').",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID    string `json:"id" jsonschema:"task id or unambiguous prefix"`
		Notes string `json:"notes" jsonschema:"replacement notes"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("task", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if err := requireWritableTask(s, identity, id); err != nil {
			return errorResult(err), nil, nil
		}
		if err := s.SetNotes(id, in.Notes); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_progress",
		Description: "Set a task's progress mode: simple, subtasks, or percentage. Example: set_progress(id='01ABC...', mode='percentage', percent=50). percent is required only when mode='percentage'.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID      string `json:"id" jsonschema:"task id or unambiguous prefix"`
		Mode    string `json:"mode" jsonschema:"simple, percentage, or subtasks"`
		Percent *int   `json:"percent,omitempty" jsonschema:"percent 0-100, required when mode=percentage"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("task", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if err := s.SetProgress(id, store.ProgressKind(in.Mode), in.Percent); err != nil {
			return errorResult(err), nil, nil
		}
		// Presence heartbeat, same as taskMutator below: best-effort refresh
		// of the writing agent's live claim on this task.
		_ = s.TouchWork("task", id, identity)
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "move_task",
		Description: "Re-parent a task, or move it to the list root by omitting parent. Example: move_task(id='01ABC...', parent='01DEF...').",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID     string `json:"id" jsonschema:"task id or unambiguous prefix"`
		Parent string `json:"parent,omitempty" jsonschema:"new parent task id or prefix; omit for root"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("task", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		// Decide the target list (the parent's list for a cross-list move, or
		// the task's own list when moving to root) and require it be writable
		// before any write — the store must not half-move a task into a list
		// the requester does not own (docs/plan/list-ownership-enforcement.md
		// §4.D, §7).
		var parentID *string
		var targetList string
		if strings.TrimSpace(in.Parent) != "" {
			pid, err := s.ResolveID("task", in.Parent)
			if err != nil {
				return errorResult(err), nil, nil
			}
			parentID = &pid
			p, err := s.GetTask(*parentID)
			if err != nil {
				return errorResult(err), nil, nil
			}
			targetList = p.ListID
		} else {
			t, err := s.GetTask(id)
			if err != nil {
				return errorResult(err), nil, nil
			}
			targetList = t.ListID
		}
		if err := requireWritable(s, identity, targetList); err != nil {
			return errorResult(err), nil, nil
		}
		if err := s.Reparent(id, parentID); err != nil {
			return errorResult(err), nil, nil
		}
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
		for _, l := range lists {
			listNames[l.List.ID] = l.List.Name
		}

		out := make([]searchResultJSON, len(tasks))
		for i, t := range tasks {
			prog, err := taskProgressJSON(s, t.ID)
			if err != nil {
				return errorResult(err), nil, nil
			}
			out[i] = searchResultJSON{
				ID:       t.ID,
				ListID:   t.ListID,
				ListName: listNames[t.ListID],
				Title:    t.Title,
				Status:   string(t.Status),
				Progress: prog,
			}
		}
		return jsonResult(out)
	})
}

// taskMutator builds a tool handler around a store call that takes a resolved
// task id and returns only an error.
func taskMutator(s *store.Store, identity string, fn func(string) error) func(context.Context, *mcp.CallToolRequest, struct {
	ID string `json:"id" jsonschema:"task id or unambiguous prefix"`
}) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID string `json:"id" jsonschema:"task id or unambiguous prefix"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("task", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if err := fn(id); err != nil {
			return errorResult(err), nil, nil
		}
		// Best-effort presence heartbeat: the write committed, so refresh the
		// writing agent's live claim on this task if it holds one. A touch
		// failure is not a write failure (docs/plan/agent-presence-heartbeat.md
		// §7), so it is deliberately swallowed.
		_ = s.TouchWork("task", id, identity)
		return jsonResult(map[string]bool{"ok": true})
	}
}

// requireWritable rejects a structural write to a list the requester does
// not own (docs/plan/list-ownership-enforcement.md §3.8). An untagged list
// (CreatedBy "") is owned by nobody and is therefore foreign to every agent:
// a human manages it via the CLI/TUI, which are deliberately unenforced. The
// check runs after ResolveID, so listID is the suffix-free id; the error
// names that id so the agent knows which list refused it. Step D wires this
// into the structural tools; it is defined here (Step C) so the identity
// read and the helper land together.
func requireWritable(s *store.Store, identity, listID string) error {
	l, err := s.GetList(listID)
	if err != nil {
		return err
	}
	if l.CreatedBy == identity {
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

// sectionRows returns the preorder task rows for a status filter, mirroring
// the CLI's sectionRows logic (docs/DESIGN.md §6, §9).
func sectionRows(s *store.Store, tasks []store.Task, status string) ([]taskRowJSON, error) {
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
		out = append(out, taskRowJSON{
			ID:       r.Task.ID,
			ParentID: r.Task.ParentID,
			Title:    r.Task.Title,
			Status:   string(r.Task.Status),
			Progress: prog,
			Depth:    r.Depth,
		})
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
func descendantRows(s *store.Store, tasks []store.Task, rootID string) ([]taskRowJSON, error) {
	rows := apptypes.DescendantsOf(apptypes.FromStoreTasks(tasks), rootID)
	out := make([]taskRowJSON, 0, len(rows))
	for _, r := range rows {
		prog, err := taskProgressJSON(s, r.Task.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, taskRowJSON{
			ID:       r.Task.ID,
			ParentID: r.Task.ParentID,
			Title:    r.Task.Title,
			Status:   string(r.Task.Status),
			Progress: prog,
			Depth:    r.Depth,
		})
	}
	return out, nil
}

func validStatusFilter(status string) bool {
	switch status {
	case "all", "pending", "in_progress", "complete":
		return true
	}
	return false
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

// addWorkTools registers the agent-presence tools: claim_work,
// release_work, list_work (docs/plan/mcp-server-enhancement.md §3.8).
// identity is the server's CRUSH_AGENT tag: an omitted agent_id defaults to
// it, so a claim made without one matches the write-heartbeat in
// TouchWork (docs/plan/mcp-agent-todo-hardening.md §4.2).
func addWorkTools(server *mcp.Server, s *store.Store, identity string) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "claim_work",
		Description: "Claim a task or list as being worked on by an agent. The TUI " +
			"shows a live spinner on the row while the claim is active. Re-claiming " +
			"by the same agent refreshes the timer (heartbeat). A different agent " +
			"holding the entity returns an error. entity_type is \"task\" or \"list\". " +
			"agent_id defaults to the server's own identity (CRUSH_AGENT); omit it " +
			"unless you need a different label — a claim under another agent_id is " +
			"not refreshed by your writes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		EntityType string `json:"entity_type"`        // "task" or "list"
		EntityID   string `json:"entity_id"`          // task or list id, or unambiguous prefix
		AgentID    string `json:"agent_id,omitempty"` // short label; default: this server's identity
		Kind       string `json:"kind,omitempty"`     // "working" or "inspecting"; default "working"
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
		activityID, err := s.ClaimWork(in.EntityType, id, in.AgentID, store.ActivityKind(in.Kind))
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]string{"id": activityID})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "release_work",
		Description: "Release an agent's claim on a task or list. The TUI spinner " +
			"stops. A no-op if the entity is not claimed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		EntityType string `json:"entity_type"`        // "task" or "list"
		EntityID   string `json:"entity_id"`          // task or list id, or unambiguous prefix
		AgentID    string `json:"agent_id,omitempty"` // default: this server's identity
	}) (*mcp.CallToolResult, any, error) {
		if in.EntityType != "task" && in.EntityType != "list" {
			return errorResult(fmt.Errorf("entity_type must be \"task\" or \"list\", got %q", in.EntityType)), nil, nil
		}
		if in.AgentID == "" {
			in.AgentID = identity
		}
		id, err := s.ResolveID(in.EntityType, in.EntityID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if err := s.ReleaseWork(in.EntityType, id, in.AgentID); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_work",
		Description: "List active agent claims (tasks/lists an agent is working on). Shows the live spinner state the TUI renders.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		work, err := s.ListWork()
		if err != nil {
			return errorResult(err), nil, nil
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
		return jsonResult(out)
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

// addResources registers read-only resources and resource templates so
// any MCP host that auto-reads resources surfaces them (docs/plan/
// mcp-server-enhancement.md §4.1).
func addResources(server *mcp.Server, s *store.Store) {
	// Static: crush:///lists — full list of lists + counts.
	server.AddResource(&mcp.Resource{
		URI:         "crush:///lists",
		Name:        "Lists",
		Description: "All task lists with pending and complete counts.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		lists, err := s.ListLists()
		if err != nil {
			return nil, err
		}
		type row struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Pending   int    `json:"pending"`
			Complete  int    `json:"complete"`
			CreatedAt int64  `json:"created_at"`
			Position  int    `json:"position"`
		}
		out := make([]row, len(lists))
		for i, l := range lists {
			out[i] = row{
				ID:        l.List.ID,
				Name:      l.List.Name,
				Pending:   l.PendingCount,
				Complete:  l.CompleteCount,
				CreatedAt: l.List.CreatedAt,
				Position:  l.List.Position,
			}
		}
		return marshalResource(req.Params.URI, out)
	})

	// Template: crush:///lists/{id} — one list's summary.
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "crush:///lists/{id}",
		Name:        "List",
		Description: "Summary of one list: name, pending/complete counts.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		id, err := templateID(req.Params.URI, "lists")
		if err != nil {
			return nil, err
		}
		resolved, err := s.ResolveID("list", id)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		lists, err := s.ListLists()
		if err != nil {
			return nil, err
		}
		for _, l := range lists {
			if l.List.ID == resolved {
				return marshalResource(req.Params.URI, struct {
					ID        string `json:"id"`
					Name      string `json:"name"`
					Pending   int    `json:"pending"`
					Complete  int    `json:"complete"`
					CreatedAt int64  `json:"created_at"`
				}{
					ID: l.List.ID, Name: l.List.Name, Pending: l.PendingCount,
					Complete: l.CompleteCount, CreatedAt: l.List.CreatedAt,
				})
			}
		}
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	})

	// Template: crush:///lists/{id}/tasks — the list's task tree.
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "crush:///lists/{id}/tasks",
		Name:        "List tasks",
		Description: "Depth-annotated task tree for one list.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		id, err := templateID(req.Params.URI, "lists")
		if err != nil {
			return nil, err
		}
		resolved, err := s.ResolveID("list", id)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		tasks, err := s.ListTasks(resolved)
		if err != nil {
			return nil, err
		}
		rows, err := sectionRows(s, tasks, "all")
		if err != nil {
			return nil, err
		}
		return marshalResource(req.Params.URI, rows)
	})

	// Template: crush:///tasks/{id} — one task's details.
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "crush:///tasks/{id}",
		Name:        "Task",
		Description: "Full details of one task including its children.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		id, err := templateID(req.Params.URI, "tasks")
		if err != nil {
			return nil, err
		}
		resolved, err := s.ResolveID("task", id)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		t, err := s.GetTask(resolved)
		if err != nil {
			return nil, err
		}
		prog, err := taskProgressJSON(s, resolved)
		if err != nil {
			return nil, err
		}
		all, err := s.ListTasks(t.ListID)
		if err != nil {
			return nil, err
		}
		children, err := descendantRows(s, all, resolved)
		if err != nil {
			return nil, err
		}
		return marshalResource(req.Params.URI, taskDetailsJSON{
			ID: t.ID, ListID: t.ListID, Title: t.Title, Notes: t.Notes,
			Status: string(t.Status), Progress: prog, CreatedAt: t.CreatedAt,
			UpdatedAt: t.UpdatedAt, CompletedAt: t.CompletedAt, Children: children,
		})
	})

	// Template: crush:///search/{query} — search results.
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "crush:///search/{query}",
		Name:        "Search",
		Description: "Fuzzy search across task titles and notes.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		query, err := templateID(req.Params.URI, "search")
		if err != nil {
			return nil, err
		}
		tasks, err := s.SearchTasks(query, nil)
		if err != nil {
			return nil, err
		}
		lists, err := s.ListLists()
		if err != nil {
			return nil, err
		}
		listNames := make(map[string]string, len(lists))
		for _, l := range lists {
			listNames[l.List.ID] = l.List.Name
		}
		out := make([]searchResultJSON, len(tasks))
		for i, t := range tasks {
			prog, err := taskProgressJSON(s, t.ID)
			if err != nil {
				return nil, err
			}
			out[i] = searchResultJSON{
				ID: t.ID, ListID: t.ListID, ListName: listNames[t.ListID],
				Title: t.Title, Status: string(t.Status), Progress: prog,
			}
		}
		return marshalResource(req.Params.URI, out)
	})
}

// addPrompts registers canned agent workflows that embed the current app
// state, so an agent that reads prompts/list can pick one and get a
// ready-made message (docs/plan/mcp-server-enhancement.md §4.2).
func addPrompts(server *mcp.Server, s *store.Store) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "crush_daily_agenda",
		Description: "Get a ready-made agent message containing the current lists and tasks, so you can triage today's work without assembling context first.",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		lists, err := s.ListLists()
		if err != nil {
			return nil, err
		}

		type taskRow struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
			Depth  int    `json:"depth"`
		}
		type listBlock struct {
			ID       string    `json:"id"`
			Name     string    `json:"name"`
			Pending  int       `json:"pending"`
			Complete int       `json:"complete"`
			Tasks    []taskRow `json:"tasks"`
		}

		blocks := make([]listBlock, 0, len(lists))
		for _, l := range lists {
			block := listBlock{
				ID: l.List.ID, Name: l.List.Name,
				Pending: l.PendingCount, Complete: l.CompleteCount,
			}
			tasks, err := s.ListTasks(l.List.ID)
			if err != nil {
				return nil, err
			}
			// Only pending/in_progress rows matter for triage; complete rows
			// are counted in Done above and would only pad the context.
			for _, r := range apptypes.Flatten(apptypes.FromStoreTasks(tasks)) {
				if r.Task.Status == apptypes.StatusComplete {
					continue
				}
				block.Tasks = append(block.Tasks, taskRow{
					ID: r.Task.ID, Title: r.Task.Title,
					Status: string(r.Task.Status), Depth: r.Depth,
				})
			}
			blocks = append(blocks, block)
		}

		b, err := json.Marshal(blocks)
		if err != nil {
			return nil, err
		}

		msg := "You are Chore Crusher's autonomous agent. Current state:\n" +
			string(b) + "\n\n" +
			"Your job: triage today's work. Create what's missing, break down what's " +
			"too big, start the next pending task (claim_work + set_progress), and " +
			"complete what's done. Call the MCP tools directly — do not narrate."

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

// templateID extracts the last path segment from a crush:/// URI after
// stripping the given prefix (e.g. "lists" from "crush:///lists/abc/tasks").
// For crush:///lists/{id}/tasks it returns "abc". For crush:///tasks/{id}
// it returns the task id segment.
func templateID(uri, prefix string) (string, error) {
	// Strip scheme: "crush:///lists/abc/tasks" -> "/lists/abc/tasks"
	path := uri
	if idx := strings.Index(path, "://"); idx >= 0 {
		path = path[idx+3:]
	}
	// Split on / and skip leading empty segment.
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid resource URI %q: expected %s/{id}", uri, prefix)
	}
	// For crush:///lists/{id}/tasks: parts = ["lists", "id", "tasks"]
	// For crush:///lists/{id}: parts = ["lists", "id"]
	// For crush:///tasks/{id}: parts = ["tasks", "id"]
	// For crush:///search/{query}: parts = ["search", "query"]
	return parts[1], nil
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
