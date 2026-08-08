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

	// Pin the identity. Unset, CRUSH_AGENT now yields a per-process tag
	// (docs/plan/session-scoped-agent-identity.md decision 1), which would make
	// every assertion about the "agent" tag unrepeatable. These tests are about
	// behaviour under a known identity, not about what the default is —
	// TestServerIdentityIsUniquePerProcess covers the default itself.
	if os.Getenv("CRUSH_AGENT") == "" {
		t.Setenv("CRUSH_AGENT", "agent")
	}

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
// same data, e.g. the comment tool's delete-mode cross-author refusal.
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

// claimWork seeds a presence claim directly in the store, under the
// test's shared DB - the layer the removed claim_work tool used to write.
// Presence and assignment are distinct axes, so these tests seed claims at
// store level and exercise assignment through the tools.
func claimWork(t *testing.T, entityType, entityID, agent string) {
	t.Helper()
	st, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.ClaimWork(entityType, entityID, agent, store.ActivityWorking); err != nil {
		t.Fatalf("seed claim %s/%s under %s: %v", entityType, entityID, agent, err)
	}
}

// releaseWork clears a presence claim at store level (the claim_work
// release=true path). Releasing a claim nobody holds is a no-op, so
// seeding order does not matter.
func releaseWork(t *testing.T, entityType, entityID, agent string) {
	t.Helper()
	st, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.ReleaseWork(entityType, entityID, agent); err != nil {
		t.Fatalf("seed release %s/%s under %s: %v", entityType, entityID, agent, err)
	}
}

// mcpToolSurface is the exact tool surface docs/DESIGN.md §9 pins, and
// removedToolNames is every tool name the consolidation steps deleted. Both
// lists are shared by the three assertions that need them — the registered
// surface, the Instructions blob and the crush_inbox prompt — because a
// consolidation step that updated only some of the copies would leave the
// others passing against a surface that no longer exists. One list per fact.
var (
	mcpToolSurface = []string{
		"my_list", "list_tasks", "show_task", "search_tasks",
		"add_task", "edit_task", "delete_task", "set_status",
		"comment", "add_list", "assign_task", "next_task",
	}
	removedToolNames = []string{
		"list_lists", "show_tasks", "toggle_task", "update_tasks", "rename_task",
		"set_notes", "move_task", "rename_list", "delete_list", "release_work",
		"list_work", "set_progress", "complete_task", "reopen_task",
		"claim_work", "list_changes", "add_comment", "delete_comment",
	}
	// The resource surface is the same fact in the same shape: two survivors
	// (docs/plan/mcp-assignment-and-priorities.md §8) and five deletions. The
	// agent-facing text has to name the survivors and must not send a session
	// at a URI the server stopped serving.
	mcpResourceSurface  = []string{"crush:///inbox", "crush://work"}
	removedResourceURIs = []string{
		"crush:///lists", "crush:///tasks/", "crush:///search",
	}
)

