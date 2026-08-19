package mainmenu

import (
	"image/color"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/constants"
)

const versionGutter = 4

// tabLabel renders one page tab as "<digit> <label>" — the digit in the
// accent color advertises the key that jumps to it directly on the tab
// itself, the same look ../cais's mainmenu uses (docs/DESIGN.md §5). Farol
// has only two tabs, so unlike cais this is two key.Bindings and a bool, not
// a page registry — only the rendering is borrowed.
func tabLabel(digit, label string, fg color.Color, bold bool) string {
	d := lipgloss.NewStyle().
		Foreground(appstyles.Active.Accent).
		Background(appstyles.Active.BackgroundContent).
		Bold(true).
		Render(digit)

	l := lipgloss.NewStyle().
		Foreground(fg).
		Background(appstyles.Active.BackgroundContent).
		Bold(bold).
		Render(" " + label)

	return d + l
}

// viewModeIcon is the single glyph prefixing the view-mode indicator,
// matching ../pulso's mainmenu.sortIcon glyph (pulso/src/components/mainmenu/View.go).
const viewModeIcon = "⇅"

// View renders the header bar: two page tabs on the left (1 Active, 2
// Archived — keys.Global.PageActive/PageArchived), the task tree's view mode
// and version low-emphasis next to the wordmark on the right. No bottom
// border — the tier-2 background against the tier-3 panels below provides
// the section break, exactly like stack-stitcher.
func (m Model) View() tea.View {
	if m.terminalWidth <= 0 {
		return tea.NewView("")
	}

	barStyle := lipgloss.NewStyle().
		Background(appstyles.Active.BackgroundContent).
		Width(m.terminalWidth)

	// Cell styles carry only the spacing; tabLabel handles color/weight so the
	// digit is never competing with a foreground set on the enclosing style.
	cellStyle := lipgloss.NewStyle().
		Background(appstyles.Active.BackgroundContent).
		Padding(0, 2)
	// The selected cell has less left padding to compensate for the external ▌.
	activeCellStyle := cellStyle.Padding(0, 2, 0, 1)

	accentBar := lipgloss.NewStyle().
		Foreground(appstyles.Active.Accent).
		Background(appstyles.Active.BackgroundContent).
		Render("▌")

	tab := func(digit, label string, selected bool) string {
		if selected {
			cell := activeCellStyle.Render(tabLabel(digit, label, appstyles.Active.TextPrimary, true))
			return lipgloss.JoinHorizontal(lipgloss.Left, accentBar, cell)
		}
		return cellStyle.Render(tabLabel(digit, label, appstyles.Active.TextDim, false))
	}

	tabs := lipgloss.JoinHorizontal(lipgloss.Left,
		tab("1", "Active", !m.archiveOpen),
		tab("2", "Archived", m.archiveOpen),
	)

	wordmarkStyle := lipgloss.NewStyle().
		Foreground(appstyles.Active.Accent).
		Background(appstyles.Active.BackgroundContent).
		Bold(true).
		Padding(0, 2)

	versionStyle := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextDim).
		Background(appstyles.Active.BackgroundContent)

	wordmark := wordmarkStyle.Render(constants.WORDMARK)
	version := versionStyle.Render(constants.Version())

	// The view-mode indicator (low-emphasis, like pulso's "all . pulso-dusk .
	// v0.3.0") sits left of the version, separated the same way. It sheds
	// before the version does: the mode is the tree's own transient state,
	// the version is the app's identity, and a narrow terminal gives up the
	// less load-bearing one first.
	//
	// It is blank while the Archive page is open: Pending/Complete/All
	// describes the task tree, which the Archive page has replaced, and
	// which page is on screen is now carried by the tabs themselves rather
	// than a text label taking over this slot.
	mode := ""
	if !m.archiveOpen && m.treeView != "" {
		mode = versionStyle.Render(viewModeIcon + " " + m.treeView + " . ")
	}

	// Drop the version, then the mode, when they would crowd the wordmark.
	// The tabs are never dropped — like cais's, they are the header's
	// primary navigation, not decoration.
	if lipgloss.Width(tabs)+lipgloss.Width(wordmark)+lipgloss.Width(mode)+lipgloss.Width(version)+versionGutter > m.terminalWidth {
		mode = ""
	}
	if lipgloss.Width(tabs)+lipgloss.Width(wordmark)+lipgloss.Width(version)+versionGutter > m.terminalWidth {
		version = ""
	}

	gapWidth := m.terminalWidth - lipgloss.Width(tabs) - lipgloss.Width(mode) - lipgloss.Width(version) - lipgloss.Width(wordmark)
	if gapWidth < 0 {
		gapWidth = 0
	}

	gap := lipgloss.NewStyle().
		Background(appstyles.Active.BackgroundContent).
		Width(gapWidth).
		Render("")

	row := lipgloss.JoinHorizontal(lipgloss.Left, tabs, gap, mode, version, wordmark)
	return tea.NewView(barStyle.Render(row))
}
