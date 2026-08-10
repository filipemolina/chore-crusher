package mainmenu

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/cmds"
)

// Model is the top menu bar. It is not focusable and handles no keys; it
// just renders the wordmark (and version, when it fits) on a tier-2 strip.
type Model struct {
	terminalWidth int
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// New builds the header component.
func New() tea.Model {
	return Model{}
}

// Update tracks the terminal width from the broadcast layout.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmds.SetBodyLayoutMsg:
		m.terminalWidth = msg.TerminalWidth
	}
	return m, nil
}
