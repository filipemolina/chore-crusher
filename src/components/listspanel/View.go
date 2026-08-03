package listspanel

import (
	"fmt"
	"io"
	"strconv"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
)

// listDelegate renders each list row as a Height-4 card with a full-height ▌
// bar, matching stack-stitcher's groups-list card contract (phase B step 1).
type listDelegate struct {
	isParentFocused bool
}

func (d listDelegate) Height() int                             { return 4 }
func (d listDelegate) Spacing() int                            { return 0 }
func (d listDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d listDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	l, ok := listItem.(apptypes.ListSummary)
	if !ok {
		return
	}

	isSelected := index == m.Index()
	rowBg := chrome.ListRowBg(isSelected, d.isParentFocused)

	barColor := appstyles.Active.TextMuted
	if isSelected {
		barColor = appstyles.Active.Accent
	}

	nameStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Background(rowBg)
	countStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Background(rowBg)

	if isSelected && d.isParentFocused {
		nameStyle = nameStyle.Bold(true)
	} else if !d.isParentFocused {
		nameStyle = nameStyle.Foreground(appstyles.Active.TextMuted)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		nameStyle.Render(l.List.Name),
		countStyle.Render(
			strconv.Itoa(l.PendingCount)+" pending · "+strconv.Itoa(l.CompleteCount)+" done"))

	wrapper := lipgloss.NewStyle().
		Width(m.Width()-1).
		Padding(1).
		Background(rowBg).
		Render(content)

	row := appstyles.FillBackground(rowBg,
		lipgloss.JoinHorizontal(lipgloss.Left, chrome.BarColumn(barColor, rowBg, wrapper), wrapper))

	fmt.Fprint(w, row)
}

// View renders the lists panel.
func (m Model) View() tea.View {
	width := chrome.PanelBodyWidth(m.body.ListsWidth)
	height := chrome.PanelBodyHeight(m.body.Height)

	var body string
	if len(m.list.Items()) == 0 {
		body = chrome.EmptyStateCard("No lists yet.\nPress n to create one.", width, height)
	} else {
		body = m.list.View()
	}

	return tea.NewView(chrome.PanelFrame("Lists", m.focused, m.body.ListsWidth, m.body.Height, body))
}
