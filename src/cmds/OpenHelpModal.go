package cmds

import tea "charm.land/bubbletea/v2"

// OpenHelpModalMsg asks AppModel to open the help overlay. Going through a
// message (rather than AppModel opening it straight from the key) is the
// same path every other modal takes.
type OpenHelpModalMsg struct{}

// OpenHelpModal opens the help overlay.
func OpenHelpModal() tea.Cmd {
	return func() tea.Msg { return OpenHelpModalMsg{} }
}
