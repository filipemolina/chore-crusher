package keybindingbar

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/keys"
)

// Model is the one-line footer that advertises the keys live in the current
// context. It keeps its own copy of keys.Context so it can render from the
// same source of truth as the help overlay.
type Model struct {
	ctx           keys.Context
	terminalWidth int
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// New builds the footer component.
func New() tea.Model {
	return Model{}
}

// WithContext returns a copy of m with ctx set for the frame about to be
// rendered, bypassing the deferred SetFooterContextMsg round trip — the
// caller renders the result immediately rather than waiting for Update to
// receive the message on a later cycle (docs/DESIGN.md's footer contract:
// hints describe the mode the app is in right now).
func (m Model) WithContext(ctx keys.Context) Model {
	m.ctx = ctx
	return m
}

// Update tracks the layout width and the context changes from AppModel.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmds.SetBodyLayoutMsg:
		m.terminalWidth = msg.TerminalWidth
	case cmds.SetFooterContextMsg:
		m.ctx = keys.Context{
			Focused:             msg.Focused,
			ListsPanelVisible:   msg.ListsPanelVisible,
			DetailsPanelVisible: msg.DetailsPanelVisible,
			TaskTreeEmpty:       msg.TaskTreeEmpty,
			HasActiveList:       msg.HasActiveList,
			Creating:            msg.Creating,
			Filtering:           msg.Filtering,
			HasModal:            msg.HasModal,
		}
	case cmds.SetFocusMsg:
		m.ctx.Focused = int(msg)
	}
	return m, nil
}
