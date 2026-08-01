package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-completer/src/cmds"
	"github.com/filipemolina/chore-completer/src/config"
)

// Init starts the app: the poll tick (which re-issues itself for the life
// of the app, docs/DESIGN.md §7) and the first lists refresh. The first
// RefreshListsMsg selects the first list and kicks off its tasks refresh, so
// no ordering between this command batch and the first WindowSizeMsg is
// assumed — the layout broadcast waits for the real terminal size.
func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		cmds.PollTick(config.PollInterval(m.cfg)),
		cmds.RefreshLists(m.store),
	)
}
