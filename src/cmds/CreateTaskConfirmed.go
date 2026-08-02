package cmds

import tea "charm.land/bubbletea/v2"

// CreateTaskConfirmedMsg is sent by AppModel after the store has created a
// task from the task tree's inline creation input. The tree consumes it to
// keep inline creation open at the next insertion point (the just-created task)
// for rapid entry, and to move the cursor onto the new task.
type CreateTaskConfirmedMsg struct {
	NewID string
	Depth int
}

// CreateTaskConfirmed returns a command that delivers the confirmation to the
// task tree.
func CreateTaskConfirmed(newID string, depth int) tea.Cmd {
	return func() tea.Msg {
		return CreateTaskConfirmedMsg{NewID: newID, Depth: depth}
	}
}
