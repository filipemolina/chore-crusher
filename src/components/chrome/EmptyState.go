package chrome

import (
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
)

// EmptyStateCard renders the one recessed empty-state surface every empty
// zone uses (docs/DESIGN.md §12 "Empty states: one recessed-card pattern"):
// a box on the BackgroundRecessed tier, rimmed with BorderCard (not
// BorderDefault - a border has to contrast with the surface it wraps, and
// BorderDefault moves toward BackgroundRecessed rather than away from it),
// padded exactly like PanelFrame (1, 2), one line of TextDim guidance text,
// left-aligned. Do not center empty-state text and do not give it its own
// bespoke padding - reusing the exact PanelFrame numbers is what makes it
// read as "this zone, currently empty" rather than a different kind of
// surface that happens to be nearby.
//
// The card always uses the recessed tier regardless of the caller's own
// focus state, so an empty state stays visually sunken even while its
// parent panel is elevated by focus - the inset is the point.
func EmptyStateCard(message string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	bg := appstyles.Active.BackgroundRecessed

	card := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(appstyles.Active.BorderCard).
		BorderBackground(bg).
		Background(bg).
		Render(lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Background(bg).Render(message))

	// The card's own joins leave unstyled padding on short lines; seal
	// against its own background before anything composes it.
	return appstyles.FillBackground(bg, card)
}
