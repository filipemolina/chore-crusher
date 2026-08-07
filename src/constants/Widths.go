package constants

// Widths and heights for the two-surface layout (docs/DESIGN.md §5). Where
// stack-stitcher already solved the same layout problem, its numbers are
// ported outright (docs/DESIGN.md §12): the Lists surface is a sidebar like
// its left panel and Tasks takes the remaining body width.

// LEFT_PANEL_WIDTH is the lists panel's share of the row when it is visible,
// ported from stack-stitcher's sidebar (one-third, lazydocker's default).
const LEFT_PANEL_WIDTH float32 = 0.33

// MIN_PANEL_WIDTH is the floor both the lists panel and the main panel are
// held at where the terminal allows it; below that the row is split evenly
// and the panels clip their own content.
const MIN_PANEL_WIDTH = 30

// AUTO_SHOW_LISTS_MIN_WIDTH is a startup policy, not a minimum rendering
// width: on the first window-size message the Lists panel opens automatically
// at this terminal width or wider, and starts hidden below it (docs/DESIGN.md
// §5). After that first frame, L is the sole authority — a resize never
// reverses a user's toggle. It is unrelated to MIN_PANEL_WIDTH, which governs
// whether a sidebar can physically render at all.
const AUTO_SHOW_LISTS_MIN_WIDTH = 120

// BODY_GUTTER_WIDTH is the blank tier-2 column rendered between the lists
// panel and the main panel so they don't touch. It is subtracted from the
// row before the panels are sized, so the gutter never pushes the layout
// past the terminal width.
const BODY_GUTTER_WIDTH = 2

// HEADER_HEIGHT and FOOTER_HEIGHT reserve one row each for the main menu
// bar and the keybinding hint bar, matching stack-stitcher's shell.
const HEADER_HEIGHT = 1
const FOOTER_HEIGHT = 1

// MIN_TERMINAL_WIDTH and MIN_TERMINAL_HEIGHT are the smallest terminal the app
// claims to support. Below either one it stops attempting the layout and
// renders a single centred "Terminal too small" line instead (docs/DESIGN.md
// §12) — the alternative is a frame that looks broken without ever saying so.
//
// 40 columns is where a task row still seats a checkbox, a title at its
// titleFloor, and a status label; 10 rows is a header, a footer, a section
// header and enough body left to be worth drawing.
const MIN_TERMINAL_WIDTH = 40
const MIN_TERMINAL_HEIGHT = 10
