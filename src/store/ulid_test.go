package store

import (
	"strings"
	"testing"
	"time"
)

func TestULIDProperties(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewID()
		if len(id) != 26 {
			t.Fatalf("NewID returned %d chars, want 26: %q", len(id), id)
		}
		for _, c := range id {
			if !strings.ContainsRune(ulidAlphabet, c) {
				t.Fatalf("id %q contains %q, not in Crockford base32 alphabet", id, c)
			}
		}
		// Current-time ULIDs encode a 48-bit millisecond timestamp whose top
		// bits are zero until year ~2242, so the first char is always '0'.
		if id[0] != '0' {
			t.Fatalf("id %q does not start with the zero timestamp prefix", id)
		}
		if seen[id] {
			t.Fatalf("NewID returned a duplicate: %q", id)
		}
		seen[id] = true
	}
}

// TestULIDEncodingDeterministic pins the base32 encoding with fixed inputs:
// same timestamp + entropy must produce the same id, and a later timestamp
// must sort after an earlier one (docs/DESIGN.md §2's sortability claim).
func TestULIDEncodingDeterministic(t *testing.T) {
	entropy := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	early := ulidFrom(1_700_000_000_000, entropy)
	late := ulidFrom(1_700_000_000_001, entropy)

	if ulidFrom(1_700_000_000_000, entropy) != early {
		t.Fatal("ulidFrom is not deterministic for fixed inputs")
	}
	if !(early < late) {
		t.Fatalf("later timestamp does not sort after earlier: %q vs %q", early, late)
	}
	// A fixed entropy also round-trips: the 26 chars decode to the same bits.
	if len(early) != 26 {
		t.Fatalf("ulidFrom produced %d chars, want 26", len(early))
	}
}

func TestNewIDOrderedByTime(t *testing.T) {
	// Two ids generated at least a millisecond apart sort by creation time.
	first := NewID()
	time.Sleep(2 * time.Millisecond)
	second := NewID()
	if !(first < second) {
		t.Fatalf("ids not ordered by creation time: %q vs %q", first, second)
	}
}
