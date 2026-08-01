package constants

// Widths and heights for the three-zone layout (docs/DESIGN.md §5). Where
// stack-stitcher already solved the same layout problem, its numbers are
// ported outright (docs/DESIGN.md §12): the lists panel is a sidebar like
// its left panel, and the task-tree/main split behaves like its two body
// panels.

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

// ADD_INPUT_HEIGHT is the task-tree zone's fixed vertical partner: the add
// input is pinned to the bottom of the main panel, always visible, always
// reachable (docs/DESIGN.md §5). Three rows is one content row inside
// chrome.PanelFrame's 1-row vertical padding — the frame's top and bottom
// edges plus the input line itself.
const ADD_INPUT_HEIGHT = 3
