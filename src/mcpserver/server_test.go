package mcpserver_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

// setupMCPAs is setupMCP with a fixed server identity: the server reads
// CRUSH_AGENT at NewServer, so the helper sets it before setupMCP opens one.
func setupMCPAs(t *testing.T, identity string) *mcp.ClientSession {
	t.Helper()
	t.Setenv("CRUSH_AGENT", identity)
	return setupMCP(t)
}

// sessionAs connects a new server+client pair under the given identity,
// pointed at dataDir — the caller sets XDG_DATA_HOME to dataDir first, so
// multiple sessions from this helper share one store (the same pattern
// TestInboxResourceReturnsMineAndForeign builds inline for a human+pi pair).
// Used where a test needs two distinct CRUSH_AGENT identities acting on the
// same data, e.g. the delete_comment cross-author refusal.
func sessionAs(t *testing.T, dataDir, identity string) *mcp.ClientSession {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", dataDir)
	t.Setenv("CRUSH_AGENT", identity)

	server, st, err := mcpserver.NewServer()
	if err != nil {
		t.Fatalf("NewServer (%s): %v", identity, err)
	}
	t.Cleanup(func() { st.Close() })

	ctx := context.Background()
	ct, transport := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("server.Connect (%s): %v", identity, err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client.Connect (%s): %v", identity, err)
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

// readResourceText reads a resource and returns its single text body. Only
// crush:///inbox and crush://work remain (docs/plan/mcp-assignment-and-priorities.md
// §8); crush://work still backfills the removed list_work tool
// (docs/plan/mcp-tool-consolidation.md §4.5).
func readResourceText(t *testing.T, session *mcp.ClientSession, uri string) string {
	t.Helper()
	res, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource %s: %v", uri, err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("ReadResource %s: expected 1 content, got %d", uri, len(res.Contents))
	}
	return res.Contents[0].Text
}

// listsJSON replaces the removed list_lists tool. It used to read the
// crush:///lists resource; that resource was deleted as a duplicate of my_list
// (docs/plan/mcp-assignment-and-priorities.md §8), so the helper now calls
// my_list and flattens {mine, foreign_lists} back into the single row array
// its callers assert against. identity fills in created_by for the agent's own
// block, which my_list omits because it is the caller's own tag by
// construction.
func listsJSON(t *testing.T, session *mcp.ClientSession, identity string) string {
	t.Helper()
	var out struct {
		Mine    map[string]any   `json:"mine"`
		Foreign []map[string]any `json:"foreign_lists"`
	}
	mustUnmarshal(t, callTool(t, session, "my_list", nil), &out)

	rows := make([]map[string]any, 0, len(out.Foreign)+1)
	if id, _ := out.Mine["id"].(string); id != "" {
		out.Mine["created_by"] = identity
		rows = append(rows, out.Mine)
	}
	rows = append(rows, out.Foreign...)

	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("marshal lists rows: %v", err)
	}
	return string(b)
}

// workJSON replaces the removed list_work tool: the crush://work resource
// serves the identical row shape.
func workJSON(t *testing.T, session *mcp.ClientSession) string {
	t.Helper()
	return readResourceText(t, session, "crush://work")
}

// releaseWork replaces the removed release_work tool with the merged
// claim_work(release=true) form.
func releaseWork(t *testing.T, session *mcp.ClientSession, args map[string]any) string {
	t.Helper()
	with := map[string]any{"release": true}
	for k, v := range args {
		with[k] = v
	}
	return callTool(t, session, "claim_work", with)
}

// TestMCPToolSurface pins the consolidated tool surface
// (docs/plan/mcp-tool-consolidation.md §2): exactly the 15 tools below, and
// none of the removed ones. A new tool must be a deliberate edit here — the
// ceiling is the point of the plan.
func TestMCPToolSurface(t *testing.T) {
	session := setupMCP(t)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}

	want := []string{
		"my_list", "list_tasks", "show_task", "search_tasks",
		"add_task", "edit_task", "delete_task", "set_progress", "complete_task",
		"reopen_task", "add_comment", "delete_comment", "add_list", "claim_work",
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("tool %q missing from the surface", name)
		}
	}
	for _, name := range []string{
		"list_lists", "show_tasks", "toggle_task", "update_tasks", "rename_task",
		"set_notes", "move_task", "rename_list", "delete_list", "release_work",
		"list_work",
	} {
		if got[name] {
			t.Errorf("removed tool %q is still registered", name)
		}
	}
	if len(res.Tools) != len(want) {
		t.Errorf("tool count = %d, want %d: %v", len(res.Tools), len(want), got)
	}
}

// TestMCPMyList guards S3: my_list auto-provisions the agent's own list
// (named after the CRUSH_AGENT tag) so the agent never has to run an
// add_list + copy-id dance before writing its first task. Idempotent: a
// second call returns the same list without creating a duplicate.
func TestMCPMyList(t *testing.T) {
	session := setupMCPAs(t, "pi")

	// First call creates the agent's list.
	var first map[string]any
	mustUnmarshal(t, callTool(t, session, "my_list", nil), &first)
	mine, ok := first["mine"].(map[string]any)
	if !ok {
		t.Fatalf("my_list (first) = %+v, want mine block", first)
	}
	if mine["id"] == "" || mine["name"] != "pi: Inbox" {
		t.Fatalf("mine block = %+v, want id set and name \"pi: Inbox\"", mine)
	}

	// Counts come along: 0 pending / 0 complete on a fresh list.
	if mine["pending"] != float64(0) || mine["complete"] != float64(0) {
		t.Fatalf("my_list counts = pending=%v complete=%v, want 0/0", mine["pending"], mine["complete"])
	}

	// Second call returns the same list (idempotent — no duplicate created).
	var second map[string]any
	mustUnmarshal(t, callTool(t, session, "my_list", nil), &second)
	mine2 := second["mine"].(map[string]any)
	if mine2["id"] != mine["id"] {
		t.Fatalf("my_list not idempotent: %v != %v", mine2["id"], mine["id"])
	}

	// list_lists sees exactly one list, matching my_list's id.
	var lists []map[string]any
	mustUnmarshal(t, listsJSON(t, session, "agent"), &lists)
	if len(lists) != 1 || lists[0]["id"] != mine["id"] {
		t.Fatalf("list_lists = %+v, want one list matching my_list", lists)
	}

	// With only the agent's own list, foreign_lists is empty (not nil).
	foreign, ok := first["foreign_lists"].([]any)
	if !ok {
		t.Fatalf("foreign_lists missing or not an array: %#v", first["foreign_lists"])
	}
	if len(foreign) != 0 {
		t.Fatalf("expected 0 foreign lists, got %d", len(foreign))
	}
}

func TestMyListIncludesForeign(t *testing.T) {
	session := setupMCPAs(t, "pi")
	// One foreign list (created by a different identity).
	callTool(t, session, "add_list", map[string]any{"name": "human inbox", "created_by": "human"})
	// Force the agent's own list to exist.
	var res map[string]any
	mustUnmarshal(t, callTool(t, session, "my_list", nil), &res)
	mine, ok := res["mine"].(map[string]any)
	if !ok || mine["name"] == nil {
		t.Fatalf("mine missing: %#v", res)
	}
	if mine["name"] != "pi: Inbox" {
		t.Errorf("mine.name = %v, want pi: Inbox", mine["name"])
	}
	foreign, ok := res["foreign_lists"].([]any)
	if !ok || len(foreign) < 1 {
		t.Fatalf("foreign_lists missing or empty: %#v", res["foreign_lists"])
	}
	row := foreign[0].(map[string]any)
	if row["created_by"] != "human" {
		t.Errorf("foreign row should carry created_by='human', got %v", row["created_by"])
	}
	if row["name"] != "human inbox" {
		t.Errorf("foreign row name = %v, want human inbox", row["name"])
	}
}

// TestMCPInstructionsUsesPrefixedToolNames guards S2: the Instructions doc the
// agent reads at session start must list tools with the host-registered
// chore_crusher_<name> prefix. An unprefixed name (the original bug) makes the
// agent's first call fail to resolve. The blob lists every tool under a TOOLS
// heading as `name(...)`; scan the whole blob (not a fixed block) so the §9
// rewrite does not force a brittle diff.
func TestMCPInstructionsUsesPrefixedToolNames(t *testing.T) {
	session := setupMCP(t)

	instructions := session.InitializeResult().Instructions
	if instructions == "" {
		t.Fatal("Instructions is empty")
	}

	// The blob lists each tool under a TOOLS heading as `name(...)` using the
	// bare tool name; the host registers them as chore_crusher_<name>. Assert
	// the bare names appear and the chore_crusher_ prefix is documented once.
	wantTools := []string{
		"my_list", "list_tasks", "show_task", "search_tasks",
		"add_task", "edit_task", "delete_task", "set_progress", "complete_task",
		"reopen_task", "add_comment", "delete_comment", "add_list", "claim_work",
	}
	lower := strings.ToLower(instructions)
	if !strings.Contains(lower, "chore_crusher_") {
		t.Fatalf("Instructions must document the chore_crusher_ prefix;\nfull text:\n%s", instructions)
	}
	for _, name := range wantTools {
		if !strings.Contains(lower, name+"(") {
			t.Fatalf("Instructions missing tool %q;\nfull text:\n%s", name, instructions)
		}
	}

	// Removed tools (no MCP registration) must not appear as callables.
	removed := []string{"list_lists", "release_work", "list_work", "rename_list",
		"delete_list", "update_tasks", "toggle_task", "rename_task", "set_notes",
		"move_task", "show_tasks"}
	for _, name := range removed {
		if strings.Contains(lower, name) {
			t.Fatalf("Instructions still names removed tool %q;\nfull text:\n%s", name, instructions)
		}
	}
}

// TestMCPInstructionsUsesIdentity guards H1: the Instructions doc must name
// the configured CRUSH_AGENT, not a hardcoded "pi" — an agent running under
// another tag is otherwise told to use pi: lists it does not own.
func TestMCPInstructionsUsesIdentity(t *testing.T) {
	session := setupMCPAs(t, "claude")

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
	session := setupMCPAs(t, "pi")

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
		"add_list",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("Instructions missing always-on todo rule element %q;\nfull text:\n%s", want, instructions)
		}
	}
}

