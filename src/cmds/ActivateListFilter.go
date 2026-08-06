package cmds

import tea "charm.land/bubbletea/v2"

// ActivateListFilterMsg tells the lists panel to enter its
// built-in filter mode: the filter bar appears and the list
// narrows to matching items.
type ActivateListFilterMsg struct{}

// ActivateListFilter asks the lists panel to start its filter (/).
func ActivateListFilter() tea.Cmd {
	return func() tea.Msg { return ActivateListFilterMsg{} }
}
