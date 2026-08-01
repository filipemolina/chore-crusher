package appstyles

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// FillBackground repaints `block` so every cell on every line carries `bg`.
//
// Why this is needed: a terminal's SGR reset (`\x1b[m`) clears the background
// along with everything else, and it applies until the next SGR. lipgloss
// closes every styled run with a reset, so any *unstyled* text that follows a
// styled run on the same line renders on the terminal's default background —
// a visible notch in an otherwise solid panel.
//
// Two things in the render pipeline produce exactly that unstyled text:
//
//   - lipgloss.JoinVertical/JoinHorizontal pad shorter blocks to the widest
//     block with a bare strings.Repeat(" ", n) — see join.go. Unlike a style's
//     own Width() padding, which alignTextHorizontal runs through the style's
//     whitespace renderer, Join's padding carries no SGR at all.
//   - bubbles components (the lists' "No items." row, for one) join their
//     inner rows the same way.
//
// Wrapping the result in an outer Background() style does not help: that only
// styles the padding the outer style itself adds, so the bare spaces already
// sitting inside the block survive untouched.
//
// So the fix has to run over the finished string. For each line we re-assert
// `bg` immediately after every reset, and open the line with it, which leaves
// any explicit inner background (a title pill, a status dot) intact while
// closing every gap around it. Foreground is deliberately not re-asserted:
// the runs we are patching are whitespace, and real text always brings its
// own foreground.
//
// Apply this once per background tier, at the point that tier is established
// (see the tier comments in this package and docs/DESIGN.md). Applying it at
// an outer tier only would paint inner panels with the outer tier's color.
func FillBackground(bg color.Color, block string) string {
	seq := backgroundSeq(bg)
	if seq == "" || block == "" {
		return block
	}

	lines := strings.Split(block, "\n")
	for i, line := range lines {
		// A reset mid-line drops us to the default background; re-open `bg`
		// straight after it so the following cells stay painted.
		line = strings.ReplaceAll(line, ansi.ResetStyle, ansi.ResetStyle+seq)

		// Open the line painted, and close it so the color does not run past
		// the block into whatever the compositor puts to the right.
		lines[i] = seq + line + ansi.ResetStyle
	}

	return strings.Join(lines, "\n")
}

// HasBackgroundBleed reports whether any line in block contains a run of
// spaces that will render on the terminal's default background: spaces that
// follow a reset with no SGR in between. It is the inverse of what
// FillBackground guarantees, and exists so tests in any package can assert the
// invariant on a fully rendered frame rather than eyeballing a screenshot.
func HasBackgroundBleed(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		if lineHasBleed(line) {
			return true
		}
	}

	return false
}

// lineHasBleed walks one line tracking whether a background is currently in
// effect, and reports the first run of spaces found while it is not.
func lineHasBleed(line string) bool {
	painted := false
	rest := line

	for rest != "" {
		idx := strings.IndexByte(rest, 0x1b)
		if idx < 0 {
			// Trailing plain text, no more escapes on this line.
			return !painted && strings.Contains(rest, " ")
		}

		if !painted && strings.Contains(rest[:idx], " ") {
			return true
		}

		seqEnd := strings.IndexByte(rest[idx:], 'm')
		if seqEnd < 0 {
			// Not an SGR sequence; nothing further to reason about.
			return false
		}

		seq := rest[idx : idx+seqEnd+1]
		painted = seq != ansi.ResetStyle && seq != "\x1b[0m"
		rest = rest[idx+seqEnd+1:]
	}

	return false
}

// backgroundSeq returns the bare SGR sequence that sets `bg` as the background,
// e.g. "\x1b[48;2;21;21;32m". It is derived from lipgloss rather than built by
// hand so that lipgloss's own color handling (including downsampling on
// terminals without truecolor) decides how the color is encoded.
func backgroundSeq(bg color.Color) string {
	if bg == nil {
		return ""
	}

	return strings.TrimSuffix(
		lipgloss.NewStyle().Background(bg).Render(""),
		ansi.ResetStyle,
	)
}
