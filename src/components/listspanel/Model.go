package listspanel

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-completer/src/appstyles"
	"github.com/filipemolina/chore-completer/src/apptypes"
	"github.com/filipemolina/chore-completer/src/cmds"
	"github.com/filipemolina/chore-completer/src/components/chrome"
)

// focusedZoneID is the zone id this component answers to
// (constants.COMPONENT_LISTS_PANEL).
const focusedZoneID = 0

// Model is the lists-panel zone. Phase 3 renders the lists the store holds —
// enough to verify the TUI and CLI share one store (docs/plans/phase-3-tui-shell.md
// "Killing and restarting the TUI against the same database file shows the
// same lists and counts phase 2's CLI created") — as plain rows inside the
// shared frame. Phase 6 (docs/plans/phase-6-lists-panel.md) replaces the
// body with the real bubbles list; the frame and focus handling below are
// what it keeps.
type Model struct {
	focused bool
	body    cmds.SetBodyLayoutMsg
	lists   []apptypes.ListSummary
}

func (m Model) Init() tea.Cmd { return nil }

// New builds the placeholder lists panel.
func New() tea.Model { return Model{} }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmds.SetBodyLayoutMsg:
		m.body = msg

	case cmds.SetFocusMsg:
		m.focused = int(msg) == focusedZoneID

	case cmds.RefreshListsMsg:
		if msg.Err == nil {
			m.lists = msg.Lists
		}
	}

	return m, nil
}

// View renders each list as one row: name, then the pending/complete counts
// the CLI prints (docs/DESIGN.md §4's count contract). The body stays inside
// chrome.PanelFrame; no literal Padding or hand-picked color appears here.
func (m Model) View() tea.View {
	width := chrome.PanelBodyWidth(m.body.ListsWidth)
	height := chrome.PanelBodyHeight(m.body.Height)

	var body string
	if len(m.lists) == 0 {
		body = chrome.EmptyStateCard("no lists yet", width, height)
	} else {
		nameStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)
		countStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextDim)

		lines := make([]string, 0, len(m.lists))
		for _, l := range m.lists {
			lines = append(lines, nameStyle.Render(l.Name)+"  "+countStyle.Render(
				strconv.Itoa(l.PendingCount)+" pending · "+strconv.Itoa(l.CompleteCount)+" done"))
		}
		body = lipgloss.JoinVertical(lipgloss.Left, lines...)
	}

	return tea.NewView(chrome.PanelFrame(m.focused, width, height, body))
}