// TestMCPToolSurface pins the consolidated tool surface
// (docs/plan/mcp-tool-consolidation.md §2, docs/plan/mcp-assignment-and-priorities.md
// §4): exactly the 12 tools below, and none of the removed ones. A new tool
// must be a deliberate edit here — the ceiling is the point of the plan.
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

	for _, name := range mcpToolSurface {
		if !got[name] {
			t.Errorf("tool %q missing from the surface", name)
		}
	}
	for _, name := range removedToolNames {
		if got[name] {
			t.Errorf("removed tool %q is still registered", name)
		}
	}
	if len(res.Tools) != len(mcpToolSurface) {
		t.Errorf("tool count = %d, want %d: %v", len(res.Tools), len(mcpToolSurface), got)
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
	lower := strings.ToLower(instructions)
	if !strings.Contains(lower, "chore_crusher_") {
		t.Fatalf("Instructions must document the chore_crusher_ prefix;\nfull text:\n%s", instructions)
	}
	for _, name := range mcpToolSurface {
		if !strings.Contains(lower, name+"(") {
			t.Fatalf("Instructions missing tool %q;\nfull text:\n%s", name, instructions)
		}
	}

	// Removed tools (no MCP registration) must not appear as callables - a
	// bare prose mention (e.g. "list_changes is folded into since") is a
	// deliberate pointer, a listing as `name(` is not.
	for _, name := range removedToolNames {
		if strings.Contains(lower, name+"(") {
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
		"set_status",
		// The loop is now grab-first (plan §3): the blob has to say that
		// assignment is durable ownership, distinct from the presence
		// spinner, or an agent reads assignee as another word for the claim
		// and skips the grab that stops two agents researching one task.
		"grab",
		"assignee_live",
		"no ttl",
		"takeover comment",
		// Priority exists to answer "what next", and no tool sets it, so the
		// blob is the only place an agent learns both facts.
		"priority",
		"high > medium > low > none",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("Instructions missing loop pointer element %q;\nfull text:\n%s", want, instructions)
		}
	}

	// Resources: the blob names both survivors and none of the five deleted
	// URIs — a documented URI the server no longer serves costs a session a
	// failed read before it learns better.
	for _, uri := range mcpResourceSurface {
		if !strings.Contains(lower, uri) {
			t.Fatalf("Instructions missing resource %q;\nfull text:\n%s", uri, instructions)
		}
	}
	for _, uri := range removedResourceURIs {
		if strings.Contains(lower, uri) {
			t.Fatalf("Instructions still names removed resource %q;\nfull text:\n%s", uri, instructions)
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
		"set_status",
		"percentage",
		"crush:///inbox",
		"before the next task",
		// The prompt is the call-by-call procedure, so it carries the loop
		// the blob only summarises: grab first, release what you abandon,
		// and the two conflicts that read alike but resolve differently.
		"next_task",
		"before you research",
		"release=true",
		"assignee_live",
		"subtree reservation",
	} {
		if !strings.Contains(loop, want) {
			t.Fatalf("crush_inbox prompt missing working-loop element %q;\nfull text:\n%s", want, tc.Text)
		}
	}

	// The prompt, not the Instructions blob, is where the working loop
	// actually lives (the blob delegates to it above), so it is the text an
	// agent follows call-by-call — it must not name a tool the server no
	// longer registers. Every consolidation step so far has had to edit this
	// prompt, and until now only the blob was guarded, so a stale tool name
	// here would have shipped green.
	for _, name := range removedToolNames {
		if strings.Contains(loop, name) {
			t.Fatalf("crush_inbox prompt still names removed tool %q;\nfull text:\n%s", name, tc.Text)
		}
	}
	for _, uri := range removedResourceURIs {
		if strings.Contains(loop, uri) {
			t.Fatalf("crush_inbox prompt still names removed resource %q;\nfull text:\n%s", uri, tc.Text)
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

	callTool(t, session, "set_status", map[string]any{"ids": []string{task["id"]}, "status": "complete"})

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
	claimWork(t, "task", theirTaskID, "claude")
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{theirTaskID},
	}), &detailsArr)
	if !detailsArr[0].AssigneeLive {
		t.Fatal("show_task assignee_live = false after claude claimed work, want true")
	}
	if detailsArr[0].Assignee != "claude" {
		t.Fatalf("presence claim changed the assignee to %q — presence must not touch assignment", detailsArr[0].Assignee)
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

func TestMCPAssignAndReleaseWork(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)

	// assign_task hands back the full task payload, already assigned to the
	// server identity — grabbing and reading a task is one call (§3).
	var got []struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Assignee   string `json:"assignee"`
		AssignedAt *int64 `json:"assigned_at"`
	}
	mustUnmarshal(t, callTool(t, session, "assign_task", map[string]any{
		"ids": []string{task["id"]},
	}), &got)
	if len(got) != 1 || got[0].ID != task["id"] || got[0].Title != "Write docs" {
		t.Fatalf("assign_task payload = %+v", got)
	}
	if got[0].Assignee != "agent" || got[0].AssignedAt == nil {
		t.Fatalf("assign_task must set assignee to the server identity; got %+v", got[0])
	}

	// release=true unassigns.
	var rel []struct {
		ID string `json:"id"`
		OK bool   `json:"ok"`
	}
	mustUnmarshal(t, callTool(t, session, "assign_task", map[string]any{
		"ids": []string{task["id"]}, "release": true,
	}), &rel)
	if len(rel) != 1 || !rel[0].OK {
		t.Fatalf("assign_task(release) = %+v, want {id,ok:true}", rel)
	}

	// The task is unassigned again (read it back, not assign again).
	var now []struct {
		Assignee   string `json:"assignee"`
		AssignedAt *int64 `json:"assigned_at"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{task["id"]},
	}), &now)
	if len(now) != 1 || now[0].Assignee != "" || now[0].AssignedAt != nil {
		t.Fatalf("after release the task must be unassigned, got %+v", now[0])
	}
}

func TestMCPAssignTaskConflict(t *testing.T) {
	dataDir := t.TempDir()

	// alice holds the task; bob tries to grab it.
	alice := sessionAs(t, dataDir, "alice")
	var list map[string]string
	mustUnmarshal(t, callTool(t, alice, "add_list", map[string]any{"name": "Work"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, alice, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)
	callTool(t, alice, "assign_task", map[string]any{"ids": []string{task["id"]}})

	// bob's assign is refused, naming the holder and the force escape. The
	// refusal is a per-row error, not a tool error (§2's fail-soft rule).
	bob := sessionAs(t, dataDir, "bob")
	var got []struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	mustUnmarshal(t, callTool(t, bob, "assign_task", map[string]any{"ids": []string{task["id"]}}), &got)
	if len(got) != 1 || got[0].Error == "" {
		t.Fatalf("assign conflict rows = %+v, want one error row", got)
	}
	if !strings.Contains(got[0].Error, `assigned to "alice"`) || !strings.Contains(got[0].Error, "force=true") {
		t.Fatalf("assign conflict error = %q, want holder + force hint", got[0].Error)
	}

	// bob's guarded status write refuses the same way (§7): per-row error.
	var srows []struct {
		Error string `json:"error"`
	}
	mustUnmarshal(t, callTool(t, bob, "set_status", map[string]any{
		"ids": []string{task["id"]}, "status": "complete",
	}), &srows)
	if len(srows) != 1 || !strings.Contains(srows[0].Error, `assigned to "alice"`) || !strings.Contains(srows[0].Error, "force=true") {
		t.Fatalf("set_status conflict rows = %+v, want holder + force hint", srows)
	}

	// alice still holds the task.
	var check []struct {
		Assignee string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, alice, "show_task", map[string]any{"ids": []string{task["id"]}}), &check)
	if len(check) != 1 || check[0].Assignee != "alice" {
		t.Fatalf("original assignment must survive a refused grab, got %+v", check)
	}
}

// TestMCPGuardedWriteForceTakeover covers the refuse-with-override rule
// (§2) end to end: with force=true bob's assign performs the takeover and
// records a takeover comment naming the previous holder; the same guard
// gates set_status, and force lets the write land.
func TestMCPGuardedWriteForceTakeover(t *testing.T) {
	dataDir := t.TempDir()

	alice := sessionAs(t, dataDir, "alice")
	var list map[string]string
	mustUnmarshal(t, callTool(t, alice, "add_list", map[string]any{"name": "Work"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, alice, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)
	callTool(t, alice, "assign_task", map[string]any{"ids": []string{task["id"]}})
	// alice's add_task auto-claimed presence; drop it so the takeover
	// comment says "no live session" deterministically (the comment
	// records the holder's staleness at the moment of takeover).
	releaseWork(t, "task", task["id"], "alice")

	// bob force-takes it through assign_task.
	bob := sessionAs(t, dataDir, "bob")
	var got []struct {
		Assignee string `json:"assignee"`
		Comments []struct {
			Author string `json:"author"`
			Note   string `json:"note"`
		} `json:"comments"`
	}
	mustUnmarshal(t, callTool(t, bob, "assign_task", map[string]any{
		"ids": []string{task["id"]}, "force": true,
	}), &got)
	if len(got) != 1 || got[0].Assignee != "bob" {
		t.Fatalf("force grab must reassign to bob, got %+v", got)
	}
	takeover := ""
	for _, c := range got[0].Comments {
		if c.Author == "bob" {
			takeover = c.Note
		}
	}
	if !strings.Contains(takeover, "bob took this task from alice") || !strings.Contains(takeover, "no live session") {
		t.Fatalf("takeover comment = %q, want 'bob took this task from alice (... no live session)'", takeover)
	}

	// The set_status guard is the same one: alice (the previous holder)
	// is refused, and bob's forced write lands.
	var srows []struct {
		Error string `json:"error"`
	}
	mustUnmarshal(t, callTool(t, alice, "set_status", map[string]any{
		"ids": []string{task["id"]}, "progress": "simple",
	}), &srows)
	if len(srows) != 1 || !strings.Contains(srows[0].Error, `assigned to "bob"`) {
		t.Fatalf("set_status conflict rows = %+v, want it to name bob", srows)
	}
	callTool(t, bob, "set_status", map[string]any{
		"ids": []string{task["id"]}, "progress": "simple", "force": true,
	})

	// edit_task and delete_task carry the same guard. They are also
	// structural (list-gated), so probe them on a list that both agents
	// may write: bob's own.
	var ownList map[string]string
	mustUnmarshal(t, callTool(t, bob, "add_list", map[string]any{"name": "bob: Own"}), &ownList)
	var ownTask map[string]string
	mustUnmarshal(t, callTool(t, bob, "add_task", map[string]any{
		"list_id": ownList["id"], "title": "own work",
	}), &ownTask)
	// alice durably assigns bob's task (assignment is not list-gated).
	callTool(t, alice, "assign_task", map[string]any{"ids": []string{ownTask["id"]}})

	// bob's edit is refused (a tool error - single-id tools fail hard),
	// and the forced edit takes the task over and lands.
	msg := callToolErr(t, bob, "edit_task", map[string]any{"id": ownTask["id"], "title": "x"})
	if !strings.Contains(msg, `assigned to "alice"`) {
		t.Fatalf("edit_task conflict error = %q, want it to name alice", msg)
	}
	var eout map[string]bool
	mustUnmarshal(t, callTool(t, bob, "edit_task", map[string]any{"id": ownTask["id"], "title": "renamed", "force": true}), &eout)
	if !eout["ok"] {
		t.Fatalf("forced edit = %+v, want ok:true", eout)
	}
	// The takeover landed: bob now durably holds the task.
	var held []struct {
		Assignee string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, bob, "show_task", map[string]any{"ids": []string{ownTask["id"]}}), &held)
	if len(held) != 1 || held[0].Assignee != "bob" {
		t.Fatalf("forced edit must take the task over, got %+v", held)
	}

	// delete_task: the takeover+delete combination - alice durably holds
	// bob's task (reassign her first), and bob's forced delete takes it
	// over and removes it.
	callTool(t, alice, "assign_task", map[string]any{"ids": []string{ownTask["id"]}, "force": true})
	callTool(t, bob, "delete_task", map[string]any{"id": ownTask["id"], "force": true})
	var all struct {
		Tasks []struct{ Title string } `json:"tasks"`
	}
	mustUnmarshal(t, callTool(t, bob, "list_tasks", map[string]any{"list_id": ownList["id"]}), &all)
	if len(all.Tasks) != 0 {
		t.Fatalf("delete left rows = %+v", all.Tasks)
	}
	// A foreign-list task stays undeletable for bob even under force - the
	// structural gate is a list-ownership matter, not an assignment one.
	msg = callToolErr(t, bob, "delete_task", map[string]any{"id": task["id"], "force": true})
	if !strings.Contains(msg, "owned by alice") {
		t.Fatalf("delete_task on alice's list = %q, want the list-owner error", msg)
	}
}

// TestMCPRefusedForcedWriteLeavesAssignmentAlone pins the ORDER of the two
// gates on edit_task and delete_task: list ownership first, the §7
// assignment guard last. The guard's force branch is itself a write — it
// reassigns the task and records a takeover comment — so running it ahead
// of a check that can still refuse leaves the task taken over by a write
// that never landed. That is the same half-happened write the re-parent
// gate exists to prevent (docs/DESIGN.md §9), and it is not covered by
// TestMCPGuardedWriteForceTakeover, whose foreign-list probe runs when the
// caller already holds the task and so returns before the force branch.
func TestMCPRefusedForcedWriteLeavesAssignmentAlone(t *testing.T) {
	dataDir := t.TempDir()

	alice := sessionAs(t, dataDir, "alice")
	var list map[string]string
	mustUnmarshal(t, callTool(t, alice, "add_list", map[string]any{"name": "Work"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, alice, "add_task", map[string]any{
		"list_id": list["id"], "title": "Write docs",
	}), &task)
	callTool(t, alice, "assign_task", map[string]any{"ids": []string{task["id"]}})

	// bob owns neither the list nor the task. Both forced structural writes
	// must be refused by list ownership.
	bob := sessionAs(t, dataDir, "bob")
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"edit_task", map[string]any{"id": task["id"], "title": "stolen", "force": true}},
		{"delete_task", map[string]any{"id": task["id"], "force": true}},
	} {
		msg := callToolErr(t, bob, tc.tool, tc.args)
		if !strings.Contains(msg, "owned by alice") {
			t.Fatalf("forced %s on alice's list = %q, want the list-owner refusal", tc.tool, msg)
		}
	}

	// The refusals left no trace: alice still holds the task, the title is
	// untouched, and no takeover comment was written.
	var got []struct {
		Title    string `json:"title"`
		Assignee string `json:"assignee"`
		Comments []struct {
			Author string `json:"author"`
			Note   string `json:"note"`
		} `json:"comments"`
	}
	mustUnmarshal(t, callTool(t, alice, "show_task", map[string]any{"ids": []string{task["id"]}}), &got)
	if len(got) != 1 || got[0].Assignee != "alice" {
		t.Fatalf("a refused forced write must not reassign, got %+v", got)
	}
	if got[0].Title != "Write docs" {
		t.Fatalf("title = %q, want the refused edit to have changed nothing", got[0].Title)
	}
	for _, c := range got[0].Comments {
		if strings.Contains(c.Note, "took this task") {
			t.Fatalf("a refused forced write recorded a takeover comment: %q", c.Note)
		}
	}
}

// TestMCPAssignTaskRelease guards the release semantics (§4): release=true
// succeeds silently when nobody holds the task; ids validation is enforced.
func TestMCPAssignTaskRelease(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "Write docs",
	}), &task)

	// add_task auto-claims presence for "pi" but assigns nothing; releasing
	// a task nobody holds is a silent no-op.
	var rel []struct {
		ID string `json:"id"`
		OK bool   `json:"ok"`
	}
	mustUnmarshal(t, callTool(t, session, "assign_task", map[string]any{
		"ids": []string{task["id"]}, "release": true,
	}), &rel)
	if len(rel) != 1 || !rel[0].OK {
		t.Fatalf("release on an unheld task = %+v, want {id,ok:true}", rel)
	}

	// Assign, then release: the task is free again.
	callTool(t, session, "assign_task", map[string]any{"ids": []string{task["id"]}})
	mustUnmarshal(t, callTool(t, session, "assign_task", map[string]any{
		"ids": []string{task["id"]}, "release": true,
	}), &rel)
	if len(rel) != 1 || !rel[0].OK {
		t.Fatalf("release after assign = %+v, want {id,ok:true}", rel)
	}

	// Validation: ids are required and capped at 50.
	msg := callToolErr(t, session, "assign_task", map[string]any{"ids": []string{}})
	if !strings.Contains(msg, "at least one id") {
		t.Fatalf("empty-ids error = %q, want the minimum named", msg)
	}
	tooMany := make([]string, 51)
	for i := range tooMany {
		tooMany[i] = task["id"]
	}
	msg = callToolErr(t, session, "assign_task", map[string]any{"ids": tooMany})
	if !strings.Contains(msg, "capped at 50") {
		t.Fatalf("51-ids error = %q, want the cap named", msg)
	}
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

	// Claim as the server identity (CRUSH_AGENT=pi), seeded at store level.
	claimWork(t, "task", task["id"], "pi")

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
		{"set_status", "set_status", map[string]any{"ids": []string{task["id"]}, "progress": "simple"}},
		{"set_status", "set_status", map[string]any{"ids": []string{task["id"]}, "status": "complete"}},
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

// TestMCPAssignDefaultsToIdentity pins §3's "assignment always lands on
// this server's identity" rule: assign_task and next_task take no agent_id,
// so the assignment always matches the CRUSH_AGENT that all the takeover
// and heartbeat logic keys off. The old claim_work's agent_id default
// (tested here before step 9) is a concept of the removed tool.
func TestMCPAssignDefaultsToIdentity(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "Write docs",
	}), &task)

	// Gray with assign_task: no agent_id anywhere.
	callTool(t, session, "assign_task", map[string]any{"ids": []string{task["id"]}})

	db, err := sql.Open("sqlite", config.DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	var assignee string
	if err := db.QueryRow(`SELECT assignee FROM Task WHERE id = ?`, task["id"]).Scan(&assignee); err != nil {
		t.Fatalf("read assignee: %v", err)
	}
	if assignee != "pi" {
		t.Fatalf("assign_task must assign to the server identity, got %q", assignee)
	}

	// next_task on a fully-held list returns the empty shape, not a task.
	var none struct {
		OK     bool   `json:"ok"`
		Reason string `json:"reason"`
	}
	mustUnmarshal(t, callTool(t, session, "next_task", map[string]any{"list_id": list["id"]}), &none)
	if none.OK || none.Reason != "no eligible task in this list" {
		t.Fatalf("next_task on a held-only list = %+v, want the empty shape", none)
	}

	// Releasing, next_task grabs it.
	callTool(t, session, "assign_task", map[string]any{"ids": []string{task["id"]}, "release": true})
	var grabbed struct {
		Assignee string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, session, "next_task", map[string]any{"list_id": list["id"]}), &grabbed)
	if grabbed.Assignee != "pi" {
		t.Fatalf("next_task must assign to the server identity, got %q", grabbed.Assignee)
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
	// No explicit presence claim first — just start setting status/progress.
	callTool(t, session, "set_status", map[string]any{
		"ids": []string{task["id"]}, "progress": "percentage", "percent": 25,
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
	releaseWork(t, "task", task["id"], "pi")

	// Another agent holds the claim; pi's writes must not touch it.
	claimWork(t, "task", task["id"], "other")

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

	callTool(t, session, "set_status", map[string]any{"ids": []string{task["id"]}, "status": "complete"})

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
	releaseWork(t, "task", task["id"], "agent")

	// Claim and verify the crush://work resource reports it. The resource is
	// only a reader now that claim_work is gone (step 9).
	claimWork(t, "task", task["id"], "claude")

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
// writes are never list-gated, so set_status succeeds on a list owned by
// another agent.
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
	callTool(t, session, "set_status", map[string]any{"ids": []string{taskID}, "progress": "simple"})
	callTool(t, session, "set_status", map[string]any{"ids": []string{taskID}, "status": "complete"})

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
	callTool(t, session, "set_status", map[string]any{"ids": []string{taskID}, "progress": "simple"})
	callTool(t, session, "set_status", map[string]any{"ids": []string{taskID}, "status": "complete"})

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
// ends, ReleaseAgentClaims (called by Run after server.Run returns) clears the
// agent's own claims so the TUI shows no stale spinners for that agent.
// The test seeds assignment and claims for two agents, then calls
// ReleaseAgentClaims for one agent's identity, and confirms the TUI sees
// only that agent's claims cleared while the other agent's claims remain -
// cleanup is presence-only (docs/DESIGN.md §3).
func TestMCPPendingClaimsClearedOnSessionEnd(t *testing.T) {
	session := setupMCPAs(t, "pi") // Set up as "pi" agent

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"],
		"title":   "Write docs",
	}), &task)

	// A durable assignment on the task, made through the tool.
	callTool(t, session, "assign_task", map[string]any{"ids": []string{task["id"]}})

	// add_task auto-claims under "agent"; release it so "pi" can claim below.
	releaseWork(t, "task", task["id"], "agent")

	// Claims seeded at store level - the claim_work tool is gone (step 9).
	claimWork(t, "task", task["id"], "pi")
	// A second claim on the list by a different agent.
	claimWork(t, "list", list["id"], "claude")

	// Both claims are visible before session-end cleanup.
	var work []struct {
		AgentID string `json:"agent_id"`
	}
	mustUnmarshal(t, workJSON(t, session), &work)
	if len(work) != 2 {
		t.Fatalf("expected 2 claims before cleanup, got %d", len(work))
	}

	// Simulate session-end cleanup: open a handle to the same DB (as Run does
	// via the shared *store.Store) and clear the pi agent's claims.
	cleanup, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open for cleanup: %v", err)
	}
	t.Cleanup(func() { cleanup.Close() })

	n, err := cleanup.ReleaseAgentClaims("pi")
	if err != nil {
		t.Fatalf("ReleaseAgentClaims: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 claim released for agent pi, got %d", n)
	}

	// The work view should now show only claude's claim
	mustUnmarshal(t, workJSON(t, session), &work)
	if len(work) != 1 {
		t.Fatalf("expected 1 claim after session-end cleanup (claude's), got %d", len(work))
	}
	if work[0].AgentID != "claude" {
		t.Fatalf("expected claude's claim to remain, got %v", work)
	}

	// Session-end cleanup clears claims but not assignments: the human can
	// see in the TUI who still holds what, and a stale assignment is a
	// release-path matter, not a session one.
	var still []struct {
		Assignee string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{"ids": []string{task["id"]}}), &still)
	if len(still) != 1 || still[0].Assignee == "" {
		t.Fatalf("assignment must survive session-end cleanup, got %+v", still)
	}
}

// TestMCPAgentLivePresenceSurvivesOtherAgentSessionEnd verifies the
// user-visible symptom from the session-end claim release scoping:
// when agent A's session ends, agent B's assignee_live remains true
// for tasks B holds.
func TestMCPAgentLivePresenceSurvivesOtherAgentSessionEnd(t *testing.T) {
	// Set up two agents sharing the same data directory
	dataDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataDir)

	// Session A: agent "pi"
	sessionA := sessionAs(t, dataDir, "pi")
	defer sessionA.Close()

	var listA map[string]string
	mustUnmarshal(t, callTool(t, sessionA, "add_list", map[string]any{"name": "Work"}), &listA)

	var taskA map[string]string
	mustUnmarshal(t, callTool(t, sessionA, "add_task", map[string]any{
		"list_id": listA["id"],
		"title":   "Write docs",
	}), &taskA)

	// Agent pi assigns and works on the task
	callTool(t, sessionA, "assign_task", map[string]any{"ids": []string{taskA["id"]}})
	callTool(t, sessionA, "set_status", map[string]any{
		"ids":    []string{taskA["id"]},
		"status": "in_progress",
	})

	// Session B: agent "claude"
	sessionB := sessionAs(t, dataDir, "claude")
	defer sessionB.Close()

	// Agent claude works on a different task (to avoid conflicts)
	var listB map[string]string
	mustUnmarshal(t, callTool(t, sessionB, "add_list", map[string]any{"name": "Work2"}), &listB)

	var taskB map[string]string
	mustUnmarshal(t, callTool(t, sessionB, "add_task", map[string]any{
		"list_id": listB["id"],
		"title":   "Read specs",
	}), &taskB)

	// Agent claude assigns and works on their task
	callTool(t, sessionB, "assign_task", map[string]any{"ids": []string{taskB["id"]}})
	callTool(t, sessionB, "set_status", map[string]any{
		"ids":    []string{taskB["id"]},
		"status": "in_progress",
	})

	// Verify both agents show as live before session end
	var workBefore []struct {
		AgentID      string `json:"agent_id"`
		AssigneeLive bool   `json:"assignee_live"`
		Assignee     string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, sessionA, "show_task", map[string]any{"ids": []string{taskA["id"]}}), &workBefore)
	if len(workBefore) != 1 {
		t.Fatalf("expected 1 task result, got %d", len(workBefore))
	}
	if !workBefore[0].AssigneeLive {
		t.Fatalf("expected agent pi's task to show assignee_live=true before session end, got %v", workBefore[0])
	}
	if workBefore[0].Assignee != "pi" {
		t.Fatalf("expected agent pi to be assignee, got %s", workBefore[0].Assignee)
	}

	var workBeforeB []struct {
		AgentID      string `json:"agent_id"`
		AssigneeLive bool   `json:"assignee_live"`
		Assignee     string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, sessionB, "show_task", map[string]any{"ids": []string{taskB["id"]}}), &workBeforeB)
	if len(workBeforeB) != 1 {
		t.Fatalf("expected 1 task result for claude, got %d", len(workBeforeB))
	}
	if !workBeforeB[0].AssigneeLive {
		t.Fatalf("expected agent claude's task to show assignee_live=true before session end, got %v", workBeforeB[0])
	}
	if workBeforeB[0].Assignee != "claude" {
		t.Fatalf("expected agent claude to be assignee, got %s", workBeforeB[0].Assignee)
	}

	// Simulate agent pi's session ending: open cleanup handle and release pi's claims
	cleanup, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open for cleanup: %v", err)
	}
	defer cleanup.Close()

	n, err := cleanup.ReleaseAgentClaims("pi")
	if err != nil {
		t.Fatalf("ReleaseAgentClaims: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 claim released for agent pi, got %d", n)
	}

	// Verify agent pi's task now shows as not live (spinner gone)
	var workAfterA []struct {
		AgentID      string `json:"agent_id"`
		AssigneeLive bool   `json:"assignee_live"`
		Assignee     string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, sessionA, "show_task", map[string]any{"ids": []string{taskA["id"]}}), &workAfterA)
	if len(workAfterA) != 1 {
		t.Fatalf("expected 1 task result after pi's session end, got %d", len(workAfterA))
	}
	if workAfterA[0].AssigneeLive {
		t.Fatalf("expected agent pi's task to show assignee_live=false after session end, got %v", workAfterA[0])
	}
	if workAfterA[0].Assignee != "pi" {
		t.Fatalf("expected agent pi to still be assignee, got %s", workAfterA[0].Assignee)
	}

	// CRITICAL: Verify agent claude's task STILL shows as live (their session continues)
	var workAfterB []struct {
		AgentID      string `json:"agent_id"`
		AssigneeLive bool   `json:"assignee_live"`
		Assignee     string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, sessionB, "show_task", map[string]any{"ids": []string{taskB["id"]}}), &workAfterB)
	if len(workAfterB) != 1 {
		t.Fatalf("expected 1 task result for claude after pi's session end, got %d", len(workAfterB))
	}
	if !workAfterB[0].AssigneeLive {
		t.Fatalf("expected agent claude's task to STILL show assignee_live=true after pi's session end, got %v", workAfterB[0])
	}
	if workAfterB[0].Assignee != "claude" {
		t.Fatalf("expected agent claude to still be assignee, got %s", workAfterB[0].Assignee)
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
	callTool(t, session, "comment", map[string]any{"task_id": a["id"], "note": "ping"})

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
	callTool(t, session, "set_status", map[string]any{"ids": []string{parent["id"]}, "status": "complete"})
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
		Name:      "comment",
		Arguments: map[string]any{"task_id": a["id"], "note": "hi"},
	})
	if err != nil {
		t.Fatalf("comment: %v", err)
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

	callTool(t, session, "comment", map[string]any{
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

// TestCommentAddRoundTrip pins the comment tool's add mode (plan step 10,
// merging docs/plan/task-comments.md §4): it succeeds on a normal task,
// attributes the comment to the server identity, and the comment appears in a
// subsequent show_task.
func TestCommentAddRoundTrip(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)

	cid, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "comment",
		Arguments: map[string]any{"task_id": task["id"], "note": "hello"},
	})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	var res struct {
		ID string `json:"id"`
	}
	mustUnmarshal(t, textContent(t, cid), &res)
	if res.ID == "" {
		t.Fatal("comment returned empty id")
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

// TestCommentAddRefusedOnDisabledList pins the list-level disable flag
// enforcement over MCP (docs/plan/task-comments.md §4).
func TestCommentAddRefusedOnDisabledList(t *testing.T) {
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

	msg := callToolErr(t, session, "comment", map[string]any{
		"task_id": task["id"], "note": "hello",
	})
	if !strings.Contains(msg, "disabled") {
		t.Errorf("expected 'disabled' error, got %q", msg)
	}
}

// TestCommentAddRejectsExplicitAuthor pins the plan's recommendation to
// reject an explicit author rather than silently ignoring it — comments are
// always attributed to the server's identity.
func TestCommentAddRejectsExplicitAuthor(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)

	msg := callToolErr(t, session, "comment", map[string]any{
		"task_id": task["id"], "note": "hello", "author": "someone-else",
	})
	if !strings.Contains(msg, "author") {
		t.Errorf("expected error mentioning author, got %q", msg)
	}
}

// TestCommentDeleteRejectsExplicitAuthor pins that the author rejection
// covers the delete mode too. delete_comment had no author parameter, so the
// old tool rejected one via its schema; the merged tool must carry author for
// add mode, and rejecting it only on the add path would turn it into a
// silently ignored parameter on the delete path — the deletion gate keys off
// the calling identity, never off anything the caller names.
func TestCommentDeleteRejectsExplicitAuthor(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)
	var comment map[string]string
	mustUnmarshal(t, callTool(t, session, "comment", map[string]any{
		"task_id": task["id"], "note": "mine",
	}), &comment)

	msg := callToolErr(t, session, "comment", map[string]any{
		"id": comment["id"], "delete": true, "force": true, "author": "someone-else",
	})
	if !strings.Contains(msg, "author") {
		t.Errorf("expected error mentioning author, got %q", msg)
	}

	// The refusal must be a refusal, not a warning: the comment survives.
	var detailsArr []struct {
		Comments []struct{ ID string } `json:"comments"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{task["id"]},
	}), &detailsArr)
	if len(detailsArr[0].Comments) != 1 {
		t.Errorf("comment deleted despite the rejected author, got %+v", detailsArr[0].Comments)
	}
}

