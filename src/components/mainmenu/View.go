package mainmenu

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/constants"
)

const versionGutter = 4

// View renders the header bar: version dimmed on the left, wordmark accented
// on the right. No bottom border — the tier-2 background against the tier-3
// panels below provides the section break, exactly like stack-stitcher.
func (m Model) View() tea.View {
	if m.terminalWidth <= 0 {
		return tea.NewView("")
	}

	barStyle := lipgloss.NewStyle().
		Background(appstyles.Active.BackgroundContent).
		Width(m.terminalWidth)

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
	mode := ""
	if m.treeView != "" {
		mode = versionStyle.Render(m.treeView + " . ")
	}

	// Drop the version when it would crowd the wordmark.
	if lipgloss.Width(wordmark)+lipgloss.Width(mode)+lipgloss.Width(version)+versionGutter > m.terminalWidth {
		mode = ""
	}
	if lipgloss.Width(wordmark)+lipgloss.Width(version)+versionGutter > m.terminalWidth {
		version = ""
	}

	gapWidth := m.terminalWidth - lipgloss.Width(wordmark) - lipgloss.Width(mode) - lipgloss.Width(version)
	if gapWidth < 0 {
		gapWidth = 0
	}

	gap := lipgloss.NewStyle().
		Background(appstyles.Active.BackgroundContent).
		Width(gapWidth).
		Render("")

	row := lipgloss.JoinHorizontal(lipgloss.Left, gap, mode, version, wordmark)
	return tea.NewView(barStyle.Render(row))
}
