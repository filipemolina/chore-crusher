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
