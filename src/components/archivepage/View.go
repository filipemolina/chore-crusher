package archivepage

import (
	"fmt"
	"image/color"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/components/chrome"
	"github.com/filipemolina/farol/src/keys"
)

// View renders the page filling the whole body — the terminal width, not
// just a Tasks-sized column, since the Archive page replaces the
// Tasks/Lists split entirely rather than sharing the row with it
// (docs/DESIGN.md §5). The keybinding bar goes blank while the page owns the
// keyboard (mirroring Details), so the page renders its own hint line in
// place of a footer copy that could show a stale Lists hint.
func (m Model) View() tea.View {
	width := m.body.TerminalWidth
	height := m.body.Height
	bg := chrome.PanelBg(m.focused)
	bodyW := max(1, chrome.PanelBodyWidth(width))
	bodyH := max(1, chrome.PanelBodyHeight(height))

	// The hint line is docked at the very bottom of every state — loading,
	// error, empty, and the normal split alike — since it stands in for the
	// footer bar, which goes blank entirely while the page owns the keyboard
	// (mirroring Details). Without it here too, the loading/empty states
	// would give no clue how to leave the page.
	hint := m.renderHint(bg)
	reserved := lipgloss.Height(hint) + 1 // +1 for the blank spacer above the hint
	contentH := max(0, bodyH-reserved)

	var content string
	switch {
	case m.loading:
		content = chrome.EmptyStateCard("Loading archived lists…", bodyW, contentH, bg)
	case m.loadErr != nil:
		content = chrome.EmptyStateCard(fmt.Sprintf("Could not load archived lists\n\n%v", m.loadErr), bodyW, contentH, bg)
	case len(m.entries) == 0:
		content = chrome.EmptyStateCard(
			"No archived lists yet\n\nLists you archive (farol lists archive, or from the Lists panel) show up here.",
			bodyW, contentH, bg)
	default:
		content = m.renderSplit(bodyW, contentH, bg)
	}

	blank := lipgloss.NewStyle().Background(bg).Width(bodyW).Render("")
	body := lipgloss.JoinVertical(lipgloss.Left, content, blank, hint)

	title := "Archived Lists"
	return tea.NewView(chrome.PanelFrameWithRightTitle(title, m.countLabel(), m.focused, width, height, body))
}

// countLabel is the flush-right label on the title row: how many archived
// lists exist, and — while a filter narrows that — how many currently match,
// the same "N shown of M" shape the Lists panel's own filter implies through
// its item count.
func (m Model) countLabel() string {
	if len(m.entries) == 0 {
		return ""
	}
	visible := len(m.visibleEntries())
	if visible == len(m.entries) {
		return fmt.Sprintf("%d archived", len(m.entries))
	}
	return fmt.Sprintf("%d of %d archived", visible, len(m.entries))
}

// renderSplit lays the list column and the preview column side by side,
// mirroring backuppage's split. The hint line is docked by the caller
// (View), not here — every state docks it the same way.
func (m Model) renderSplit(bodyW, bodyH int, bg color.Color) string {
	listCol := m.renderListColumn(m.listWidth, bodyH, bg)
	previewCol := m.renderPreviewColumn(m.previewWidth, bodyH, bg)
	return lipgloss.JoinHorizontal(lipgloss.Top, listCol, previewCol)
}