func TestMCPInstructionsHasWorkingLoop(t *testing.T) {
	session := setupMCPAs(t, "pi")

	instructions := session.InitializeResult().Instructions
	if instructions == "" {
		t.Fatal("Instructions is empty")
	}
	// The blob stays slim (§9): it points at the crush_inbox prompt for the
	// full loop instead of embedding it. Assert it names the loop and the
	// opener, and that the loop itself lives in the crush_inbox prompt.
	lower := strings.ToLower(instructions)
	for _, want := range []string{
		"working loop",
		"crush_inbox",
		"crush:///inbox",
		"set_progress",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("Instructions missing loop pointer element %q;\nfull text:\n%s", want, instructions)
		}
	}

	res, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "crush_inbox"})
	if err != nil {
		t.Fatalf("GetPrompt crush_inbox: %v", err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(res.Messages))
	}
	tc, ok := res.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", res.Messages[0].Content)
	}
	loop := strings.ToLower(tc.Text)
	for _, want := range []string{
		"get your tasks from chore crusher",
		"keep its status current",
		"set_progress",
		"percentage",
		"crush:///inbox",
		"before the next task",
	} {
		if !strings.Contains(loop, want) {
			t.Fatalf("crush_inbox prompt missing working-loop element %q;\nfull text:\n%s", want, tc.Text)
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

	var res struct {
		Tasks []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
			Depth  int    `json:"depth"`
		} `json:"tasks"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"],
		"status":  "pending",
	}), &res)
	rows := res.Tasks
	if len(rows) != 1 || rows[0].Title != "Write tests" || rows[0].Status != "pending" {
		t.Fatalf("pending rows = %+v", rows)
	}

	callTool(t, session, "complete_task", map[string]any{"ids": []string{task["id"]}})

	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"],
		"status":  "complete",
	}), &res)
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

	var res struct {
		Tasks []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Depth  int    `json:"depth"`
			Status string `json:"status"`
		} `json:"tasks"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"],
	}), &res)
	rows := res.Tasks
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

	var detailsArr []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Assignee string `json:"assignee"`
		Priority string `json:"priority"`
		Children []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Depth    int    `json:"depth"`
			Status   string `json:"status"`
			Assignee string `json:"assignee"`
			Priority string `json:"priority"`
		} `json:"children"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{parent["id"]},
	}), &detailsArr)
	if len(detailsArr) != 1 {
		t.Fatalf("show_task returned %d rows, want 1", len(detailsArr))
	}
	details := detailsArr[0]

	if details.ID != parent["id"] || details.Title != "Renovation" {
		t.Fatalf("show_task root = %+v", details)
	}
	// The assignment/priority fields land on the root and every descendant
	// row (docs/plan/mcp-assignment-and-priorities.md §8); nothing is
	// assigned here, so they report their zero values.
	if details.Assignee != "" || details.Priority != "none" {
		t.Fatalf("show_task root assignee/priority = %q/%q, want \"\"/none",
			details.Assignee, details.Priority)
	}
	if len(details.Children) != 2 {
		t.Fatalf("want 2 children (child + grandchild), got %+v", details.Children)
	}
	byID := make(map[string]int)
	for _, c := range details.Children {
		byID[c.ID] = c.Depth
		if c.Assignee != "" || c.Priority != "none" {
			t.Fatalf("child %s assignee/priority = %q/%q, want \"\"/none",
				c.ID, c.Assignee, c.Priority)
		}
	}
	if byID[child["id"]] != 1 {
		t.Fatalf("child depth = %d, want 1", byID[child["id"]])
	}
	if byID[grand["id"]] != 2 {
		t.Fatalf("grandchild depth = %d, want 2", byID[grand["id"]])
	}

	// The crush:///tasks/{id} resource used to be asserted here as a second
	// code path; it was deleted as a duplicate of show_task
	// (docs/plan/mcp-assignment-and-priorities.md §8), and show_task's own
	// children are already asserted above.
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

	var res struct {
		Tasks []struct{ Title string } `json:"tasks"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"],
	}), &res)
	if len(res.Tasks) != 0 {
		t.Fatalf("expected no rows after delete, got %+v", res.Tasks)
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

func TestMCPTaskShapesCarryListOwner(t *testing.T) {
	session := setupMCP(t)

	// A list owned by the default identity ("agent"). Tasks can be added
	// through the enforced surface.
	var ownList map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Mine"}), &ownList)

	var ownTask map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": ownList["id"],
		"title":   "Own task",
	}), &ownTask)

	// A list owned by another agent ("claude"). Seed a task via the shared
	// DB — the server (identity "agent") cannot write to it.
	var theirList map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{
		"name":       "claude: Backlog",
		"created_by": "claude",
	}), &theirList)

	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	theirTaskID := store.NewID()
	now := time.Now().Unix()
	// Seed the task already assigned to claude at high priority, so the read
	// shapes' assignee/assigned_at/assignee_live/priority fields have real
	// values to carry (docs/plan/mcp-assignment-and-priorities.md §8).
	if _, err := db.Exec(
		`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind, progress_pct, position, created_at, updated_at, completed_at, assignee, assigned_at, priority)
		 VALUES (?, ?, NULL, ?, '', 'pending', 'none', NULL, 0, ?, ?, NULL, 'claude', ?, 'high')`,
		theirTaskID, theirList["id"], "Their task", now, now, now,
	); err != nil {
		t.Fatalf("seed task on claude's list: %v", err)
	}

	// show_task carries list_owner and the assignment fields on the task.
	var detailsArr []struct {
		ListOwner    string `json:"list_owner"`
		Assignee     string `json:"assignee"`
		AssignedAt   *int64 `json:"assigned_at"`
		AssigneeLive bool   `json:"assignee_live"`
		Priority     string `json:"priority"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{theirTaskID},
	}), &detailsArr)
	if len(detailsArr) != 1 {
		t.Fatalf("show_task returned %d rows, want 1", len(detailsArr))
	}
	if detailsArr[0].ListOwner != "claude" {
		t.Fatalf("show_task list_owner = %q, want claude", detailsArr[0].ListOwner)
	}
	if detailsArr[0].Assignee != "claude" || detailsArr[0].AssignedAt == nil || detailsArr[0].Priority != "high" {
		t.Fatalf("show_task assignment fields = %+v, want assignee claude with assigned_at and priority high", detailsArr[0])
	}
	// claude has no live presence claim yet: the assignment is stale.
	if detailsArr[0].AssigneeLive {
		t.Fatal("show_task assignee_live = true, want false with no live claim")
	}

	// list_tasks carries list_owner and the assignment fields on every row.
	var res struct {
		Tasks []struct {
			ID           string `json:"id"`
			ListOwner    string `json:"list_owner"`
			Assignee     string `json:"assignee"`
			AssigneeLive bool   `json:"assignee_live"`
			Priority     string `json:"priority"`
		} `json:"tasks"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": theirList["id"],
	}), &res)
	rows := res.Tasks
	if len(rows) == 0 || rows[0].ListOwner != "claude" {
		t.Fatalf("list_tasks list_owner = %+v, want claude on every row", rows)
	}
	if rows[0].Assignee != "claude" || rows[0].AssigneeLive || rows[0].Priority != "high" {
		t.Fatalf("list_tasks assignment fields = %+v, want claude / stale / high", rows[0])
	}

	// search_tasks carries list_owner and the assignment fields per result
	// (results span both lists).
	var results []struct {
		ID           string `json:"id"`
		ListID       string `json:"list_id"`
		ListOwner    string `json:"list_owner"`
		Title        string `json:"title"`
		Assignee     string `json:"assignee"`
		AssigneeLive bool   `json:"assignee_live"`
		Priority     string `json:"priority"`
	}
	mustUnmarshal(t, callTool(t, session, "search_tasks", map[string]any{"query": "task"}), &results)
	var foundAgent, foundClaude bool
	for _, r := range results {
		switch r.ListOwner {
		case "agent":
			foundAgent = true
			if r.Assignee != "" || r.Priority != "none" {
				t.Fatalf("search_tasks own row assignment fields = %+v, want unassigned/none", r)
			}
		case "claude":
			foundClaude = true
			if r.Assignee != "claude" || r.AssigneeLive || r.Priority != "high" {
				t.Fatalf("search_tasks assignment fields = %+v, want claude / stale / high", r)
			}
		}
	}
	if !foundAgent || !foundClaude {
		t.Fatalf("search_tasks list_owner results = %+v, want both agent and claude", results)
	}

	// Light a live presence claim under claude: assignee_live flips without
	// the assignment itself changing — presence and assignment stay distinct
	// axes (docs/DESIGN.md §3).
	callTool(t, session, "claim_work", map[string]any{
		"entity_type": "task",
		"entity_id":   theirTaskID,
		"agent_id":    "claude",
	})
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{theirTaskID},
	}), &detailsArr)
	if !detailsArr[0].AssigneeLive {
		t.Fatal("show_task assignee_live = false after claude claimed work, want true")
	}
	if detailsArr[0].Assignee != "claude" {
		t.Fatalf("claim_work changed the assignee to %q — presence must not touch assignment", detailsArr[0].Assignee)
	}

	// The own list's task also has the right list_owner.
	var ownDetailsArr []struct {
		ListOwner string `json:"list_owner"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{ownTask["id"]},
	}), &ownDetailsArr)
	if len(ownDetailsArr) != 1 {
		t.Fatalf("show_task returned %d rows, want 1", len(ownDetailsArr))
	}
	if ownDetailsArr[0].ListOwner != "agent" {
		t.Fatalf("show_task own-task list_owner = %q, want agent", ownDetailsArr[0].ListOwner)
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

	// add_task auto-claims under the server identity ("agent"); release it so
	// "claude" can claim below (docs/plan/mcp-presence-on-all-writes.md).
	releaseWork(t, session, map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "agent",
	})

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
	mustUnmarshal(t, workJSON(t, session), &work)
	if len(work) != 1 || work[0].EntityID != task["id"] || work[0].AgentID != "claude" {
		t.Fatalf("list_work = %+v", work)
	}

	// Release it.
	releaseWork(t, session, map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "claude",
	})

	mustUnmarshal(t, workJSON(t, session), &work)
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

	// add_task auto-claims under "agent"; release it so "a1" can claim below.
	releaseWork(t, session, map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "agent",
	})

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
	mustUnmarshal(t, workJSON(t, session), &work)
	if len(work) != 1 || work[0].AgentID != "a1" {
		t.Fatalf("expected a1 still holding, got %+v", work)
	}
}

// TestMCPClaimWorkRelease guards §4.5: release=true stops the spinner using
// the server's own identity when agent_id is omitted, and is a no-op on an
// unclaimed entity.
func TestMCPClaimWorkRelease(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "pi: Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "Write docs",
	}), &task)

	// add_task auto-claimed under "pi"; release with agent_id omitted must
	// default to the server identity and clear that claim.
	var out map[string]bool
	mustUnmarshal(t, callTool(t, session, "claim_work", map[string]any{
		"entity_type": "task", "entity_id": task["id"], "release": true,
	}), &out)
	if !out["ok"] {
		t.Fatalf("claim_work(release) = %+v, want {ok:true}", out)
	}

	var work []struct {
		AgentID string `json:"agent_id"`
	}
	mustUnmarshal(t, workJSON(t, session), &work)
	if len(work) != 0 {
		t.Fatalf("claim_work(release) left claims = %+v", work)
	}

	// Releasing again is a no-op, not an error.
	callTool(t, session, "claim_work", map[string]any{
		"entity_type": "task", "entity_id": task["id"], "release": true,
	})

	// release=false (omitted) still claims and returns an activity id.
	var claim map[string]string
	mustUnmarshal(t, callTool(t, session, "claim_work", map[string]any{
		"entity_type": "task", "entity_id": task["id"],
	}), &claim)
	if claim["id"] == "" {
		t.Fatalf("claim_work without release = %+v, want an activity id", claim)
	}

	// A bad entity_type is still rejected on the release path.
	callToolErr(t, session, "claim_work", map[string]any{
		"entity_type": "project", "entity_id": task["id"], "release": true,
	})
}

func TestMCPStatusWritesRefreshClaim(t *testing.T) {
	session := setupMCPAs(t, "pi")

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
		{"set_progress", "set_progress", map[string]any{"ids": []string{task["id"]}, "mode": "simple"}},
		{"complete_task", "complete_task", map[string]any{"ids": []string{task["id"]}}},
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
	session := setupMCPAs(t, "pi")

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

	callTool(t, session, "complete_task", map[string]any{"ids": []string{task["id"]}})

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

func TestStatusWriteAutoClaims(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "work",
	}), &task)
	// No explicit claim_work first — just start setting progress.
	callTool(t, session, "set_progress", map[string]any{
		"ids": []string{task["id"]}, "mode": "percentage", "percent": 25,
	})
	var work []map[string]any
	mustUnmarshal(t, workJSON(t, session), &work)
	found := false
	for _, w := range work {
		if w["entity_id"] == task["id"] && w["agent_id"] == "pi" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("status write should have auto-claimed the task; work=%#v", work)
	}
}

func TestMCPStatusWritesDoNotTouchForeignClaims(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)

	// add_task auto-claims under "pi"; release it so "other" can hold the claim
	// below, so we can assert pi's status write does not refresh it.
	releaseWork(t, session, map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "pi",
	})

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

	callTool(t, session, "complete_task", map[string]any{"ids": []string{task["id"]}})

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

	// add_task auto-claims under "agent"; release it so "claude" can claim below.
	releaseWork(t, session, map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "agent",
	})

	// Claim and verify the crush://work resource reports it. The resource is
	// only reader now that list_work is gone (§4.5).
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
}

// TestMCPResourcesListed pins the resource surface at exactly two entries and
// zero templates. The five that used to live here — crush:///lists,
// crush:///lists/{id}, crush:///lists/{id}/tasks, crush:///tasks/{id} and
// crush:///search/{query} — were row-for-row duplicates of my_list /
// list_tasks / show_task / search_tasks and were deleted
// (docs/plan/mcp-assignment-and-priorities.md §8). This test is what stops
// them coming back: a new field belongs on the tool, not on a second surface
// that has to be kept in sync with it.
func TestMCPResourcesListed(t *testing.T) {
	session := setupMCP(t)

	res, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources(): %v", err)
	}
	found := make(map[string]bool, len(res.Resources))
	for _, r := range res.Resources {
		found[r.URI] = true
	}
	want := map[string]bool{"crush:///inbox": true, "crush://work": true}
	if len(found) != len(want) {
		t.Fatalf("resources = %+v, want exactly %v", res.Resources, want)
	}
	for uri := range want {
		if !found[uri] {
			t.Fatalf("resource %q not in resources: %+v", uri, res.Resources)
		}
	}

	// No resource templates at all: every template was a tool duplicate.
	tres, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates(): %v", err)
	}
	if len(tres.ResourceTemplates) != 0 {
		t.Fatalf("resource templates = %+v, want none", tres.ResourceTemplates)
	}
}

func TestInboxResourceReturnsMineAndForeign(t *testing.T) {
	// Use a shared data dir so two server identities (human + pi) see the same store.
	dataDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataDir)

	// human session: create a list + two pending tasks (one with notes).
	t.Setenv("CRUSH_AGENT", "human")
	humanServer, humanStore, err := mcpserver.NewServer()
	if err != nil {
		t.Fatalf("NewServer (human): %v", err)
	}
	t.Cleanup(func() { humanStore.Close() })
	ctx := context.Background()
	hct, hst := mcp.NewInMemoryTransports()
	hss, err := humanServer.Connect(ctx, hst, nil)
	if err != nil {
		t.Fatalf("human server.Connect: %v", err)
	}
	t.Cleanup(func() { hss.Close() })
	hClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	hSession, err := hClient.Connect(ctx, hct, nil)
	if err != nil {
		t.Fatalf("human client.Connect: %v", err)
	}
	t.Cleanup(func() { hSession.Close() })

	var hlist map[string]string
	mustUnmarshal(t, callTool(t, hSession, "add_list", map[string]any{"name": "human"}), &hlist)
	mustUnmarshal(t, callTool(t, hSession, "add_task", map[string]any{"list_id": hlist["id"], "title": "F1", "notes": "why"}), &struct{}{})
	callTool(t, hSession, "add_task", map[string]any{"list_id": hlist["id"], "title": "F2"})

	// pi session: read the inbox resource.
	t.Setenv("CRUSH_AGENT", "pi")
	piServer, _, err := mcpserver.NewServer()
	if err != nil {
		t.Fatalf("NewServer (pi): %v", err)
	}
	pct, pst := mcp.NewInMemoryTransports()
	pss, err := piServer.Connect(ctx, pst, nil)
	if err != nil {
		t.Fatalf("pi server.Connect: %v", err)
	}
	t.Cleanup(func() { pss.Close() })
	pClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	piSession, err := pClient.Connect(ctx, pct, nil)
	if err != nil {
		t.Fatalf("pi client.Connect: %v", err)
	}
	t.Cleanup(func() { piSession.Close() })
	callTool(t, piSession, "my_list", nil) // force pi's own list to exist

	res, err := piSession.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "crush:///inbox"})
	if err != nil {
		t.Fatalf("ReadResource crush:///inbox: %v", err)
	}
	var got map[string]any
	mustUnmarshal(t, res.Contents[0].Text, &got)

	mine, ok := got["mine"].(map[string]any)
	if !ok || mine["name"] == nil {
		t.Fatalf("mine missing: %#v", got)
	}
	if mine["name"] != "pi: Inbox" {
		t.Errorf("mine.name = %v, want pi: Inbox", mine["name"])
	}

	foreign, ok := got["foreign_lists"].([]any)
	if !ok || len(foreign) == 0 {
		t.Fatalf("foreign_lists missing: %#v", got["foreign_lists"])
	}
	fBlock := foreign[0].(map[string]any)
	tasks, ok := fBlock["tasks"].([]any)
	if !ok || len(tasks) != 2 {
		t.Fatalf("expected 2 tasks in foreign block, got %#v", tasks)
	}
	byTitle := map[string]map[string]any{}
	for _, r := range tasks {
		row := r.(map[string]any)
		byTitle[row["title"].(string)] = row
	}
	if byTitle["F1"]["has_notes"] != true || byTitle["F1"]["notes"] != "why" {
		t.Errorf("F1 should carry has_notes=true + notes body, got %#v", byTitle["F1"])
	}
	if byTitle["F2"]["has_notes"] != false {
		t.Errorf("F2 should carry has_notes=false, got %#v", byTitle["F2"])
	}
	if _, present := byTitle["F2"]["notes"]; present {
		t.Errorf("F2 must not have a notes body (empty notes), got %#v", byTitle["F2"])
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
	for _, name := range []string{"crush_inbox", "crush_breakdown"} {
		if !found[name] {
			t.Fatalf("prompt %q not listed: %+v", name, res.Prompts)
		}
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
// structural write tool (add_task, edit_task, delete_task) errors on a list
// owned by another agent — not just one of them. The server here acts as
// "pi"; the list is created as "claude". (rename_list/delete_list are no
// longer MCP tools — docs/plan/mcp-tool-consolidation.md §4.4.)
func TestMCPForeignListWriteRefused(t *testing.T) {
	session := setupMCPAs(t, "pi")

	// A list owned by claude (created explicitly as claude via the enforced
	// add_list surface). The server here is "pi", so every structural write
	// to it must be refused.
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{
		"name": "claude: Backlog", "created_by": "claude",
	}), &list)

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

	// One table for every gated structural tool: a new gated tool is added as
	// a row here, not as a hand-written case (hardening §5.I, H14).
	for _, tc := range []struct {
		name string
		tool string
		args map[string]any
	}{
		{"add_task", "add_task", map[string]any{"list_id": list["id"], "title": "intrude"}},
		{"edit_task rename", "edit_task", map[string]any{"id": taskID, "title": "hijack"}},
		{"edit_task notes", "edit_task", map[string]any{"id": taskID, "notes": "tamper"}},
		// edit_task moving to root on a foreign list is refused before any write.
		{"edit_task move", "edit_task", map[string]any{"id": taskID, "to_root": true}},
		{"delete_task", "delete_task", map[string]any{"id": taskID, "force": true}},
	} {
		msg := callToolErr(t, session, tc.tool, tc.args)
		if !strings.Contains(msg, "owned by claude") {
			t.Fatalf("%s foreign-list error = %q, want it to name the owner", tc.name, msg)
		}
	}

	// The task was never moved/deleted: claude's list still has exactly one
	// task, proving no structural write slipped through.
	var res struct {
		Tasks []struct{ Title string } `json:"tasks"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"],
	}), &res)
	if len(res.Tasks) != 1 || res.Tasks[0].Title != "real work" {
		t.Fatalf("foreign-list structural writes leaked: rows = %+v", res.Tasks)
	}
}

