package cmds

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// PollTickMsg is the self-rescheduling poll tick (docs/DESIGN.md §7):
// AppModel re-issues PollTick on every PollTickMsg, which is what makes the
// poll recurring for the life of the app.
type PollTickMsg time.Time

// PollTick fires once after interval. Re-issue it from inside the handler
// that receives PollTickMsg to schedule the next cycle — the same
// self-rescheduling shape as stack-stitcher's RefreshContainersTick.
func PollTick(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return PollTickMsg(t)
	})
}
