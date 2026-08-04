package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/config"
	"github.com/filipemolina/chore-crusher/src/constants"
	"github.com/filipemolina/chore-crusher/src/store"
)

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

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "chore-crusher",
		Version: constants.Version(),
	}, nil)

	addListTools(server, s)
	addTaskTools(server, s)
	addWorkTools(server, s)
	addWorkResource(server, s)
	addResources(server, s)

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
func addListTools(server *mcp.Server, s *store.Store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_lists",
		Description: "List every task list with pending and complete counts.",
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
		return jsonResult(out)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_list",
		Description: "Create a new task list and return its id.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		Name string `json:"name" jsonschema:"name of the new list"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.CreateList(in.Name)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]string{"id": id})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rename_list",
		Description: "Rename an existing task list.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID   string `json:"id" jsonschema:"list id or unambiguous prefix"`
		Name string `json:"name" jsonschema:"new name"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("list", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if err := s.RenameList(id, in.Name); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_list",
		Description: "Delete a task list and every task in it. Requires force=true.",
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
		if err := s.DeleteList(id); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})
}

// addTaskTools registers the task-oriented MCP tools.
func addTaskTools(server *mcp.Server, s *store.Store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List the tasks in one list as a depth-annotated tree. Use status=all (default), pending, in_progress, or complete.",
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
		Description: "Add a task to a list. Optionally provide a parent task id to nest it, and notes.",
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
		Description: "Show a task's details, including its children as depth-annotated rows.",
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

		children, err := taskRows(s, apptypes.Flatten(apptypes.FromStoreTasks(descendantsOf(all, id))))
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
		Description: "Mark a task complete. This cascades to all descendants.",
	}, taskMutator(s, func(id string) error { return s.Complete(id) }))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reopen_task",
		Description: "Reopen a task (mark pending). This does not cascade to children.",
	}, taskMutator(s, func(id string) error { return s.Reopen(id) }))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "toggle_task",
		Description: "Toggle a task between complete and pending.",
	}, taskMutator(s, func(id string) error { return s.Toggle(id) }))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_task",
		Description: "Delete a task and its descendants. Requires force=true.",
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
		if err := s.DeleteTask(id); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rename_task",
		Description: "Rename a task.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID    string `json:"id" jsonschema:"task id or unambiguous prefix"`
		Title string `json:"title" jsonschema:"new title"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("task", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if err := s.RenameTask(id, in.Title); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_notes",
		Description: "Replace a task's notes (whole text, not append). Pass an empty string to clear notes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID    string `json:"id" jsonschema:"task id or unambiguous prefix"`
		Notes string `json:"notes" jsonschema:"replacement notes"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("task", in.ID)
		if err != nil {
			return errorResult(err), nil, nil
		}
		if err := s.SetNotes(id, in.Notes); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_progress",
		Description: "Set a task's progress mode: simple, subtasks, or percentage. Percent is required only for percentage.",
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
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "move_task",
		Description: "Re-parent a task. Omit parent (or pass empty string) to move it to the list root.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID     string `json:"id" jsonschema:"task id or unambiguous prefix"`
		Parent string `json:"parent,omitempty" jsonschema:"new parent task id or prefix; omit for root"`
	}) (*mcp.CallToolResult, any, error) {
		id, err := s.ResolveID("task", in.ID)
		if err != nil {
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
		if err := s.Reparent(id, parentID); err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(map[string]bool{"ok": true})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_tasks",
		Description: "Fuzzy search task titles and notes across all lists, or within one list if list_id is given. Title matches rank before notes-only matches.",
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
func taskMutator(s *store.Store, fn func(string) error) func(context.Context, *mcp.CallToolRequest, struct {
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
		return jsonResult(map[string]bool{"ok": true})
	}
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

// taskRows converts flattened apptypes rows into the JSON row shape,
// computing derived progress per row.
func taskRows(s *store.Store, rows []apptypes.Row) ([]taskRowJSON, error) {
	out := make([]taskRowJSON, len(rows))
	for i, r := range rows {
		prog, err := taskProgressJSON(s, r.Task.ID)
		if err != nil {
			return nil, err
		}
		out[i] = taskRowJSON{
			ID:       r.Task.ID,
			ParentID: r.Task.ParentID,
			Title:    r.Task.Title,
			Status:   string(r.Task.Status),
			Progress: prog,
			Depth:    r.Depth,
		}
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

// descendantsOf filters one list's flat tasks down to a task's descendants,
// returning them in the same preorder the CLI uses for show's children.
func descendantsOf(tasks []store.Task, rootID string) []store.Task {
	children := make(map[string][]store.Task)
	for _, t := range tasks {
		if t.ParentID != nil {
			children[*t.ParentID] = append(children[*t.ParentID], t)
		}
	}

	var out []store.Task
	var walk func(id string)
	walk = func(id string) {
		for _, c := range children[id] {
			out = append(out, c)
			walk(c.ID)
		}
	}
	walk(rootID)
	return out
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
func addWorkTools(server *mcp.Server, s *store.Store) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "claim_work",
		Description: "Claim a task or list as being worked on by an agent. The TUI " +
			"shows a live spinner on the row while the claim is active. Re-claiming " +
			"by the same agent refreshes the timer (heartbeat). A different agent " +
			"holding the entity returns an error. entity_type is \"task\" or \"list\".",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		EntityType string `json:"entity_type"` // "task" or "list"
		EntityID   string `json:"entity_id"`   // task or list id, or unambiguous prefix
		AgentID    string `json:"agent_id,omitempty"` // short label; default "agent"
		Kind       string `json:"kind,omitempty"`     // "working" or "inspecting"; default "working"
	}) (*mcp.CallToolResult, any, error) {
		if in.EntityType != "task" && in.EntityType != "list" {
			return errorResult(fmt.Errorf("entity_type must be \"task\" or \"list\", got %q", in.EntityType)), nil, nil
		}
		if in.AgentID == "" {
			in.AgentID = "agent"
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
		EntityType string `json:"entity_type"` // "task" or "list"
		EntityID   string `json:"entity_id"`   // task or list id, or unambiguous prefix
		AgentID    string `json:"agent_id,omitempty"` // default "agent"
	}) (*mcp.CallToolResult, any, error) {
		if in.EntityType != "task" && in.EntityType != "list" {
			return errorResult(fmt.Errorf("entity_type must be \"task\" or \"list\", got %q", in.EntityType)), nil, nil
		}
		if in.AgentID == "" {
			in.AgentID = "agent"
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
		children, err := taskRows(s, apptypes.Flatten(apptypes.FromStoreTasks(descendantsOf(all, resolved))))
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
