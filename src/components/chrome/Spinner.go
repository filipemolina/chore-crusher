package chrome

// spinner is the 8 braille frames advanced by AnimTickMsg. Brailes are 3
// bytes in UTF-8, so this must be an array (not a byte slice): a byte
// slice would split a rune. (docs/DESIGN.md section 12: one shared symbol.)
var spinner = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧"}

// Spinner returns the braille spinner glyph for the given frame index.
// Frame is taken modulo 8 so callers can just increment without bounds
// checking.
func Spinner(frame int) string {
	return spinner[frame%len(spinner)]
}
