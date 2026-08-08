package listspanel

import (
	"fmt"
	"image/color"
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
// When an agent has claimed the list, a spinner glyph and short agent id
// are appended after the count line (§3.7); when only a task inside the
// list is claimed, a bare spinner is appended (§3.4).
type listDelegate struct {
	isParentFocused bool
	work            map[string]apptypes.AgentActivity
	claimedLists    map[string]bool
	animFrame       int
}

func (d listDelegate) Height() int                             { return 4 }
func (d listDelegate) Spacing() int                            { return 0 }
func (d listDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// spinnerFg is the agent-spinner color rule (§3.7): Accent on the selected
// row, TextDim otherwise — matching the task tree's spinner and this
// delegate's bar rule.
func spinnerFg(isSelected bool) color.Color {
	if isSelected {
		return appstyles.Active.Accent
	}
	return appstyles.Active.TextDim
}

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

	countLine := strconv.Itoa(l.PendingCount) + " pending · " + strconv.Itoa(l.CompleteCount) + " done"
	// A collaborative list accepts structural edits from any agent, not just
	// its own created_by owner (docs/DESIGN.md §9, "Tag a list as
	// collaborative") — an existing text tier (TextDim, the same one the
	// count line itself uses), not a new glyph, so no §12 glyph entry is
	// needed for it.
	if l.List.Collaborative {
		countLine += " · shared"
	}

	// If this list is claimed by an agent, append the spinner + agent id,
	// colored per the row's selection: Accent when selected, TextDim otherwise
	// — matching the task-tree spinner (§3.7). If instead only a task inside
	// the list is claimed, append a bare spinner: the row is an aggregate, so
	// no single agent id is named.
	claimedLine := countLine
	if a, ok := d.work[l.List.ID]; ok {
		spinner := chrome.Spinner(d.animFrame)
		agent := a.AgentID
		if len(agent) > 6 {
			agent = agent[:6]
		}
		claimedLine = countLine + " " + lipgloss.NewStyle().
			Foreground(spinnerFg(isSelected)).
			Background(rowBg).
			Render(spinner+" "+agent)
	} else if d.claimedLists[l.List.ID] {
		spinner := chrome.Spinner(d.animFrame)
		claimedLine = countLine + " " + lipgloss.NewStyle().
			Foreground(spinnerFg(isSelected)).
			Background(rowBg).
			Render(spinner)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		nameStyle.Render(l.List.Name),
		countStyle.Render(claimedLine))

	wrapper := lipgloss.NewStyle().
		Width(m.Width() - 1).
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
	bg := chrome.PanelBg(m.focused)

	var body string
	if len(m.list.Items()) == 0 {
		body = chrome.EmptyStateCard("No lists yet.\nPress n to create one.", width, height, bg)
	} else {
		content := m.list.View()
		// The bubbles list shipped dots at the bottom for multi-page lists;
		// this app renders its own "N below" overflow indicator instead
		// (docs/DESIGN.md §12).
		if below := m.listsBelow(); below > 0 {
			footer := lipgloss.NewStyle().
				Background(bg).
				Foreground(appstyles.Active.TextDim).
				Width(width).
				Render(fmt.Sprintf("%d below", below))
			body = chrome.PanelBodyWithFooter(width, height, bg, content, footer)
		} else {
			body = content
		}
	}

	return tea.NewView(chrome.PanelFrame("Lists", m.focused, m.body.ListsWidth, m.body.Height, body))
}
