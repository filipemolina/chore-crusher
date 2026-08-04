package mcpserver_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/filipemolina/chore-crusher/src/config"
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
	var work []struct {
		AgentID string `json:"agent_id"`
	}
	mustUnmarshal(t, callTool(t, session, "list_work", nil), &work)
	if len(work) != 1 || work[0].AgentID != "a1" {
		t.Fatalf("expected a1 still holding, got %+v", work)
	}
}

func TestMCPStatusWritesRefreshClaim(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "pi")
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)

	// Claim as the server identity (CRUSH_AGENT=pi).
	callTool(t, session, "claim_work", map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "pi",
	})

	// Age the claim through a second connection to the same store file.
	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	for _, tc := range []struct {
		name string
		tool string
		args map[string]any
	}{
		{"set_progress", "set_progress", map[string]any{"id": task["id"], "mode": "simple"}},
		{"complete_task", "complete_task", map[string]any{"id": task["id"]}},
	} {
		// Age the claim inside the TTL window; a status write must push it forward.
		aged := time.Now().Add(-30 * time.Second).Unix()
		if _, err := db.Exec(
			`UPDATE AgentActivity SET acquired_at = ? WHERE entity_type = 'task' AND entity_id = ?`,
			aged, task["id"],
		); err != nil {
			t.Fatalf("%s: age claim: %v", tc.name, err)
		}

		callTool(t, session, tc.tool, tc.args)

		var after int64
		if err := db.QueryRow(
			`SELECT acquired_at FROM AgentActivity WHERE entity_type = 'task' AND entity_id = ?`,
			task["id"],
		).Scan(&after); err != nil {
			t.Fatalf("%s: read acquired_at: %v", tc.name, err)
		}
		if after <= aged {
			t.Fatalf("%s: status write must refresh the claim (acquired_at %d, want > %d)", tc.name, after, aged)
		}
	}
}

func TestMCPStatusWritesDoNotTouchForeignClaims(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "pi")
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)

	// Another agent holds the claim; pi's writes must not touch it.
	callTool(t, session, "claim_work", map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "other",
	})

	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	aged := time.Now().Add(-30 * time.Second).Unix()
	if _, err := db.Exec(
		`UPDATE AgentActivity SET acquired_at = ? WHERE entity_type = 'task' AND entity_id = ?`,
		aged, task["id"],
	); err != nil {
		t.Fatalf("age claim: %v", err)
	}

	callTool(t, session, "complete_task", map[string]any{"id": task["id"]})

	var after int64
	if err := db.QueryRow(
		`SELECT acquired_at FROM AgentActivity WHERE entity_type = 'task' AND entity_id = ?`,
		task["id"],
	).Scan(&after); err != nil {
		t.Fatalf("read acquired_at: %v", err)
	}
	if after != aged {
		t.Fatalf("pi's write must not refresh another agent's claim (acquired_at %d, want %d)", after, aged)
	}
}

func TestMCPWorkResource(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)

	// Claim and verify crush://work resource matches list_work.
	callTool(t, session, "claim_work", map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "claude",
	})

	// Read the resource.
	res, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "crush://work",
	})
	if err != nil {
		t.Fatalf("ReadResource crush://work: %v", err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(res.Contents))
	}
	if res.Contents[0].MIMEType != "application/json" {
		t.Fatalf("MIMEType = %q, want application/json", res.Contents[0].MIMEType)
	}

	var resourceWork []struct {
		EntityID string `json:"entity_id"`
		AgentID  string `json:"agent_id"`
	}
	mustUnmarshal(t, res.Contents[0].Text, &resourceWork)
	if len(resourceWork) != 1 || resourceWork[0].EntityID != task["id"] || resourceWork[0].AgentID != "claude" {
		t.Fatalf("crush://work = %+v", resourceWork)
	}

	// Compare with list_work tool.
	var toolWork []struct {
		EntityID string `json:"entity_id"`
		AgentID  string `json:"agent_id"`
	}
	mustUnmarshal(t, callTool(t, session, "list_work", nil), &toolWork)
	if len(toolWork) != 1 || toolWork[0].EntityID != resourceWork[0].EntityID {
		t.Fatalf("list_work and crush://work diverged: tool=%+v resource=%+v", toolWork, resourceWork)
	}
}

