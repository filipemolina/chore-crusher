package tasktree

import (
	"strconv"
	"strings"

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

// View renders the task tree: Pending and Complete sections with full hierarchy.
func (m Model) View() tea.View {
	width := chrome.PanelBodyWidth(m.body.MainWidth)
	height := chrome.PanelBodyHeight(m.body.TreeHeight)

	var body string
	if !m.activeList || len(m.rows) == 0 {
		body = chrome.EmptyStateCard("Add a task to get started", width, height)
	} else {
		pending, complete := m.splitSections()
		body = m.renderSections(pending, complete, width)
	}

	return tea.NewView(chrome.PanelFrame(m.focused, width, height, body))
}

// splitSections splits visible rows into Pending and Complete based on root task status.
func (m *Model) splitSections() (pending, complete []apptypes.Row) {
	visible := m.visibleRows()
	for _, row := range visible {
		if row.Depth == 0 {
			if row.Task.Status == apptypes.StatusComplete {
				complete = append(complete, row)
			} else {
				pending = append(pending, row)
			}
		} else {
			// Non-root: follow parent
			if row.Task.ParentID != nil {
				parent := m.findRow(*row.Task.ParentID)
				if parent != nil && parent.Task.Status == apptypes.StatusComplete {
					complete = append(complete, row)
				} else {
					pending = append(pending, row)
				}
			} else {
				pending = append(pending, row)
			}
		}
	}
	return
}

// renderSections renders Pending and Complete sections with their tasks.
func (m *Model) renderSections(pending, complete []apptypes.Row, width int) string {
	var lines []string

	if len(pending) > 0 {
		lines = append(lines, primary(true).Render("Pending")+" "+
			muted().Render("("+strconv.Itoa(len(pending))+")"))
		for _, row := range pending {
			lines = append(lines, m.renderRow(row, width))
		}
	} else {
		lines = append(lines, chrome.EmptyStateCard("No tasks yet", width, 3))
	}

	if len(complete) > 0 {
		if len(pending) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, primary(true).Render("Complete")+" "+
			muted().Render("("+strconv.Itoa(len(complete))+")"))
		for _, row := range complete {
			lines = append(lines, m.renderRow(row, width))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Top, lines...)
}

// renderRow renders one task row with proper indent, glyph, checkbox, and title.
func (m *Model) renderRow(row apptypes.Row, width int) string {
	indent := strings.Repeat(" ", 2*row.Depth)
	glyph := " "
	if row.HasChildren {
		if m.collapsed[row.Task.ID] {
			glyph = "▸"
		} else {
			glyph = "▾"
		}
	}

	var checkbox string
	textStyle := appstyles.Active.TextPrimary
	if row.Task.Status == apptypes.StatusComplete {
		checkbox = "[x]"
		textStyle = appstyles.Active.TextMuted
	} else if row.Task.Status == apptypes.StatusInProgress {
		checkbox = "[~]"
	} else {
		checkbox = "[ ]"
	}

	title := chrome.Truncate(row.Task.Title, width-len(indent)-3)
	if row.Task.Status == apptypes.StatusComplete {
		title = muted().Render(title)
	} else {
		title = lipgloss.NewStyle().Foreground(textStyle).Render(title)
	}

	isSelected := row.Task.ID == m.selectedID
	rowStyle := lipgloss.NewStyle()
	if isSelected {
		rowStyle = rowStyle.Background(appstyles.Active.ModalBg)
	}

	content := indent + glyph + " " + checkbox + " " + title
	return rowStyle.Render(appstyles.FillBackground(
		chrome.PanelBg(isSelected),
		content,
	))
}

func primary(bold bool) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)
	if bold {
		s = s.Bold(true)
	}
	return s
}

func muted() lipgloss.Style { return lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted) }
func dim() lipgloss.Style   { return lipgloss.NewStyle().Foreground(appstyles.Active.TextDim) }
