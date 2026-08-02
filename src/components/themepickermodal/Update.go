package themepickermodal

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/keys"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var finalCmds []tea.Cmd

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, keys.Overlay.Cancel):
			// Restore the theme the user started with, then close.
			appstyles.SetTheme(m.originalTheme)
			return m, cmds.CloseModal(nil)

		case key.Matches(keyMsg, keys.Overlay.Submit):
			if item, ok := m.list.SelectedItem().(apptypes.ThemeItem); ok {
				// Close first, then apply-and-persist as the follow-up, so
				// the modal is gone before the action runs.
				return m, cmds.CloseModal(cmds.ApplyTheme(item.Name))
			}
		}
	}

	// Track the cursor index before the list updates so we can detect
	// movement and preview the new theme live.
	prevIndex := m.list.Index()

	var listCmd tea.Cmd
	m.list, listCmd = m.list.Update(msg)
	finalCmds = append(finalCmds, listCmd)

	// If the cursor moved, preview the theme under it. This repaints the
	// entire UI behind the modal on the next frame - the point of live
	// preview. The original theme is preserved in m.originalTheme so Esc
	// still restores correctly.
	if m.list.Index() != prevIndex {
		if item, ok := m.list.SelectedItem().(apptypes.ThemeItem); ok {
			appstyles.SetTheme(item.Name)
		}
	}

	return m, tea.Batch(finalCmds...)
}
