package tasktree

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
)

// focusedZoneID is the zone id this component answers to
// (constants.COMPONENT_TASK_TREE). The value is part of the focus protocol,
// so it is written out here rather than imported — a zone must not
// accidentally answer to another zone's id.
const focusedZoneID = 1

// View renders raw content for Bubble Tea's Model contract. Taskspanel calls
// ViewInPanel with the exact inner Tasks dimensions during normal composition.
func (m Model) View() tea.View {
	return tea.NewView(m.ViewInPanel(chrome.PanelBodyWidth(m.body.MainWidth), chrome.PanelBodyHeight(m.body.Height), chrome.PanelBg(m.focused)))
}

// ViewInPanel renders the task tree as raw Tasks-surface content. Taskspanel
// owns the enclosing frame, title, elevation, and footer composition.
func (m Model) ViewInPanel(width, height int, bg color.Color) string {
	m.filterInput.SetWidth(max(0, width-6))

	switch {
	case !m.activeList:
		return appstyles.FillBackground(bg, lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Render("Add a task to get started"))
	case m.creating:
		// Inline creation mode: render the sections and splice the create row
		// at the insertion point. On an empty list this renders just the
		// create row, which is the empty state.
		pending, complete := m.splitSections()
		return appstyles.FillBackground(bg, m.renderSections(pending, complete, width, bg))
	case m.filterActive():
		return appstyles.FillBackground(bg, m.renderFiltered(width, bg))
	default:
		pending, complete := m.splitSections()
		return appstyles.FillBackground(bg, m.renderSections(pending, complete, width, bg))
	}
}