// TestCommentAddRefusedOnMissingTask verifies the existence check.
func TestCommentAddRefusedOnMissingTask(t *testing.T) {
	session := setupMCP(t)
	msg := callToolErr(t, session, "comment", map[string]any{
		"task_id": "01ARZ", "note": "hello",
	})
	if !strings.Contains(msg, "not found") {
		t.Errorf("expected 'not found' error, got %q", msg)
	}
}

// TestCommentDeleteRefusesAnotherAuthor pins the ownership rule that
// makes the comment tool's delete mode safe to expose to agents at all: an
// identity may never delete a comment it did not write, even on a list it
// owns.
func TestCommentDeleteRefusesAnotherAuthor(t *testing.T) {
	dataDir := t.TempDir()

	piSession := sessionAs(t, dataDir, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, piSession, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, piSession, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)
	var comment map[string]string
	mustUnmarshal(t, callTool(t, piSession, "comment", map[string]any{
		"task_id": task["id"], "note": "pi's comment",
	}), &comment)

	claudeSession := sessionAs(t, dataDir, "claude")
	msg := callToolErr(t, claudeSession, "comment", map[string]any{
		"id": comment["id"], "delete": true, "force": true,
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

// TestCommentDeleteOwnSucceeds pins the success path: an identity may
// delete its own comment.
func TestCommentDeleteOwnSucceeds(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)
	var comment map[string]string
	mustUnmarshal(t, callTool(t, session, "comment", map[string]any{
		"task_id": task["id"], "note": "mine",
	}), &comment)

	var ok map[string]bool
	mustUnmarshal(t, callTool(t, session, "comment", map[string]any{
		"id": comment["id"], "delete": true, "force": true,
	}), &ok)
	if !ok["ok"] {
		t.Errorf("comment delete = %+v, want ok:true", ok)
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

// TestCommentDeleteRequiresForce mirrors TestMCPDeleteTaskRequiresForce.
func TestCommentDeleteRequiresForce(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)
	var comment map[string]string
	mustUnmarshal(t, callTool(t, session, "comment", map[string]any{
		"task_id": task["id"], "note": "mine",
	}), &comment)

	// With the two comment tools merged, force can no longer be a
	// schema-required field (add mode never uses it) — the refusal is the
	// handler's own check, so the message text is now ours to pin.
	msg := callToolErr(t, session, "comment", map[string]any{
		"id": comment["id"], "delete": true,
	})
	if !strings.Contains(msg, "force=true") {
		t.Errorf("expected 'deleting a comment requires force=true', got %q", msg)
	}

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

// TestCommentAllowedOnAssignedTask pins §4: posting a comment is never
// blocked by assignment — the §7 guard does not apply to comment, because
// leaving a note on another agent's task is how coordination is meant to
// work. Here the task is durably held by pi; claude comments on it with no
// force and no refusal, and the note lands attributed to claude.
func TestCommentAllowedOnAssignedTask(t *testing.T) {
	dataDir := t.TempDir()

	piSession := sessionAs(t, dataDir, "pi")
	claudeSession := sessionAs(t, dataDir, "claude")

	var list map[string]string
	mustUnmarshal(t, callTool(t, piSession, "add_list", map[string]any{"name": "Home"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, piSession, "add_task", map[string]any{
		"list_id": list["id"], "title": "task",
	}), &task)

	// pi durably grabs the task; the assign payload proves it is held.
	var assigned []struct {
		Assignee string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, piSession, "assign_task", map[string]any{
		"ids": []string{task["id"]},
	}), &assigned)
	if len(assigned) != 1 || assigned[0].Assignee != "pi" {
		t.Fatalf("task must be durably assigned to pi, got %+v", assigned)
	}

	// claude comments on pi's task — no force, no assignment-guard refusal.
	var res struct {
		ID string `json:"id"`
	}
	mustUnmarshal(t, callTool(t, claudeSession, "comment", map[string]any{
		"task_id": task["id"], "note": "headsup, working alongside",
	}), &res)
	if res.ID == "" {
		t.Fatal("comment returned empty id")
	}

	// The comment is there for pi to see, attributed to claude.
	var detailsArr []struct {
		Comments []struct {
			Author string `json:"author"`
			Note   string `json:"note"`
		} `json:"comments"`
	}
	mustUnmarshal(t, callTool(t, piSession, "show_task", map[string]any{
		"ids": []string{task["id"]},
	}), &detailsArr)
	if len(detailsArr) != 1 || len(detailsArr[0].Comments) != 1 {
		t.Fatalf("show_task comments = %+v, want 1", detailsArr[0].Comments)
	}
	if detailsArr[0].Comments[0].Author != "claude" || detailsArr[0].Comments[0].Note != "headsup, working alongside" {
		t.Errorf("comment = %+v, want claude's note", detailsArr[0].Comments[0])
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

func TestCommentAutoClaims(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "work",
	}), &task)
	// add_task now auto-claims too, so release first to prove comment's add
	// mode claims.
	releaseWork(t, "task", task["id"], "pi")
	callTool(t, session, "comment", map[string]any{
		"task_id": task["id"], "note": "checking in",
	})
	if !hasClaim(t, session, task["id"], "pi") {
		t.Errorf("comment add should have auto-claimed the task")
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
		releaseWork(t, "task", tk["id"], "pi")
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

// taskPriority reads one task's priority through show_task — the read shape
// an agent actually sees, not the store row underneath it.
func taskPriority(t *testing.T, session *mcp.ClientSession, id string) string {
	t.Helper()
	var arr []map[string]any
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{"ids": []string{id}}), &arr)
	if len(arr) != 1 {
		t.Fatalf("show_task(%s) returned %d rows, want 1", id, len(arr))
	}
	p, _ := arr[0]["priority"].(string)
	return p
}

// TestAddTaskPriority covers the priority parameter §2's final-surface table
// gives add_task: it lands on the created task, an omitted one leaves the
// column at its 'none' default (never SetPriority(""), which the store
// rejects — plan §6.5), and a bad value is refused BEFORE the task is
// created, so a rejected call leaves nothing behind.
func TestAddTaskPriority(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)

	var hi map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "urgent", "priority": "high",
	}), &hi)
	if got := taskPriority(t, session, hi["id"]); got != "high" {
		t.Errorf("add_task(priority=high) stored %q, want high", got)
	}

	var plain map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "whenever",
	}), &plain)
	if got := taskPriority(t, session, plain["id"]); got != "none" {
		t.Errorf("add_task without priority stored %q, want none", got)
	}

	msg := callToolErr(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "bogus", "priority": "urgent",
	})
	if !strings.Contains(msg, "invalid priority") {
		t.Errorf("add_task(priority=urgent) error = %q, want the invalid-priority message", msg)
	}
	var listed struct {
		Tasks []struct {
			Title string `json:"title"`
		} `json:"tasks"`
	}
	mustUnmarshal(t, callTool(t, session, "list_tasks", map[string]any{"list_id": list["id"]}), &listed)
	if len(listed.Tasks) != 2 {
		t.Fatalf("list has %d tasks after a rejected add, want 2 — the task was created anyway: %+v", len(listed.Tasks), listed.Tasks)
	}
}