// renderListColumn renders the filter row and the window of archived-list
// rows starting at m.listScroll (or an inline "no matches" message when the
// filter leaves nothing) — Update keeps m.listScroll clamped to the current
// entry count and selection, so render just windows into it rather than
// re-deriving the offset itself.
func (m Model) renderListColumn(width, height int, bg color.Color) string {
	if width < 1 {
		width = 1
	}

	filterRow := m.renderFilterRow(width, bg)
	visible := m.visibleEntries()

	var rows []string
	if len(visible) == 0 {
		rows = append(rows, lipgloss.NewStyle().
			Foreground(appstyles.Active.TextDim).
			Background(bg).
			Width(width).
			Render("No archived lists match \""+m.filterInput.Value()+"\""))
	} else {
		start := min(m.listScroll, len(visible))
		end := min(len(visible), start+m.listViewportRows())
		for i := start; i < end; i++ {
			rows = append(rows, m.renderRow(i, visible[i], width))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, append([]string{filterRow, chrome.PanelRule(width)}, rows...)...)
	return chrome.PanelBodyWithFooter(width, height, bg, content, "")
}

// renderFilterRow renders the name-filter input, sealed to the panel's
// current focus tier the same way every other themed input in this codebase
// is (chrome.SealInput, docs/DESIGN.md §12).
func (m Model) renderFilterRow(width int, bg color.Color) string {
	fi := m.filterInput
	fi.SetWidth(max(0, width-2))
	chrome.SealInput(&fi, bg, bg)
	return lipgloss.NewStyle().Width(width).Background(bg).Render(fi.View())
}

// renderRow renders one archived-list row: the name, its archived-at
// relative time, and its task counts, with the selected row lifted the same
// way every other selectable row in this codebase is (chrome.ListRowBg) and
// an accent bar on the left when selected (mirroring backuppage's own row).
func (m Model) renderRow(idx int, e apptypes.ListSummary, width int) string {
	isSelected := idx == m.selectedIdx
	// The selection highlight lifts only while the list column itself has
	// keyboard focus — with previewFocused true it drops to the unfocused
	// tier, the same elevated-vs-lifted cue the Lists/Tasks split uses across
	// panels, so it reads as "still selected, but tab is scrolling the
	// preview right now" rather than looking like the highlight vanished.
	rowBg := chrome.ListRowBg(isSelected, m.focused && !m.previewFocused)

	name := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextPrimary).
		Background(rowBg).
		Render(chrome.Truncate(e.List.Name, max(1, width-1)))

	meta := fmt.Sprintf("archived %s · %d task%s", relativeTime(e.List.ArchivedAt), e.PendingCount+e.CompleteCount, plural(e.PendingCount+e.CompleteCount))
	metaLine := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextDim).
		Background(rowBg).
		Width(max(1, width-1)).
		Render(chrome.Truncate(meta, max(1, width-1)))

	row := lipgloss.JoinVertical(lipgloss.Left, name, metaLine)

	barColor := rowBg
	if isSelected && m.focused && !m.previewFocused {
		barColor = appstyles.Active.Accent
	}
	bar := chrome.BarColumn(barColor, rowBg, row)

	full := lipgloss.JoinHorizontal(lipgloss.Left, bar, row)
	return appstyles.FillBackground(rowBg, full)
}

// renderPreviewColumn renders the selected archived list's tasks, read-only:
// a status glyph (mirroring the task tree's own vocabulary, docs/DESIGN.md
// §12) plus the title, indented by depth. It is plain text, not the
// interactive tree — there is nothing here to select or edit, only scroll
// (ArchivePage.Navigate/GoToStart/GoToEnd, while FocusPreview has moved
// keyboard focus here).
//
// The header doubles as this column's focus cue (bold/primary when it has
// keyboard focus, dim otherwise — the list column's own cue is its row
// highlight tier, renderRow) and, once there are tasks, as the "N above /
// N below" overflow indicator the task tree's own pinned-header suffix uses
// (docs/DESIGN.md §12), simplified here since this column has no sections to
// pin.
func (m Model) renderPreviewColumn(width, height int, bg color.Color) string {
	if width < 1 {
		width = 1
	}

	sel, ok := m.selectedEntry()
	label := ""
	if ok {
		label = "preview · " + sel.List.Name
	}

	var content string
	switch {
	case !ok:
		content = ""
	case m.previewLoading:
		content = lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Background(bg).Render("Loading…")
	case m.previewErr != nil:
		content = lipgloss.NewStyle().Foreground(appstyles.Active.Danger).Background(bg).Render(fmt.Sprintf("failed to load tasks: %v", m.previewErr))
	case len(m.previewRows) == 0:
		content = lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Background(bg).Render("No tasks in this list.")
	default:
		visible := max(0, height-1)
		content = m.renderPreviewRows(width, visible)
		above, below := m.previewScroll, max(0, len(m.previewRows)-m.previewScroll-visible)
		if suffix := scrollSuffix(above, below); suffix != "" {
			label += "  " + suffix
		}
	}

	headerStyle := lipgloss.NewStyle().Background(bg).Width(width)
	if m.focused && m.previewFocused {
		headerStyle = headerStyle.Foreground(appstyles.Active.TextPrimary).Bold(true)
	} else {
		headerStyle = headerStyle.Foreground(appstyles.Active.TextDim)
	}
	header := ""
	if ok {
		header = headerStyle.Render(chrome.Truncate(label, width))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, header, content)
	return lipgloss.NewStyle().Width(width).MaxHeight(height).Render(appstyles.FillBackground(bg, body))
}