func TestMCPListsResource(t *testing.T) {
	session := setupMCP(t)

	callTool(t, session, "add_list", map[string]any{"name": "Home"})

	res, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "crush:///lists",
	})
	if err != nil {
		t.Fatalf("ReadResource crush:///lists: %v", err)
	}

	var lists []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	mustUnmarshal(t, res.Contents[0].Text, &lists)
	if len(lists) != 1 || lists[0].Name != "Home" {
		t.Fatalf("crush:///lists = %+v", lists)
	}
}

func TestMCPTaskResource(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)

	res, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "crush:///tasks/" + task["id"],
	})
	if err != nil {
		t.Fatalf("ReadResource crush:///tasks/%s: %v", task["id"], err)
	}

	var details struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	mustUnmarshal(t, res.Contents[0].Text, &details)
	if details.ID != task["id"] || details.Title != "Write docs" {
		t.Fatalf("crush:///tasks = %+v", details)
	}
}

func TestMCPSearchResource(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Draft proposal",
	})

	res, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "crush:///search/proposal",
	})
	if err != nil {
		t.Fatalf("ReadResource crush:///search/proposal: %v", err)
	}

	var results []struct {
		Title string `json:"title"`
	}
	mustUnmarshal(t, res.Contents[0].Text, &results)
	if len(results) != 1 || results[0].Title != "Draft proposal" {
		t.Fatalf("crush:///search/proposal = %+v", results)
	}
}

func TestMCPResourcesListed(t *testing.T) {
	session := setupMCP(t)

	// The MCP server should advertise resources and resource templates.
	res, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources(): %v", err)
	}
	if len(res.Resources) == 0 {
		t.Fatalf("expected at least one resource, got %d", len(res.Resources))
	}
	found := false
	for _, r := range res.Resources {
		if r.URI == "crush:///lists" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("crush:///lists not in resources: %+v", res.Resources)
	}

	tres, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates(): %v", err)
	}
	if len(tres.ResourceTemplates) == 0 {
		t.Fatalf("expected at least one resource template, got %d", len(tres.ResourceTemplates))
	}
	found = false
	for _, rt := range tres.ResourceTemplates {
		if rt.URITemplate == "crush:///tasks/{id}" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("crush:///tasks/{id} not in templates: %+v", tres.ResourceTemplates)
	}
}

func TestMCPPromptsListed(t *testing.T) {
	session := setupMCP(t)

	res, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListPrompts(): %v", err)
	}
	if len(res.Prompts) == 0 {
		t.Fatalf("expected at least one prompt, got %d", len(res.Prompts))
	}
	found := make(map[string]bool, len(res.Prompts))
	for _, p := range res.Prompts {
		found[p.Name] = true
	}
	for _, name := range []string{"crush_daily_agenda", "crush_breakdown"} {
		if !found[name] {
			t.Fatalf("prompt %q not listed: %+v", name, res.Prompts)
		}
	}
}

func TestMCPDailyAgendaPrompt(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)

	callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Buy groceries",
	})

	res, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name: "crush_daily_agenda",
	})
	if err != nil {
		t.Fatalf("GetPrompt crush_daily_agenda: %v", err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(res.Messages))
	}
	if res.Messages[0].Role != "user" {
		t.Fatalf("role = %q, want user", res.Messages[0].Role)
	}
	tc, ok := res.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", res.Messages[0].Content)
	}
	if !strings.Contains(tc.Text, "Home") || !strings.Contains(tc.Text, "Buy groceries") {
		t.Fatalf("daily agenda text missing list/task: %s", tc.Text)
	}
	if !strings.Contains(tc.Text, "claim_work") {
		t.Fatalf("daily agenda text missing tool guidance: %s", tc.Text)
	}
}

func TestMCPBreakdownPrompt(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Ship the project",
		"notes":   "needs a README and CI",
	}), &task)

	res, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      "crush_breakdown",
		Arguments: map[string]string{"task_id": task["id"]},
	})
	if err != nil {
		t.Fatalf("GetPrompt crush_breakdown: %v", err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(res.Messages))
	}
	tc, ok := res.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", res.Messages[0].Content)
	}
	if !strings.Contains(tc.Text, "Ship the project") || !strings.Contains(tc.Text, "README") {
		t.Fatalf("breakdown text missing task/notes: %s", tc.Text)
	}
}

func TestMain(m *testing.M) {
	// Tests set XDG_DATA_HOME explicitly; make sure the default HOME-based
	// path is not accidentally used when t.Setenv is active.
	os.Exit(m.Run())
}
