package keybindingbar

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/components/chrome"
	"github.com/filipemolina/farol/src/keys"
)

// View renders the hint bar. Context-sensitive keys sit on the left; the
// always-available global keys sit on the right. Whole hints are dropped from
// the left when the bar would not otherwise fit, so the footer never wraps.
func (m Model) View() tea.View {
	if m.terminalWidth <= 0 {
		return tea.NewView("")
	}

	// While Details owns the keyboard the footer goes blank entirely. Details
	// renders its own hint line inside the modal, next to the controls it
	// describes; a second copy down here said the same things in different
	// words ("esc close" vs "esc cancel") and listed a different subset, so
	// the two contradicted each other. The bar still paints its full-width
	// background so the layout height never moves.
	if m.ctx.DetailsPanelVisible {
		return tea.NewView(appstyles.FillBackground(appstyles.Active.BackgroundContent,
			barStyle(m.terminalWidth).Render("")))
	}

	left := keys.Active(m.ctx)
	right := globalsNotInLeft(left, m.ctx)

	leftHints := chrome.RenderKeyHints(hintsFrom(left), appstyles.Active.TextDim)
	rightHints := chrome.RenderKeyHints(hintsFrom(right), appstyles.Active.TextDim)

	// Add sort mode indicator to the right side
	sortIndicator := ""
	if m.sortMode != 0 { // SortManual is 0
		sortIndicator = lipgloss.NewStyle().
			Foreground(appstyles.Active.TextMuted).
			Background(appstyles.Active.BackgroundContent).
			Render(" ⇅ " + m.sortMode.String())
	}

	const padding = 2 // one column each side
	avail := max(0, m.terminalWidth-padding)
	minSep := 1

	for len(left) > 1 && lipgloss.Width(leftHints)+lipgloss.Width(rightHints)+lipgloss.Width(sortIndicator)+minSep > avail {
		left = left[:len(left)-1]
		leftHints = chrome.RenderKeyHints(hintsFrom(left), appstyles.Active.TextDim)
	}

	// If the right group still will not fit, truncate the separator so the
	// bar can clip rather than wrap. Whole-hint shedding already ran above.
	sepWidth := max(1, avail-lipgloss.Width(leftHints)-lipgloss.Width(rightHints)-lipgloss.Width(sortIndicator))
	sep := lipgloss.NewStyle().
		Background(appstyles.Active.BackgroundContent).
		Width(sepWidth).
		Render("")

	line := lipgloss.JoinHorizontal(lipgloss.Left, leftHints, sep, rightHints, sortIndicator)
	return tea.NewView(appstyles.FillBackground(appstyles.Active.BackgroundContent, barStyle(m.terminalWidth).Render(line)))
}

// barStyle is the footer's one-line full-width box, shared by the rendered bar
// and the blank one Details shows in its place.
func barStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Background(appstyles.Active.BackgroundContent).
		Width(width).
		MaxWidth(width).
		Height(1).
		MaxHeight(1).
		Padding(0, 1)
}

// globalsNotInLeft returns the always-live keys for ctx, omitting anything
// already represented in the left group so the footer does not advertise the
// same key twice. Context matters: while the create input is live,
// keys.GlobalsFor drops tab/shift+tab and ?, which do nothing there, and with
// no side panel open it drops tab/shift+tab, which have nothing to cycle.
func globalsNotInLeft(left []key.Binding, ctx keys.Context) []key.Binding {
	globals := keys.GlobalsFor(ctx)
	out := make([]key.Binding, 0, len(globals))
	for _, g := range globals {
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