// TestEditTaskPriority covers §6.5's dangerous row: the parameter's PRESENCE
// is what means "set it". An edit that omits priority must leave the stored
// value alone — a rename silently clearing a high someone set is the bug the
// pointer type exists to prevent — and "" is a rejected value, not a quiet
// 'none'. A rejected value is caught before the rename is written.
func TestEditTaskPriority(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "orig",
	}), &task)

	// priority alone satisfies the needs-a-field check.
	callTool(t, session, "edit_task", map[string]any{"id": task["id"], "priority": "high"})
	if got := taskPriority(t, session, task["id"]); got != "high" {
		t.Fatalf("edit_task(priority=high) stored %q, want high", got)
	}

	// A title-only edit leaves it high.
	callTool(t, session, "edit_task", map[string]any{"id": task["id"], "title": "renamed"})
	if got := taskPriority(t, session, task["id"]); got != "high" {
		t.Errorf("priority = %q after a title-only edit, want high (omitted means unchanged)", got)
	}

	// Explicit "" is invalid, not 'none'.
	if msg := callToolErr(t, session, "edit_task", map[string]any{"id": task["id"], "priority": ""}); !strings.Contains(msg, "invalid priority") {
		t.Errorf("edit_task(priority='') error = %q, want the invalid-priority message", msg)
	}
	// And a bad value refuses the whole call, rename included.
	if msg := callToolErr(t, session, "edit_task", map[string]any{
		"id": task["id"], "title": "half-written", "priority": "urgent",
	}); !strings.Contains(msg, "invalid priority") {
		t.Errorf("edit_task(priority=urgent) error = %q, want the invalid-priority message", msg)
	}
	var arr []map[string]any
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{"ids": []string{task["id"]}}), &arr)
	if arr[0]["title"] != "renamed" {
		t.Errorf("title = %v after a rejected priority, want renamed — the rename was written before the value was validated", arr[0]["title"])
	}
	if arr[0]["priority"] != "high" {
		t.Errorf("priority = %v after a rejected edit, want high", arr[0]["priority"])
	}
}

