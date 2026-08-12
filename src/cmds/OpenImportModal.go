package cmds

import tea "charm.land/bubbletea/v2"

// OpenImportModalMsg asks AppModel to open the import modal. Going through a
// message (rather than opening it straight from the key) is the same path
// every other modal takes (OpenHelpModalMsg, OpenThemePickerMsg, ...).
type OpenImportModalMsg struct{}

// OpenImportModal opens the import modal, exactly like OpenThemePicker for
// its modal: AppModel — the only place that knows the store — builds the
// modal with what it needs.
func OpenImportModal() tea.Cmd {
	return func() tea.Msg { return OpenImportModalMsg{} }
}
