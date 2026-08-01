package cmds

import tea "charm.land/bubbletea/v2"

// SetFocusMsg asks AppModel to move focus to the given zone
// (constants.COMPONENT_*). A component requests focus by zone id rather than
// by index because the set of focusable zones is computed at runtime — the
// lists panel drops out of the cycle while hidden — and only AppModel knows
// the current layout (docs/DESIGN.md §5). AppModel ignores a request for a
// zone that is not currently focusable.
type SetFocusMsg int

// SetFocus asks AppModel to focus the zone with the given component id.
func SetFocus(zone int) tea.Cmd {
	return func() tea.Msg {
		return SetFocusMsg(zone)
	}
}