// TestEditTaskPriorityForeignListRefused pins priority on the structural side
// of the ownership line: re-ranking is a steer about what to pick up next, so
// it is refused on a foreign list exactly like a rename — unlike
// status/progress, which any agent may change anywhere.
func TestEditTaskPriorityForeignListRefused(t *testing.T) {
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
	msg := callToolErr(t, session, "edit_task", map[string]any{"id": foreignTaskID, "priority": "high"})
	if !strings.Contains(msg, "owned by claude") {
		t.Errorf("foreign-list priority edit error = %q, want it to name the owner", msg)
	}
	if got := taskPriority(t, session, foreignTaskID); got != "none" {
		t.Errorf("foreign task priority = %q after the refusal, want none", got)
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
	mustUnmarshal(t, callTool(t, session, "set_status", map[string]any{
		"ids": ids, "status": "complete",
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

func TestSetStatusBatchPercentage(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var a, b map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "a"}), &a)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "b"}), &b)
	var res []map[string]any
	mustUnmarshal(t, callTool(t, session, "set_status", map[string]any{
		"ids": []string{a["id"], b["id"]}, "progress": "percentage", "percent": 50,
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

// TestSetStatusReopensAndSetsProgressOnCompleteTask pins the §4 fix: the old
// set_progress refused a completed task ("reopen it before setting
// progress"), so the agent needed two calls. set_status applies the reopen
// first, so one call returns the task to open AND sets its percentage — the
// documented example set_status(ids, status='in_progress',
// progress='percentage', percent=10) on a complete task.
// TestSetStatusInProgressPreservesProgress pins the one transition set_status
// has to invent. The store has no "start this task" write: SetProgress is the
// only path that flips pending → in_progress, so status='in_progress' with no
// progress of its own re-applies the task's existing progress fields. That
// must start a plain pending task (progress 'none') AND leave a percentage
// already on the task alone — a start marker that silently reset a task to 0%
// would be worse than no start marker.
func TestSetStatusInProgressPreservesProgress(t *testing.T) {
	session := setupMCP(t)
	var list, plain, measured map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "plain"}), &plain)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "measured"}), &measured)

	var started []map[string]any
	mustUnmarshal(t, callTool(t, session, "set_status", map[string]any{
		"ids": []string{plain["id"]}, "status": "in_progress",
	}), &started)
	assertOKRows(t, started)

	var seeded []map[string]any
	mustUnmarshal(t, callTool(t, session, "set_status", map[string]any{
		"ids": []string{measured["id"]}, "progress": "percentage", "percent": 60,
	}), &seeded)
	assertOKRows(t, seeded)
	var restarted []map[string]any
	mustUnmarshal(t, callTool(t, session, "set_status", map[string]any{
		"ids": []string{measured["id"]}, "status": "in_progress",
	}), &restarted)
	assertOKRows(t, restarted)

	var details []map[string]any
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{plain["id"], measured["id"]},
	}), &details)
	if len(details) != 2 {
		t.Fatalf("show_task returned %d rows, want 2", len(details))
	}
	if details[0]["status"] != "in_progress" {
		t.Errorf("plain task status = %v, want in_progress", details[0]["status"])
	}
	if plainProg, ok := details[0]["progress"].(map[string]any); !ok || plainProg["kind"] != "none" {
		t.Errorf("plain task progress = %#v, want kind none — starting it invented no progress", details[0]["progress"])
	}
	if details[1]["status"] != "in_progress" {
		t.Errorf("measured task status = %v, want in_progress", details[1]["status"])
	}
	prog, ok := details[1]["progress"].(map[string]any)
	if !ok {
		t.Fatalf("measured task progress = %#v, want a percentage block", details[1]["progress"])
	}
	if int(prog["percent"].(float64)) != 60 {
		t.Errorf("status='in_progress' clobbered the percentage: got %v, want 60", prog["percent"])
	}
}

