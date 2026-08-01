package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-completer/src/apptypes"
	"github.com/filipemolina/chore-completer/src/cmds"
	"github.com/filipemolina/chore-completer/src/components/addinput"
	"github.com/filipemolina/chore-completer/src/components/listspanel"
	"github.com/filipemolina/chore-completer/src/components/tasktree"
	"github.com/filipemolina/chore-completer/src/config"
	"github.com/filipemolina/chore-completer/src/constants"
	"github.com/filipemolina/chore-completer/src/keys"
	"github.com/filipemolina/chore-completer/src/store"
)

// AppModel is the top-level Bubble Tea model: it owns the store handle, the
// config, the terminal dimensions, and the three zones of the layout
// (docs/DESIGN.md §5). Components never read tea.WindowSizeMsg — this is
// the only place that does, and it broadcasts the derived layout (step 5 of
// docs/plans/phase-3-tui-shell.md, and stack-stitcher's docs/DESIGN.md §5
// "Body" for the "no page is active at startup" trap).
type AppModel struct {
	store             *store.Store
	cfg               config.Config
	terminalWidth     int
	terminalHeight    int
	bodyLayout        cmds.SetBodyLayoutMsg
	focusedZone       int
	listsPanelVisible bool
	activeListID      string
	lists             []apptypes.ListSummary
	activeModal       tea.Model
	lastError         string

	components struct {
		ListsPanel tea.Model
		TaskTree   tea.Model
		AddInput   tea.Model
	}
}

// GetInitialModel builds the app model. The lists panel starts visible and
// the task tree starts focused — the app's premise is "spend your time in
// one list" (docs/DESIGN.md §5), so the tree is where the cursor lands.
func GetInitialModel(s *store.Store, cfg config.Config) tea.Model {
	m := AppModel{
		store:             s,
		cfg:               cfg,
		focusedZone:       constants.COMPONENT_TASK_TREE,
		listsPanelVisible: true,
	}
	m.components.ListsPanel = listspanel.New()
	m.components.TaskTree = tasktree.New()
	m.components.AddInput = addinput.New(s, "")
	return m
}

// helpContext snapshots what the help overlay needs to dim the keys that do
// nothing right now. Phase 3 has no per-zone key groups yet, so the overlay
// is driven by the focus and visibility facts alone; phases 4-6 fill the
// rest in (src/keys.Keys.go Context).
func (m AppModel) helpContext() keys.Context {
	return keys.Context{
		Focused:           m.focusedZone,
		ListsPanelVisible: m.listsPanelVisible,
		HasActiveList:     m.activeListID != "",
	}
}
