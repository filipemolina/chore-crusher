package cmds

import tea "charm.land/bubbletea/v2"

// SelectListMsg tells AppModel which list to switch to.
type SelectListMsg struct {
	ListID string
}

// SelectList returns a command that broadcasts the selected list.
func SelectList(listID string) tea.Cmd {
	return func() tea.Msg {
		return SelectListMsg{ListID: listID}
	}
}
