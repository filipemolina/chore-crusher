package cmds

import tea "charm.land/bubbletea/v2"

// OpenThemePickerMsg asks AppModel to open the theme picker modal. Going
// through a message (rather than AppModel opening it straight from the key)
// is the same path every other modal takes.
type OpenThemePickerMsg struct{}

// OpenThemePicker opens the theme picker: the list of registered themes with
// live preview on cursor movement and persist-on-confirm.
func OpenThemePicker() tea.Cmd {
	return func() tea.Msg { return OpenThemePickerMsg{} }
}
