package cmds

import tea "charm.land/bubbletea/v2"

// SelectListInPanelMsg moves the lists panel's highlight to the given list id
// without broadcasting a SelectListMsg back to AppModel. It is the one-way
// startup alignment: AppModel reopens the last-active list from the Setting
// table (docs/DESIGN.md §7), a choice the panel cannot make for itself, so it
// commands the panel to match rather than letting the panel's first-refresh
// auto-select (which would pick index 0 and clobber the saved list).
type SelectListInPanelMsg struct {
	ListID string
}

// SelectListInPanel returns a command that aligns the lists panel highlight.
func SelectListInPanel(listID string) tea.Cmd {
	return func() tea.Msg {
		return SelectListInPanelMsg{ListID: listID}
	}
}
