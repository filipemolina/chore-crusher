package mcpserver_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/filipemolina/chore-crusher/src/mcpserver"
)

// setupMCP starts an in-memory MCP server backed by a fresh temporary store
// and returns the client session. The session and store are cleaned up when
// the test ends.
func setupMCP(t *testing.T) *mcp.ClientSession {
	t.Helper()

	// Keep every test isolated on its own temp data directory.
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	server, store, err := mcpserver.NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()

	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })

	return cs
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool %q: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("tool %q returned error: %s", name, textContent(t, res))
	}
	return textContent(t, res)
}

func callToolErr(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool %q: %v", name, err)
	}
	if !res.IsError {
		t.Fatalf("tool %q expected error, got: %s", name, textContent(t, res))
	}
	return textContent(t, res)
}

func textContent(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

func mustUnmarshal(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("unmarshal %q: %v", s, err)
	}
}

func TestMCPListLists(t *testing.T) {
	session := setupMCP(t)

	if got := callTool(t, session, "list_lists", nil); got != "[]" {
		t.Fatalf("list_lists = %q, want []", got)
	}

	var created map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &created)
	if created["id"] == "" {
		t.Fatalf("add_list returned empty id")
	}

	var lists []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Pending  int    `json:"pending"`
		Complete int    `json:"complete"`
	}
	mustUnmarshal(t, callTool(t, session, "list_lists", nil), &lists)
	if len(lists) != 1 || lists[0].Name != "Home" {
		t.Fatalf("list_lists = %+v, want one Home list", lists)
	}
}

func TestMCPAddAndCompleteTask(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Projects"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write tests",
	}), &task)
	if task["id"] == "" {
		t.Fatalf("add_task returned empty id")
	}

	var rows []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
		Depth  int    `json:"depth"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"],
		"status":  "pending",
	}), &rows)
	if len(rows) != 1 || rows[0].Title != "Write tests" || rows[0].Status != "pending" {
		t.Fatalf("pending rows = %+v", rows)
	}

	callTool(t, session, "complete_task", map[string]any{"id": task["id"]})

	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"],
		"status":  "complete",
	}), &rows)
	if len(rows) != 1 || rows[0].Status != "complete" {
		t.Fatalf("complete rows = %+v", rows)
	}
}

func TestMCPNestedTask(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)

	var parent map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Renovation",
	}), &parent)

	var child map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Buy paint",
		"parent":  parent["id"],
	}), &child)

	var rows []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Depth  int    `json:"depth"`
		Status string `json:"status"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"],
	}), &rows)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %+v", rows)
	}
	if rows[0].Title != "Renovation" || rows[0].Depth != 0 {
		t.Fatalf("first row = %+v", rows[0])
	}
	if rows[1].Title != "Buy paint" || rows[1].Depth != 1 {
		t.Fatalf("second row = %+v", rows[1])
	}
}

func TestMCPDeleteTaskRequiresForce(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Temporary",
	}), &task)

	callToolErr(t, session, "delete_task", map[string]any{"id": task["id"]})

	callTool(t, session, "delete_task", map[string]any{"id": task["id"], "force": true})

	var rows []struct{ Title string }
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"],
	}), &rows)
	if len(rows) != 0 {
		t.Fatalf("expected no rows after delete, got %+v", rows)
	}
}

func TestMCPSearchTasks(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Draft proposal",
		"notes":   "Include budget section",
	})

	var results []struct {
		ID       string `json:"id"`
		ListName string `json:"list_name"`
		Title    string `json:"title"`
	}
	mustUnmarshal(t, callTool(t, session, "search_tasks", map[string]any{"query": "budget"}), &results)
	if len(results) != 1 || results[0].Title != "Draft proposal" || results[0].ListName != "Work" {
		t.Fatalf("search results = %+v", results)
	}
}

func TestMCPClaimAndReleaseWork(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)

	// Claim the task.
	var claim map[string]string
	mustUnmarshal(t, callTool(t, session, "claim_work", map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "claude",
	}), &claim)
	if claim["id"] == "" {
		t.Fatalf("claim_work returned empty id")
	}

	// list_work should show it.
	var work []struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		AgentID    string `json:"agent_id"`
	}
	mustUnmarshal(t, callTool(t, session, "list_work", nil), &work)
	if len(work) != 1 || work[0].EntityID != task["id"] || work[0].AgentID != "claude" {
		t.Fatalf("list_work = %+v", work)
	}

	// Release it.
	callTool(t, session, "release_work", map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "claude",
	})

	mustUnmarshal(t, callTool(t, session, "list_work", nil), &work)
	if len(work) != 0 {
		t.Fatalf("expected empty after release, got %+v", work)
	}
}

func TestMCPClaimWorkConflict(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)

	callTool(t, session, "claim_work", map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "a1",
	})

	// Second agent should get an error.
	callToolErr(t, session, "claim_work", map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "a2",
	})

	// Original claim still holds.
	var work []struct{ AgentID string `json:"agent_id"` }
	mustUnmarshal(t, callTool(t, session, "list_work", nil), &work)
	if len(work) != 1 || work[0].AgentID != "a1" {
		t.Fatalf("expected a1 still holding, got %+v", work)
	}
}

func TestMain(m *testing.M) {
	// Tests set XDG_DATA_HOME explicitly; make sure the default HOME-based
	// path is not accidentally used when t.Setenv is active.
	os.Exit(m.Run())
}
