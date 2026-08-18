package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestClaimClaimsPresence pins the core behaviour: a claim (explicit, under
// FAROL_AGENT) leaves a live presence row on the entity, visible to farol work
// and the TUI spinner. The JSON payload echoes the row id and entity so a
// caller can reason about what it holds.
func TestClaimClaimsPresence(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")

	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "t"))

	var res claimResultJSON
	mustJSONCLI(t, data, &res, "claim", tid, "--json")
	if !res.OK || res.ID == "" {
		t.Fatalf("claim ok/id: %+v", res)
	}
	if res.EntityType != "task" || res.EntityID != tid || res.Kind != "working" {
		t.Errorf("claim payload = %+v, want task/%s/working", res, tid)
	}

	// The claim must be live: farol work shows it.
	var work []workClaimJSON
	mustJSONCLI(t, data, &work, "work", "--json")
	if len(work) != 1 || work[0].EntityID != tid {
		t.Errorf("work after claim = %+v, want exactly the claimed task", work)
	}
}

// TestClaimKindInspecting pins the --kind flag: inspecting is a distinct claim
// kind from working, and the store accepts it.
func TestClaimKindInspecting(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")

	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "t"))

	var res claimResultJSON
	mustJSONCLI(t, data, &res, "claim", tid, "--kind", "inspecting", "--json")
	if res.Kind != "inspecting" {
		t.Errorf("claim kind = %q, want inspecting", res.Kind)
	}
}

// TestClaimBadKind is a usage error: an unknown --kind must not reach the store.
func TestClaimBadKind(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "t"))

	code, out, _ := runCLI(t, data, "claim", tid, "--kind", "bogus", "--json")
	if code != 1 { // parseActivityKind returns a domain error -> exit 1
		t.Fatalf("claim bad kind: exit %d, want 1", code)
	}
	// Under --json the error is on stdout as {"error": ...}.
	if !strings.Contains(out, "unknown kind") {
		t.Errorf("stdout = %q, want 'unknown kind'", out)
	}
}

// TestClaimResolvesList pins that claim accepts a list id too (resolveEntity
// tries task then list), so a list-level claim lights the lists-panel spinner.
func TestClaimResolvesList(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))

	var res claimResultJSON
	mustJSONCLI(t, data, &res, "claim", lid, "--json")
	if res.EntityType != "list" || res.EntityID != lid {
		t.Errorf("claim list payload = %+v, want list/%s", res, lid)
	}
}

// TestReleaseClearsClaim pins the release path: after a claim, release removes
// the spinner. A release with nothing held is a normal no-op (exit 0, cleared=0).
func TestReleaseClearsClaim(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "t"))

	mustCLI(t, data, "claim", tid)
	mustCLI(t, data, "release", tid)

	var work []workClaimJSON
	mustJSONCLI(t, data, &work, "work", "--json")
	if len(work) != 0 {
		t.Errorf("work after release = %+v, want empty", work)
	}
}

// TestReleaseAllClearsEveryClaim pins --all: it clears every claim this agent
// holds across entities, reusing store.ReleaseAgentClaims.
func TestReleaseAllClearsEveryClaim(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	t1 := strings.TrimSpace(mustCLI(t, data, "add", lid, "t1"))
	t2 := strings.TrimSpace(mustCLI(t, data, "add", lid, "t2"))

	mustCLI(t, data, "claim", t1)
	mustCLI(t, data, "claim", t2)

	var res releaseResultJSON
	mustJSONCLI(t, data, &res, "release", "--all", "--json")
	if res.Cleared != 2 {
		t.Errorf("release --all cleared = %d, want 2", res.Cleared)
	}
	var work []workClaimJSON
	mustJSONCLI(t, data, &work, "work", "--json")
	if len(work) != 0 {
		t.Errorf("work after release --all = %+v, want empty", work)
	}
}

// TestReleaseIsNoOpWhenUnheld pins the ReleaseWork contract: releasing an
// entity you never claimed is not an error.
func TestReleaseIsNoOpWhenUnheld(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "t"))

	var res releaseResultJSON
	mustJSONCLI(t, data, &res, "release", tid, "--json")
	if !res.OK {
		t.Errorf("release unheld: ok = %v, want true", res.OK)
	}
}

// TestClaimConflictRefuses pins the no-steal rule: a claim held by another
// agent is refused (domain error), and the spinner stays with the holder.
func TestClaimConflictRefuses(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "t"))

	// pi claims.
	mustCLI(t, data, "claim", tid)

	// A different agent tries to claim the same task.
	t.Setenv("FAROL_AGENT", "claude")
	code, out, _ := runCLI(t, data, "claim", tid, "--json")
	if code != 1 {
		t.Fatalf("conflicting claim: exit %d, want 1", code)
	}
	if !strings.Contains(out, "claimed by another agent") {
		t.Errorf("stdout = %q, want 'claimed by another agent'", out)
	}

	// The spinner still belongs to pi.
	t.Setenv("FAROL_AGENT", "pi")
	var work []workClaimJSON
	mustJSONCLI(t, data, &work, "work", "--json")
	if len(work) != 1 || work[0].AgentID != "pi" {
		t.Errorf("work = %+v, want one claim held by pi", work)
	}
}

// TestSkillEmitsMarkdown pins farol skill: it prints the reference prose in
// human mode (contains the identity contract and the JSON contract sections)
// and emits one JSON value under --json.
func TestSkillEmitsMarkdown(t *testing.T) {
	data := t.TempDir()
	code, out, _ := runCLI(t, data, "skill")
	if code != 0 {
		t.Fatalf("skill: exit %d, want 0", code)
	}
	for _, want := range []string{"FAROL_AGENT", "farol inbox", "Presence vs. assignment", "JSON contract", "Progress discipline", "write-heartbeat"} {
		if !strings.Contains(out, want) {
			t.Errorf("skill output missing %q", want)
		}
	}
}

// TestSkillJSONIsOneValue pins the §9 contract for the doc command: --json
// emits exactly one JSON value wrapping the prose.
func TestSkillJSONIsOneValue(t *testing.T) {
	data := t.TempDir()
	code, out, _ := runCLI(t, data, "skill", "--json")
	if code != 0 {
		t.Fatalf("skill --json: exit %d, want 0", code)
	}
	var v struct {
		Skill string `json:"skill"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("skill --json: stdout %q not one JSON value: %v", out, err)
	}
	if v.Skill == "" {
		t.Errorf("skill --json: empty skill field")
	}
}
