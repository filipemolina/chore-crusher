package cmds

import tea "charm.land/bubbletea/v2"

// SetBodyLayoutMsg is the exact box each body zone must render into.
// AppModel is the single source of truth for these numbers: it guarantees
// ListsWidth + BODY_GUTTER_WIDTH + MainWidth == the terminal width, and
// TreeHeight + InputHeight == Height (docs/DESIGN.md §5).
//
// Components must render at exactly this size rather than deriving their own
// from tea.WindowSizeMsg. WindowSizeMsg only reaches the components of the
// page that is active when it arrives, so any page that wasn't active at
// resize time would otherwise render at width 0 — the trap stack-stitcher's
// docs/DESIGN.md §5 "Body" paragraph names ("no page is active at startup").
// All the numbers travel in one message so they can never be out of sync
// mid-frame.
//
// ListsWidth is the lists panel's width, or 0 while the panel is hidden (the
// panel is out of the layout and out of the focus cycle until L is pressed).
// MainWidth is the main panel's width; TreeHeight and InputHeight describe
// the main panel's vertical split, with the add input pinned to the bottom
// (fixed ADD_INPUT_HEIGHT rows, always visible). TerminalWidth is the full
// terminal width, needed by chrome (header/footer) that spans the frame.
type SetBodyLayoutMsg struct {
	Height        int
	ListsWidth    int
	MainWidth     int
	TreeHeight    int
	InputHeight   int
	TerminalWidth int
}

// SetBodyLayout broadcasts the layout to every component.
func SetBodyLayout(height, listsWidth, mainWidth, treeHeight, inputHeight, terminalWidth int) tea.Cmd {
	return func() tea.Msg {
		return SetBodyLayoutMsg{
			Height:        height,
			ListsWidth:    listsWidth,
			MainWidth:     mainWidth,
			TreeHeight:    treeHeight,
			InputHeight:   inputHeight,
			TerminalWidth: terminalWidth,
		}
	}
}
