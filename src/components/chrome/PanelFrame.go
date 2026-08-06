package chrome

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
)

const panelTitleChromeHeight = 2

// PanelFrame renders the shared Lists or Tasks surface: its accent title chip,
// fixed (1, 2) padding, focus-lift background, and background seal. The frame
// receives the total surface box and is the only place that derives its inner
// geometry (docs/DESIGN.md §12 "Two shared frames").
func PanelFrame(title string, isFocused bool, width, height int, body string) string {
	return PanelFrameWithRightTitle(title, "", isFocused, width, height, body)
}

// PanelFrameWithRightTitle is PanelFrame with an optional second label pinned
// flush-right on the same title row, bold, in the panel's primary text color.
// The Tasks surface uses it to show the active list's name beside the "Tasks"
// chip (docs/DESIGN.md §12). An empty rightTitle renders exactly like
// PanelFrame — just the left accent chip — so callers with nothing to show pass
// "". The right label is truncated to the space the chip leaves, so it never
// pushes the frame wider.
func PanelFrameWithRightTitle(leftTitle, rightTitle string, isFocused bool, width, height int, body string) string {
	bg := PanelBg(isFocused)
	header := titleRow(leftTitle, rightTitle, bg, width)
	content := appstyles.FillBackground(bg, lipgloss.JoinVertical(lipgloss.Left, header, "", body))

	return FitBox(WrapperStyle.Background(bg), width, height).Render(content)
}

// titleRow renders the accent chip and, when rightTitle is non-empty and there
// is room, a flush-right bold label on the same line. The row is laid out
// across the panel body width so the right label lands on the body's right
// edge; a width too small to hold both falls back to the chip alone.
func titleRow(leftTitle, rightTitle string, bg color.Color, width int) string {
	chip := appstyles.NormalTitle().MarginLeft(2).Render(leftTitle)
	if rightTitle == "" {
		return chip
	}

	contentWidth := PanelBodyWidth(width)
	chipW := lipgloss.Width(chip)
	budget := contentWidth - chipW - 1
	if contentWidth <= 0 || budget <= 0 {
		return chip
	}

	right := lipgloss.NewStyle().
		Bold(true).
		Foreground(appstyles.Active.TextPrimary).
		Background(bg).
		Render(Truncate(rightTitle, budget))

	gap := max(1, contentWidth-chipW-lipgloss.Width(right))
	spacer := lipgloss.NewStyle().Background(bg).Width(gap).Render("")
	return lipgloss.JoinHorizontal(lipgloss.Top, chip, spacer, right)
}

// PanelBodyWithFooter clips content before joining a non-empty footer at the
// final body row. An empty footer reserves no row.
func PanelBodyWithFooter(width, availableHeight int, bg color.Color, content, footer string) string {
	footerHeight := 0
	if footer != "" {
		footerHeight = lipgloss.Height(footer)
	}

	contentHeight := max(0, availableHeight-footerHeight)
	parts := make([]string, 0, 3)
	if contentHeight > 0 {
		content = lipgloss.NewStyle().MaxHeight(contentHeight).Render(content)
		parts = append(parts, content)
		if gap := contentHeight - lipgloss.Height(content); gap > 0 {
			parts = append(parts, lipgloss.NewStyle().Background(bg).Width(max(0, width)).Height(gap).Render(""))
		}
	}
	if footer != "" {
		parts = append(parts, footer)
	}

	body := lipgloss.JoinVertical(lipgloss.Left, parts...)
	if availableHeight > 0 {
		body = lipgloss.NewStyle().MaxHeight(availableHeight).Render(body)
	}
	return body
}

// PanelBodyWidth and PanelBodyHeight are the space a panel body gets inside a
// frame of the given total size: the frame's own padding and title chrome
// taken off. Callers size their content with these rather than hardcoded
// offsets, so a frame change cannot silently push content out of its surface.
func PanelBodyWidth(total int) int {
	frameW, _ := WrapperStyle.GetFrameSize()

	return max(0, total-frameW)
}

func PanelBodyHeight(total int) int {
	_, frameH := WrapperStyle.GetFrameSize()

	return max(0, total-frameH-panelTitleChromeHeight)
}

// PanelRule is the thin horizontal line that separates the Pending and
// Complete sections inside the Tasks panel.
func PanelRule(width int) string {
	return lipgloss.NewStyle().
		Foreground(appstyles.Active.BorderDefault).
		Width(width).
		Render(strings.Repeat("─", max(width, 0)))
}