// TestMCPOwnerCanWriteEverything guards that the owner of a list can run every
// structural tool on it (the mirror of TestMCPForeignListWriteRefused).
func TestMCPOwnerCanWriteEverything(t *testing.T) {
	session := setupMCPAs(t, "claude")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{
		"name": "claude: Backlog", "created_by": "claude",
	}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "real work",
	}), &task)

	callTool(t, session, "edit_task", map[string]any{"id": task["id"], "title": "renamed"})
	callTool(t, session, "edit_task", map[string]any{"id": task["id"], "notes": "annotated"})

	// edit_task moving to root within the owner's own list succeeds.
	callTool(t, session, "edit_task", map[string]any{"id": task["id"], "to_root": true})

	// delete_task requires force and succeeds for the owner.
	callTool(t, session, "delete_task", map[string]any{"id": task["id"], "force": true})
	var res struct {
		Tasks []struct{ Title string } `json:"tasks"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"],
	}), &res)
	if len(res.Tasks) != 0 {
		t.Fatalf("owner delete_task left rows = %+v", res.Tasks)
	}
}

// TestMCPStatusToolsOpenOnForeignList guards §5 assertion 3: status/progress
// tools are never gated, so complete_task / set_progress succeed on a list
// owned by another agent.
func TestMCPStatusToolsOpenOnForeignList(t *testing.T) {
	session := setupMCPAs(t, "pi")

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
	callTool(t, session, "set_progress", map[string]any{"ids": []string{taskID}, "mode": "simple"})
	callTool(t, session, "complete_task", map[string]any{"ids": []string{taskID}})

	var res struct {
		Tasks []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"tasks"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": list["id"], "status": "complete",
	}), &res)
	rows := res.Tasks
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
	session := setupMCPAs(t, "pi")

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
	msg = callToolErr(t, session, "edit_task", map[string]any{
		"id": taskID, "title": "Renamed",
	})
	if !strings.Contains(msg, "no one (untagged)") {
		t.Fatalf("edit_task untagged-list error = %q, want it to say untagged", msg)
	}
	msg = callToolErr(t, session, "delete_task", map[string]any{
		"id": taskID, "force": true,
	})
	if !strings.Contains(msg, "no one (untagged)") {
		t.Fatalf("delete_task untagged-list error = %q, want it to say untagged", msg)
	}

	// Yet status/progress writes succeed on the untagged list's task — the
	// read + status/progress-only rule holds for *every* identity.
	callTool(t, session, "set_progress", map[string]any{"ids": []string{taskID}, "mode": "simple"})
	callTool(t, session, "complete_task", map[string]any{"ids": []string{taskID}})

	var res struct {
		Tasks []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"tasks"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": listID, "status": "complete",
	}), &res)
	rows := res.Tasks
	if len(rows) != 1 || rows[0].ID != taskID || rows[0].Status != "complete" {
		t.Fatalf("untagged-list status write did not take: rows = %+v", rows)
	}
}

// TestMCPCollaborativeListAllowsStructuralEdits pins the "Tag a list as
// collaborative" feature's whole point: an explicit opt-in flag lets a
// foreign (here, untagged — the realistic human-list shape) list accept
// structural writes from any agent, where TestMCPUntaggedListForeignToEveryAgent
// just proved the same list refuses them with the flag off. There is no MCP
// tool to set the flag (human-only, from the TUI's list-rename modal), so
// this seeds it directly, the same way the untagged-list test seeds
// created_by="".
func TestMCPCollaborativeListAllowsStructuralEdits(t *testing.T) {
	session := setupMCPAs(t, "pi")

	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	listID := store.NewID()
	taskID := store.NewID()
	now := time.Now().Unix()
	if _, err := db.Exec(
		`INSERT INTO List (id, name, created_at, position, created_by, collaborative) VALUES (?, ?, ?, 0, "", 1)`,
		listID, "Shared backlog", now,
	); err != nil {
		t.Fatalf("seed collaborative list: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind, progress_pct, position, created_at, updated_at, completed_at)
		 VALUES (?, ?, NULL, 'file this', '', 'pending', 'none', NULL, 0, ?, ?, NULL)`,
		taskID, listID, now, now,
	); err != nil {
		t.Fatalf("seed task on collaborative list: %v", err)
	}

	// pi does not own this list (created_by=""), but collaborative=1 lets
	// every structural tool through anyway.
	var added map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": listID, "title": "new work",
	}), &added)
	callTool(t, session, "edit_task", map[string]any{"id": taskID, "title": "renamed"})
	callTool(t, session, "edit_task", map[string]any{"id": taskID, "notes": "annotated"})
	callTool(t, session, "delete_task", map[string]any{"id": taskID, "force": true})

	var res struct {
		Tasks []struct{ Title string } `json:"tasks"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{
		"list_id": listID,
	}), &res)
	rows := res.Tasks
	if len(rows) != 1 || rows[0].Title != "new work" {
		t.Fatalf("collaborative-list structural writes did not take: rows = %+v", rows)
	}
}

// TestMCPAddListDefaultsToIdentity guards §5 assertion 5 (default owner):
// add_list with no created_by yields a list_lists entry whose created_by is
// the server identity.
func TestMCPAddListDefaultsToIdentity(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var created map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{
		"name": "My list",
	}), &created)

	var lists []struct {
		ID        string `json:"id"`
		CreatedBy string `json:"created_by"`
	}
	mustUnmarshal(t, listsJSON(t, session, "pi"), &lists)
	if len(lists) != 1 || lists[0].ID != created["id"] || lists[0].CreatedBy != "pi" {
		t.Fatalf("list_lists = %+v, want created_by=pi for the new list", lists)
	}
}

// TestMCPListListsIncludesCreatedBy guards §5 assertion 5: list_lists reports
// the owner of each list.
func TestMCPListListsIncludesCreatedBy(t *testing.T) {
	session := setupMCPAs(t, "pi")

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
	mustUnmarshal(t, listsJSON(t, session, "pi"), &lists)
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
	session := setupMCPAs(t, "pi")

	msg := callToolErr(t, session, "add_list", map[string]any{
		"name": "x", "created_by": "p i",
	})
	if !strings.Contains(msg, "created_by must match") {
		t.Fatalf("add_list bad tag error = %q, want the pattern error", msg)
	}
}

// TestMCPPendingClaimsClearedOnSessionEnd verifies H13: when the MCP session
// ends, ReleaseAllClaims (called by Run after server.Run returns) clears every
// claim so the TUI shows no stale spinners. The test makes claims through the
// MCP interface, then calls ReleaseAllClaims on a separate store handle to the
// same DB (mirroring what Run does after server.Run returns), and confirms
// list_work goes empty.
func TestMCPPendingClaimsClearedOnSessionEnd(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)

	// add_task auto-claims under "agent"; release it so "pi" can claim below.
	releaseWork(t, session, map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "agent",
	})

	// Claim via the MCP tool (defaults to identity "agent").
	callTool(t, session, "claim_work", map[string]any{
		"entity_type": "task",
		"entity_id":   task["id"],
		"agent_id":    "pi",
	})
	// A second claim on the list by a different agent.
	callTool(t, session, "claim_work", map[string]any{
		"entity_type": "list",
		"entity_id":   list["id"],
		"agent_id":    "claude",
	})

	// Both claims are visible before session-end cleanup.
	var work []struct {
		AgentID string `json:"agent_id"`
	}
	mustUnmarshal(t, workJSON(t, session), &work)
	if len(work) != 2 {
		t.Fatalf("expected 2 claims before cleanup, got %d", len(work))
	}

	// Simulate session-end cleanup: open a handle to the same DB (as Run does
	// via the shared *store.Store) and clear all claims.
	cleanup, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open for cleanup: %v", err)
	}
	t.Cleanup(func() { cleanup.Close() })

	n, err := cleanup.ReleaseAllClaims()
	if err != nil {
		t.Fatalf("ReleaseAllClaims: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 claims released, got %d", n)
	}

	// list_work is now empty — the TUI has no spinners to show.
	mustUnmarshal(t, workJSON(t, session), &work)
	if len(work) != 0 {
		t.Fatalf("expected 0 claims after session-end cleanup, got %d", len(work))
	}
}

// listTasksResponse is the list_tasks envelope — a bare array until step 7
// turned it into {tasks, elided, budget_exceeded}: elided names the rows the
// §5.3 body budget dropped whole (fetch them with show_task), and
// budget_exceeded is true exactly when any body was dropped.
type listTasksResponse struct {
	Tasks          []map[string]any `json:"tasks"`
	Elided         []string         `json:"elided"`
	BudgetExceeded bool             `json:"budget_exceeded"`
}

// listTasks calls the list_tasks tool and returns its envelope.
func listTasks(t *testing.T, session *mcp.ClientSession, args map[string]any) listTasksResponse {
	t.Helper()
	var res listTasksResponse
	mustUnmarshal(t, callTool(t, session, "list_tasks", args), &res)
	return res
}

func TestListTasksReportsNotesFlags(t *testing.T) {
	session := setupMCP(t)
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "with notes", "notes": "hello",
	})
	callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "no notes",
	})
	res := listTasks(t, session, map[string]any{"list_id": list["id"]})
	if len(res.Tasks) != 2 {
		t.Fatalf("want 2 rows, got %d", len(res.Tasks))
	}
	if res.BudgetExceeded || len(res.Elided) != 0 {
		t.Errorf("no bodies are inlined here, so nothing can elide: %#v", res.Elided)
	}
	byTitle := map[string]map[string]any{}
	for _, r := range res.Tasks {
		byTitle[r["title"].(string)] = r
	}
	if byTitle["with notes"]["has_notes"] != true {
		t.Errorf("with-notes row missing has_notes=true: %#v", byTitle["with notes"])
	}
	if int(byTitle["with notes"]["notes_len"].(float64)) != 5 {
		t.Errorf("notes_len for 'hello' should be 5, got %v", byTitle["with notes"]["notes_len"])
	}
	if byTitle["no notes"]["has_notes"] != false {
		t.Errorf("no-notes row should have has_notes=false: %#v", byTitle["no notes"])
	}
	if _, present := byTitle["with notes"]["notes"]; present {
		t.Errorf("notes body must NOT appear without include=notes")
	}
}

func TestListTasksIncludeNotes(t *testing.T) {
	session := setupMCP(t)
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "short", "notes": "abc",
	})
	longNote := strings.Repeat("x", 2500)
	callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "long", "notes": longNote,
	})
	res := listTasks(t, session, map[string]any{
		"list_id": list["id"], "include": []string{"notes"},
	})
	byTitle := map[string]map[string]any{}
	for _, r := range res.Tasks {
		byTitle[r["title"].(string)] = r
	}

	if byTitle["short"]["notes"] != "abc" {
		t.Errorf("short notes should inline verbatim, got %v", byTitle["short"]["notes"])
	}
	if len(byTitle["long"]["notes"].(string)) != 2500 {
		t.Errorf("long notes must come back WHOLE — the 2000-char truncation is gone — got %d", len(byTitle["long"]["notes"].(string)))
	}
	// The notes_truncated field was deleted with the truncation it described
	// (§5.3): a body is now inlined whole or dropped whole, never cut.
	for _, r := range res.Tasks {
		if _, tr := r["notes_truncated"]; tr {
			t.Errorf("notes_truncated must not exist any more: %#v", r)
		}
	}
}

func TestListTasksUnknownIncludeRejected(t *testing.T) {
	session := setupMCP(t)
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	msg := callToolErr(t, session, "list_tasks", map[string]any{
		"list_id": list["id"], "include": []string{"bogus"},
	})
	if !strings.Contains(msg, "unknown include") {
		t.Errorf("expected 'unknown include' error, got %q", msg)
	}
}

func TestListTasksOmitsEmptyProgress(t *testing.T) {
	session := setupMCP(t)
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "A"})
	raw := callTool(t, session, "list_tasks", map[string]any{"list_id": list["id"]})
	if strings.Contains(raw, `"progress"`) {
		t.Errorf("no-progress task should not include a progress key; got: %s", raw)
	}
	if !strings.Contains(raw, "budget_exceeded") {
		t.Errorf("the envelope must carry budget_exceeded; got: %s", raw)
	}
}

// The five TestListChanges* cases fold into list_tasks(since=...): the
// list_changes tool is gone, and `since` is how change-detection is asked
// for now (docs/plan/mcp-assignment-and-priorities.md §4).

func TestListTasksSinceReturnsOnlyChanged(t *testing.T) {
	session := setupMCP(t)
	var list, a, b map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	mustUnmarshal(t, callTool(t, session, "add_task",
		map[string]any{"list_id": list["id"], "title": "a"}), &a)
	mustUnmarshal(t, callTool(t, session, "add_task",
		map[string]any{"list_id": list["id"], "title": "b"}), &b)

	cutoff := time.Now().Unix()
	time.Sleep(1100 * time.Millisecond)
	callTool(t, session, "edit_task", map[string]any{"id": a["id"], "title": "a2"})

	rows := listTasks(t, session, map[string]any{
		"list_id": list["id"], "since": cutoff,
	}).Tasks
	if len(rows) != 1 || rows[0]["id"] != a["id"] {
		t.Fatalf("want only task a in changes, got %#v (b=%s)", rows, b["id"])
	}
}

func TestListTasksSinceIncludesNotes(t *testing.T) {
	session := setupMCP(t)
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "a", "notes": "hello",
	})
	rows := listTasks(t, session, map[string]any{
		"list_id": list["id"], "since": 0, "include": []string{"notes"},
	}).Tasks
	if len(rows) != 1 || rows[0]["notes"] != "hello" {
		t.Fatalf("include=notes should inline the body, got %#v", rows)
	}
}

func TestListTasksSinceSeesNewComment(t *testing.T) {
	session := setupMCP(t)
	var list, a map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	mustUnmarshal(t, callTool(t, session, "add_task",
		map[string]any{"list_id": list["id"], "title": "a"}), &a)

	cutoff := time.Now().Unix()
	time.Sleep(1100 * time.Millisecond)
	callTool(t, session, "add_comment", map[string]any{"task_id": a["id"], "note": "ping"})

	rows := listTasks(t, session, map[string]any{
		"list_id": list["id"], "since": cutoff,
	}).Tasks
	if len(rows) != 1 || rows[0]["id"] != a["id"] {
		t.Fatalf("a new comment should surface in list_tasks(since); got %#v", rows)
	}
}

func TestListTasksSinceUnknownIncludeRejected(t *testing.T) {
	session := setupMCP(t)
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	msg := callToolErr(t, session, "list_tasks", map[string]any{
		"list_id": list["id"], "since": 0, "include": []string{"bogus"},
	})
	if !strings.Contains(msg, "unknown include") {
		t.Errorf("expected 'unknown include' error, got %q", msg)
	}
}

func TestListTasksSinceBeforeAnyTimestamp(t *testing.T) {
	session := setupMCP(t)
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "a"})
	rows := listTasks(t, session, map[string]any{
		"list_id": list["id"], "since": 0,
	}).Tasks
	if len(rows) != 1 {
		t.Fatalf("since=0 should return all tasks, got %d", len(rows))
	}
}

// TestListTasksSkeletonAncestorContextOnly pins §5.2: a pending child of a
// complete parent matches the default 'open' filter, and its non-matching
// ancestor comes back as a context_only skeleton so the parent_id chain and
// depth stay meaningful — the skeleton never carries an inlined body even
// under include=notes, and the same row is a full row again (no
// context_only) when it matches the filter in its own right.
func TestListTasksSkeletonAncestorContextOnly(t *testing.T) {
	session := setupMCP(t)
	var list, parent map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "parent", "notes": "parent body",
	}), &parent)
	callTool(t, session, "complete_task", map[string]any{"ids": []string{parent["id"]}})
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "child", "parent": parent["id"],
	}), &map[string]string{})

	res := listTasks(t, session, map[string]any{
		"list_id": list["id"], "include": []string{"notes"},
	})
	if len(res.Tasks) != 2 {
		t.Fatalf("want skeleton parent + child, got %#v", res.Tasks)
	}
	parentRow, childRow := res.Tasks[0], res.Tasks[1]
	if parentRow["title"] != "parent" || childRow["title"] != "child" {
		t.Fatalf("preorder: parent must precede the child; got %#v", res.Tasks)
	}
	if parentRow["context_only"] != true {
		t.Errorf("the complete parent must be context_only=true under the open filter: %#v", parentRow)
	}
	if childRow["depth"] != float64(1) || childRow["parent_id"] != parent["id"] {
		t.Errorf("child must keep depth=1 and its parent link; got %#v", childRow)
	}
	// Skeletons never inline their bodies (even under include=notes), but do
	// report the size flags so the caller can decide to show_task them.
	if _, hasBody := parentRow["notes"]; hasBody {
		t.Errorf("skeleton must not inline its notes: %#v", parentRow)
	}
	if parentRow["has_notes"] != true || int(parentRow["notes_len"].(float64)) != len("parent body") {
		t.Errorf("skeleton must still report has_notes/notes_len: %#v", parentRow)
	}
	// The child matches in its own right: full row with its body inlined.
	if _, isSkeleton := childRow["context_only"]; isSkeleton {
		t.Errorf("matching child must not be context_only: %#v", childRow)
	}

	// Under status=complete the parent matches the filter in its own right:
	// full row again, and the pending child is not a skeleton (skeletons only
	// walk upward from a match, never down to non-matching descendants).
	res = listTasks(t, session, map[string]any{
		"list_id": list["id"], "status": "complete", "include": []string{"notes"},
	})
	if len(res.Tasks) != 1 {
		t.Fatalf("status=complete should match only the parent, got %#v", res.Tasks)
	}
	if _, isSkeleton := res.Tasks[0]["context_only"]; isSkeleton {
		t.Errorf("a row matching in its own right is not a skeleton: %#v", res.Tasks[0])
	}
	if res.Tasks[0]["notes"] != "parent body" {
		t.Errorf("full parent row must inline its notes: %#v", res.Tasks[0])
	}
}

// TestListTasksBudgetWholeOrNotAll pins §5.3 and decision 8: ten notes of
// 5000 chars double the 40000-byte notesBudget, so the first eight come
// back WHOLE and the last two stay in the response without a body — their
// ids land in elided, never a body cut to a prefix. budget_exceeded
// reports that the budget was hit, and a request without include never
// touches it.
func TestListTasksBudgetWholeOrNotAtAll(t *testing.T) {
	session := setupMCP(t)
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	const noteLen = 5000
	for i := 0; i < 10; i++ {
		callTool(t, session, "add_task", map[string]any{
			"list_id": list["id"], "title": fmt.Sprintf("t%d", i),
			"notes": strings.Repeat("n", noteLen),
		})
	}

	res := listTasks(t, session, map[string]any{
		"list_id": list["id"], "include": []string{"notes"},
	})
	// 8 x 5000 = 40000 exactly fits; the 9th row would push past, so rows 9
	// and 10 keep their place in tasks (with has_notes/notes_len) but their
	// ids land in elided and no body is inlined for them.
	if len(res.Tasks) != 10 || len(res.Elided) != 2 {
		t.Fatalf("want all 10 rows with 2 elided, got tasks=%d elided=%v", len(res.Tasks), res.Elided)
	}
	if !res.BudgetExceeded {
		t.Errorf("budget_exceeded must be true when rows were elided")
	}
	bodies := 0
	inlined := map[string]bool{}
	for _, r := range res.Tasks {
		body, ok := r["notes"].(string)
		if !ok {
			continue
		}
		bodies++
		if len(body) != noteLen {
			t.Fatalf("note body was cut mid-text: len=%d want %d — whole or not at all", len(body), noteLen)
		}
		inlined[r["id"].(string)] = true
	}
	if bodies != 8 {
		t.Errorf("want exactly 8 inlined bodies, got %d", bodies)
	}
	for _, id := range res.Elided {
		if inlined[id] {
			t.Errorf("id %s appears both inlined and elided", id)
		}
	}

	// Without include=notes no bodies are requested, so the budget is never
	// hit: all ten rows come back and nothing is elided.
	res = listTasks(t, session, map[string]any{"list_id": list["id"]})
	if len(res.Tasks) != 10 || len(res.Elided) != 0 || res.BudgetExceeded {
		t.Fatalf("no include means no budget: tasks=%d elided=%v exceeded=%v", len(res.Tasks), res.Elided, res.BudgetExceeded)
	}
}

func TestShowTasksBatch(t *testing.T) {
	session := setupMCP(t)
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var a, b map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "A", "notes": "aa",
	}), &a)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "B",
	}), &b)
	// Add a comment to A so we can assert the merged show_task includes it.
	cid, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "add_comment",
		Arguments: map[string]any{"task_id": a["id"], "note": "hi"},
	})
	if err != nil {
		t.Fatalf("add_comment: %v", err)
	}
	var cres struct {
		ID string `json:"id"`
	}
	mustUnmarshal(t, textContent(t, cid), &cres)

	var got []map[string]any
	mustUnmarshal(t, callTool(t, session, "show_task",
		map[string]any{"ids": []string{a["id"], b["id"], "does-not-exist"}}), &got)
	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d", len(got))
	}
	if got[0]["title"] != "A" || got[0]["notes"] != "aa" {
		t.Errorf("row 0: %#v", got[0])
	}
	// The assignment/priority fields are present on every row
	// (docs/plan/mcp-assignment-and-priorities.md §8); nothing is assigned
	// here, so they report their zero values.
	if got[0]["assignee"] != "" || got[0]["priority"] != "none" || got[0]["assignee_live"] != false {
		t.Errorf("row 0 assignment fields = assignee %#v priority %#v assignee_live %#v, want \"\"/none/false",
			got[0]["assignee"], got[0]["priority"], got[0]["assignee_live"])
	}
	if got[1]["title"] != "B" {
		t.Errorf("row 1: %#v", got[1])
	}
	// The merged tool must include comments (the old singular show_task did;
	// the old batch show_tasks omitted them — §4.1 pins the fix).
	comments, ok := got[0]["comments"].([]any)
	if !ok || len(comments) != 1 {
		t.Fatalf("row 0 comments = %#v, want 1 comment", got[0]["comments"])
	}
	if cm, _ := comments[0].(map[string]any); cm["id"] != cres.ID {
		t.Errorf("row 0 comment id = %#v, want %q", cm["id"], cres.ID)
	}
	if _, hasErr := got[2]["error"]; !hasErr {
		t.Errorf("row 2 must be an error row, got %#v", got[2])
	}
}

// TestMCPShowTaskReturnsDescendantNotesAndComments pins step 6 of the
// assignment plan: show_task is self-contained — every descendant row comes
// back with its FULL notes and its comments, uncapped. The child's notes are
// deliberately longer than list_tasks' old 2000-char truncation limit to
// prove the subtree path never cuts a body mid-text (decision 8).
func TestMCPShowTaskReturnsDescendantNotesAndComments(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)

	var parent map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Renovation",
	}), &parent)

	longNotes := strings.Repeat("paint spec ", 300) // ~3.3 KB, past any truncation limit
	var child map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Buy paint",
		"parent":  parent["id"],
		"notes":   longNotes,
	}), &child)

	var grand map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Pick colour",
		"parent":  child["id"],
	}), &grand)

	callTool(t, session, "add_comment", map[string]any{
		"task_id": child["id"],
		"note":    "checking in",
	})

	var detailsArr []struct {
		ID       string `json:"id"`
		Children []struct {
			ID       string `json:"id"`
			Notes    string `json:"notes"`
			HasNotes bool   `json:"has_notes"`
			NotesLen int    `json:"notes_len"`
			Comments []struct {
				Note string `json:"note"`
			} `json:"comments"`
		} `json:"children"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{parent["id"]},
	}), &detailsArr)
	if len(detailsArr) != 1 {
		t.Fatalf("show_task returned %d rows, want 1", len(detailsArr))
	}

	rows := make(map[string]int)
	for i, c := range detailsArr[0].Children {
		rows[c.ID] = i
	}
	childIdx, ok := rows[child["id"]]
	if !ok {
		t.Fatalf("child row missing from show_task children: %+v", detailsArr[0].Children)
	}
	crow := detailsArr[0].Children[childIdx]
	if crow.Notes != longNotes {
		t.Fatalf("child notes came back %d chars, want the full %d uncapped", len(crow.Notes), len(longNotes))
	}
	if !crow.HasNotes || crow.NotesLen != len(longNotes) {
		t.Fatalf("child has_notes/notes_len = %v/%d, want true/%d", crow.HasNotes, crow.NotesLen, len(longNotes))
	}
	if len(crow.Comments) != 1 || crow.Comments[0].Note != "checking in" {
		t.Fatalf("child comments = %+v, want the one comment", crow.Comments)
	}

	grandIdx, ok := rows[grand["id"]]
	if !ok {
		t.Fatalf("grandchild row missing from show_task children: %+v", detailsArr[0].Children)
	}
	if gcomments := detailsArr[0].Children[grandIdx].Comments; len(gcomments) != 0 {
		t.Fatalf("grandchild comments = %+v, want none", gcomments)
	}
}

