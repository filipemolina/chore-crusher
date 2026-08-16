package cmds

import tea "charm.land/bubbletea/v2"

// OpenAboutModalMsg asks AppModel to open the about modal. Going through a
// message (rather than AppModel opening it straight from the key) is the
// same path every other modal takes.
type OpenAboutModalMsg struct{}

// OpenAboutModal opens the about modal.
func OpenAboutModal() tea.Cmd {
	return func() tea.Msg { return OpenAboutModalMsg{} }
}
