package keybindingbar

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/keys"
)

// View renders the hint bar. Context-sensitive keys sit on the left; the
// always-available global keys sit on the right. Whole hints are dropped from
// the left when the bar would not otherwise fit, so the footer never wraps.
func (m Model) View() tea.View {
	if m.terminalWidth <= 0 {
		return tea.NewView("")
	}

	left := keys.Active(m.ctx)
	right := globalsNotInLeft(left)

	leftHints := chrome.RenderKeyHints(hintsFrom(left), appstyles.Active.TextDim)
	rightHints := chrome.RenderKeyHints(hintsFrom(right), appstyles.Active.TextDim)

	const padding = 2 // one column each side
	avail := max(0, m.terminalWidth-padding)
	minSep := 1

	for len(left) > 1 && lipgloss.Width(leftHints)+lipgloss.Width(rightHints)+minSep > avail {
		left = left[:len(left)-1]
		leftHints = chrome.RenderKeyHints(hintsFrom(left), appstyles.Active.TextDim)
	}

	// If the right group still will not fit, truncate the separator so the
	// bar can clip rather than wrap. Whole-hint shedding already ran above.
	sepWidth := max(1, avail-lipgloss.Width(leftHints)-lipgloss.Width(rightHints))
	sep := lipgloss.NewStyle().
		Background(appstyles.Active.BackgroundContent).
		Width(sepWidth).
		Render("")

	barStyle := lipgloss.NewStyle().
		Background(appstyles.Active.BackgroundContent).
		Width(m.terminalWidth).
		MaxWidth(m.terminalWidth).
		Height(1).
		MaxHeight(1).
		Padding(0, 1)

	line := lipgloss.JoinHorizontal(lipgloss.Left, leftHints, sep, rightHints)
	return tea.NewView(appstyles.FillBackground(appstyles.Active.BackgroundContent, barStyle.Render(line)))
}

// globalsNotInLeft returns the always-live keys, omitting anything already
// represented in the left group so the footer does not advertise the same key
// twice.
func globalsNotInLeft(left []key.Binding) []key.Binding {
	out := make([]key.Binding, 0, len(keys.Globals()))
	for _, g := range keys.Globals() {
		if !bindingIn(left, g) {
			out = append(out, g)
		}
	}
	return out
}

// bindingIn reports whether haystack already contains a binding with the same
// keystrokes and help text.
func bindingIn(haystack []key.Binding, needle key.Binding) bool {
	nKeys := needle.Keys()
	nHelp := needle.Help()
	for _, b := range haystack {
		bKeys := b.Keys()
		if len(bKeys) != len(nKeys) {
			continue
		}
		match := true
		for i := range bKeys {
			if bKeys[i] != nKeys[i] {
				match = false
				break
			}
		}
		bHelp := b.Help()
		if match && bHelp.Key == nHelp.Key && bHelp.Desc == nHelp.Desc {
			return true
		}
	}
	return false
}

func hintsFrom(bindings []key.Binding) []chrome.KeyHint {
	hints := make([]chrome.KeyHint, 0, len(bindings))
	for _, b := range bindings {
		hints = append(hints, chrome.HintFor(b))
	}
	return hints
}