func TestShowTasksCap(t *testing.T) {
	session := setupMCP(t)
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = "x"
	}
	msg := callToolErr(t, session, "show_task", map[string]any{"ids": ids})
	if !strings.Contains(msg, "capped at 50") {
		t.Errorf("expected cap error, got %q", msg)
	}
}

// TestMCPAddCommentRoundTrip pins the add_comment tool (docs/plan/task-comments.md §4):
// it succeeds on a normal task, attributes the comment to the server identity,
// and the comment appears in a subsequent show_task.
func TestMCPAddCommentRoundTrip(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)

	cid, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "add_comment",
		Arguments: map[string]any{"task_id": task["id"], "note": "hello"},
	})
	if err != nil {
		t.Fatalf("add_comment: %v", err)
	}
	var res struct {
		ID string `json:"id"`
	}
	mustUnmarshal(t, textContent(t, cid), &res)
	if res.ID == "" {
		t.Fatal("add_comment returned empty id")
	}

	var detailsArr []struct {
		ID       string `json:"id"`
		Comments []struct {
			ID     string `json:"id"`
			Author string `json:"author"`
			Note   string `json:"note"`
		} `json:"comments"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{task["id"]},
	}), &detailsArr)
	if len(detailsArr) != 1 {
		t.Fatalf("show_task returned %d rows, want 1", len(detailsArr))
	}
	details := detailsArr[0]
	if len(details.Comments) != 1 {
		t.Fatalf("show_task comments = %d, want 1", len(details.Comments))
	}
	if details.Comments[0].ID != res.ID {
		t.Errorf("comment id = %q, want %q", details.Comments[0].ID, res.ID)
	}
	if details.Comments[0].Author != "pi" {
		t.Errorf("comment author = %q, want pi (server identity)", details.Comments[0].Author)
	}
	if details.Comments[0].Note != "hello" {
		t.Errorf("comment note = %q, want hello", details.Comments[0].Note)
	}
}

// TestMCPAddCommentRefusedOnDisabledList pins the list-level disable flag
// enforcement over MCP (docs/plan/task-comments.md §4).
func TestMCPAddCommentRefusedOnDisabledList(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)

	// No MCP tool toggles comments_disabled yet (deferred per the plan);
	// disable via a direct SQL write to the live store.
	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE List SET comments_disabled = 1 WHERE id = ?`, list["id"]); err != nil {
		t.Fatalf("disable: %v", err)
	}

	msg := callToolErr(t, session, "add_comment", map[string]any{
		"task_id": task["id"], "note": "hello",
	})
	if !strings.Contains(msg, "disabled") {
		t.Errorf("expected 'disabled' error, got %q", msg)
	}
}

