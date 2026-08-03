package helpoverlay

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/keys"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width

	case tea.KeyPressMsg:
		// Either of the two closes: ? is the toggle that opened it, esc is
		// the cancel every overlay answers. Only the overlay closes - the
		// keys never quit the app from here, because the overlay owns the
		// keyboard while it is open.
		switch {
		case key.Matches(msg, keys.Global.Help),
			key.Matches(msg, keys.Overlay.Cancel):
			return m, cmds.CloseModal(nil)
		}
	}

	return m, nil
}
