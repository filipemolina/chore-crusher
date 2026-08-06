package cmds

import tea "charm.land/bubbletea/v2"

// CloseDetailsSideMsg asks AppModel to hide the Details side panel and return
// focus to the task tree. The Details component emits this — only AppModel
// changes visibility and focus, so the component never mutates the layout
// itself. Follow, if set, is appended to the batch of commands run once the
// panel is gone: a save hands off its RefreshTasks that way, while a clean or
// discarded close passes nil.
type CloseDetailsSideMsg struct {
	Follow tea.Cmd
}

// CloseDetailsSide closes the Details side panel, running follow once it is
// gone (nil for a plain dismiss).
func CloseDetailsSide(follow tea.Cmd) tea.Cmd {
	return func() tea.Msg { return CloseDetailsSideMsg{Follow: follow} }
}
