package tasktree

import (
	"fmt"
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
	} else if m.filterActive() {
		body = m.renderFiltered(width)
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

// renderFiltered renders the /-filter view: the filter bar over the flat
// filtered row list. The Pending/Complete section headers are suppressed while
// filtering — there is no honest way to split a half-filtered set into them
// (docs/plans/phase-8-search.md step 1).
func (m *Model) renderFiltered(width int) string {
	rows, matched := matchVisible(m.rows, m.filterQuery)

	lines := []string{m.renderFilterBar()}
	if len(rows) == 0 {
		lines = append(lines, chrome.EmptyStateCard("No tasks match", width, 3))
	} else {
		for _, row := range rows {
			// Only dim ancestors of a real match; when the query is empty (the
			// input is open but nothing typed yet) nothing is dimmed.
			dimmed := m.filterQuery != "" && !matched[row.Task.ID]
			lines = append(lines, m.renderFilterRow(row, width, dimmed))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Top, lines...)
}

// renderFilterBar shows the live input while typing ([/ query]) or, once a
// query is applied, a dimmed summary ([/ query — esc to clear]).
func (m *Model) renderFilterBar() string {
	slash := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Bold(true).Render("/")
	dim := muted().Render
	if m.filterTyping {
		return slash + " " + m.filterInput.View()
	}
	return slash + " " + dim(m.filterQuery) + "  " + dim("esc to clear")
}

// renderFilterRow renders one filtered row. A directly-matched row renders like
// a normal task row; an ancestor that only stays visible to anchor a match
// renders dimmed so the two are distinguishable (docs/plans/phase-8-search.md
// step 1's unmatched styling).
func (m *Model) renderFilterRow(row apptypes.Row, width int, dimmed bool) string {
	if dimmed {
		indent := strings.Repeat("  ", row.Depth)
		return dim().Render(chrome.Truncate(indent+"[…] "+row.Task.Title, width))
	}
	return m.renderRow(row, width)
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

	// Calculate available width for title + progress suffix
	prefixWidth := len(indent) + 3 // indent + glyph + space + checkbox + space

	// Compute progress suffix if applicable
	var progressSuffix string
	if row.Task.Status == apptypes.StatusInProgress {
		switch row.Task.ProgressKind {
		case apptypes.ProgressPercentage:
			if row.Task.ProgressPct != nil {
				progressSuffix = fmt.Sprintf(" (%d%%)", *row.Task.ProgressPct)
			}
		case apptypes.ProgressSubtasks:
			pct, displayAsSimple := apptypes.DerivedPercent(m.rows, row.Task.ID)
			if !displayAsSimple {
				progressSuffix = fmt.Sprintf(" (%d%%)", pct)
			}
		}
	}

	// Truncate title to fit the suffix
	suffixWidth := len(progressSuffix)
	titleWidth := width - prefixWidth - suffixWidth
	if titleWidth < 1 {
		// No room for title, show nothing
		titleWidth = 0
	}
	title := chrome.Truncate(row.Task.Title, titleWidth)

	if row.Task.Status == apptypes.StatusComplete {
		title = muted().Render(title)
	} else {
		title = lipgloss.NewStyle().Foreground(textStyle).Render(title)
	}

	// Render progress suffix in TextMuted
	if progressSuffix != "" {
		progressSuffix = muted().Render(progressSuffix)
	}

	isSelected := row.Task.ID == m.selectedID
	rowStyle := lipgloss.NewStyle()
	if isSelected {
		rowStyle = rowStyle.Background(appstyles.Active.ModalBg)
	}

	content := indent + glyph + " " + checkbox + " " + title + progressSuffix
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
