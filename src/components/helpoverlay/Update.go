package helpoverlay

import (
	"strings"

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
		// While the filter input is open it claims the keyboard — the same
		// rule the task tree's own `/`-filter follows.
		if m.filterTyping {
			return m.handleFilterKey(msg)
		}

		// A filter is applied (not typing): esc clears it back to the full
		// catalog rather than closing the overlay outright, matching the
		// task tree's "esc clears before esc closes" precedent.
		if m.filterApplied && key.Matches(msg, keys.Overlay.Cancel) {
			m.clearFilter()
			return m, nil
		}

		switch {
		// / opens the filter, narrowing the catalog to entries whose key or
		// description fuzzy-matches — the same affordance and binding the
		// task tree's own filter uses.
		case key.Matches(msg, keys.Global.Filter):
			m.filterTyping = true
			m.filterInput.Focus()
			return m, nil

		// Either of the two closes: ? is the toggle that opened it, esc is
		// the cancel every overlay answers. Only the overlay closes - the
		// keys never quit the app from here, because the overlay owns the
		// keyboard while it is open.
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

// handleFilterKey handles a keystroke while the filter input is open: esc
// clears the filter and closes the input, enter applies the query and blurs
// the input (leaving the filtered view active, arrows scrolling it again),
// anything else types into the input and narrows the catalog live.
func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, keys.Overlay.Submit) {
		m.filterTyping = false
		m.filterApplied = strings.TrimSpace(m.filterInput.Value()) != ""
		m.filterInput.Blur()
		m.offset = 0
		return m, nil
	}

	if key.Matches(msg, keys.Overlay.Cancel) {
		m.clearFilter()
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	// Live narrowing, the same as the task tree's filter: every keystroke
	// lands in filterQuery so the catalog updates as the user types instead
	// of waiting for enter.
	m.filterQuery = m.filterInput.Value()
	m.offset = 0
	return m, cmd
}

// clearFilter returns the overlay to the full catalog, used by esc while
// typing and esc on an applied filter.
func (m *Model) clearFilter() {
	m.filterTyping = false
	m.filterApplied = false
	m.filterQuery = ""
	m.filterInput.Reset()
	m.filterInput.Blur()
	m.offset = 0
}
