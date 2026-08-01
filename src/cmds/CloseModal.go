package cmds

import tea "charm.land/bubbletea/v2"

// CloseModalMsg tells AppModel to clear the active modal. Follow, if set,
// is appended to the batch of commands run once the modal is gone — this is
// how a modal hands off the action it collected input for (e.g. actually
// applying a chosen theme) without needing to know about AppModel itself.
type CloseModalMsg struct {
	Follow tea.Cmd
}

// CloseModal closes the currently-open modal, running follow once it is
// gone (nil for a plain dismiss).
func CloseModal(follow tea.Cmd) tea.Cmd {
	return func() tea.Msg { return CloseModalMsg{Follow: follow} }
}
