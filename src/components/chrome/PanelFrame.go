package chrome

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
)

const panelTitleChromeHeight = 3

// PanelFrame renders the shared Lists or Tasks surface: its accent title chip,
// fixed (1, 2) padding, focus-lift background, and background seal. The frame
// receives the total surface box and is the only place that derives its inner
// geometry (docs/DESIGN.md §12 "Two shared frames").
func PanelFrame(title string, isFocused bool, width, height int, body string) string {
	bg := PanelBg(isFocused)
	titleRow := appstyles.NormalTitle().MarginLeft(2).Render(title)
	content := appstyles.FillBackground(bg, lipgloss.JoinVertical(lipgloss.Left, "", titleRow, "", body))

	return FitBox(WrapperStyle.Background(bg), width, height).Render(content)
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
