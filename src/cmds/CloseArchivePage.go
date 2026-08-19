package cmds

import tea "charm.land/bubbletea/v2"

// CloseArchivePageMsg asks AppModel to hide the Archived Lists page and
// return focus to the task tree. The Archive page component emits this —
// only AppModel changes visibility and focus, so the component never
// mutates the layout itself. Follow, if set, is appended to the batch of
// commands run once the page is gone (mirroring CloseDetailsSideMsg).
type CloseArchivePageMsg struct {
	Follow tea.Cmd
}

// CloseArchivePage closes the Archived Lists page, running follow once it is
// gone (nil for a plain dismiss).
func CloseArchivePage(follow tea.Cmd) tea.Cmd {
	return func() tea.Msg { return CloseArchivePageMsg{Follow: follow} }
}