// scrollSuffix renders the "N above" / "N below" overflow text, empty when
// nothing is hidden in either direction.
func scrollSuffix(above, below int) string {
	switch {
	case above > 0 && below > 0:
		return fmt.Sprintf("%d above · %d below", above, below)
	case above > 0:
		return fmt.Sprintf("%d above", above)
	case below > 0:
		return fmt.Sprintf("%d below", below)
	default:
		return ""
	}
}

// renderPreviewRows renders the window of task lines starting at
// m.previewScroll, up to maxLines of them — Update keeps previewScroll
// clamped to previewViewportRows() against the current row count, so render
// just windows into it.
func (m Model) renderPreviewRows(width, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	rows := m.previewRows
	start := min(m.previewScroll, len(rows))
	end := min(len(rows), start+maxLines)

	lines := make([]string, 0, end-start)
	for _, r := range rows[start:end] {
		lines = append(lines, m.renderPreviewRow(r, width))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderPreviewRow renders one task line: the same checkbox glyph vocabulary
// the task tree uses (◻ pending, ◼ in-progress/complete, tinted by status,
// docs/DESIGN.md §12), indented by depth, title truncated to width.
func (m Model) renderPreviewRow(r apptypes.Row, width int) string {
	indent := ""
	for range r.Depth {
		indent += "  "
	}

	checkbox := "◻"
	checkboxFg := appstyles.Active.TextMuted
	textFg := appstyles.Active.TextPrimary
	switch r.Task.Status {
	case apptypes.StatusInProgress:
		checkbox = "◼"
		checkboxFg = appstyles.Active.StatusInProgress
	case apptypes.StatusComplete:
		checkbox = "◼"
		checkboxFg = appstyles.Active.StatusComplete
		textFg = appstyles.Active.TextMuted
	}

	glyph := lipgloss.NewStyle().Foreground(checkboxFg).Render(checkbox)
	title := lipgloss.NewStyle().Foreground(textFg).Render(chrome.Truncate(r.Task.Title, max(1, width-len(indent)-2)))
	return indent + glyph + " " + title
}

// renderHint renders the page's own keybinding hint line, in place of the
// footer bar (which goes blank while the page owns the keyboard, mirroring
// Details). It is context-sensitive the same way the footer itself is:
// filtering shows only what the filter input answers to; browsing shows
// navigation plus whichever of "filter"/"clear filter" applies, plus back.
func (m Model) renderHint(bg color.Color) string {
	navDesc := "navigate"
	if m.previewFocused {
		navDesc = "scroll"
	}

	var hints []chrome.KeyHint
	switch {
	case m.filtering:
		hints = []chrome.KeyHint{
			chrome.HintAs(keys.Overlay.Submit, "done"),
			chrome.HintAs(keys.Overlay.Cancel, "clear"),
		}
	case m.filterInput.Value() != "":
		hints = []chrome.KeyHint{
			chrome.HintAs(keys.ArchivePage.Navigate, navDesc),
			chrome.HintAs(keys.ArchivePage.FocusPreview, "switch column"),
			chrome.HintAs(keys.ArchivePage.Filter, "edit filter"),
			chrome.HintAs(keys.Global.Back, "clear filter"),
		}
	default:
		hints = []chrome.KeyHint{
			chrome.HintAs(keys.ArchivePage.Navigate, navDesc),
			chrome.HintAs(keys.ArchivePage.FocusPreview, "switch column"),
			chrome.HintFor(keys.ArchivePage.Filter),
			chrome.HintFor(keys.Global.Back),
		}
	}
	if !m.filtering {
		if _, ok := m.selectedEntry(); ok {
			hints = append(hints, chrome.HintFor(keys.ArchivePage.Unarchive), chrome.HintFor(keys.ArchivePage.Delete))
		}
	}
	line := chrome.RenderKeyHints(hints, appstyles.Active.TextDim)
	return lipgloss.NewStyle().Background(bg).Render(line)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// relativeTime renders a unix timestamp as a short "N ago" label, falling
// back to an absolute date past a month — coarse buckets are enough for
// "when did I archive this", and it avoids importing a full humanize
// dependency for one label.
func relativeTime(ts *int64) string {
	if ts == nil {
		return "unknown"
	}
	d := time.Since(time.Unix(*ts, 0))
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		n := int(d / time.Minute)
		return fmt.Sprintf("%dm ago", n)
	case d < 24*time.Hour:
		n := int(d / time.Hour)
		return fmt.Sprintf("%dh ago", n)
	case d < 30*24*time.Hour:
		n := int(d / (24 * time.Hour))
		return fmt.Sprintf("%dd ago", n)
	default:
		return time.Unix(*ts, 0).UTC().Format("2006-01-02")
	}
}
