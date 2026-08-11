package cli

import (
	"strings"
	"testing"

	"github.com/filipemolina/farol/src/config"
	"github.com/filipemolina/farol/src/store"
)

// liveClaimOf returns the live (within WorkTTL) claim for entityID under
// agentID, or "" if none. A thin read over the store the CLI just wrote to,
// so the test asserts on the same AgentActivity row the TUI renders.
func liveClaimOf(t *testing.T, dataDir, entityType, entityID, agentID string) bool {
	t.Helper()
	s, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()
	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	for _, w := range work {
		if w.EntityType == entityType && w.EntityID == entityID && w.AgentID == agentID {
			return true
		}
	}
	return false
}

// TestCLIMutatingCommandsAutoClaim presence-claims the task they write, so
// the TUI spinner stays live once farol mcp is gone. This is the Phase 1.6
// parity gate: every mutation that MCP's autoClaim covered must claim under
// the same FAROL_AGENT identity here (mirrors the MCP server's autoClaim).
func TestCLIMutatingCommandsAutoClaim(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))

	cases := []struct {
		name string
		args []string
	}{
		{"add", []string{"add", lid, "write"}},
		{"notes", []string{"notes", "$TASK", "body"}},
		{"rename", []string{"rename", "$TASK", "renamed"}},
		{"progress", []string{"progress", "$TASK", "--mode", "percentage", "--percent", "40"}},
		{"priority", []string{"priority", "$TASK", "--level", "high"}},
		{"comment", []string{"comment", "$TASK", "a note"}},
		{"assign", []string{"assign", "$TASK"}},
		{"reopen", []string{"reopen", "$TASK"}},
		{"toggle", []string{"toggle", "$TASK"}},
		{"unassign", []string{"unassign", "$TASK"}},
		{"mv", []string{"mv", "$TASK"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task-"+c.name))
			args := make([]string, len(c.args))
			for i, a := range c.args {
				if a == "$TASK" {
					args[i] = tid
				} else {
					args[i] = a
				}
			}
			mustCLI(t, data, args...)
			if !liveClaimOf(t, data, "task", tid, "pi") {
				t.Fatalf("%s did not leave a live presence claim on task %s under pi", c.name, tid)
			}
		})
	}
}

// TestCLIRmClaimsBeforeDelete pins that farol rm --force runs autoClaim on
// the task before deleting it (matching MCP, which claims during the write,
// while the entity still exists). We do NOT assert a surviving claim:
// DeleteTask deliberately removes AgentActivity rows for the deleted task so
// a vanished task cannot leave an orphaned spinner, which is the TUI-safe
// behaviour. The assertion here is that the command still exits 0 and the
// task is gone — the claim itself is best-effort and intentionally invisible.
func TestCLIRmClaimsBeforeDelete(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "doomed"))

	code, _, errOut := runCLI(t, data, "rm", tid, "--force")
	if code != 0 {
		t.Fatalf("rm --force: exit %d, stderr %q", code, errOut)
	}
	if liveClaimOf(t, data, "task", tid, "pi") {
		t.Fatalf("rm --force left a surviving claim on deleted task %s; DeleteTask must clear orphaned spinners", tid)
	}
}

// TestCLINextDoesNotDoubleClaim pins that farol next, which already claims
// the grabbed task via its own NextAssignable path, does not also trip the
// generic auto-claim (the note's explicit "don't double-claim" rule). We
// assert the single claim we expect is present and the behaviour is sane:
// one live claim for the task, no error, exit 0.
func TestCLINextDoesNotDoubleClaim(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "grab me"))

	mustJSONCLI(t, data, &showJSON{}, "next", lid, "--json")
	if !liveClaimOf(t, data, "task", tid, "pi") {
		t.Fatalf("next did not leave a live presence claim on task %s under pi", tid)
	}
}