func TestSetStatusReopensAndSetsProgressOnCompleteTask(t *testing.T) {
	session := setupMCP(t)
	var list, task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{"list_id": list["id"], "title": "a"}), &task)
	var completed []map[string]any
	mustUnmarshal(t, callTool(t, session, "set_status", map[string]any{
		"ids": []string{task["id"]}, "status": "complete",
	}), &completed)
	assertOKRows(t, completed)

	// One call on the complete task: reopen, then set progress; the
	// comment lands after the state change, recording the final state.
	var res []map[string]any
	mustUnmarshal(t, callTool(t, session, "set_status", map[string]any{
		"ids": []string{task["id"]}, "status": "in_progress", "progress": "percentage", "percent": 10,
		"comment": "reopened and set to 10%",
	}), &res)
	assertOKRows(t, res)

	var detailArr []map[string]any
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{"ids": []string{task["id"]}}), &detailArr)
	if len(detailArr) != 1 {
		t.Fatalf("show_task returned %d rows, want 1", len(detailArr))
	}
	detail := detailArr[0]
	if detail["status"] != "in_progress" {
		t.Errorf("status = %v, want in_progress", detail["status"])
	}
	prog := detail["progress"].(map[string]any)
	if int(prog["percent"].(float64)) != 10 {
		t.Errorf("want 10%%, got %v", prog["percent"])
	}
	comments, ok := detail["comments"].([]any)
	if !ok || len(comments) != 1 {
		t.Fatalf("comments = %#v, want the set_status comment", detail["comments"])
	}
	if note := comments[0].(map[string]any)["note"]; note != "reopened and set to 10%" {
		t.Errorf("comment note = %v, want the set_status comment", note)
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
	// set_status returns per-id rows (not a tool error), so a bad id is a
	// row with an error, not a call failure.
	var res []map[string]any
	mustUnmarshal(t, callTool(t, session, "set_status", map[string]any{
		"ids": []string{good["id"], "does-not-exist"}, "status": "complete",
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
	msg := callToolErr(t, session, "set_status", map[string]any{"ids": ids, "status": "complete"})
	if !strings.Contains(msg, "capped at 50") {
		t.Errorf("expected cap error, got %q", msg)
	}
}

func TestSetStatusPercentRequired(t *testing.T) {
	session := setupMCPAs(t, "pi")
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "T"}), &list)
	var tk map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "a",
	}), &tk)
	msg := callToolErr(t, session, "set_status", map[string]any{
		"ids": []string{tk["id"]}, "progress": "percentage",
	})
	if !strings.Contains(msg, "requires percent") {
		t.Errorf("percentage without percent should error, got %q", msg)
	}
}

func TestSetStatusRejectsEmptyRequest(t *testing.T) {
	session := setupMCPAs(t, "pi")

	// §4: at least one of status, progress, comment is required. None of the
	// three is not "leave everything alone" — it is a caller bug, rejected
	// before any write.
	msg := callToolErr(t, session, "set_status", map[string]any{"ids": []string{"x"}})
	if !strings.Contains(msg, "at least one of status, progress, comment") {
		t.Errorf("empty set_status should error, got %q", msg)
	}

	// Unknown status and progress values are rejected up front too.
	msg = callToolErr(t, session, "set_status", map[string]any{"ids": []string{"x"}, "status": "bogus"})
	if !strings.Contains(msg, "invalid status") {
		t.Errorf("bogus status should error, got %q", msg)
	}
	msg = callToolErr(t, session, "set_status", map[string]any{"ids": []string{"x"}, "progress": "bogus"})
	if !strings.Contains(msg, "invalid progress") {
		t.Errorf("bogus progress should error, got %q", msg)
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
	callTool(t, session, "set_status", map[string]any{"ids": []string{tk["id"]}, "status": "complete"})
	var work []map[string]any
	mustUnmarshal(t, workJSON(t, session), &work)
	found := false
	for _, w := range work {
		if w["entity_id"] == tk["id"] && w["agent_id"] == "pi" {
			found = true
		}
	}
	if !found {
		t.Errorf("set_status should auto-claim each touched task; work=%#v", work)
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
	callTool(t, session, "set_status", map[string]any{"ids": []string{tk["id"]}, "status": "complete"})
	var res []map[string]any
	mustUnmarshal(t, callTool(t, session, "set_status", map[string]any{
		"ids": []string{tk["id"]}, "status": "pending",
	}), &res)
	if len(res) != 1 || res[0]["ok"] != true {
		t.Fatalf("set_status batch row not ok: %#v", res)
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
	callTool(t, session, "set_status", map[string]any{"ids": []string{a["id"]}, "status": "complete"})

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
	callTool(t, session, "set_status", map[string]any{"ids": []string{parent["id"]}, "status": "complete"})
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

// TestMCPNextTaskGrabsDifferentTasks guards §3's next_task contract: each
// call atomically grabs the top eligible task, so two calls on a list of
// two free tasks return two different tasks; priority (high > medium > low
// > none) beats tree order.
func TestMCPNextTaskGrabsDifferentTasks(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	// Added low-priority first, so tree order would give it to the first
	// call; priority must override.
	var low, high map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "low",
	}), &low)
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "high",
	}), &high)

	// Seed priorities at store level: SetPriority is not a tool (decision 6
	// kept the surface slim; the TUI sets it).
	st, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.SetPriority(high["id"], store.PriorityHigh); err != nil {
		t.Fatalf("SetPriority high: %v", err)
	}
	if err := st.SetPriority(low["id"], store.PriorityLow); err != nil {
		t.Fatalf("SetPriority low: %v", err)
	}

	var first, second struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Assignee string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, session, "next_task", map[string]any{"list_id": list["id"]}), &first)
	if first.ID != high["id"] {
		t.Fatalf("first next_task = %s (%s), want the high-priority task", first.Title, first.ID)
	}
	if first.Assignee != "agent" {
		t.Fatalf("first next_task assignee = %q, want the server identity", first.Assignee)
	}
	mustUnmarshal(t, callTool(t, session, "next_task", map[string]any{"list_id": list["id"]}), &second)
	if second.ID != low["id"] {
		t.Fatalf("second next_task = %s (%s), want the remaining task", second.Title, second.ID)
	}
	if first.ID == second.ID {
		t.Fatal("two next_task calls returned the same task")
	}
}

