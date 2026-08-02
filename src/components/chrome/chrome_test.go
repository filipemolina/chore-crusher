package chrome

import (
	"image/color"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/filipemolina/chore-crusher/src/appstyles"
)

// TestTruncateNeverEndsMidRuneOrMidEscape is the mechanical backstop for
// docs/DESIGN.md §12's "never truncate to a fragment" rule: for a sweep of
// widths against both plain and styled input, Truncate must never return a
// string that ends mid-rune or mid-escape-sequence (an \x1b that has not yet
// reached its terminating 'm'), and its visible width must never exceed the
// requested width.
func TestTruncateNeverEndsMidRuneOrMidEscape(t *testing.T) {
	styled := "\x1b[31mBuy paint for the fence\x1b[0m"
	plain := "日本語のテキスト with emoji 🎨 and accents: café"
	inputs := []string{styled, plain, "a", "", "💡💡💡"}

	for _, s := range inputs {
		t.Run(string(rune(len(s)))+"chars", func(t *testing.T) {
			for w := 0; w <= 24; w++ {
				out := Truncate(s, w)
				assertTruncation(t, s, w, out)
			}
		})
	}
}

// assertTruncation checks the three invariants: the visible width never
// exceeds w, the string is valid UTF-8 that never ends mid-escape-sequence,
// and the visible text is a prefix of the input's visible text (so
// Truncate only ever removes a suffix, never reorders or invents).
func assertTruncation(t *testing.T, input string, w int, out string) {
	t.Helper()

	if got := ansi.StringWidth(out); got > max(w, 0) {
		t.Errorf("Truncate(%q, %d) = %q: width %d, want ≤ %d", input, w, out, got, max(w, 0))
	}

	// No dangling escape: every \x1b must be followed by a terminating 'm'
	// before any non-escape text, and the string cannot end inside a
	// sequence. (An unescaped \x1b at the very end is mid-sequence.)
	if i := strings.IndexByte(out, 0x1b); i >= 0 {
		rest := out[i:]
		for rest != "" {
			j := strings.IndexByte(rest, 0x1b)
			if j < 0 {
				break
			}
			seqEnd := strings.IndexByte(rest[j:], 'm')
			if seqEnd < 0 {
				t.Errorf("Truncate(%q, %d) = %q: ends mid-escape-sequence", input, w, out)
				return
			}
			rest = rest[j+seqEnd+1:]
		}
	}

	if !utf8.ValidString(out) {
		t.Errorf("Truncate(%q, %d) = %q: not valid UTF-8", input, w, out)
		return
	}

	// The visible text must be a prefix of the input's visible text, once
	// the trailing ellipsis is discounted.
	stripped := ansi.Strip(out)
	trimmed := strings.TrimSuffix(stripped, "…")
	if !strings.HasPrefix(ansi.Strip(input), trimmed) {
		t.Errorf("Truncate(%q, %d) = %q: visible text %q is not a prefix of input", input, w, out, trimmed)
	}
}

// The trailing marker must be a single ellipsis, never the input's own last
// characters followed by anything else.
func TestTruncateAppendsSingleEllipsis(t *testing.T) {
	out := Truncate("abcdef", 4)
	if got, want := ansi.Strip(out), "abc…"; got != want {
		t.Errorf("Truncate(\"abcdef\", 4) = %q, want visible %q", out, want)
	}
	if n := strings.Count(ansi.Strip(out), "…"); n != 1 {
		t.Errorf("expected exactly one ellipsis, got %d", n)
	}
}

func TestTruncateFittingStringUnchanged(t *testing.T) {
	if got := Truncate("short", 20); got != "short" {
		t.Errorf("Truncate(\"short\", 20) = %q, want unchanged", got)
	}
	if got := Truncate("", 5); got != "" {
		t.Errorf("Truncate(\"\", 5) = %q, want empty", got)
	}
	if got := Truncate("abc", 0); got != "" {
		t.Errorf("Truncate(\"abc\", 0) = %q, want empty", got)
	}
}

