package chrome

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
)

// EmptyStateCard renders the one recessed empty-state surface every
// empty zone uses (docs/DESIGN.md §12 "Empty states"): a box on the given
// background tier, padded exactly like PanelFrame (1, 2), one line of
// TextDim guidance text, left-aligned. Do not center empty-state text and
// do not give it its own bespoke padding - reusing the exact PanelFrame
// numbers is what makes it read as "this zone, currently empty".
//
// bg is the background color the card should match — typically the panel's
// own background (PanelBg) so the empty state reads as the same surface
// as the title bar above it.
func EmptyStateCard(message string, width, height int, bg color.Color) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	card := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(1, 2).
		Background(bg).
		Render(lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Render(message))

	// The card's own joins leave unstyled padding on short lines; seal
	// against the provided background before anything composes it.
	return appstyles.FillBackground(bg, card)
}
