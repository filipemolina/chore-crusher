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
	"github.com/filipemolina/chore-crusher/src/store"
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

// TestMCPMyList guards S3: my_list auto-provisions the agent's own list
// (named after the CRUSH_AGENT tag) so the agent never has to run an
// add_list + copy-id dance before writing its first task. Idempotent: a
// second call returns the same list without creating a duplicate.
func TestMCPMyList(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "pi")
	session := setupMCP(t)

	// First call creates the agent's list.
	var first map[string]any
	mustUnmarshal(t, callTool(t, session, "my_list", nil), &first)
	if first["id"] == "" || first["name"] != "pi: Inbox" {
		t.Fatalf("my_list (first) = %+v, want id set and name \"pi: Inbox\"", first)
	}

	// Counts come along: 0 pending / 0 complete on a fresh list.
	if first["pending"] != float64(0) || first["complete"] != float64(0) {
		t.Fatalf("my_list counts = pending=%v complete=%v, want 0/0", first["pending"], first["complete"])
	}

	// Second call returns the same list (idempotent — no duplicate created).
	var second map[string]any
	mustUnmarshal(t, callTool(t, session, "my_list", nil), &second)
	if second["id"] != first["id"] {
		t.Fatalf("my_list not idempotent: %v != %v", second["id"], first["id"])
	}

	// list_lists sees exactly one list, matching my_list's id.
	var lists []map[string]any
	mustUnmarshal(t, callTool(t, session, "list_lists", nil), &lists)
	if len(lists) != 1 || lists[0]["id"] != first["id"] {
		t.Fatalf("list_lists = %+v, want one list matching my_list", lists)
	}
}

// TestMCPInstructionsUsesPrefixedToolNames guards S2: the QUICK REFERENCE in
// the Instructions doc the agent reads at session start must list tools with the
// host-registered chore_crusher_<name> prefix. An unprefixed name (the original
// bug) makes the agent's first call fail to resolve.
func TestMCPInstructionsUsesPrefixedToolNames(t *testing.T) {
	session := setupMCP(t)

	instructions := session.InitializeResult().Instructions
	if instructions == "" {
		t.Fatal("Instructions is empty")
	}

	lines := strings.Split(instructions, "\n")
	inQuickRef := false
	found := 0
	var unprefixed []string
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "QUICK REFERENCE"):
			inQuickRef = true
			continue
		case inQuickRef && strings.TrimSpace(line) == "":
			// blank line ends the QUICK REFERENCE block
			inQuickRef = false
			continue
		case inQuickRef:
			tool := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if tool == "" {
				continue
			}
			found++
			name := tool
			if i := strings.Index(name, "("); i >= 0 {
				name = name[:i]
			}
			if !strings.HasPrefix(name, "chore_crusher_") {
				unprefixed = append(unprefixed, tool)
			}
		}
	}
	if found == 0 {
		t.Fatal("no tools found in Instructions QUICK REFERENCE block")
	}
	if len(unprefixed) > 0 {
		t.Fatalf("Instructions QUICK REFERENCE lists unprefixed tool names (the agent would fail to call them): %v", unprefixed)
	}
}

// TestMCPInstructionsUsesIdentity guards H1: the Instructions doc must name
// the configured CRUSH_AGENT, not a hardcoded "pi" — an agent running under
// another tag is otherwise told to use pi: lists it does not own.
func TestMCPInstructionsUsesIdentity(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "claude")
	session := setupMCP(t)

	instructions := session.InitializeResult().Instructions
	if instructions == "" {
		t.Fatal("Instructions is empty")
	}
	if !strings.Contains(instructions, "claude:") {
		t.Fatalf("Instructions must name the configured identity (claude);\nfull text:\n%s", instructions)
	}
	if strings.Contains(instructions, "pi:") {
		t.Fatalf("Instructions must not hardcode pi: when CRUSH_AGENT=claude;\nfull text:\n%s", instructions)
	}
}

