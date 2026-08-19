package model

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/components/archivepage"
	"github.com/filipemolina/farol/src/components/detailspanel"
	"github.com/filipemolina/farol/src/components/keybindingbar"
	"github.com/filipemolina/farol/src/components/listspanel"
	"github.com/filipemolina/farol/src/components/mainmenu"
	"github.com/filipemolina/farol/src/components/taskspanel"
	"github.com/filipemolina/farol/src/config"
	"github.com/filipemolina/farol/src/constants"
	"github.com/filipemolina/farol/src/keys"
	"github.com/filipemolina/farol/src/store"
)

// AppModel is the top-level Bubble Tea model: it owns the store handle, the
// config, the terminal dimensions, and the three zones of the layout
// (docs/DESIGN.md §5). Components never read tea.WindowSizeMsg — this is
// the only place that does, and it broadcasts the derived layout (the "no
// page is active at startup" trap, docs/DESIGN.md §5).
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
	// archivePageVisible tracks the Archived Lists page, the exclusive
	// full-body surface that replaces Tasks/Lists (docs/DESIGN.md §5).
	// Unlike Details it is not a centered modal: while it is true, renderBody
	// returns the archive page's own View instead of the Tasks/Lists split,
	// and it owns the keyboard the same way Details does. It carries no task
	// id — it has nothing analogous to detailsTaskID to track.
	archivePageVisible bool
	activeListID       string
	lists              []apptypes.ListSummary
	activeModal        tea.Model
	lastError          string
	sortMode           apptypes.SortMode

	// animFrame is the current spinner frame (0..7), advanced by AnimTickMsg.
	// animActive tracks whether any agent claim is live — the spinner only
	// ticks when this is true.
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
		ArchivePage   tea.Model
	}
}

// GetInitialModel builds the app model. It does no database work: the
// constructor returns immediately with no active list so Bubble Tea can render
// the first frame (the Tasks panel's initial-load animation) before the
// opening Lists query completes. The first RefreshListsMsg — issued from Init —
// adopts the first list or creates the default list (constants.DEFAULT_LIST_NAME)
// when the store is empty (see AppModel.Update), preserving the invariant that
// a successful first load ends with an active list. The task tree is the startup focus zone — the
// app's premise is "spend your time in one list" (docs/DESIGN.md §5) — so the
// tree's keys live from the first frame and inline creation can begin before
// any focus change.
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
	m.components.DetailsPanel = detailspanel.New(s)
	m.components.ArchivePage = archivepage.New(s)
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
	filtering := false
	if tasks, ok := m.components.TaskPanel.(interface{ FilterActive() bool }); ok {
		filtering = tasks.FilterActive()
	}

	return keys.Context{
		Focused:             m.focusedZone,
		ListsPanelVisible:   m.listsPanelRendered(),
		DetailsPanelVisible: m.detailsPanelVisible,
		ArchivePageVisible:  m.archivePageVisible,
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

// createInputLive reports whether the task tree's inline create input is not
// merely on screen but actively taking keystrokes (IsCreating delegates to the
// tree's createLive). While it is true, creating a task focuses only the text
// input: focus keys (tab/shift+tab) must not carry the cursor, and the
// half-typed title, off to another panel mid-entry. A parked input (esc on an
// empty list) is not live, so focus may move there.
func (m AppModel) createInputLive() bool {
	tasks, ok := m.components.TaskPanel.(interface{ IsCreating() bool })
	return ok && tasks.IsCreating()
}

// footerContextCmd returns the command that updates the footer with the
// current context.
func (m AppModel) footerContextCmd() tea.Cmd {
	ctx := m.helpContext()
	return cmds.SetFooterContext(ctx.Focused, ctx.ListsPanelVisible, ctx.DetailsPanelVisible, ctx.ArchivePageVisible, ctx.TaskTreeEmpty, ctx.HasActiveList, ctx.Creating, ctx.Filtering, ctx.HasModal, m.sortMode)
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
