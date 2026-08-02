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

// BODY_GUTTER_WIDTH is the blank tier-2 column rendered between the lists
// panel and the main panel so they don't touch. It is subtracted from the
// row before the panels are sized, so the gutter never pushes the layout
// past the terminal width.
const BODY_GUTTER_WIDTH = 2

// HEADER_HEIGHT and FOOTER_HEIGHT reserve one row each for the main menu
// bar and the keybinding hint bar, matching stack-stitcher's shell.
const HEADER_HEIGHT = 1
const FOOTER_HEIGHT = 1