// TestTruncatePreservesStyles checks that a styled string that fits keeps
// its escape sequences intact — truncation must not strip or mangle them.
func TestTruncatePreservesStyles(t *testing.T) {
	styled := "\x1b[31mred text\x1b[0m"
	out := Truncate(styled, 50)
	if out != styled {
		t.Errorf("fitting styled string changed: got %q, want %q", out, styled)
	}

	out = Truncate(styled, 6)
	if !strings.Contains(out, "\x1b[31m") {
		t.Errorf("truncated styled string lost its opening sequence: %q", out)
	}
}

// The empty-state card must render at exactly the box it was given, with no
// unpainted cells, under every registered theme — the recessed surface is
// where a missing seal would show the panel behind it.
func TestEmptyStateCardFillsBoxAndSeals(t *testing.T) {
	for _, theme := range appstyles.Themes {
		t.Run(theme.Name, func(t *testing.T) {
			orig := appstyles.Active
			appstyles.Active = theme
			defer func() { appstyles.Active = orig }()

			card := EmptyStateCard("nothing here yet", 40, 6, color.White)
			if got, want := lipgloss.Width(card), 40; got != want {
				t.Errorf("EmptyStateCard width = %d, want %d", got, want)
			}
			if got, want := lipgloss.Height(card), 6; got != want {
				t.Errorf("EmptyStateCard height = %d, want %d", got, want)
			}
			if appstyles.HasBackgroundBleed(card) {
				t.Errorf("EmptyStateCard has unpainted cells under %s:\n%q", theme.Name, card)
			}
		})
	}
}

func TestEmptyStateCardZeroBox(t *testing.T) {
	if got := EmptyStateCard("x", 0, 5, color.White); got != "" {
		t.Errorf("zero width should render nothing, got %q", got)
	}
}

func TestPanelFrameTitlesAndSealsEveryTheme(t *testing.T) {
	for _, theme := range appstyles.Themes {
		t.Run(theme.Name, func(t *testing.T) {
			original := appstyles.Active
			appstyles.Active = theme
			defer func() { appstyles.Active = original }()

			frame := PanelFrame("Tasks", true, 32, 9, "body")
			if got, want := lipgloss.Width(frame), 32; got != want {
				t.Errorf("PanelFrame width = %d, want %d", got, want)
			}
			if got, want := lipgloss.Height(frame), 9; got != want {
				t.Errorf("PanelFrame height = %d, want %d", got, want)
			}
			if appstyles.HasBackgroundBleed(frame) {
				t.Errorf("PanelFrame has unpainted cells under %s: %q", theme.Name, frame)
			}

			lines := strings.Split(ansi.Strip(frame), "\n")
			if !strings.Contains(lines[1], "Tasks") || strings.TrimSpace(lines[2]) != "" || !strings.Contains(lines[3], "body") {
				t.Errorf("title chrome = %q, want title then blank row then body", ansi.Strip(frame))
			}
			accentMarker := lipgloss.NewStyle().Background(theme.Accent).Render("x")
			accentSGR := accentMarker[:strings.Index(accentMarker, "x")]
			if !strings.Contains(frame, accentSGR) {
				t.Errorf("title chip did not use the active theme accent")
			}
			if strings.ContainsAny(ansi.Strip(frame), "╭╮╰╯│─") {
				t.Errorf("PanelFrame introduced a border: %q", ansi.Strip(frame))
			}
		})
	}
}

func TestPanelBodyWithFooterPinsFooterAndClipsContent(t *testing.T) {
	body := PanelBodyWithFooter(20, 4, appstyles.Active.BackgroundPanel, "one\ntwo\nthree\nfour", "input")
	lines := strings.Split(ansi.Strip(body), "\n")
	if got, want := len(lines), 4; got != want {
		t.Fatalf("PanelBodyWithFooter lines = %d, want %d: %q", got, want, ansi.Strip(body))
	}
	if got, want := strings.TrimSpace(lines[3]), "input"; got != want {
		t.Errorf("footer line = %q, want %q", got, want)
	}
	if strings.Contains(ansi.Strip(body), "four") {
		t.Errorf("overflowing content was not clipped: %q", ansi.Strip(body))
	}
}
