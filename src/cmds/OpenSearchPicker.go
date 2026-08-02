package cmds

import tea "charm.land/bubbletea/v2"

// OpenSearchPickerMsg asks AppModel to open the cross-list search picker
// (the F modal). It goes through a message the same way every other modal is
// opened, so AppModel — the only place that knows the terminal height and
// the store — builds the picker with what it needs.
type OpenSearchPickerMsg struct{}

// OpenSearchPicker opens the cross-list search picker modal.
func OpenSearchPicker() tea.Cmd {
	return func() tea.Msg { return OpenSearchPickerMsg{} }
}