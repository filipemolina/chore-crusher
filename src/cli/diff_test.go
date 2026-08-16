package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestDiffReturnsAddedOrChangedSince pins `farol diff <list-id> [--since
// <unix-seconds>]` (backlog #6): --json emits an array of the show --json
// task shape for tasks whose created_at/updated_at is strictly after the
// timestamp, an empty array when nothing changed, and a domain-error object
// (exit 1) for an unknown list id (docs/DESIGN.md §9).
func TestDiffReturnsAddedOrChangedSince(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))

	// since 0: both tasks come back, in the show --json task shape.
	mustCLI(t, data, "add", lid, "one")
	mustCLI(t, data, "add", lid, "two")
	var payload []showJSON
	mustJSONCLI(t, data, &payload, "diff", lid, "--since", "0", "--json")
	if len(payload) != 2 {
		t.Fatalf("diff since 0: %d tasks, want 2: %+v", len(payload), payload)
	}
	if payload[0].Title == "" || payload[0].ListID != lid {
		t.Errorf("diff rows must carry the show --json task shape, got %+v", payload[0])
	}

	// A timestamp after both creations: nothing changed, and --json must be
	// an empty array, not null.
	mustJSONCLI(t, data, &payload, "diff", lid, "--since", "9999999999", "--json")
	if len(payload) != 0 {
		t.Fatalf("diff after all creations: %d tasks, want []", len(payload))
	}

	// Unknown list id: domain-error object on stdout, nothing on stderr,
	// exit 1 (the §9 failure contract).
	code, out, errOut := runCLI(t, data, "diff", "00000000", "--since", "0", "--json")
	if code != 1 {
		t.Errorf("diff bad id: exit %d, want 1", code)
	}
	if errOut != "" {
		t.Errorf("stderr must stay empty in --json mode, got %q", errOut)
	}
	var errShape struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &errShape); err != nil || errShape.Error == "" {
		t.Errorf("diff bad id --json = %q, want an {\"error\": ...} object", out)
	}
}

// TestDiffJSONIsOneValue pins the §9 mechanical contract for the new command:
// --json writes exactly one JSON value (a bare array) to stdout, never text
// wrapped around it.
func TestDiffJSONIsOneValue(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	mustCLI(t, data, "add", lid, "one")

	code, out, errOut := runCLI(t, data, "diff", lid, "--since", "0", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("diff --json stdout = %q, want a bare array starting with '['", out)
	}
	var v any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Errorf("stdout %q is not one JSON value: %v", out, err)
	}
}