// TestMCPAddCommentRejectsExplicitAuthor pins the plan's recommendation to
// reject an explicit author rather than silently ignoring it — comments are
// always attributed to the server's identity.
func TestMCPAddCommentRejectsExplicitAuthor(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)

	msg := callToolErr(t, session, "add_comment", map[string]any{
		"task_id": task["id"], "note": "hello", "author": "someone-else",
	})
	if !strings.Contains(msg, "author") {
		t.Errorf("expected error mentioning author, got %q", msg)
	}
}

// TestMCPAddCommentRefusedOnMissingTask verifies the existence check.
func TestMCPAddCommentRefusedOnMissingTask(t *testing.T) {
	session := setupMCP(t)
	msg := callToolErr(t, session, "add_comment", map[string]any{
		"task_id": "01ARZ", "note": "hello",
	})
	if !strings.Contains(msg, "not found") {
		t.Errorf("expected 'not found' error, got %q", msg)
	}
}

// TestMCPDeleteCommentRefusesAnotherAuthor pins the ownership rule that
// makes delete_comment safe to expose to agents at all: an identity may
// never delete a comment it did not write, even on a list it owns.
func TestMCPDeleteCommentRefusesAnotherAuthor(t *testing.T) {
	dataDir := t.TempDir()

	piSession := sessionAs(t, dataDir, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, piSession, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, piSession, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)
	var comment map[string]string
	mustUnmarshal(t, callTool(t, piSession, "add_comment", map[string]any{
		"task_id": task["id"], "note": "pi's comment",
	}), &comment)

	claudeSession := sessionAs(t, dataDir, "claude")
	msg := callToolErr(t, claudeSession, "delete_comment", map[string]any{
		"id": comment["id"], "force": true,
	})
	if !strings.Contains(msg, "owned by pi") {
		t.Errorf("expected error naming pi as owner, got %q", msg)
	}
	if !strings.Contains(msg, "only delete your own comments") {
		t.Errorf("expected the ownership-gate wording, got %q", msg)
	}

	// The comment must still exist — a refused delete is not a partial one.
	var detailsArr []struct {
		Comments []struct{ ID string } `json:"comments"`
	}
	mustUnmarshal(t, callTool(t, piSession, "show_task", map[string]any{
		"ids": []string{task["id"]},
	}), &detailsArr)
	if len(detailsArr[0].Comments) != 1 {
		t.Fatalf("comment should survive a refused delete, got %+v", detailsArr[0].Comments)
	}
}

