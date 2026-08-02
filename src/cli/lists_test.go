package cli

import (
	"strings"
	"testing"
)

func TestListsLifecycle(t *testing.T) {
	data := t.TempDir()

	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home renovation"))
	if lid == "" {
		t.Fatal("lists add printed no id")
	}

	// Human table shows the list; exact counts are pinned by the --json
	// assertion below (tabwriter pads columns, so don't assert on spacing).
	out := mustCLI(t, data, "lists")
	if !strings.Contains(out, "Home renovation") {
		t.Errorf("lists table %q, want the new list", out)
	}

	// Rename takes effect.
	mustCLI(t, data, "lists", "rename", lid, "Renovation")
	out = mustCLI(t, data, "lists")
	if strings.Contains(out, "Home renovation") || !strings.Contains(out, "Renovation") {
		t.Errorf("lists after rename %q", out)
	}

	// JSON shape: exactly the documented fields, one list.
	var payload []listJSON
	mustJSONCLI(t, data, &payload, "lists", "--json")
	if len(payload) != 1 || payload[0].ID != lid || payload[0].Name != "Renovation" ||
		payload[0].Pending != 0 || payload[0].Complete != 0 {
		t.Errorf("lists --json: %+v", payload)
	}

	// rm refuses without --force — and must not delete anything.
	code, _, errOut := runCLI(t, data, "lists", "rm", lid)
	if code != 1 || !strings.Contains(errOut, "--force") {
		t.Errorf("rm without --force: exit %d stderr %q, want exit 1 naming --force", code, errOut)
	}
	out = mustCLI(t, data, "lists")
	if !strings.Contains(out, "Renovation") {
		t.Errorf("list deleted despite missing --force: %q", out)
	}

	// rm --force deletes; human output for an empty result is empty, JSON is [].
	mustCLI(t, data, "lists", "rm", lid, "--force")
	if out := mustCLI(t, data, "lists"); out != "" {
		t.Errorf("lists after rm --force: %q, want empty human output", out)
	}
	var empty []listJSON
	mustJSONCLI(t, data, &empty, "lists", "--json")
	if len(empty) != 0 {
		t.Errorf("lists --json after rm: %+v, want an empty array", empty)
	}
}

func TestListsAddRejectsEmptyName(t *testing.T) {
	data := t.TempDir()
	code, out, errOut := runCLI(t, data, "lists", "add", "   ")
	if code != 1 {
		t.Errorf("empty name: exit %d, want 1 (store's domain error)", code)
	}
	if out != "" || !strings.Contains(errOut, "crush: ") {
		t.Errorf("empty name: stdout %q stderr %q, want the human error shape", out, errOut)
	}
}

// TestPrefixResolution: any <list-id>/<task-id> argument accepts an
// unambiguous prefix (docs/DESIGN.md §9).
func TestPrefixResolution(t *testing.T) {
	data := t.TempDir()
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "Buy paint"))

	mustCLI(t, data, "rename", tid[:8], "Renamed task")
	var payload []taskRowJSON
	mustJSONCLI(t, data, &payload, "tasks", lid[:6], "--json")
	if payload[0].Title != "Renamed task" {
		t.Errorf("prefix rename/resolve: %+v", payload)
	}
}
