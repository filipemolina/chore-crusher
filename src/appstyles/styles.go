package appstyles

import "charm.land/lipgloss/v2"

// Computed styles built from the active theme. A raw color is a Theme field
// (see Theme.go) and is read directly - appstyles.Active.TextPrimary, say.
// Anything that needs actual style logic - more than reading one field -
// lives here as a function instead of a package-level var, so it re-reads
// Active on every call rather than freezing whichever theme was active at
// package init.

// NormalTitle is THE accent title chip: bold ink derived from the theme
// accent on an accent background, with one space of padding on each side.
// Every title - the lists panel and task tree frame titles, and every modal
// heading - renders through this one style, so a theme or style change here
// lands on all of them at once. See chrome.PanelFrame.
//
// The style deliberately carries no margin: the left gutter is the container's
// job. A panel frame adds MarginLeft(2) to match the lists panel's gutter.
func NormalTitle() lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(InkOn(Active.Accent)).
		Background(Active.Accent).
		Padding(0, 1)
}