// TestMCPDeleteCommentOwnSucceeds pins the success path: an identity may
// delete its own comment.
func TestMCPDeleteCommentOwnSucceeds(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)
	var comment map[string]string
	mustUnmarshal(t, callTool(t, session, "add_comment", map[string]any{
		"task_id": task["id"], "note": "mine",
	}), &comment)

	var ok map[string]bool
	mustUnmarshal(t, callTool(t, session, "delete_comment", map[string]any{
		"id": comment["id"], "force": true,
	}), &ok)
	if !ok["ok"] {
		t.Errorf("delete_comment = %+v, want ok:true", ok)
	}

	var detailsArr []struct {
		Comments []struct{ ID string } `json:"comments"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{task["id"]},
	}), &detailsArr)
	if len(detailsArr[0].Comments) != 0 {
		t.Errorf("comment should be gone, got %+v", detailsArr[0].Comments)
	}
}

// TestMCPDeleteCommentRequiresForce mirrors TestMCPDeleteTaskRequiresForce.
func TestMCPDeleteCommentRequiresForce(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)
	var comment map[string]string
	mustUnmarshal(t, callTool(t, session, "add_comment", map[string]any{
		"task_id": task["id"], "note": "mine",
	}), &comment)

	// A missing force key is refused by the tool schema itself before the
	// handler runs (the same behavior TestMCPDeleteTaskRequiresForce pins for
	// delete_task) — callToolErr only asserts that it errors, not the exact
	// message, since that message comes from schema validation, not this code.
	callToolErr(t, session, "delete_comment", map[string]any{"id": comment["id"]})

	var detailsArr []struct {
		Comments []struct{ ID string } `json:"comments"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{task["id"]},
	}), &detailsArr)
	if len(detailsArr[0].Comments) != 1 {
		t.Errorf("comment deleted despite missing force, got %+v", detailsArr[0].Comments)
	}
}

// hasClaim reports whether list_work shows a live claim on taskID by agent.
func hasClaim(t *testing.T, session *mcp.ClientSession, taskID, agent string) bool {
	t.Helper()
	var work []map[string]any
	mustUnmarshal(t, workJSON(t, session), &work)
	for _, w := range work {
		if w["entity_id"] == taskID && w["agent_id"] == agent {
			return true
		}
	}
	return false
}

func TestAddCommentAutoClaims(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "work",
	}), &task)
	// add_task now auto-claims too, so release first to prove add_comment claims.
	releaseWork(t, session, map[string]any{
		"entity_type": "task", "entity_id": task["id"], "agent_id": "pi",
	})
	callTool(t, session, "add_comment", map[string]any{
		"task_id": task["id"], "note": "checking in",
	})
	if !hasClaim(t, session, task["id"], "pi") {
		t.Errorf("add_comment should have auto-claimed the task")
	}
}

