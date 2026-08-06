package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/detailspanel"
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
	store          *store.Store
	cfg            config.Config
	terminalWidth  int
	terminalHeight int
	bodyLayout     cmds.SetBodyLayoutMsg
	focusedZone    int
	// listsPanelVisible is the user's Lists preference, not whether it has
	// width this frame — listsPanelRendered() is that derived predicate. On the
	// first window-size message the preference is seeded from terminal width
	// (docs/DESIGN.md §5); after that L is the only thing that flips it.
	listsPanelVisible bool
	// layoutInitialized guards the one-time startup width policy so a later
	// resize never re-applies it over a user's L toggle.
	layoutInitialized bool
	// detailsPanelVisible and detailsTaskID track the exclusive Details side
	// surface (docs/DESIGN.md §5). Details replaces Lists on the right and is
	// never in the tab cycle: it is entered and left by explicit open/close
	// transitions, and starts hidden with an empty task id.
	detailsPanelVisible bool
	detailsTaskID       string
	activeListID        string
	lists               []apptypes.ListSummary
	activeModal         tea.Model
	lastError           string

	// animFrame is the current spinner frame (0..7), advanced by AnimTickMsg.
	// animActive tracks whether any agent claim is live — the spinner only
	// ticks when this is true (docs/plan/mcp-server-enhancement.md §3.6).
	animFrame  int
	animActive bool

	// createDraft is an inline creation the tree has submitted but AppModel has
	// not yet written. It is resolved against the next RefreshTasksMsg's rows
	// (fresh from the store) so an insert or delete during typing can't anchor
	// the new task to a stale selection.
	createDraft *cmds.CreateTaskFromInputMsg

	components struct {
		MainMenu      tea.Model
		KeybindingBar tea.Model
		ListsPanel    tea.Model
		TaskPanel     tea.Model
		DetailsPanel  tea.Model
	}
}

// GetInitialModel builds the app model. The lists panel starts hidden,
// and if the store has no lists yet a default "New List" is created so the
// add input always has somewhere to create its first task. The task tree is
// the startup focus zone — the app's premise is "spend your time in one
// list" (docs/DESIGN.md §5) — so the tree's keys live from the first frame
// and inline creation can begin before any focus change.
func GetInitialModel(s *store.Store, cfg config.Config) tea.Model {
	activeListID := ""
	if s != nil {
		if lists, err := s.ListLists(); err == nil {
			if len(lists) > 0 {
				activeListID = lists[0].List.ID
			} else {
				if id, err := s.CreateList("New List", ""); err == nil {
					activeListID = id
				}
			}
		}
	}

	m := AppModel{
		store:             s,
		cfg:               cfg,
		focusedZone:       constants.COMPONENT_TASK_TREE,
		listsPanelVisible: false,
		activeListID:      activeListID,
	}
	m.components.MainMenu = mainmenu.New()
	m.components.KeybindingBar = keybindingbar.New()
	m.components.ListsPanel = listspanel.New()
	m.components.TaskPanel = taskspanel.New(s, activeListID)
	m.components.DetailsPanel = detailspanel.New(s)
	return m
}

// helpContext snapshots what the help overlay and keybinding bar need to
// know about the screen. Keeping it in one place keeps the footer and the
// overlay in lockstep.
func (m AppModel) helpContext() keys.Context {
	creating := false
	if tasks, ok := m.components.TaskPanel.(interface{ IsCreating() bool }); ok {
		creating = tasks.IsCreating()
	}
	// Filtering is not yet exported by the tree; leave false until the
	// tree wires the /-filter state into AppModel (phase A step 2).
	filtering := false

	return keys.Context{
		Focused:             m.focusedZone,
		ListsPanelVisible:   m.listsPanelRendered(),
		DetailsPanelVisible: m.detailsPanelVisible,
		TaskTreeEmpty:       m.taskTreeEmpty(),
		HasActiveList:       m.activeListID != "",
		Creating:            creating,
		Filtering:           filtering,
		HasModal:            m.activeModal != nil,
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
	return cmds.SetFooterContext(ctx.Focused, ctx.ListsPanelVisible, ctx.DetailsPanelVisible, ctx.TaskTreeEmpty, ctx.HasActiveList, ctx.Creating, ctx.Filtering, ctx.HasModal)
}

// listsPanelRendered reports whether the Lists panel actually occupies width
// on this frame: the user preference is on AND the current layout gave it
// columns. A too-narrow terminal drives ListsWidth to 0 without touching the
// preference, so the panel yields cleanly and returns on a later resize. This
// is the predicate for focus, footer, and render decisions — listsPanelVisible
// alone is only the stored intent (docs/DESIGN.md §5).
func (m AppModel) listsPanelRendered() bool {
	return m.listsPanelVisible && m.bodyLayout.ListsWidth > 0
}