// splitSections splits displayed rows into Pending and Complete based on root
// task status. displayedRows is used (not visibleRows) so the sections match
// the on-screen order the cursor actually navigates (phase B step 3).
func (m *Model) splitSections() (pending, complete []apptypes.Row) {
	for _, row := range m.displayedRows() {
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

// renderSections renders the Pending and Complete sections, splicing the
// inline create row in immediately after the reference task (its insertion
// point) within whichever section that task lives. The create row is never
// shown as a "no tasks yet" card: on an empty list it is the only row,
// placed at the end.
func (m *Model) renderSections(pending, complete []apptypes.Row, width int, bg color.Color) string {
	var lines []string
	placedCreate := false

	if len(pending) > 0 {
		lines = append(lines, sectionHeader("Pending", len(pending)))
		lines, placedCreate = m.appendSectionRows(lines, pending, width, bg, placedCreate)
	}

	if len(pending) > 0 && len(complete) > 0 {
		lines = append(lines, chrome.PanelRule(width))
	}

	if len(complete) > 0 {
		lines = append(lines, sectionHeader("Complete", len(complete)))
		lines, placedCreate = m.appendSectionRows(lines, complete, width, bg, placedCreate)
	}

	if m.creating && !placedCreate {
		lines = append(lines, m.renderCreateRow(width, bg))
	}

	return lipgloss.JoinVertical(lipgloss.Top, lines...)
}

// appendSectionRows renders each row of a section, inserting the create row
// immediately after the row whose id matches createBeforeID (the insertion
// reference) when in creating mode.
func (m *Model) appendSectionRows(lines []string, rows []apptypes.Row, width int, bg color.Color, placed bool) ([]string, bool) {
	for _, row := range rows {
		lines = append(lines, m.renderRow(row, width, bg))
		if m.creating && !placed && row.Task.ID == m.createBeforeID {
			lines = append(lines, m.renderCreateRow(width, bg))
			placed = true
		}
	}
	return lines, placed
}

// sectionHeader renders a bold TextPrimary name followed by a dimmed count,
// the same "name, then a muted count" shape the lists panel uses for a list
// row (docs/DESIGN.md §12).
func sectionHeader(name string, count int) string {
	return primary(true).Render(name) + " " + muted().Render("("+strconv.Itoa(count)+")")
}

// renderFiltered renders the /-filter view: the filter bar over the flat
// filtered row list. The Pending/Complete section headers are suppressed while
// filtering — there is no honest way to split a half-filtered set into them
// (docs/plans/phase-8-search.md step 1).
func (m *Model) renderFiltered(width int, bg color.Color) string {
	rows, matched := matchVisible(m.rows, m.filterQuery)

	lines := []string{m.renderFilterBar()}
	if len(rows) == 0 {
		lines = append(lines, chrome.EmptyStateCard("No tasks match", width, 3))
	} else {
		for _, row := range rows {
			// Only dim ancestors of a real match; when the query is empty (the
			// input is open but nothing typed yet) nothing is dimmed.
			dimmed := m.filterQuery != "" && !matched[row.Task.ID]
			lines = append(lines, m.renderFilterRow(row, width, dimmed, bg))
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
func (m *Model) renderFilterRow(row apptypes.Row, width int, dimmed bool, bg color.Color) string {
	if dimmed {
		indent := strings.Repeat("  ", row.Depth)
		return dim().Render(chrome.Truncate(indent+"[…] "+row.Task.Title, width))
	}
	return m.renderRow(row, width, bg)
}

// renderRow renders one task row with proper indent, glyph, checkbox, title,
// status, and progress columns. The layout is computed by renderTaskRowBase.
func (m *Model) renderRow(row apptypes.Row, width int, bg color.Color) string {
	indent := strings.Repeat(" ", 2*row.Depth)
	glyph := " "
	if row.HasChildren {
		if m.collapsed[row.Task.ID] {
			glyph = "▸"
		} else {
			glyph = "▾"
		}
	}

	checkbox := "[ ]"
	checkboxFg := appstyles.Active.TextMuted
	textFg := appstyles.Active.TextPrimary
	if row.Task.Status == apptypes.StatusComplete {
		checkbox = "[x]"
		checkboxFg = appstyles.Active.StatusComplete
		textFg = appstyles.Active.TextMuted
	} else if row.Task.Status == apptypes.StatusInProgress {
		checkbox = "[~]"
		checkboxFg = appstyles.Active.StatusInProgress
	}

	title := row.Task.Title
	if row.Task.Status == apptypes.StatusComplete {
		title = lipgloss.NewStyle().Foreground(textFg).Render(title)
	}

	status := statusLabel(row.Task.Status)
	progress := progressLabel(row, m.rows)

	checkboxColored := lipgloss.NewStyle().Foreground(checkboxFg).Render(checkbox)
	return m.renderTaskRowBase(indent, glyph, checkboxColored, title, status, progress,
		3, width, bg, row.Task.ID == m.selectedID)
}

// renderCreateRow renders the inline "new task" row as a Cursor-style bar:
// full remaining width on ModalBg, leading → prompt (accent), placeholder
// or typed text, empty status/progress, and the selected-row background.
// Placeholder is "Add a follow-up" when the level offset is non-zero
// (phase B step 5).
func (m *Model) renderCreateRow(width int, bg color.Color) string {
	glyph := "-"
	switch m.createLevelOffset {
	case +1:
		glyph = "+"
	case -1:
		glyph = "^"
	}

	selectedDepth := 0
	if m.selectedID != "" {
		if row := m.findRow(m.selectedID); row != nil {
			selectedDepth = row.Depth
		}
	}
	indent := strings.Repeat(" ", max(0, 2*(selectedDepth+m.createLevelOffset)))

	// prefix = indent + →(1) + space + glyph(1) + space + checkbox slot
	prefixWidth := len(indent) + 1 + 1 + 1 + 1 + 1
	m.createInput.SetWidth(max(1, width-prefixWidth))

	arrow := lipgloss.NewStyle().Foreground(appstyles.Active.Accent).Render("→")
	checkboxSlot := lipgloss.NewStyle().Render(" ")

	var title string
	if m.createInput.Value() == "" {
		title = lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Render(m.createInput.Placeholder)
	} else {
		title = m.createInput.View()
	}

	rowContent := lipgloss.JoinHorizontal(lipgloss.Left,
		indent, arrow, " ", glyph, " ", checkboxSlot, " ", title)

	rowStyle := lipgloss.NewStyle()
	if true { // create row is always the active row
		rowStyle = rowStyle.Background(appstyles.Active.ModalBg)
	}

	return rowStyle.Render(appstyles.FillBackground(bg, rowContent))
}

// taskRowCols describes the computed width of each column in a task row.
type taskRowCols struct {
	checkbox int
	title    int
	status   int
	progress int
}

// computeTaskRowCols distributes tableWidth among the task row's columns.
// checkbox is never dropped; title is never dropped; status and progress are
// dropped whole (in that order) when the table is too narrow.
func computeTaskRowCols(tableWidth, checkboxWidth int, status, progress string) taskRowCols {
	cols := taskRowCols{checkbox: checkboxWidth}

	statusW := 0
	progressW := 0
	if status != "" {
		statusW = len(status) + 1 // +1 for trailing gap
	}
	if progress != "" {
		progressW = len(progress) + 1 // +1 for trailing gap
	}

	// Drop order: progress first, then status
	if progressW > 0 && tableWidth-statusW-progressW < 1 {
		progressW = 0
	}
	if statusW > 0 && tableWidth-statusW-progressW < 1 {
		statusW = 0
	}

	cols.status = statusW
	cols.progress = progressW
	cols.title = max(1, tableWidth-statusW-progressW)

	return cols
}

// statusLabel returns the display label for a task status.
func statusLabel(status apptypes.Status) string {
	switch status {
	case apptypes.StatusPending:
		return "pending"
	case apptypes.StatusInProgress:
		return "in progress"
	case apptypes.StatusComplete:
		return "complete"
	}
	return ""
}

// progressLabel returns the display label for an in-progress task's progress,
// or "" when the task has no progress to show.
func progressLabel(row apptypes.Row, rows []apptypes.Row) string {
	if row.Task.Status != apptypes.StatusInProgress {
		return ""
	}
	switch row.Task.ProgressKind {
	case apptypes.ProgressPercentage:
		if row.Task.ProgressPct != nil {
			return fmt.Sprintf("%d%%", *row.Task.ProgressPct)
		}
	case apptypes.ProgressSubtasks:
		pct, displayAsSimple := apptypes.DerivedPercent(rows, row.Task.ID)
		if !displayAsSimple {
			return fmt.Sprintf("%d%%", pct)
		}
	}
	return ""
}

// renderTaskRowBase renders a single task row with the shared column layout.
func (m *Model) renderTaskRowBase(indent, glyph, checkbox, title, status, progress string,
	checkboxWidth, width int, bg color.Color, isSelected bool) string {
// prefix = indent + glyph(1) + space + checkbox + space. The glyph (▾/▸/
// blank) is a single display cell, so it is counted here rather than with
// len() (▾ is multi-byte). prefixWidth must equal the columns JoinHorizontal
// spends on the fixed left side, or the table budget over-runs the panel.
	glyphWidth := 1
	prefixWidth := len(indent) + glyphWidth + 1 + checkboxWidth + 1
	tableWidth := width - prefixWidth
	if tableWidth < 1 {
		tableWidth = 1
	}

	hasStatus := status != ""
	hasProgress := progress != ""
	cols := computeTaskRowCols(tableWidth, checkboxWidth, status, progress)

	checkboxCell := lipgloss.NewStyle().Width(cols.checkbox).Render(checkbox)

	titleText := chrome.Truncate(title, max(1, cols.title-1))
	titleCell := lipgloss.NewStyle().Width(cols.title).Render(titleText)

	var statusCell, progressCell string
	if cols.status > 0 && hasStatus {
		statusCell = lipgloss.NewStyle().
			Foreground(appstyles.Active.TextMuted).
			Width(cols.status).
			Render(chrome.Truncate(status, max(1, cols.status-1)))
	}
	if cols.progress > 0 && hasProgress {
		progressCell = lipgloss.NewStyle().
			Foreground(appstyles.Active.TextDim).
			Width(cols.progress).
			Render(chrome.Truncate(progress, max(1, cols.progress-1)))
	}

	rowContent := lipgloss.JoinHorizontal(lipgloss.Left,
		indent,
		glyph,
		" ",
		checkboxCell,
		" ",
		titleCell,
		statusCell,
		progressCell,
	)

	rowStyle := lipgloss.NewStyle()
	if isSelected {
		rowStyle = rowStyle.Background(appstyles.Active.ModalBg)
	}

	return rowStyle.Render(appstyles.FillBackground(bg, rowContent))
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
