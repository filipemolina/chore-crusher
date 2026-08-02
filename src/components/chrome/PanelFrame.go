package chrome

import (
	"github.com/filipemolina/chore-crusher/src/appstyles"
)

// PanelFrame renders the shared zone frame: the fixed (1, 2) padding, the
// focus-lift background chosen by PanelBg, and the background seal. Every
// zone - the lists panel, the task tree, the add input - renders inside this
// one helper, so a component never sets its own padding, border, or corner
// treatment, and never branches on focus itself (docs/DESIGN.md §12 "The
// chrome-package contract").
//
// The panel is where tier 3/4 is established, so it is where the tier's
// background has to be sealed in: a body's own joins leave unstyled gaps
// that appstyles.FillBackground closes before FitBox's Width() padding is
// applied (that padding is styled by lipgloss already).
func PanelFrame(isFocused bool, width, height int, body string) string {
	bg := PanelBg(isFocused)
	content := appstyles.FillBackground(bg, body)

	return FitBox(WrapperStyle.Background(bg), width, height).Render(content)
}

// PanelBodyWidth and PanelBodyHeight are the space a zone body gets inside a
// frame of the given total size: the frame's own padding taken off. Callers
// size their content with these rather than with hardcoded offsets, so a
// change to WrapperStyle's padding doesn't silently push content out of the
// zone.
func PanelBodyWidth(total int) int {
	frameW, _ := WrapperStyle.GetFrameSize()

	return max(0, total-frameW)
}

func PanelBodyHeight(total int) int {
	_, frameH := WrapperStyle.GetFrameSize()

	return max(0, total-frameH)
}
