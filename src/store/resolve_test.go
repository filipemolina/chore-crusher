package store

import (
	"strings"
	"testing"
)

func TestResolveIDUniquePrefix(t *testing.T) {
	s := newTestStore(t)
	lid := mustList(t, s, "list")

	full, err := s.ResolveID("list", lid)
	if err != nil {
		t.Fatalf("ResolveID with full id: %v", err)
	}
	if full != lid {
		t.Fatalf("ResolveID returned %q, want %q", full, lid)
	}

	// Every ULID starts with '0' (see ulid_test), so a prefix of the first
	// several chars is an unambiguous prefix of this one list.
	prefix := lid[:8]
	got, err := s.ResolveID("list", prefix)
	if err != nil {
		t.Fatalf("ResolveID with prefix %q: %v", prefix, err)
	}
	if got != lid {
		t.Fatalf("ResolveID(%q) = %q, want %q", prefix, got, lid)
	}
}

func TestResolveIDNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.ResolveID("task", "no-such-id")
	if err == nil {
		t.Fatal("ResolveID on a missing id did not error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("ResolveID error %q does not say not found", err)
	}
}

// TestResolveIDAmbiguousPrefix is the required coverage: a prefix matching
// more than one row is an error, never a silent pick of the first match.
func TestResolveIDAmbiguousPrefix(t *testing.T) {
	s := newTestStore(t)
	mustList(t, s, "first")
	mustList(t, s, "second")

	// Both ULIDs share the '0' leading char (current-time ULIDs), so "0" is
	// ambiguous across the two lists.
	_, err := s.ResolveID("list", "0")
	if err == nil {
		t.Fatal("ResolveID with an ambiguous prefix did not error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ResolveID error %q does not say ambiguous", err)
	}
}

func TestResolveIDEmptyPrefix(t *testing.T) {
	s := newTestStore(t)
	mustList(t, s, "list")
	if _, err := s.ResolveID("list", ""); err == nil {
		t.Fatal("ResolveID with an empty prefix did not error")
	}
}
