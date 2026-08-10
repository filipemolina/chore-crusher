package cmds

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/apptypes"
)

// SetSortModeMsg updates the current sort mode for the task tree.
type SetSortModeMsg struct {
	Mode apptypes.SortMode
}

// SetSortMode creates a command that sets the sort mode.
func SetSortMode(mode apptypes.SortMode) tea.Cmd {
	return func() tea.Msg {
		return SetSortModeMsg{Mode: mode}
	}
}

// CycleSortModeMsg cycles to the next sort mode.
type CycleSortModeMsg struct{}

// CycleSortMode creates a command that cycles to the next sort mode.
func CycleSortMode() tea.Cmd {
	return func() tea.Msg {
		return CycleSortModeMsg{}
	}
}
