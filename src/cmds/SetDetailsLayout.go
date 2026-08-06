package cmds

import tea "charm.land/bubbletea/v2"

// SetDetailsLayoutMsg is the outer box the Details modal renders into. Unlike
// the Lists/Tasks surfaces (SetBodyLayoutMsg), Details is no longer a body
// surface competing for the row — it is a centered modal layered over the
// page, so it is sized from the terminal directly (about 90% of each axis)
// rather than from the body split. AppModel is still the single source of
// truth for the numbers: it computes them on open and on every resize, and the
// component renders at exactly this size.
type SetDetailsLayoutMsg struct {
	Width  int
	Height int
}

// SetDetailsLayout broadcasts the Details modal's outer box.
func SetDetailsLayout(width, height int) tea.Cmd {
	return func() tea.Msg {
		return SetDetailsLayoutMsg{Width: width, Height: height}
	}
}
