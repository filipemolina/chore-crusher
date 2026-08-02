package chrome

import (
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
)

// EmptyStateCard renders the one recessed-card pattern every empty zone uses
// (docs/DESIGN.md §12 "Empty states"): a box on the BackgroundRecessed tier,
// rimmed with BorderCard (not BorderDefault - a border has to contrast with
// the surface it wraps, and BorderDefault moves *toward* BackgroundRecessed
// rather than away from it), padded exactly like PanelFrame (1, 2), one line
// of TextDim guidance text, left-aligned. Do not center empty-state text and
// do not give it its own bespoke padding - reusing the exact PanelFrame
// numbers is what makes it read as "this zone, currently empty".
//
// Phase 4's empty Pending section and phase 6's empty lists panel call this
// same function; a second bespoke box renderer is exactly how two zones
// drift apart.
func EmptyStateCard(message string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	bg := appstyles.Active.BackgroundRecessed

	card := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(appstyles.Active.BorderCard).
		BorderBackground(bg).
		Background(bg).
		Render(lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Render(message))

	// The card's own joins leave unstyled padding on short lines; seal
	// against the recessed surface before anything composes it.
	return appstyles.FillBackground(bg, card)
}
