package cmds

import tea "charm.land/bubbletea/v2"

// ActivateFilterMsg tells the task tree to enter its local `/` fuzzy-filter
// mode: an in-place text input appears and the visible rows narrow to each
// query's matches plus their ancestor chains. The tree reads this because /
// is a global key handled by AppModel, which cannot mutate a component's
// state directly.
type ActivateFilterMsg struct{}

// ActivateFilter asks the task tree to start a local filter (/).
func ActivateFilter() tea.Cmd {
	return func() tea.Msg { return ActivateFilterMsg{} }
}
