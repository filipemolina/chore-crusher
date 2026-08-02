package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/keybindingbar"
	"github.com/filipemolina/chore-crusher/src/components/listspanel"
	"github.com/filipemolina/chore-crusher/src/components/mainmenu"
	"github.com/filipemolina/chore-crusher/src/components/taskspanel"
	"github.com/filipemolina/chore-crusher/src/config"
	"github.com/filipemolina/chore-crusher/src/constants"
	"github.com/filipemolina/chore-crusher/src/keys"
	"github.com/filipemolina/chore-crusher/src/store"
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
		MainMenu      tea.Model
		KeybindingBar tea.Model
		ListsPanel    tea.Model
		TaskPanel     tea.Model
	}
}

// GetInitialModel builds the app model. The lists panel starts hidden
// (toggled by L), and the task tree starts focused — the app's premise is
// "spend your time in one list" (docs/DESIGN.md §5), so the tree is where
// the cursor lands.
func GetInitialModel(s *store.Store, cfg config.Config) tea.Model {
	m := AppModel{
		store:             s,
		cfg:               cfg,
		focusedZone:       constants.COMPONENT_TASK_TREE,
		listsPanelVisible: false,
	}
	m.components.MainMenu = mainmenu.New()
	m.components.KeybindingBar = keybindingbar.New()
	m.components.ListsPanel = listspanel.New()
	m.components.TaskPanel = taskspanel.New(s, "")
	return m
}

// helpContext snapshots what the help overlay and keybinding bar need to
// know about the screen. Keeping it in one place keeps the footer and the
// overlay in lockstep.
func (m AppModel) helpContext() keys.Context {
	return keys.Context{
		Focused:           m.focusedZone,
		ListsPanelVisible: m.listsPanelVisible,
		TaskTreeEmpty:     m.taskTreeEmpty(),
		HasActiveList:     m.activeListID != "",
	}
}

// taskTreeEmpty reports whether the task tree has no rows right now. It is
// conservative: before the first refresh there are no rows, so the footer
// advertises the add-input keys rather than navigation keys.
func (m AppModel) taskTreeEmpty() bool {
	tasks, ok := m.components.TaskPanel.(interface{ IsEmpty() bool })
	return !ok || tasks.IsEmpty()
}

// footerContextCmd returns the command that updates the footer with the
// current context.
func (m AppModel) footerContextCmd() tea.Cmd {
	ctx := m.helpContext()
	return cmds.SetFooterContext(ctx.Focused, ctx.ListsPanelVisible, ctx.TaskTreeEmpty, ctx.HasActiveList)
}
