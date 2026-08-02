// Package taskspanel composes the task tree and add input into the one Tasks
// surface. The children retain their keyboard behavior; this package owns only
// their shared frame and the input's bottom-pinned placement.
package taskspanel

import (
	"image/color"

	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/addinput"
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

type inputView interface {
	ViewInPanel(width, height int, bg color.Color) string
}

// Model owns the two keyboard controls inside the Tasks surface.
type Model struct {
	focused bool
	body    cmds.SetBodyLayoutMsg
	tree    tea.Model
	input   tea.Model
}

func New(st *store.Store, activeListID string) tea.Model {
	return Model{
		tree:  tasktree.New(),
		input: addinput.New(st, activeListID),
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if layout, ok := msg.(cmds.SetBodyLayoutMsg); ok {
		m.body = layout
	}
	if focus, ok := msg.(cmds.SetFocusMsg); ok {
		m.focused = int(focus) == constants.COMPONENT_TASK_TREE || int(focus) == constants.COMPONENT_ADD_INPUT
	}

	var treeCmd, inputCmd tea.Cmd
	m.tree, treeCmd = m.tree.Update(msg)
	m.input, inputCmd = m.input.Update(msg)
	return m, tea.Batch(treeCmd, inputCmd)
}

func (m Model) View() tea.View {
	width := chrome.PanelBodyWidth(m.body.MainWidth)
	height := chrome.PanelBodyHeight(m.body.Height)
	bg := chrome.PanelBg(m.focused)

	content := m.tree.(treeView).ViewInPanel(width, max(0, height-1), bg)
	footer := m.input.(inputView).ViewInPanel(width, 1, bg)
	body := chrome.PanelBodyWithFooter(width, height, bg, content, footer)

	return tea.NewView(chrome.PanelFrame("Tasks", m.focused, m.body.MainWidth, m.body.Height, body))
}

// IsEmpty reports whether the task tree has rows, for footer context.
func (m Model) IsEmpty() bool { return m.tree.(treeView).IsEmpty() }

// Rows returns task-tree rows for model-level behavior tests.
func (m Model) Rows() []apptypes.Row { return m.tree.(treeView).Rows() }

// SelectedID returns the selected task id for model-level behavior tests.
func (m Model) SelectedID() string { return m.tree.(treeView).SelectedID() }
