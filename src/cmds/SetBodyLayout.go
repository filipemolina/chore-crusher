package cmds

import tea "charm.land/bubbletea/v2"

// SetBodyLayoutMsg is the exact outer box each body surface renders into.
// AppModel is the single source of truth for these numbers: it guarantees
// ListsWidth + BODY_GUTTER_WIDTH + MainWidth == the terminal width.
//
// Components must render at exactly this size rather than deriving their own
// from tea.WindowSizeMsg. WindowSizeMsg only reaches the components of the
// page that is active when it arrives, so any page that wasn't active at
// resize time would otherwise render at width 0 — the "no page is active at
// startup" trap (docs/DESIGN.md §5).
// All the numbers travel in one message so they can never be out of sync
// mid-frame.
//
// ListsWidth is the lists panel's width, or 0 while the panel is hidden (the
// panel is out of the layout and out of the focus cycle until L is pressed).
// MainWidth is the Tasks surface's width. Taskspanel owns its internal tree and
// one-row input-footer split. ListsWidth and MainWidth are both total
// outer-surface widths. TerminalWidth is the full terminal width, needed by
// chrome that spans the frame. Details is not here — it is a modal layered over
// the body, sized from the terminal via SetDetailsLayoutMsg, not a body column.
type SetBodyLayoutMsg struct {
	Height        int
	ListsWidth    int
	MainWidth     int
	TerminalWidth int
}

// SetBodyLayout broadcasts the layout to every component.
func SetBodyLayout(height, listsWidth, mainWidth, terminalWidth int) tea.Cmd {
	return func() tea.Msg {
		return SetBodyLayoutMsg{
			Height:        height,
			ListsWidth:    listsWidth,
			MainWidth:     mainWidth,
			TerminalWidth: terminalWidth,
		}
	}
}
