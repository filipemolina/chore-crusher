package tasktree

import (
	"testing"
	"unicode/utf8"
)

// TestFitTitleAndSuffixDropOrder sweeps a constant prefix over a range of
// panel widths and asserts the phase-9 drop order (docs/DESIGN.md §12, and
// the %s comment above) at every step: the trailing percentage is shed
// whole or not at all — never as a fragment like "(7" — and the title never
// shrinks while shedding the percentage could buy it columns back.
func TestFitTitleAndSuffixDropOrder(t *testing.T) {
	const prefix = 3 // indent + glyph + space + checkbox + space, root-depth row
	cases := []struct {
		title, suffix string
	}{
		{"Water the ferns", "(63%)"},
		{"a", "(100%)"},
		{"singleverylongwordwithnospacestotruncateagainst", "(7%)"},
	}
	for _, c := range cases {
		for width := 0; width <= 60; width++ {
			title, suffix := fitTitleAndSuffix(c.title, c.suffix, prefix, width)

			// The suffix is atomic: shed whole or kept whole, never a fragment.
			if suffix != "" && suffix != c.suffix {
				t.Fatalf("width %d: suffix = %q, want %q or \"\" (never a fragment)",
					width, suffix, c.suffix)
			}

			// The rendered pair + prefix must never overflow the panel (title
			// measured by runes: chrome.Truncate appends a single-column “…”).
			avail := width - prefix
			if avail > 0 && utf8.RuneCountInString(title)+len(suffix) > avail {
				t.Fatalf("width %d: %q + %q exceeds the %d columns a %d-prefix panel allows",
					width, title, suffix, avail, width)
			}

			// Drop-order: when the suffix was shed, the title must not be
			// shorter than the full title if it fits — the suffix yields its
			// columns to the title, not the other way around.
			fitsFullRow := len(c.title)+len(c.suffix) <= avail
			if suffix == "" && fitsFullRow {
				t.Fatalf("width %d: shed the suffix (%q) though the whole row fits; title %q",
					width, c.suffix, title)
			}
		}
	}
}