// TestMCPNextTaskEmptyBoard guards §4's shape: nothing eligible is NOT an
// error — the tool returns {ok:false, reason:'no eligible task in this
// list'}, and the reason string is part of the contract agents act on.
func TestMCPNextTaskEmptyBoard(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)

	var empty struct {
		OK     bool   `json:"ok"`
		Reason string `json:"reason"`
	}
	mustUnmarshal(t, callTool(t, session, "next_task", map[string]any{"list_id": list["id"]}), &empty)
	if empty.OK || empty.Reason != "no eligible task in this list" {
		t.Fatalf("next_task on an empty list = %+v, want {ok:false, reason:'no eligible task in this list'}", empty)
	}

	// A held task is not eligible either: an assigned task is not free.
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "held",
	}), &task)
	callTool(t, session, "assign_task", map[string]any{"ids": []string{task["id"]}})
	var after struct {
		OK     bool   `json:"ok"`
		Reason string `json:"reason"`
	}
	mustUnmarshal(t, callTool(t, session, "next_task", map[string]any{"list_id": list["id"]}), &after)
	if after.OK || after.Reason != "no eligible task in this list" {
		t.Fatalf("next_task with only a held task = %+v, want the empty shape", after)
	}
}

// TestMCPGrabClaimsPresence pins that a grab is a write for presence
// purposes, on both grab paths. docs/DESIGN.md §3 defines assignee != "" with
// assignee_live false as ABANDONED work, so an agent that has just grabbed a
// task and is demonstrably alive must not read back that way — otherwise the
// TUI's stale tier lights up on work nobody has let go of, and the §4 conflict
// text tells a second agent "no live session" about an agent still holding the
// keyboard. Asserted from a SECOND identity's read, so it pins the claim in
// the store rather than a value patched into the returned payload.
func TestMCPGrabClaimsPresence(t *testing.T) {
	for _, tc := range []struct {
		name string
		grab func(t *testing.T, session *mcp.ClientSession, listID, taskID string)
	}{
		{"assign_task", func(t *testing.T, session *mcp.ClientSession, _, taskID string) {
			callTool(t, session, "assign_task", map[string]any{"ids": []string{taskID}})
		}},
		{"next_task", func(t *testing.T, session *mcp.ClientSession, listID, _ string) {
			callTool(t, session, "next_task", map[string]any{"list_id": listID})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			alice := sessionAs(t, dataDir, "alice")

			var list map[string]string
			mustUnmarshal(t, callTool(t, alice, "add_list", map[string]any{"name": "Work"}), &list)
			var task map[string]string
			mustUnmarshal(t, callTool(t, alice, "add_task", map[string]any{
				"list_id": list["id"], "title": "Write docs",
			}), &task)

			// add_task already claimed under alice; drop it so the only claim
			// this test can observe is the one the grab itself makes.
			releaseWork(t, "task", task["id"], "alice")

			tc.grab(t, alice, list["id"], task["id"])

			// bob is a different session against the same store.
			bob := sessionAs(t, dataDir, "bob")
			var got []struct {
				Assignee     string `json:"assignee"`
				AssigneeLive bool   `json:"assignee_live"`
			}
			mustUnmarshal(t, callTool(t, bob, "show_task", map[string]any{
				"ids": []string{task["id"]},
			}), &got)
			if len(got) != 1 || got[0].Assignee != "alice" {
				t.Fatalf("%s: task should be held by alice, got %+v", tc.name, got)
			}
			if !got[0].AssigneeLive {
				t.Fatalf("%s: assignee_live = false right after the grab — a live "+
					"holder reads as abandoned (docs/DESIGN.md §3)", tc.name)
			}

			// And the claim is a real row on crush://work, under alice.
			var work []struct {
				AgentID  string `json:"agent_id"`
				EntityID string `json:"entity_id"`
			}
			mustUnmarshal(t, workJSON(t, bob), &work)
			found := false
			for _, w := range work {
				if w.AgentID == "alice" && w.EntityID == task["id"] {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s: no crush://work claim by alice on the grabbed task, got %+v",
					tc.name, work)
			}
		})
	}
}

// TestMCPAssignReleaseDoesNotClaimPresence is the other half of the rule above:
// releasing is letting go, so it must not light a spinner on a task the agent
// no longer holds. Uses the release-when-nobody-held-it path, which succeeds
// silently (§4), so no earlier grab can have left a claim behind.
func TestMCPAssignReleaseDoesNotClaimPresence(t *testing.T) {
	dataDir := t.TempDir()
	alice := sessionAs(t, dataDir, "alice")

	var list map[string]string
	mustUnmarshal(t, callTool(t, alice, "add_list", map[string]any{"name": "Work"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, alice, "add_task", map[string]any{
		"list_id": list["id"], "title": "Write docs",
	}), &task)
	releaseWork(t, "task", task["id"], "alice")

	bob := sessionAs(t, dataDir, "bob")
	var rel []struct {
		OK bool `json:"ok"`
	}
	mustUnmarshal(t, callTool(t, bob, "assign_task", map[string]any{
		"ids": []string{task["id"]}, "release": true,
	}), &rel)
	if len(rel) != 1 || !rel[0].OK {
		t.Fatalf("release of an unheld task = %+v, want a silent success", rel)
	}

	var work []struct {
		AgentID string `json:"agent_id"`
	}
	mustUnmarshal(t, workJSON(t, bob), &work)
	for _, w := range work {
		if w.AgentID == "bob" {
			t.Fatalf("release opened a presence claim under bob: %+v", work)
		}
	}
}

// TestMCPAssignSubtreeConflict guards decision 4: a task whose ancestor or
// descendant is held by a different agent is refused EVEN with force, and
// the error names the blocker and the release escape hatch - the exact
// remediation that does not loop. (force CAN take a task from its holder;
// it CANNOT override the subtree reservation.)
func TestMCPAssignSubtreeConflict(t *testing.T) {
	dataDir := t.TempDir()

	alice := sessionAs(t, dataDir, "alice")
	var list map[string]string
	mustUnmarshal(t, callTool(t, alice, "add_list", map[string]any{"name": "Work"}), &list)
	var parent, child map[string]string
	mustUnmarshal(t, callTool(t, alice, "add_task", map[string]any{
		"list_id": list["id"], "title": "parent",
	}), &parent)
	mustUnmarshal(t, callTool(t, alice, "add_task", map[string]any{
		"list_id": list["id"], "title": "child", "parent": parent["id"],
	}), &child)

	// alice holds the parent; bob cannot grab the child, force or not.
	callTool(t, alice, "assign_task", map[string]any{"ids": []string{parent["id"]}})
	bob := sessionAs(t, dataDir, "bob")
	var rows []struct {
		Error string `json:"error"`
	}
	mustUnmarshal(t, callTool(t, bob, "assign_task", map[string]any{"ids": []string{child["id"]}}), &rows)
	msg := ""
	if len(rows) > 0 {
		msg = rows[0].Error
	}
	if msg == "" {
		t.Fatalf("assign-conflict rows = %+v, want an error row", rows)
	}
	for _, want := range []string{"blocked by its ancestor", parent["id"], "release that task first", "assign_task(release=true, force=true)"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("ancestor-conflict error = %q, missing %q", msg, want)
		}
	}
	if strings.Contains(msg, "pass force=true to take it") {
		t.Fatalf("subtree conflict must NOT hint force, got %q", msg)
	}
	var frows []struct {
		Error string `json:"error"`
	}
	mustUnmarshal(t, callTool(t, bob, "assign_task", map[string]any{"ids": []string{child["id"]}, "force": true}), &frows)
	if len(frows) == 0 || !strings.Contains(frows[0].Error, "blocked by its ancestor") {
		t.Fatalf("forced subtree grab must still be refused, got %+v", frows)
	}

	// The mirror case: alice holds the child; bob cannot grab the parent.
	callTool(t, alice, "assign_task", map[string]any{"ids": []string{child["id"]}, "force": true})
	var drows []struct {
		Error string `json:"error"`
	}
	mustUnmarshal(t, callTool(t, bob, "assign_task", map[string]any{"ids": []string{parent["id"]}, "force": true}), &drows)
	if len(drows) == 0 || !strings.Contains(drows[0].Error, "blocked by its descendant") {
		t.Fatalf("descendant-conflict error = %+v, want the descendant wording", drows)
	}

	// The escape hatch works: releasing the blocker frees the subtree.
	callTool(t, alice, "assign_task", map[string]any{"ids": []string{child["id"]}, "release": true})
	callTool(t, alice, "assign_task", map[string]any{"ids": []string{parent["id"]}, "release": true})
	callTool(t, bob, "assign_task", map[string]any{"ids": []string{parent["id"]}})
}

