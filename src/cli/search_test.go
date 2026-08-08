package cli

import (
	"strings"
	"testing"
)

func TestSearch(t *testing.T) {
	data := t.TempDir()
	l1 := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Home renovation"))
	l2 := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Garden"))
	t1 := strings.TrimSpace(mustCLI(t, data, "add", l1, "Buy paint"))
	mustCLI(t, data, "add", l1, "Weed the garden")
	mustCLI(t, data, "add", l2, "Paint the fence")

	// Title matches across lists, each with its list name in context.
	out := mustCLI(t, data, "search", "paint")
	if !strings.Contains(out, "Home renovation") || !strings.Contains(out, "Buy paint") ||
		!strings.Contains(out, "Garden") || !strings.Contains(out, "Paint the fence") {
		t.Errorf("search paint: %q, want both matches with their list names", out)
	}

	// --list scopes the search.
	out = mustCLI(t, data, "search", "paint", "--list", l2)
	if !strings.Contains(out, "Paint the fence") || strings.Contains(out, "Buy paint") {
		t.Errorf("search paint --list: %q, want only the Garden hit", out)
	}

	// Notes-only matches still surface (a real hit, weaker than a title one).
	mustCLI(t, data, "notes", t1, "replace the kitchen sink")
	out = mustCLI(t, data, "search", "kitchen")
	if !strings.Contains(out, "Buy paint") {
		t.Errorf("search kitchen: %q, want the notes-only match", out)
	}

	// JSON shape: one object per hit with id, list, status, progress.
	var payload []searchResultJSON
	mustJSONCLI(t, data, &payload, "search", "paint", "--json")
	if len(payload) != 2 {
		t.Fatalf("search paint --json: %d hits, want 2", len(payload))
	}
	for _, r := range payload {
		if r.ID == "" || r.ListName == "" || r.Status == "" {
			t.Errorf("search hit missing fields: %+v", r)
		}
		// Human-created lists (no --owner) carry an empty list_owner.
		if r.ListOwner != "" {
			t.Errorf("search hit list_owner = %q, want empty for unowned list", r.ListOwner)
		}
	}

	// No matches: empty human output, empty JSON array, exit 0.
	if out := mustCLI(t, data, "search", "zzzz-no-such-text"); out != "" {
		t.Errorf("search no-match: %q, want empty human output", out)
	}
	var none []searchResultJSON
	mustJSONCLI(t, data, &none, "search", "zzzz-no-such-text", "--json")
	if len(none) != 0 {
		t.Errorf("search no-match --json: %+v, want an empty array", none)
	}
}

// TestSearchJSONCarriesListOwner verifies that list_owner appears on search
// results: empty for human-created (unowned) lists and the agent tag for
// lists created via --owner (CONTRIBUTING rule 6 shape: list_owner is the
// parent list's created_by, exposed alongside list_id and list_name).
func TestSearchJSONCarriesListOwner(t *testing.T) {
	data := t.TempDir()
	owned := strings.TrimSpace(mustCLI(t, data, "lists", "add", "pi: Backlog", "--owner", "pi"))
	mustCLI(t, data, "add", owned, "Plan migration")

	var payload []searchResultJSON
	mustJSONCLI(t, data, &payload, "search", "migration", "--json")
	if len(payload) != 1 {
		t.Fatalf("search: %d hits, want 1", len(payload))
	}
	if payload[0].ListOwner != "pi" {
		t.Errorf("list_owner = %q, want 'pi'", payload[0].ListOwner)
	}
}

// TestSearchJSONCarriesAssignmentAndPriority pins the two fields
// docs/DESIGN.md §9 lists in the `search --json` row shape. They are easy to
// miss: `search` builds its own row type rather than reusing taskRowJSON, so
// a field added to the tasks/show rows does not reach search for free.
func TestSearchJSONCarriesAssignmentAndPriority(t *testing.T) {
	data := t.TempDir()
	l := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Backlog"))
	id := strings.TrimSpace(mustCLI(t, data, "add", l, "Plan migration"))
	mustCLI(t, data, "assign", id)
	mustCLI(t, data, "priority", id, "--level", "high")

	var payload []searchResultJSON
	mustJSONCLI(t, data, &payload, "search", "migration", "--json")
	if len(payload) != 1 {
		t.Fatalf("search: %d hits, want 1", len(payload))
	}
	if payload[0].Assignee != "agent" {
		t.Errorf("assignee = %q, want 'agent'", payload[0].Assignee)
	}
	if payload[0].Priority != "high" {
		t.Errorf("priority = %q, want 'high'", payload[0].Priority)
	}
}

// TestSearchJSONUnassignedRowsAreExplicit pins the default shape: an
// untouched task reports assignee "" and priority "none", not an omitted
// field — a caller reads the row, never the absence of a key.
func TestSearchJSONUnassignedRowsAreExplicit(t *testing.T) {
	data := t.TempDir()
	l := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Backlog"))
	mustCLI(t, data, "add", l, "Plan migration")

	var payload []searchResultJSON
	mustJSONCLI(t, data, &payload, "search", "migration", "--json")
	if len(payload) != 1 {
		t.Fatalf("search: %d hits, want 1", len(payload))
	}
	if payload[0].Assignee != "" {
		t.Errorf("assignee = %q, want empty", payload[0].Assignee)
	}
	if payload[0].Priority != "none" {
		t.Errorf("priority = %q, want 'none'", payload[0].Priority)
	}
}
