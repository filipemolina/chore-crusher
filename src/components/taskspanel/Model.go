// Package taskspanel composes the task tree into the Tasks surface. Inline
// creation (the "new task" row) lives inside the tree itself, so this package
// owns only the tree and its shared panel frame — there is no bottom-pinned
// add input.
package taskspanel

import (
	"image/color"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/components/tasktree"
	"github.com/filipemolina/chore-crusher/src/constants"
	"github.com/filipemolina/chore-crusher/src/store"
)

type treeView interface {
	ViewInPanel(width, height int, bg color.Color) string
	Rows() []apptypes.Row
	SelectedID() string
	IsEmpty() bool
}

// Model owns the task tree inside the Tasks surface. Inline creation is
// handled by the tree, so there is no separate add-input component here.
//
// loading is the one-shot initial-database-load state: it starts true and the
// panel shows an animated "Loading" label until the very first RefreshListsMsg
// (success or error) arrives, after which it is permanently false — later poll
// refreshes never restore it (docs/DESIGN.md §7). The spinner drives only that
// animation; its theme color is applied at render time, never cached in the
// spinner's Style (a live theme switch would leave a cached color stale).
type Model struct {
	focused  bool
	body     cmds.SetBodyLayoutMsg
	tree     tea.Model
	listName string
	loading  bool
	spinner  spinner.Model
}

func New(st *store.Store, activeListID string) tea.Model {
	return Model{
		tree:    tasktree.New(),
		loading: true,
		spinner: spinner.New(spinner.WithSpinner(spinner.Ellipsis)),
	}
}

// Init starts the initial-load spinner. AppModel batches this so the first
// frame animates while the opening Lists query is still outstanding.
func (m Model) Init() tea.Cmd { return m.spinner.Tick }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if layout, ok := msg.(cmds.SetBodyLayoutMsg); ok {
		m.body = layout
	}
	if focus, ok := msg.(cmds.SetFocusMsg); ok {
		m.focused = int(focus) == constants.COMPONENT_TASK_TREE
	}
	// The active list's name travels with its rows; keep it for the header.
	if refresh, ok := msg.(cmds.RefreshTasksMsg); ok && refresh.Err == nil {
		m.listName = refresh.ListName
	}
	// The first Lists refresh — success or error — ends the initial-load
	// animation for good. Guarding on m.loading keeps a later poll refresh
	// from ever forwarding another spinner tick.
	if _, ok := msg.(cmds.RefreshListsMsg); ok {
		m.loading = false
	}

	var cmdList []tea.Cmd
	// Advance the spinner only while loading, and only for its own tick, so it
	// never claims a share of any other message and stops cleanly once loaded.
	if m.loading {
		if _, ok := msg.(spinner.TickMsg); ok {
			var spCmd tea.Cmd
			m.spinner, spCmd = m.spinner.Update(msg)
			cmdList = append(cmdList, spCmd)
		}
	}

	var treeCmd tea.Cmd
	m.tree, treeCmd = m.tree.Update(msg)
	cmdList = append(cmdList, treeCmd)
	return m, tea.Batch(cmdList...)
}

func (m Model) View() tea.View {
	width := chrome.PanelBodyWidth(m.body.MainWidth)
	height := chrome.PanelBodyHeight(m.body.Height)
	bg := chrome.PanelBg(m.focused)

	// The tree owns its full body height: with the add input removed there is
	// no footer to reserve a row for. PanelBodyWithFooter treats an empty
	// footer as zero-height, so this passes the tree's render through at full
	// height (see chrome.PanelBodyWithFooter). While the initial Lists query is
	// still outstanding the body is the sealed loading label instead.
	var content string
	if m.loading {
		content = m.loadingBody(width, max(0, height), bg)
	} else {
		content = m.tree.(treeView).ViewInPanel(width, max(0, height), bg)
	}
	body := chrome.PanelBodyWithFooter(width, height, bg, content, "")

	return tea.NewView(chrome.PanelFrameWithRightTitle("Tasks", m.listName, m.focused, m.body.MainWidth, m.body.Height, body))
}

// loadingBody renders the centered "Loading" label and its ellipsis animation,
// sealed to the panel body box. The label uses the panel tier's primary text
// and the ellipsis the accent, both read fresh from appstyles.Active at draw
// time so a live theme switch repaints them (the plan's render-time-color
// rule; no themed style is cached on the spinner).
func (m Model) loadingBody(width, height int, bg color.Color) string {
	label := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Background(bg).Render("Loading")
	ellipsis := lipgloss.NewStyle().Foreground(appstyles.Active.Accent).Background(bg).Render(m.spinner.View())
	line := lipgloss.JoinHorizontal(lipgloss.Top, label, ellipsis)

	box := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(bg).
		Align(lipgloss.Center, lipgloss.Center).
		Render(line)
	return appstyles.FillBackground(bg, box)
}

// OwnsKeyboard reports whether the task tree has claimed the keyboard for
// itself (inline creating, or typing a /-filter). AppModel uses this to
// suppress global keys while the user is actively typing in the tree.
func (m Model) OwnsKeyboard() bool {
	if tree, ok := m.tree.(interface{ OwnsKeyboard() bool }); ok && tree.OwnsKeyboard() {
		return true
	}
	return false
}

// KeepsEsc reports whether the task tree needs esc for itself (inline create
// cancel, filter clear). AppModel's "back" checks this before it takes
// focus away.
func (m Model) KeepsEsc() bool {
	if tree, ok := m.tree.(interface{ KeepsEsc() bool }); ok && tree.KeepsEsc() {
		return true
	}
	return false
}

// IsCreating reports whether the task tree is in inline creation mode, for
// footer context and model-level tests.
func (m Model) IsCreating() bool {
	if tree, ok := m.tree.(interface{ IsCreating() bool }); ok && tree.IsCreating() {
		return true
	}
	return false
}

// IsEmpty reports whether the task tree has rows, for footer context.
func (m Model) IsEmpty() bool { return m.tree.(treeView).IsEmpty() }

// Rows returns task-tree rows for model-level behavior tests.
func (m Model) Rows() []apptypes.Row { return m.tree.(treeView).Rows() }

// SelectedID returns the selected task id for model-level behavior tests.
func (m Model) SelectedID() string { return m.tree.(treeView).SelectedID() }

// FilterActive reports whether the tree's /-filter is open or applied, for
// AppModel-level tests.
func (m Model) FilterActive() bool {
	if tree, ok := m.tree.(interface{ FilterActive() bool }); ok && tree.FilterActive() {
		return true
	}
	return false
}
