package searchpicker

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-completer/src/cmds"
	"github.com/filipemolina/chore-completer/src/keys"
)

// navBinding is the up/down arrow pair the picker uses to move the result
// cursor. It deliberately excludes j/k — those are printable characters the
// text input needs, so only the arrows navigate (docs/plans/phase-8-search.md).
var navBinding = key.NewBinding(key.WithKeys("up", "down"))

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keys.Overlay.Cancel):
			return m, cmds.CloseModal(nil)

		case key.Matches(msg, keys.Overlay.Submit):
			if len(m.results) == 0 || m.cursor < 0 {
				return m, nil
			}
			r := m.results[m.cursor]
			// Close first, then jump as the follow-up, so the modal is gone
			// before AppModel switches lists and moves the selection.
			return m, cmds.CloseModal(cmds.JumpToTask(r.TaskID, r.ListID))

		case key.Matches(msg, navBinding):
			dir := 1
			if msg.String() == "up" {
				dir = -1
			}
			if len(m.results) > 0 {
				m.cursor += dir
				m.clampCursor()
			}
			return m, nil

		default:
			m.errMsg = ""
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.runSearch()
			return m, cmd
		}
	}

	// Non-key messages (cursor blink) go straight to the input.
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}