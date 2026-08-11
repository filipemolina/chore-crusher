package appstyles

import "testing"

// TestHasDefaultForeground pins the invariant that keeps light themes legible:
// any visible glyph drawn with no foreground SGR in effect reads in the
// terminal's own default color, which vanishes on farol-day's light panels.
// The table transcribes the SGR state machine lineHasDefaultForeground walks:
// a foreground is in effect from the first fg-setting sequence until a reset
// clears it, and only non-space glyphs count.
func TestHasDefaultForeground(t *testing.T) {
	tests := []struct {
		name  string
		block string
		want  bool
	}{
		{"plain text", "hello", true},
		{"styled text", "\x1b[38;2;232;164;74mhello\x1b[0m", false},
		{"styled then unstyled tail", "\x1b[38;2;232;164;74mhello\x1b[0m world", true},
		{"unstyled prefix before style", "hi \x1b[38;2;232;164;74mhello\x1b[0m", true},
		{"bare reset clears fg", "\x1b[38;2;232;164;74ma\x1b[m b", true},
		{"explicit 39 clears fg", "\x1b[38;2;232;164;74ma\x1b[39m b", true},
		{"standard fg code", "\x1b[31mred\x1b[0m", false},
		{"bright fg code", "\x1b[91mbright\x1b[0m", false},
		{"extended 256", "\x1b[38;5;208morange\x1b[0m", false},
		{"extended rgb", "\x1b[38;2;10;20;30mnavy\x1b[0m", false},
		{"combined fg and bg", "\x1b[38;2;10;20;30;48;2;40;50;60mboth\x1b[0m", false},
		{"bg alone does not set fg", "\x1b[48;2;40;50;60minvisible\x1b[0m", true},
		{"bold alone does not set fg", "\x1b[1mbold\x1b[0m", true},
		{"fg survives bg reset", "\x1b[38;2;10;20;30;48;2;40;50;60mtext\x1b[49mstill\x1b[0m", false},
		{"fg reset then new fg", "\x1b[38;2;10;20;30ma\x1b[0m\x1b[38;2;40;50;60mb\x1b[0m", false},
		{"spaces only", "     ", false},
		{"styled spaces with unstyled gap", "\x1b[38;2;10;20;30m   \x1b[0m   ", false},
		{"empty string", "", false},
		{"multiline first line clean", "\x1b[38;2;10;20;30mok\x1b[0m\nplain", true},
		{"multiline second line clean", "plain\n\x1b[38;2;10;20;30mok\x1b[0m", true},
		{"multiline all clean", "\x1b[38;2;10;20;30mok\x1b[0m\n\x1b[38;2;10;20;30mok\x1b[0m", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasDefaultForeground(tc.block); got != tc.want {
				t.Errorf("HasDefaultForeground(%q) = %v, want %v", tc.block, got, tc.want)
			}
		})
	}
}
