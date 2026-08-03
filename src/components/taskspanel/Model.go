// Package taskspanel composes the task tree into the Tasks surface. Inline
// creation (the "new task" row) lives inside the tree itself, so this package
// owns only the tree and its shared panel frame — there is no bottom-pinned
// add input. See docs/plan/task-row-redesign-and-inline-creation.md.
package taskspanel

import (
	"image/color"

	tea "charm.land/bubbletea/v2"
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
type Model struct {
	focused bool
	body    cmds.SetBodyLayoutMsg
	tree    tea.Model
}

func New(st *store.Store, activeListID string) tea.Model {
	return Model{
		tree: tasktree.New(),
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if layout, ok := msg.(cmds.SetBodyLayoutMsg); ok {
		m.body = layout
	}
	if focus, ok := msg.(cmds.SetFocusMsg); ok {
		m.focused = int(focus) == constants.COMPONENT_TASK_TREE
	}

	var treeCmd tea.Cmd
	m.tree, treeCmd = m.tree.Update(msg)
	return m, treeCmd
}

func (m Model) View() tea.View {
	width := chrome.PanelBodyWidth(m.body.MainWidth)
	height := chrome.PanelBodyHeight(m.body.Height)
	bg := chrome.PanelBg(m.focused)

	// The tree owns its full body height: with the add input removed there is
	// no footer to reserve a row for. PanelBodyWithFooter treats an empty
	// footer as zero-height, so this passes the tree's render through at full
	// height (see chrome.PanelBodyWithFooter).
	content := m.tree.(treeView).ViewInPanel(width, max(0, height), bg)
	body := chrome.PanelBodyWithFooter(width, height, bg, content, "")

	return tea.NewView(chrome.PanelFrame("Tasks", m.focused, m.body.MainWidth, m.body.Height, body))
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