// TestMCPInstructionsAlwaysOnTodoRule guards S1: the Instructions doc the agent
// reads at session start must steer it away from the host's built-in todo tool
// and toward a pi:-owned list. Without this line the agent's base AGENTS.md
// wins and it tracks work in the wrong (non-Chore-Crusher) store.
func TestMCPInstructionsAlwaysOnTodoRule(t *testing.T) {
	// The doc names the configured identity, so pin it to the agent this test
	// expects (the default would be "agent").
	t.Setenv("CRUSH_AGENT", "pi")
	session := setupMCP(t)

	instructions := session.InitializeResult().Instructions
	if instructions == "" {
		t.Fatal("Instructions is empty")
	}

	lower := strings.ToLower(instructions)
	for _, want := range []string{
		// The explicit, verbatim rule from S1.
		"do not use the host's built-in todo tool",
		// Actionable guidance: where the agent should actually track its work.
		"pi: ",
		"chore_crusher_add_list",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("Instructions missing always-on todo rule element %q;\nfull text:\n%s", want, instructions)
		}
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

// TestMCPShowTaskIncludesChildren guards S6: show_task must return a task's
// descendants in its "children" field. The old code flattened descendantsOf()
// through apptypes.Flatten, which only emits ParentID==nil rows, so a
// pure-descendant set flattened to nothing and "children" was always [].
func TestMCPShowTaskIncludesChildren(t *testing.T) {
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

	// A grandchild to confirm the whole subtree is returned, not just
	// direct children.
	var grand map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Pick colour",
		"parent":  child["id"],
	}), &grand)

	var details struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Children []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Depth  int    `json:"depth"`
			Status string `json:"status"`
		} `json:"children"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"task_id": parent["id"],
	}), &details)

	if details.ID != parent["id"] || details.Title != "Renovation" {
		t.Fatalf("show_task root = %+v", details)
	}
	if len(details.Children) != 2 {
		t.Fatalf("want 2 children (child + grandchild), got %+v", details.Children)
	}
	byID := make(map[string]int)
	for _, c := range details.Children {
		byID[c.ID] = c.Depth
	}
	if byID[child["id"]] != 1 {
		t.Fatalf("child depth = %d, want 1", byID[child["id"]])
	}
	if byID[grand["id"]] != 2 {
		t.Fatalf("grandchild depth = %d, want 2", byID[grand["id"]])
	}

	// The crush:///tasks/{id} resource shares the same code path and must
	// also return the children.
	res, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "crush:///tasks/" + parent["id"],
	})
	if err != nil {
		t.Fatalf("ReadResource crush:///tasks/%s: %v", parent["id"], err)
	}
	var resDetails struct {
		Children []struct {
			ID string `json:"id"`
		} `json:"children"`
	}
	mustUnmarshal(t, res.Contents[0].Text, &resDetails)
	if len(resDetails.Children) != 2 {
		t.Fatalf("resource children = %+v, want 2", resDetails.Children)
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

// TestMCPClaimDefaultsToIdentity pins §4.2 / §6 assertion 1: claim_work
// without agent_id claims under the server identity (CRUSH_AGENT), so the
// write-heartbeat in complete_task still refreshes the spinner. Before the
// fix the claim landed under "agent" and TouchWork (identity "pi") never
// matched it.
func TestMCPClaimDefaultsToIdentity(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "pi")
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)

	// No agent_id: the claim must be stored under the server identity (pi),
	// not the literal default "agent".
	callTool(t, session, "claim_work", map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
	})

	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	var agentID string
	if err := db.QueryRow(
		`SELECT agent_id FROM AgentActivity WHERE entity_type = 'task' AND entity_id = ?`,
		task["id"],
	).Scan(&agentID); err != nil {
		t.Fatalf("read agent_id: %v", err)
	}
	if agentID != "pi" {
		t.Fatalf("claim without agent_id must default to the server identity, got %q", agentID)
	}

	// Age the claim inside the TTL window; complete_task (a write-heartbeat)
	// must push it forward — only possible when the claim's agent matches.
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
	if after <= aged {
		t.Fatalf("complete_task must refresh a claim made without agent_id (acquired_at %d, want > %d)", after, aged)
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

	var created map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &created)

	res, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "crush:///lists",
	})
	if err != nil {
		t.Fatalf("ReadResource crush:///lists: %v", err)
	}

	var lists []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		CreatedBy string `json:"created_by"`
	}
	mustUnmarshal(t, res.Contents[0].Text, &lists)
	if len(lists) != 1 || lists[0].Name != "Home" {
		t.Fatalf("crush:///lists = %+v", lists)
	}

	// H5: the resource row must carry created_by, matching list_lists — no
	// CRUSH_AGENT is set here, so the default identity is "agent".
	if lists[0].CreatedBy != "agent" {
		t.Fatalf("crush:///lists created_by = %q, want the server identity (agent)", lists[0].CreatedBy)
	}
	var toolLists []struct {
		ID        string `json:"id"`
		CreatedBy string `json:"created_by"`
	}
	mustUnmarshal(t, callTool(t, session, "list_lists", nil), &toolLists)
	if len(toolLists) != 1 || toolLists[0].CreatedBy != lists[0].CreatedBy {
		t.Fatalf("list_lists and crush:///lists diverged on created_by: tool=%+v resource=%+v", toolLists, lists)
	}

	// The single-list resource carries the same owner.
	res, err = session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "crush:///lists/" + created["id"],
	})
	if err != nil {
		t.Fatalf("ReadResource crush:///lists/{id}: %v", err)
	}
	var one struct {
		ID        string `json:"id"`
		CreatedBy string `json:"created_by"`
	}
	mustUnmarshal(t, res.Contents[0].Text, &one)
	if one.ID != created["id"] || one.CreatedBy != "agent" {
		t.Fatalf("crush:///lists/{id} = %+v, want id %q owned by agent", one, created["id"])
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

// TestMCPForeignListWriteRefused guards §4.D / §5 assertion 2: every
// structural write tool (add_task, rename_task, set_notes, move_task,
// delete_task, rename_list, delete_list) errors on a list owned by another
// agent — not just one of them. The server here acts as "pi"; the list is
// created as "claude".
func TestMCPForeignListWriteRefused(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "pi")
	session := setupMCP(t)

	// A list owned by claude (created explicitly as claude via the enforced
	// add_list surface). The server here is "pi", so every structural write
	// to it must be refused.
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{
		"name": "claude: Backlog", "created_by": "claude",
	}), &list)

	// add_task on a foreign list is refused.
	msg := callToolErr(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "intrude",
	})
	if !strings.Contains(msg, "owned by claude") {
		t.Fatalf("add_task foreign-list error = %q, want it to name the owner", msg)
	}

	// Seed a task on claude's list (the CLI shape) so we can probe the
	// task-content tools. The server identity cannot be switched after
	// NewServer, so seed directly via the shared DB.
	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	taskID := store.NewID()
	now := time.Now().Unix()
	if _, err := db.Exec(
		`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind, progress_pct, position, created_at, updated_at, completed_at)
		 VALUES (?, ?, NULL, 'real work', '', 'pending', 'none', NULL, 0, ?, ?, NULL)`,
		taskID, list["id"], now, now,
	); err != nil {
		t.Fatalf("seed task on claude's list: %v", err)
	}

	for _, tc := range []struct {
		name string
		tool string
		args map[string]any
	}{
		{"rename_task", "rename_task", map[string]any{"id": taskID, "title": "hijack"}},
		{"set_notes", "set_notes", map[string]any{"id": taskID, "notes": "tamper"}},
		{"delete_task", "delete_task", map[string]any{"id": taskID, "force": true}},
		{"rename_list", "rename_list", map[string]any{"id": list["id"], "name": "claude: Hijacked"}},
		{"delete_list", "delete_list", map[string]any{"id": list["id"], "force": true}},
	} {
		msg := callToolErr(t, session, tc.tool, tc.args)
		if !strings.Contains(msg, "owned by claude") {
			t.Fatalf("%s foreign-list error = %q, want it to name the owner", tc.name, msg)
		}
	}

	// move_task to the foreign list is refused before any write.
	msg = callToolErr(t, session, "move_task", map[string]any{
		"id": taskID, "parent": "",
	})
	if !strings.Contains(msg, "owned by claude") {
		t.Fatalf("move_task foreign-list error = %q, want it to name the owner", msg)
	}

	// The task was never moved/deleted: claude's list still has exactly one
	// task, proving no structural write slipped through.
	var rows []struct{ Title string }
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"],
	}), &rows)
	if len(rows) != 1 || rows[0].Title != "real work" {
		t.Fatalf("foreign-list structural writes leaked: rows = %+v", rows)
	}
}

// TestMCPOwnerCanWriteEverything guards that the owner of a list can run every
// structural tool on it (the mirror of TestMCPForeignListWriteRefused).
func TestMCPOwnerCanWriteEverything(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "claude")
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{
		"name": "claude: Backlog", "created_by": "claude",
	}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "real work",
	}), &task)

	callTool(t, session, "rename_task", map[string]any{"id": task["id"], "title": "renamed"})
	callTool(t, session, "set_notes", map[string]any{"id": task["id"], "notes": "annotated"})
	callTool(t, session, "rename_list", map[string]any{"id": list["id"], "name": "claude: Renamed"})

	// move_task within the owner's own list (to root) succeeds.
	callTool(t, session, "move_task", map[string]any{"id": task["id"]})

	// delete_task requires force and succeeds for the owner.
	callTool(t, session, "delete_task", map[string]any{"id": task["id"], "force": true})
	var rows []struct{ Title string }
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"],
	}), &rows)
	if len(rows) != 0 {
		t.Fatalf("owner delete_task left rows = %+v", rows)
	}

	callTool(t, session, "delete_list", map[string]any{"id": list["id"], "force": true})
	if got := callTool(t, session, "list_lists", nil); got != "[]" {
		t.Fatalf("list_lists = %q, want [] after owner deletes the list", got)
	}
}

// TestMCPStatusToolsOpenOnForeignList guards §5 assertion 3: status/progress
// tools are never gated, so complete_task / set_progress succeed on a list
// owned by another agent.
func TestMCPStatusToolsOpenOnForeignList(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "pi")
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{
		"name": "claude: Backlog", "created_by": "claude",
	}), &list)

	// Seed a task on claude's list (CLI shape); the server is "pi" so an MCP
	// add_task would be refused — seed directly via the shared DB.
	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	taskID := store.NewID()
	now := time.Now().Unix()
	if _, err := db.Exec(
		`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind, progress_pct, position, created_at, updated_at, completed_at)
		 VALUES (?, ?, NULL, 'needs status', '', 'pending', 'none', NULL, 0, ?, ?, NULL)`,
		taskID, list["id"], now, now,
	); err != nil {
		t.Fatalf("seed task on claude's list: %v", err)
	}

	// pi (foreign) may still flip status/progress.
	callTool(t, session, "set_progress", map[string]any{"id": taskID, "mode": "simple"})
	callTool(t, session, "complete_task", map[string]any{"id": taskID})

	var rows []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"], "status": "complete",
	}), &rows)
	if len(rows) != 1 || rows[0].Status != "complete" {
		t.Fatalf("foreign status write did not take: rows = %+v", rows)
	}
}

// TestMCPUntaggedListForeignToEveryAgent guards §5 assertion 4: a list with
// no owner (created the human/CLI way, created_by="") is foreign to *every*
// agent identity — structural writes error for all of them, yet
// status/progress writes succeed. MCP's add_list defaults the owner to the
// server identity, so an untagged list cannot be produced through the
// enforced surface; this test seeds it directly (the CLI shape) via the same
// DB the MCP server opened.
func TestMCPUntaggedListForeignToEveryAgent(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "pi")
	session := setupMCP(t)

	// Seed an untagged list + a task, mirroring the CLI/TUI (created_by="").
	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	listID := store.NewID()
	taskID := store.NewID()
	now := time.Now().Unix()
	if _, err := db.Exec(
		`INSERT INTO List (id, name, created_at, position, created_by) VALUES (?, ?, ?, 0, "")`,
		listID, "Groceries", now,
	); err != nil {
		t.Fatalf("seed untagged list: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind, progress_pct, position, created_at, updated_at, completed_at)
		 VALUES (?, ?, NULL, 'buy milk', '', 'pending', 'none', NULL, 0, ?, ?, NULL)`,
		taskID, listID, now, now,
	); err != nil {
		t.Fatalf("seed task on untagged list: %v", err)
	}

	// pi is refused from structurally writing to the untagged list.
	msg := callToolErr(t, session, "add_task", map[string]any{
		"list_id": listID, "title": "sneak in",
	})
	if !strings.Contains(msg, "no one (untagged)") {
		t.Fatalf("add_task untagged-list error = %q, want it to say untagged", msg)
	}
	msg = callToolErr(t, session, "rename_list", map[string]any{
		"id": listID, "name": "Renamed",
	})
	if !strings.Contains(msg, "no one (untagged)") {
		t.Fatalf("rename_list untagged-list error = %q, want it to say untagged", msg)
	}
	msg = callToolErr(t, session, "delete_list", map[string]any{
		"id": listID, "force": true,
	})
	if !strings.Contains(msg, "no one (untagged)") {
		t.Fatalf("delete_list untagged-list error = %q, want it to say untagged", msg)
	}

	// Yet status/progress writes succeed on the untagged list's task — the
	// read + status/progress-only rule holds for *every* identity.
	callTool(t, session, "set_progress", map[string]any{"id": taskID, "mode": "simple"})
	callTool(t, session, "complete_task", map[string]any{"id": taskID})

	var rows []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": listID, "status": "complete",
	}), &rows)
	if len(rows) != 1 || rows[0].ID != taskID || rows[0].Status != "complete" {
		t.Fatalf("untagged-list status write did not take: rows = %+v", rows)
	}
}

