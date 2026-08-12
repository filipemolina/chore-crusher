package cmds

import tea "charm.land/bubbletea/v2"

// OpenExportModalMsg asks AppModel to open the export modal. Going through a
// message (rather than opening it straight from the key) is the same path
// every other modal takes (OpenHelpModalMsg, OpenThemePickerMsg, ...).
type OpenExportModalMsg struct{}

// OpenExportModal opens the export modal, exactly like OpenThemePicker for
// its modal: AppModel — the only place that knows the store and the
// highlighted list — builds the modal with what it needs.
func OpenExportModal() tea.Cmd {
	return func() tea.Msg { return OpenExportModalMsg{} }
}
