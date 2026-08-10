package appstyles

import (
	"strconv"
	"strings"
)

// HasDefaultForeground reports whether any visible glyph in block renders
// with no foreground SGR in effect, i.e. in the terminal's own default
// color. It is the foreground analogue of HasBackgroundBleed, and exists
// for the same reason: a sealed background is not enough when the text
// itself carries no color.
//
// Why this matters: a style that sets only a background leaves the
// foreground unset, so the glyph draws in whatever the user's terminal
// calls "normal text". Nearly every terminal default is light, which reads
// on the app's dark panels but vanishes on a light theme's panels
// (crush-day made the bug visible: pending task titles rendered white on
// warm off-white). The fix is never here — foreground cannot be repainted
// mechanically the way FillBackground repaints whitespace — so this
// function only asserts the invariant, and components keep it by drawing
// every glyph from an appstyles.Active tier.
func HasDefaultForeground(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		if lineHasDefaultForeground(line) {
			return true
		}
	}

	return false
}

// lineHasDefaultForeground walks one line tracking whether a foreground is
// currently in effect, and reports the first visible glyph found while it
// is not. Spaces carry no visible information, so only non-space glyphs
// count — the same reasoning FillBackground's whitespace patching uses.
func lineHasDefaultForeground(line string) bool {
	fgSet := false
	rest := line

	for rest != "" {
		idx := strings.IndexByte(rest, 0x1b)
		if idx < 0 {
			// Trailing plain text, no more escapes on this line.
			return !fgSet && hasVisibleGlyph(rest)
		}
		if !fgSet && hasVisibleGlyph(rest[:idx]) {
			return true
		}

		seqEnd := strings.IndexByte(rest[idx:], 'm')
		if seqEnd < 0 {
			// Not an SGR sequence; nothing further to reason about.
			return false
		}

		seq := rest[idx : idx+seqEnd+1]
		fgSet = sgrUpdateForeground(seq, fgSet)
		rest = rest[idx+seqEnd+1:]
	}

	return false
}

// hasVisibleGlyph reports whether s contains at least one non-space rune.
// Escape sequences never appear inside s — the caller splits on them first.
func hasVisibleGlyph(s string) bool {
	for _, r := range s {
		if r != ' ' {
			return true
		}
	}

	return false
}

// sgrUpdateForeground interprets one SGR sequence ("\x1b[...m") as a state
// transition on "a foreground is in effect", carrying the prior state in and
// out: a sequence that only touches the background or attributes must leave
// an already-set foreground alone (lipgloss emits "\x1b[49m" to drop a
// background while keeping the text color). It understands the reset codes,
// the standard and bright foreground ranges, the explicit-clear code, and
// the extended-color forms lipgloss emits.
func sgrUpdateForeground(seq string, fgSet bool) bool {
	// The full resets clear everything, foreground included.
	if seq == "\x1b[m" || seq == "\x1b[0m" {
		return false
	}

	// Strip the "\x1b[" prefix and the trailing "m".
	params := seq[len("\x1b[") : len(seq)-1]

	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			continue
		}
		switch {
		case n == 0:
			fgSet = false
		case n == 38 && i+1 < len(parts):
			// Extended foreground: 38;5;n or 38;2;r;g;b — consume the
			// sub-parameters so they are not misread as bare codes.
			if parts[i+1] == "5" {
				i++
			} else if parts[i+1] == "2" {
				i += 3
			}
			fgSet = true
		case n == 39:
			fgSet = false
		case (n >= 30 && n <= 37) || (n >= 90 && n <= 97):
			fgSet = true
		}
	}

	return fgSet
}
