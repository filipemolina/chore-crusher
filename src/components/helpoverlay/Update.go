package helpoverlay

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/keys"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		m.offset = min(m.offset, m.maxOffset())

	case tea.KeyPressMsg:
		// Either of the two closes: ? is the toggle that opened it, esc is
		// the cancel every overlay answers. Only the overlay closes - the
		// keys never quit the app from here, because the overlay owns the
		// keyboard while it is open.
		switch {
		case key.Matches(msg, keys.Global.Help),
			key.Matches(msg, keys.Overlay.Cancel):
			return m, cmds.CloseModal(nil)

		// The catalog is longer than a terminal, so it scrolls. These are
		// Overlay.Navigation's own keystrokes — the binding every overlay
		// already advertises for "the arrows move within me".
		case key.Matches(msg, keys.Overlay.Navigation):
			switch msg.String() {
			case "up", "k":
				m.offset = max(0, m.offset-1)
			default:
				m.offset = min(m.maxOffset(), m.offset+1)
			}
		}
	}

	return m, nil
}
