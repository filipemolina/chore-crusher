package tasktree

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-completer/src/appstyles"
	"github.com/filipemolina/chore-completer/src/apptypes"
	"github.com/filipemolina/chore-completer/src/components/chrome"
)

// focusedZoneID is the zone id this component answers to
// (constants.COMPONENT_TASK_TREE). The value is part of the focus protocol,
// so it is written out here rather than imported — a zone must not
// accidentally answer to another zone's id.
const focusedZoneID = 1

// View renders the placeholder tree body inside chrome.PanelFrame, through
// chrome.PanelBg for its focus state — the frame, the focus-lift color, and
// the truncation all come from the chrome package (docs/plans/phase-3-tui-shell.md
// step 10's verification). No literal Padding or hand-picked color may
// appear here.
func (m Model) View() tea.View {
	width := chrome.PanelBodyWidth(m.body.MainWidth)
	height := chrome.PanelBodyHeight(m.body.TreeHeight)

	var body string
	switch {
	case !m.activeList:
		body = "(task tree — phase 4)\n\nno list selected"
	case len(m.rows) == 0:
		body = "(task tree — phase 4)\n\nno tasks in this list"
	default:
		body = "(task tree — phase 4)"
		if m.selectedID != "" {
			if idx := rowIndex(m.rows, m.selectedID); idx >= 0 {
				body += "\n\n" + muted().Render("selected: "+m.rows[idx].Task.Title)
			}
		}
		body += "\n" + dim().Render(strconv.Itoa(len(m.rows))+" tasks loaded")
	}

	return tea.NewView(chrome.PanelFrame(m.focused, width, height, body))
}

// rowIndex returns the index of the row with the given task id, or -1.
func rowIndex(rows []apptypes.Row, id string) int {
	for i, r := range rows {
		if r.Task.ID == id {
			return i
		}
	}
	return -1
}

func muted() lipgloss.Style { return lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted) }
func dim() lipgloss.Style   { return lipgloss.NewStyle().Foreground(appstyles.Active.TextDim) }
