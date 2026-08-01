package chrome

import "github.com/charmbracelet/x/ansi"

// Truncate hard-truncates s to w display columns, appending a single
// ellipsis when it is shortened. lipgloss Width wraps rather than truncates,
// so cells are pre-truncated to keep every row on a single line
// (docs/DESIGN.md §12 "Truncation").
//
// The rule the DESIGN pins: cut to width - 1 display cells and append a
// single …, never mid-escape-sequence. Ported from stack-stitcher's
// chrome.Truncate with one correction: the original cut with
// runewidth.Truncate, which is not ANSI-aware and slices through an escape
// sequence the moment a styled string no longer fits ("\x1b[3…"). This uses
// ansi.Truncate instead - the same parser-driven truncator lipgloss itself
// enforces MaxWidth with - which keeps escape sequences intact while
// counting display cells, and treats the text as graphemes so a rune or
// combining cluster is never split. The phase-3 plan's test (Truncate never
// ends mid-rune or mid-escape-sequence for a sweep of widths) is the
// mechanical backstop; the original implementation fails it on the first
// styled input.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}