func TestAddTaskAutoClaims(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "new",
	}), &task)
	if !hasClaim(t, session, task["id"], "pi") {
		t.Errorf("add_task should have auto-claimed the new task")
	}
}

func TestEditTaskAutoClaims(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	newTask := func(title string) string {
		var tk map[string]string
		mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
			"list_id": list["id"], "title": title,
		}), &tk)
		// Clear the add_task auto-claim so each sub-case proves its own write claims.
		releaseWork(t, session, map[string]any{
			"entity_type": "task", "entity_id": tk["id"], "agent_id": "pi",
		})
		return tk["id"]
	}

	rename := newTask("a")
	callTool(t, session, "edit_task", map[string]any{"id": rename, "title": "a2"})
	if !hasClaim(t, session, rename, "pi") {
		t.Errorf("edit_task title should auto-claim")
	}

	notes := newTask("b")
	callTool(t, session, "edit_task", map[string]any{"id": notes, "notes": "n"})
	if !hasClaim(t, session, notes, "pi") {
		t.Errorf("edit_task notes should auto-claim")
	}

	parent := newTask("p")
	child := newTask("c")
	callTool(t, session, "edit_task", map[string]any{"id": child, "parent": parent})
	if !hasClaim(t, session, child, "pi") {
		t.Errorf("edit_task parent should auto-claim the moved task")
	}
}

// TestEditTaskTitleOnlyLeavesNotes verifies a title-only edit does not touch
// the notes body (§4.3: omitted fields are left unchanged).
func TestEditTaskTitleOnlyLeavesNotes(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "orig", "notes": "keep me",
	}), &task)
	callTool(t, session, "edit_task", map[string]any{"id": task["id"], "title": "renamed"})
	var detailArr []map[string]any
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{"ids": []string{task["id"]}}), &detailArr)
	d := detailArr[0]
	if d["title"] != "renamed" {
		t.Errorf("title = %v, want renamed", d["title"])
	}
	if d["notes"] != "keep me" {
		t.Errorf("notes = %v, want keep me (unchanged)", d["notes"])
	}
}

// TestEditTaskNotesClear verifies notes=” clears the notes body.
func TestEditTaskNotesClear(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "t", "notes": "erase me",
	}), &task)
	callTool(t, session, "edit_task", map[string]any{"id": task["id"], "notes": ""})
	var detailArr []map[string]any
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{"ids": []string{task["id"]}}), &detailArr)
	if detailArr[0]["notes"] != "" {
		t.Errorf("notes = %v, want empty", detailArr[0]["notes"])
	}
}

// TestEditTaskReparentAndForeignParent checks the happy re-parent path and the
// refusal when the target parent lives on a foreign list (§4.3 target-list rule).
func TestEditTaskReparentAndForeignParent(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var parent, child map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "p",
	}), &parent)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "c",
	}), &child)

	// Happy path: re-parent within the owner's own list.
	callTool(t, session, "edit_task", map[string]any{"id": child["id"], "parent": parent["id"]})
	rows := listTasks(t, session, map[string]any{"list_id": list["id"]}).Tasks
	var childRow map[string]any
	for _, r := range rows {
		if r["id"] == child["id"] {
			childRow = r
		}
	}
	if childRow == nil || childRow["parent_id"] != parent["id"] {
		t.Errorf("after re-parent: childRow = %#v, want parent_id=%v", childRow, parent["id"])
	}

	// Foreign parent: seed a task on a claude-owned list, then try to re-parent
	// the pi-owned child under it — must be refused with the owner name.
	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	foreignListID := store.NewID()
	foreignTaskID := store.NewID()
	now := time.Now().Unix()
	if _, err := db.Exec(
		`INSERT INTO List (id, name, created_by, position, created_at)
		 VALUES (?, ?, 'claude', 0, ?)`,
		foreignListID, "claude: Backlog", now,
	); err != nil {
		t.Fatalf("seed foreign list: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind, progress_pct, position, created_at, updated_at, completed_at)
		 VALUES (?, ?, NULL, 'foreign', '', 'pending', 'none', NULL, 0, ?, ?, NULL)`,
		foreignTaskID, foreignListID, now, now,
	); err != nil {
		t.Fatalf("seed foreign task: %v", err)
	}
	msg := callToolErr(t, session, "edit_task", map[string]any{"id": child["id"], "parent": foreignTaskID})
	if !strings.Contains(msg, "owned by claude") {
		t.Errorf("foreign-parent edit error = %q, want it to name the owner", msg)
	}
	// Child must not have moved.
	rows = listTasks(t, session, map[string]any{"list_id": list["id"]}).Tasks
	for _, r := range rows {
		if r["id"] == child["id"] && r["parent_id"] != parent["id"] {
			t.Errorf("child moved after refused re-parent: parent_id = %v", r["parent_id"])
		}
	}
}

// TestEditTaskToRoot moves a task to its list root.
func TestEditTaskToRoot(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var parent, child map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "p"}), &parent)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "c", "parent": parent["id"]}), &child)
	callTool(t, session, "edit_task", map[string]any{"id": child["id"], "to_root": true})
	rows := listTasks(t, session, map[string]any{"list_id": list["id"]}).Tasks
	var childRow map[string]any
	for _, r := range rows {
		if r["id"] == child["id"] {
			childRow = r
		}
	}
	if childRow == nil || childRow["parent_id"] != nil {
		t.Errorf("after to_root: childRow = %#v, want parent_id=nil (root)", childRow)
	}
}

// TestEditTaskParentAndToRoot errors when both are supplied.
func TestEditTaskParentAndToRoot(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var a, b map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "a"}), &a)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "b"}), &b)
	msg := callToolErr(t, session, "edit_task", map[string]any{"id": a["id"], "parent": b["id"], "to_root": true})
	if !strings.Contains(msg, "parent or to_root, not both") {
		t.Errorf("both-parent-and-to_root error = %q, want the mutual-exclusion message", msg)
	}
}

// TestEditTaskNoField errors when no field is supplied.
func TestEditTaskNoField(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "t"}), &task)
	msg := callToolErr(t, session, "edit_task", map[string]any{"id": task["id"]})
	if !strings.Contains(msg, "at least one of title, notes, parent, to_root") {
		t.Errorf("no-field error = %q, want the needs-a-field message", msg)
	}
}

// TestEditTaskForeignListRefusesContent guards §6: title/notes edits on a task
// the server does not own are refused (mirrors the old rename_task/set_notes
// refusal, now under edit_task).
func TestEditTaskForeignListRefusesContent(t *testing.T) {
	session := setupMCPAs(t, "pi")
	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	foreignListID := store.NewID()
	foreignTaskID := store.NewID()
	now := time.Now().Unix()
	if _, err := db.Exec(
		`INSERT INTO List (id, name, created_by, position, created_at)
		 VALUES (?, ?, 'claude', 0, ?)`,
		foreignListID, "claude: Backlog", now,
	); err != nil {
		t.Fatalf("seed foreign list: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind, progress_pct, position, created_at, updated_at, completed_at)
		 VALUES (?, ?, NULL, 'real work', '', 'pending', 'none', NULL, 0, ?, ?, NULL)`,
		foreignTaskID, foreignListID, now, now,
	); err != nil {
		t.Fatalf("seed foreign task: %v", err)
	}
	msg := callToolErr(t, session, "edit_task", map[string]any{"id": foreignTaskID, "title": "hijack"})
	if !strings.Contains(msg, "owned by claude") {
		t.Errorf("foreign-list title edit error = %q, want it to name the owner", msg)
	}
}

func TestDeleteTaskDoesNotClaim(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "doomed",
	}), &task)
	callTool(t, session, "delete_task", map[string]any{"id": task["id"], "force": true})
	if hasClaim(t, session, task["id"], "pi") {
		t.Errorf("delete_task must NOT leave a claim on the deleted task")
	}
}

// assertOKRows asserts that every row in a batch result is {id,ok:true}.
func assertOKRows(t *testing.T, res []map[string]any) {
	t.Helper()
	for _, r := range res {
		if r["ok"] != true {
			t.Errorf("row not ok: %#v", r)
		}
	}
}

func TestCompleteTaskBatchOK(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var ids []string
	for _, title := range []string{"a", "b", "c"} {
		var tk map[string]string
		mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
			"list_id": list["id"], "title": title,
		}), &tk)
		ids = append(ids, tk["id"])
	}
	var res []map[string]any
	mustUnmarshal(t, callTool(t, session, "complete_task", map[string]any{
		"ids": ids,
	}), &res)
	if len(res) != 3 {
		t.Fatalf("want 3 result rows, got %d", len(res))
	}
	assertOKRows(t, res)
	// All three now complete.
	rows := listTasks(t, session, map[string]any{
		"list_id": list["id"], "status": "complete",
	}).Tasks
	if len(rows) != 3 {
		t.Errorf("want 3 complete tasks, got %d", len(rows))
	}
}

func TestSetProgressBatchPercentage(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var a, b map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "a"}), &a)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "b"}), &b)
	var res []map[string]any
	mustUnmarshal(t, callTool(t, session, "set_progress", map[string]any{
		"ids": []string{a["id"], b["id"]}, "mode": "percentage", "percent": 50,
	}), &res)
	assertOKRows(t, res)
	var detailArr []map[string]any
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{"ids": []string{a["id"]}}), &detailArr)
	if len(detailArr) != 1 {
		t.Fatalf("show_task returned %d rows, want 1", len(detailArr))
	}
	detail := detailArr[0]
	prog := detail["progress"].(map[string]any)
	if int(prog["percent"].(float64)) != 50 {
		t.Errorf("want 50%%, got %v", prog["percent"])
	}
}

func TestBatchStatusPartialFailure(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var good map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "g",
	}), &good)
	// complete_task returns per-id rows (not a tool error), so a bad id is a
	// row with an error, not a call failure.
	var res []map[string]any
	mustUnmarshal(t, callTool(t, session, "complete_task", map[string]any{
		"ids": []string{good["id"], "does-not-exist"},
	}), &res)
	if len(res) != 2 {
		t.Fatalf("want 2 rows, got %d", len(res))
	}
	if res[0]["ok"] != true {
		t.Errorf("good id should succeed: %#v", res[0])
	}
	if _, hasErr := res[1]["error"]; !hasErr {
		t.Errorf("bad id should be an error row: %#v", res[1])
	}
	// The good write was NOT rolled back.
	rows := listTasks(t, session, map[string]any{
		"list_id": list["id"], "status": "complete",
	}).Tasks
	if len(rows) != 1 {
		t.Errorf("good task should still be complete despite the bad id; got %d complete", len(rows))
	}
}

