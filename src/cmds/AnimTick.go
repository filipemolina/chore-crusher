package cmds

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// AnimInterval is the spinner frame rate — 125ms ≈ 8fps, the smallest
// cheap tick that still looks smooth for a braille spinner.
const AnimInterval = 125 * time.Millisecond

// AnimTickMsg advances the agent-presence spinner. It self-reschedules
// only while AppModel.animActive is true (at least one spinner visible),
// so it costs nothing when no agent is working.
type AnimTickMsg time.Time

// AnimTick fires once after interval. Re-issue it from inside the handler
// that receives AnimTickMsg to schedule the next cycle — the same
// self-rescheduling shape as PollTick (docs/plan/mcp-server-enhancement.md §3.5).
func AnimTick(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return AnimTickMsg(t)
	})
}

// AnimFrameMsg carries the current spinner frame (0..7) to the components.
type AnimFrameMsg struct{ Frame int }

// SetAnimFrame returns a cmd that produces an AnimFrameMsg with the given
// frame index. Components call this to broadcast the current frame after
// advancing it in Update.
func SetAnimFrame(frame int) tea.Cmd {
	return func() tea.Msg { return AnimFrameMsg{Frame: frame} }
}
