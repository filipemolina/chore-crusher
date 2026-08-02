package cmds

import tea "charm.land/bubbletea/v2"

// StartCreatingMsg tells the task tree to enter inline creation mode.
type StartCreatingMsg struct{}

// StartCreating returns a command that asks the task tree to enter inline
// creation mode.
func StartCreating() tea.Cmd {
	return func() tea.Msg {
		return StartCreatingMsg{}
	}
}