// TestMCPAssignRejectsAgentID guards §3: assign_task takes no agent_id —
// an agent may only assign work to itself; assigning work TO another agent
// is a human action taken from the TUI.
func TestMCPAssignRejectsAgentID(t *testing.T) {
	session := setupMCP(t)

	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "docs",
	}), &task)

	msg := callToolErr(t, session, "assign_task", map[string]any{
		"ids": []string{task["id"]}, "agent_id": "claude",
	})
	if !strings.Contains(msg, "agent_id is not a supported parameter") {
		t.Fatalf("agent_id rejection = %q, want the rejection named", msg)
	}

	// Nothing was assigned.
	var got []struct {
		Assignee string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{"ids": []string{task["id"]}}), &got)
	if len(got) != 1 || got[0].Assignee != "" {
		t.Fatalf("agent_id call must not assign anything, got %+v", got)
	}
}

// TestTwoUnconfiguredSessionsCannotWriteEachOthersTasks is the core of the
// step: the measured failure this plan exists to fix, inverted.
//
// Before per-session identity, two clients with CRUSH_AGENT unset both acted as
// "agent", so B completed a task A held, renamed it, and deleted a task A
// created — no refusal, no force, no takeover comment, and no trail for the
// human. Every guard compares against identity, and equal tags compare equal.
func TestTwoUnconfiguredSessionsCannotWriteEachOthersTasks(t *testing.T) {
	dataDir := t.TempDir()

	// Both sessions come up with CRUSH_AGENT unset, the default setup.
	sessionUnset := func(t *testing.T) *mcp.ClientSession {
		t.Helper()
		t.Setenv("XDG_DATA_HOME", dataDir)
		t.Setenv("CRUSH_AGENT", "")

		server, st, err := mcpserver.NewServer()
		if err != nil {
			t.Fatalf("NewServer: %v", err)
		}
		t.Cleanup(func() { st.Close() })
		ctx := context.Background()
		ct, transport := mcp.NewInMemoryTransports()
		ss, err := server.Connect(ctx, transport, nil)
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

	a, b := sessionUnset(t), sessionUnset(t)

	// A owns a list with a task, and grabs it.
	var list map[string]string
	mustUnmarshal(t, callTool(t, a, "add_list", map[string]any{"name": "Work"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, a, "add_task", map[string]any{
		"list_id": list["id"], "title": "Write docs",
	}), &task)

	var grabbed []struct {
		Assignee string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, a, "assign_task", map[string]any{
		"ids": []string{task["id"]},
	}), &grabbed)
	holder := grabbed[0].Assignee
	if holder == "" {
		t.Fatal("assign_task left the task unassigned")
	}

	// B must be refused on all three writes. Before the fix every one landed.
	var statusRows []struct {
		Error string `json:"error"`
	}
	mustUnmarshal(t, callTool(t, b, "set_status", map[string]any{
		"ids": []string{task["id"]}, "status": "complete",
	}), &statusRows)
	if len(statusRows) != 1 || statusRows[0].Error == "" {
		t.Fatalf("B's set_status on A's task must be refused, got %+v", statusRows)
	}
	if !strings.Contains(statusRows[0].Error, holder) {
		t.Fatalf("the refusal must name the holder %q, got %q", holder, statusRows[0].Error)
	}

	if msg := callToolErr(t, b, "edit_task", map[string]any{
		"id": task["id"], "title": "renamed by B",
	}); msg == "" {
		t.Fatal("B's edit_task on A's list must be refused")
	}
	if msg := callToolErr(t, b, "delete_task", map[string]any{
		"id": task["id"], "force": true,
	}); msg == "" {
		t.Fatal("B's delete_task on A's task must be refused")
	}

	// The task is untouched: still pending, still A's.
	var after []struct {
		Title    string `json:"title"`
		Status   string `json:"status"`
		Assignee string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, a, "show_task", map[string]any{
		"ids": []string{task["id"]},
	}), &after)
	if after[0].Status == "complete" || after[0].Title != "Write docs" || after[0].Assignee != holder {
		t.Fatalf("A's task was modified by B despite the refusals: %+v", after[0])
	}

	// force still works across the two identities, and leaves the trail.
	var forced []struct {
		OK bool `json:"ok"`
	}
	mustUnmarshal(t, callTool(t, b, "set_status", map[string]any{
		"ids": []string{task["id"]}, "status": "complete", "force": true,
	}), &forced)
	if len(forced) != 1 || !forced[0].OK {
		t.Fatalf("force must still work, got %+v", forced)
	}
	var withComments []struct {
		Comments []struct {
			Author string `json:"author"`
			Note   string `json:"note"`
		} `json:"comments"`
	}
	mustUnmarshal(t, callTool(t, a, "show_task", map[string]any{
		"ids": []string{task["id"]},
	}), &withComments)
	var takeover string
	for _, c := range withComments[0].Comments {
		if strings.Contains(c.Note, "took this task from") {
			takeover = c.Note
		}
	}
	if takeover == "" {
		t.Fatalf("force must record a takeover comment, got %+v", withComments[0].Comments)
	}
	if !strings.Contains(takeover, holder) {
		t.Fatalf("the takeover comment must name the previous holder %q, got %q", holder, takeover)
	}
}

// TestSessionEndReleasesAssignmentsAndEmptyInbox covers decisions 3 and 5. It
// mirrors TestMCPPendingClaimsClearedOnSessionEnd's separate-handle simulation
// of what Run does after server.Run returns.
//
// A per-session identity never comes back, so anything it still holds at exit
// is stranded — no future session answers to that tag, and no other agent can
// take the work without force.
func TestSessionEndReleasesAssignmentsAndEmptyInbox(t *testing.T) {
	session := setupMCPAs(t, "pi")

	// my_list get-or-creates "pi: Inbox" — the auto-created list decision 5
	// sweeps. It stays empty here.
	var mine struct {
		Mine struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"mine"`
	}
	mustUnmarshal(t, callTool(t, session, "my_list", map[string]any{}), &mine)
	if mine.Mine.Name != "pi: Inbox" {
		t.Fatalf("my_list = %q, want \"pi: Inbox\"", mine.Mine.Name)
	}

	// Work lives on a separate list, and pi holds it.
	var list map[string]string
	mustUnmarshal(t, callTool(t, session, "add_list", map[string]any{"name": "Work"}), &list)
	var task map[string]string
	mustUnmarshal(t, callTool(t, session, "add_task", map[string]any{
		"list_id": list["id"], "title": "Write docs",
	}), &task)
	callTool(t, session, "assign_task", map[string]any{"ids": []string{task["id"]}})

	cleanup, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open for cleanup: %v", err)
	}
	t.Cleanup(func() { cleanup.Close() })

	n, err := cleanup.UnassignAgent("pi")
	if err != nil {
		t.Fatalf("UnassignAgent: %v", err)
	}
	if n != 1 {
		t.Fatalf("UnassignAgent released %d tasks, want 1", n)
	}
	deleted, err := cleanup.DeleteEmptyAgentInbox("pi")
	if err != nil {
		t.Fatalf("DeleteEmptyAgentInbox: %v", err)
	}
	if !deleted {
		t.Fatal("the empty auto-created Inbox should have been removed")
	}

	// The assignment is gone; the task and its list are not.
	var after []struct {
		Title    string `json:"title"`
		Assignee string `json:"assignee"`
	}
	mustUnmarshal(t, callTool(t, session, "show_task", map[string]any{
		"ids": []string{task["id"]},
	}), &after)
	if after[0].Assignee != "" {
		t.Fatalf("assignment survived session end: %+v", after[0])
	}
	if after[0].Title != "Write docs" {
		t.Fatalf("session-end cleanup altered the task: %+v", after[0])
	}
	if _, err := cleanup.GetList(list["id"]); err != nil {
		t.Fatalf("the work list must survive session end: %v", err)
	}
}

// TestSessionEndKeepsAnInboxThatHasTasks is decision 5's safety rule. An Inbox
// the agent actually put work in is left alone: sweeping it would destroy the
// work as a side effect of tidying up, and the shutdown path has no business
// deciding that.
func TestSessionEndKeepsAnInboxThatHasTasks(t *testing.T) {
	session := setupMCPAs(t, "pi")

	var mine struct {
		Mine struct {
			ID string `json:"id"`
		} `json:"mine"`
	}
	mustUnmarshal(t, callTool(t, session, "my_list", map[string]any{}), &mine)
	callTool(t, session, "add_task", map[string]any{
		"list_id": mine.Mine.ID, "title": "real work",
	})

	cleanup, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open for cleanup: %v", err)
	}
	t.Cleanup(func() { cleanup.Close() })

	deleted, err := cleanup.DeleteEmptyAgentInbox("pi")
	if err != nil {
		t.Fatalf("DeleteEmptyAgentInbox: %v", err)
	}
	if deleted {
		t.Fatal("an Inbox holding a task must not be swept at session end")
	}
	if _, err := cleanup.GetList(mine.Mine.ID); err != nil {
		t.Fatalf("the Inbox must still exist: %v", err)
	}
}