// TestMCPAddListDefaultsToIdentity guards §5 assertion 5 (default owner):
// add_list with no created_by yields a list_lists entry whose created_by is
// the server identity.
func TestMCPAddListDefaultsToIdentity(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "pi")
	session := setupMCP(t)

	var created map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{
		"name": "My list",
	}), &created)

	var lists []struct {
		ID        string `json:"id"`
		CreatedBy string `json:"created_by"`
	}
	mustUnmarshal(t, callTool(t, session, "list_lists", nil), &lists)
	if len(lists) != 1 || lists[0].ID != created["id"] || lists[0].CreatedBy != "pi" {
		t.Fatalf("list_lists = %+v, want created_by=pi for the new list", lists)
	}
}

// TestMCPListListsIncludesCreatedBy guards §5 assertion 5: list_lists reports
// the owner of each list.
func TestMCPListListsIncludesCreatedBy(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "pi")
	session := setupMCP(t)

	var owned map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{
		"name": "mine", "created_by": "pi",
	}), &owned)
	var theirs map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{
		"name": "theirs", "created_by": "claude",
	}), &theirs)

	// An untagged list (created_by="") cannot be produced through the
	// enforced MCP surface — add_list defaults the owner to the identity —
	// so seed it directly (the CLI shape) via the shared DB.
	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	untaggedID := store.NewID()
	now := time.Now().Unix()
	if _, err := db.Exec(
		`INSERT INTO List (id, name, created_at, position, created_by) VALUES (?, ?, ?, 0, "")`,
		untaggedID, "shared", now,
	); err != nil {
		t.Fatalf("seed untagged list: %v", err)
	}

	byID := make(map[string]string)
	var lists []struct {
		ID        string `json:"id"`
		CreatedBy string `json:"created_by"`
	}
	mustUnmarshal(t, callTool(t, session, "list_lists", nil), &lists)
	for _, l := range lists {
		byID[l.ID] = l.CreatedBy
	}
	if byID[owned["id"]] != "pi" {
		t.Fatalf("owned list created_by = %q, want pi", byID[owned["id"]])
	}
	if byID[theirs["id"]] != "claude" {
		t.Fatalf("their list created_by = %q, want claude", byID[theirs["id"]])
	}
	if byID[untaggedID] != "" {
		t.Fatalf("untagged list created_by = %q, want empty", byID[untaggedID])
	}
}

// TestMCPAddListRejectsBadTag guards §4.C: an explicit created_by that fails
// the tag pattern is rejected.
func TestMCPAddListRejectsBadTag(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "pi")
	session := setupMCP(t)

	msg := callToolErr(t, session, "add_list", map[string]any{
		"name": "x", "created_by": "p i",
	})
	if !strings.Contains(msg, "created_by must match") {
		t.Fatalf("add_list bad tag error = %q, want the pattern error", msg)
	}
}

func TestMain(m *testing.M) {
	// Tests set XDG_DATA_HOME explicitly; make sure the default HOME-based
	// path is not accidentally used when t.Setenv is active.
	os.Exit(m.Run())
}
