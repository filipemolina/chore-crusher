package chrome

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
)

// emptyStateFrame is the width and height the card's own chrome costs on top
// of its message: Padding(1, 2) plus the one-cell rounded border on each side.
const (
	emptyStateFrameW = 2*2 + 2 // padding left+right, border left+right
	emptyStateFrameH = 1*2 + 2 // padding top+bottom, border top+bottom
)

// EmptyStateCard renders the one recessed empty-state surface every empty
// zone uses (docs/DESIGN.md §12 "Empty states: one recessed-card pattern"):
// a box on the BackgroundRecessed tier, rimmed with BorderCard (not
// BorderDefault - a border has to contrast with the surface it wraps, and
// BorderDefault moves toward BackgroundRecessed rather than away from it),
// padded exactly like PanelFrame (1, 2), TextDim guidance text, left-aligned.
// Do not center empty-state text and do not give it its own bespoke padding -
// reusing the exact PanelFrame numbers is what makes it read as "this zone,
// currently empty" rather than a different kind of surface that happens to be
// nearby.
//
// The CARD is sized to its message and centered in the width x height space
// the caller gives it; only the text inside it stays left-aligned. It used to
// stretch to fill that space, which put a two-line message inside a 28-row box
// and read as a large broken panel rather than a note. The returned block
// still occupies the full space, with everything around the card painted in
// surroundBg - the caller's own tier, since the space around the card belongs
// to the panel, not to the recessed card.
//
// The card always uses the recessed tier regardless of the caller's own
// focus state, so an empty state stays visually sunken even while its
// parent panel is elevated by focus - the inset is the point.
func EmptyStateCard(message string, width, height int, surroundBg color.Color) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	bg := appstyles.Active.BackgroundRecessed

	// The message decides the card's size, so it is truncated to what the
	// space can actually hold before anything is measured from it.
	lines := strings.Split(message, "\n")
	for i, line := range lines {
		lines[i] = Truncate(line, max(1, width-emptyStateFrameW))
	}
	message = strings.Join(lines, "\n")

	card := lipgloss.NewStyle().
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(appstyles.Active.BorderCard).
		BorderBackground(bg).
		Background(bg).
		// A box shorter than the card's own chrome has no honest layout; clamp
		// rather than overflow the panel that asked for it.
		MaxHeight(height).
		Render(lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Background(bg).Render(message))

	// The card's own joins leave unstyled padding on short lines; seal
	// against its own background before it is placed.
	card = appstyles.FillBackground(bg, card)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, card,
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(surroundBg)))
}