func TestBatchStatusCap(t *testing.T) {
	session := setupMCPAs(t, "noagent")
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = "x"
	}
	msg := callToolErr(t, session, "complete_task", map[string]any{"ids": ids})
	if !strings.Contains(msg, "capped at 50") {
		t.Errorf("expected cap error, got %q", msg)
	}
}

func TestSetProgressPercentRequired(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var tk map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "a",
	}), &tk)
	msg := callToolErr(t, session, "set_progress", map[string]any{
		"ids": []string{tk["id"]}, "mode": "percentage",
	})
	if !strings.Contains(msg, "requires percent") {
		t.Errorf("percentage without percent should error, got %q", msg)
	}
}

func TestBatchStatusAutoClaims(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var tk map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "a",
	}), &tk)
	callTool(t, session, "complete_task", map[string]any{"ids": []string{tk["id"]}})
	var work []map[string]any
	mustUnmarshal(t, workJSON(t, session), &work)
	found := false
	for _, w := range work {
		if w["entity_id"] == tk["id"] && w["agent_id"] == "pi" {
			found = true
		}
	}
	if !found {
		t.Errorf("complete_task should auto-claim each touched task; work=%#v", work)
	}
}

func TestReopenTaskBatchOK(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var tk map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "a",
	}), &tk)
	callTool(t, session, "complete_task", map[string]any{"ids": []string{tk["id"]}})
	var res []map[string]any
	mustUnmarshal(t, callTool(t, session, "reopen_task", map[string]any{
		"ids": []string{tk["id"]},
	}), &res)
	if len(res) != 1 || res[0]["ok"] != true {
		t.Fatalf("reopen_task batch row not ok: %#v", res)
	}
	rows := listTasks(t, session, map[string]any{
		"list_id": list["id"], "status": "pending",
	}).Tasks
	if len(rows) != 1 {
		t.Errorf("want 1 pending task after reopen, got %d", len(rows))
	}
}

func TestMain(m *testing.M) {
	// Tests set XDG_DATA_HOME explicitly; make sure the default HOME-based
	// path is not accidentally used when t.Setenv is active.
	os.Exit(m.Run())
}

// TestInboxAssigneeLiveAcrossLists pins that assignee_live is correct on
// every list in the inbox, not just the first. The inbox calls sectionRows
// once per list, so the presence map is read once by the resource and passed
// down; reading it inside sectionRows instead would run one ListWork query
// per list (docs/plan/mcp-assignment-and-priorities.md §8).
//
// Liveness is a property of the AGENT, not the task, so the two rows need two
// different assignees: pi (writing, therefore present) on its own list, and a
// tag that never appears in AgentActivity on the foreign one. The second is
// the stale-assignment tier (§3), and it has to stay false across the
// mine/foreign_lists split.
func TestInboxAssigneeLiveAcrossLists(t *testing.T) {
	data := t.TempDir()
	piSession := sessionAs(t, data, "pi")
	claudeSession := sessionAs(t, data, "claude")

	var mineList, foreignList map[string]string
	mustUnmarshal(t, callTool(t, piSession, "add_list", map[string]any{"name": "pi: Board"}), &mineList)
	mustUnmarshal(t, callTool(t, claudeSession, "add_list", map[string]any{"name": "claude: Board"}), &foreignList)

	var liveTask, staleTask map[string]string
	mustUnmarshal(t, callTool(t, piSession, "add_task", map[string]any{
		"list_id": mineList["id"], "title": "Held by a live agent",
	}), &liveTask)
	mustUnmarshal(t, callTool(t, claudeSession, "add_task", map[string]any{
		"list_id": foreignList["id"], "title": "Held by a dead agent",
	}), &staleTask)

	db, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := db.AssignTask(liveTask["id"], "pi", false); err != nil {
		t.Fatalf("AssignTask pi: %v", err)
	}
	// "ghost" never holds a presence claim, so this row is the stale tier.
	if err := db.AssignTask(staleTask["id"], "ghost", false); err != nil {
		t.Fatalf("AssignTask ghost: %v", err)
	}
	if _, err := db.ClaimWork("task", liveTask["id"], "pi", store.ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}
	db.Close()

	type row struct {
		ID           string `json:"id"`
		Assignee     string `json:"assignee"`
		AssigneeLive bool   `json:"assignee_live"`
	}
	type block struct {
		Tasks []row `json:"tasks"`
	}
	var inbox struct {
		Mine    block   `json:"mine"`
		Foreign []block `json:"foreign_lists"`
	}
	mustUnmarshal(t, readResourceText(t, piSession, "crush:///inbox"), &inbox)

	seen := map[string]row{}
	for _, b := range append([]block{inbox.Mine}, inbox.Foreign...) {
		for _, r := range b.Tasks {
			seen[r.ID] = r
		}
	}

	got, ok := seen[liveTask["id"]]
	if !ok {
		t.Fatal("inbox missing the task on pi's own list")
	}
	if got.Assignee != "pi" || !got.AssigneeLive {
		t.Errorf("live row: assignee=%q live=%v, want pi/true", got.Assignee, got.AssigneeLive)
	}

	got, ok = seen[staleTask["id"]]
	if !ok {
		t.Fatal("inbox missing the task on the foreign list")
	}
	if got.Assignee != "ghost" || got.AssigneeLive {
		t.Errorf("stale row: assignee=%q live=%v, want ghost/false — the stale tier would never fire", got.Assignee, got.AssigneeLive)
	}
}

// TestUnassignedRowIsNeverLive pins that an unassigned row reports
// assignee_live false even while another agent is demonstrably live. The
// field answers "is THIS row's holder at the keyboard", and a row with no
// holder has no answer but false — the TUI's stale tier reads
// assignee != "" && !assignee_live, so a true here would be invisible there
// but wrong everywhere else.
func TestUnassignedRowIsNeverLive(t *testing.T) {
	data := t.TempDir()
	session := sessionAs(t, data, "pi")

	var list, held, free map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "pi: Board"}), &list)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "Owned",
	}), &held)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "Nobody owns this",
	}), &free)

	db, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := db.AssignTask(held["id"], "pi", false); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	if _, err := db.ClaimWork("task", held["id"], "pi", store.ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}
	db.Close()

	var details []struct {
		Assignee     string `json:"assignee"`
		AssigneeLive bool   `json:"assignee_live"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{held["id"], free["id"]},
	}), &details)

	if details[0].Assignee != "pi" || !details[0].AssigneeLive {
		t.Fatalf("held task: assignee=%q live=%v, want pi/true", details[0].Assignee, details[0].AssigneeLive)
	}
	if details[1].Assignee != "" {
		t.Fatalf("free task assignee = %q, want empty", details[1].Assignee)
	}
	if details[1].AssigneeLive {
		t.Error("unassigned task reports assignee_live true while another agent is live")
	}
}

// TestListTasksSinceSurfacesCompletedTasks pins the DESIGN §9 change-detection
// contract against the §4 `open` default. DESIGN writes the call as
// `list_tasks(list_id, since=<unix>)` and promises it returns tasks whose
// activity changed — explicitly including "status/progress edited". Completing
// a task is the most common change there is, so composing `since` with the
// default `open` filter (which excludes complete) would make the documented
// two-argument call silently blind to it: `since` absorbed `list_changes`, and
// `list_changes` never had a status filter. An explicit status still wins.
func TestListTasksSinceSurfacesCompletedTasks(t *testing.T) {
	session := setupMCP(t)
	var list, a map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	mustUnmarshal(t, callTool(t, session, "add_task",
		map[string]any{"list_id": list["id"], "title": "a"}), &a)

	cutoff := time.Now().Unix()
	time.Sleep(1100 * time.Millisecond)
	callTool(t, session, "complete_task", map[string]any{"ids": []string{a["id"]}})

	rows := listTasks(t, session, map[string]any{
		"list_id": list["id"], "since": cutoff,
	}).Tasks
	if len(rows) != 1 || rows[0]["id"] != a["id"] {
		t.Fatalf("completing a task must surface in list_tasks(since); got %#v", rows)
	}
	if rows[0]["context_only"] == true {
		t.Errorf("the changed task is a match in its own right, not a skeleton")
	}

	// An explicit status is still honoured over the since-widened default.
	open := listTasks(t, session, map[string]any{
		"list_id": list["id"], "since": cutoff, "status": "open",
	}).Tasks
	if len(open) != 0 {
		t.Errorf("explicit status=open must still exclude the completed task, got %#v", open)
	}
}

// TestListTasksElidedNamesOnlyRowsWithBodies pins what `elided` is for: it
// names rows whose body the §5.3 budget withheld, so the agent can fetch them
// with show_task. A row with no notes and no comments had no body to withhold,
// so naming it sends the agent on a pointless round-trip — the exact cost §2
// exists to remove.
func TestListTasksElidedNamesOnlyRowsWithBodies(t *testing.T) {
	session := setupMCP(t)
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	big := strings.Repeat("x", 25000)
	for _, title := range []string{"big1", "big2", "big3"} {
		callTool(t, session, "add_task", map[string]any{
			"list_id": list["id"], "title": title, "notes": big,
		})
	}
	callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "bare"})

	res := listTasks(t, session, map[string]any{
		"list_id": list["id"], "include": []string{"notes"},
	})
	if !res.BudgetExceeded {
		t.Fatalf("three 25000-char notes must exceed the 40000 budget")
	}
	for _, id := range res.Elided {
		for _, r := range res.Tasks {
			if r["id"] == id && r["title"] == "bare" {
				t.Errorf("a row with no body must not be named in elided: %v", r["title"])
			}
		}
	}
}

// TestInboxSkeletonRowsCarryNoNotes pins §5.2 on the one remaining resource:
// list_tasks' per-task filter also drives crush:///inbox, so the inbox now
// produces context_only skeleton rows too. A skeleton is tree scaffolding, not
// content — it must never carry an inlined body on any surface.
func TestInboxSkeletonRowsCarryNoNotes(t *testing.T) {
	session := setupMCP(t)
	var list, parent map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "parent", "notes": "parent body",
	}), &parent)
	callTool(t, session, "complete_task", map[string]any{"ids": []string{parent["id"]}})
	callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "child", "parent": parent["id"],
	})

	body := readResourceText(t, session, "crush:///inbox")
	var inbox struct {
		Mine struct {
			Tasks []map[string]any `json:"tasks"`
		} `json:"mine"`
	}
	mustUnmarshal(t, body, &inbox)
	found := false
	for _, r := range inbox.Mine.Tasks {
		if r["context_only"] != true {
			continue
		}
		found = true
		if r["notes"] != nil && r["notes"] != "" {
			t.Errorf("inbox skeleton row %v must not carry notes, got %q", r["title"], r["notes"])
		}
	}
	if !found {
		t.Fatalf("expected a context_only skeleton in the inbox; got %#v", inbox.Mine.Tasks)
	}
}
