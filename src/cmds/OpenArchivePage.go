package cmds

import tea "charm.land/bubbletea/v2"

// OpenArchivePageMsg signals that the Archived Lists page should open.
type OpenArchivePageMsg struct{}

// OpenArchivePage creates a command that opens the Archived Lists page.
func OpenArchivePage() tea.Cmd {
	return func() tea.Msg { return OpenArchivePageMsg{} }
}
