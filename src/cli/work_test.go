package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/filipemolina/farol/src/store"
)

// TestWorkJSONMirrorsResource pins the CLI equivalent of the retired
// farol:///work resource: the --json payload is a bare array, and each row
// carries exactly the fields the resource did (id, entity_type, entity_id,
// agent_id, kind, acquired_at) with the same names. A host that read the
// resource reads the same rows here.
func TestWorkJSONMirrorsResource(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")

	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "claimed task"))

	db, err := openTestStore(t, data)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.ClaimWork("task", tid, "pi", store.ActivityWorking); err != nil {
		t.Fatalf("claim task: %v", err)
	}
	if _, err := db.ClaimWork("list", lid, "claude", store.ActivityInspecting); err != nil {
		t.Fatalf("claim list: %v", err)
	}
	db.Close()

	var claims []workClaimJSON
	mustJSONCLI(t, data, &claims, "work", "--json")

	if len(claims) != 2 {
		t.Fatalf("work returned %d claims, want 2", len(claims))
	}
	seen := map[string]workClaimJSON{}
	for _, c := range claims {
		// The resource's exact field set and names.
		if c.ID == "" || c.EntityType == "" || c.EntityID == "" ||
			c.AgentID == "" || c.Kind == "" || c.AcquiredAt == 0 {
			t.Errorf("claim %+v missing a required field", c)
		}
		seen[c.EntityType+":"+c.EntityID] = c
	}
	taskClaim, ok := seen["task:"+tid]
	if !ok {
		t.Fatalf("no task claim for %q; claims = %+v", tid, claims)
	}
	if taskClaim.AgentID != "pi" || taskClaim.Kind != "working" {
		t.Errorf("task claim = %+v, want agent pi / kind working", taskClaim)
	}
	listClaim, ok := seen["list:"+lid]
	if !ok {
		t.Fatalf("no list claim for %q; claims = %+v", lid, claims)
	}
	if listClaim.AgentID != "claude" || listClaim.Kind != "inspecting" {
		t.Errorf("list claim = %+v, want agent claude / kind inspecting", listClaim)
	}
}

// TestWorkEmptyPrintsNothing pins the §9 no-output rule for reads: with no
// live claims the command exits 0, prints nothing in human mode, and emits
// `[]` in --json mode — a normal state, not an error.
func TestWorkEmptyPrintsNothing(t *testing.T) {
	data := t.TempDir()

	if out := mustCLI(t, data, "work"); out != "" {
		t.Errorf("human work with no claims printed %q, want nothing", out)
	}

	var claims []workClaimJSON
	mustJSONCLI(t, data, &claims, "work", "--json")
	if claims == nil || len(claims) != 0 {
		t.Errorf("json work with no claims = %+v, want empty array", claims)
	}
}

// TestWorkJSONIsOneValue pins the §9 contract mechanically: --json writes
// exactly one JSON value to stdout (a bare array), not text wrapped around
// it. The helper parse must succeed and the raw bytes must start with '['.
func TestWorkJSONIsOneValue(t *testing.T) {
	data := t.TempDir()
	code, out, errOut := runCLI(t, data, "work", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("empty stdout in --json mode; want one JSON value")
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("json work stdout = %q, want a bare array starting with '['", out)
	}
	var v any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Errorf("stdout %q is not one JSON value: %v", out, err)
	}
}

// TestWorkHumanTable pins the human rendering: a header row and one row per
// live claim, the entity column `type:id`, the resolved title, and an age
// suffix — plain text, no ANSI escapes (§9). It also pins that an agent
// without a presence claim does not appear (presence, not assignment).
func TestWorkHumanTable(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")

	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "pi: Board", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "active task"))

	// Assign a task to an agent WITHOUT a presence claim: it must NOT show
	// in `farol work` (which mirrors presence, not assignment).
	db, err := openTestStore(t, data)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AssignTask(tid, "ghost", false); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if _, err := db.ClaimWork("task", tid, "pi", store.ActivityWorking); err != nil {
		t.Fatalf("claim: %v", err)
	}
	db.Close()

	out := mustCLI(t, data, "work")
	// The table is tab-separated (tabwriter); parse it rather than matching
	// literal tab bytes. The header must carry the five columns in order.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 {
		t.Fatalf("human work printed no lines: %q", out)
	}
	header := strings.Fields(lines[0])
	wantHeader := []string{"AGENT", "ENTITY", "TITLE", "KIND", "AGE"}
	if !reflect.DeepEqual(header, wantHeader) {
		t.Errorf("human work header = %v, want %v (out %q)", header, wantHeader, out)
	}
	// The body must contain pi's task claim with the resolved title and an
	// age suffix, exactly mirroring the resource's claims.
	if !strings.Contains(out, "pi") || !strings.Contains(out, "task:"+tid) {
		t.Errorf("human work: %q, want a row for pi's task claim", out)
	}
	if !strings.Contains(out, "active task") {
		t.Errorf("human work: %q, want the resolved task title", out)
	}
	if !strings.Contains(out, "working") || !strings.Contains(out, "ago") {
		t.Errorf("human work: %q, want kind 'working' and an age suffix", out)
	}
	// The presence claim is by pi; the ghost assignment must not surface a
	// second row (assignment != presence).
	if strings.Count(out, "active task") != 1 {
		t.Errorf("human work: %q, want exactly one row for the task (presence only)", out)
	}
}
