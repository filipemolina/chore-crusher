package searchpicker

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-completer/src/appstyles"
	"github.com/filipemolina/chore-completer/src/components/chrome"
	"github.com/filipemolina/chore-completer/src/keys"
)

func (m Model) View() tea.View {
	body := lipgloss.JoinVertical(lipgloss.Left,
		chrome.ModalTitle("Search all lists"),
		m.input.View(),
		"",
		m.renderResults(),
		m.renderError(),
		"",
		m.renderHints(),
	)

	bg := appstyles.Active.ModalBg
	return tea.NewView(chrome.ModalSurface(bg, body))
}

// renderResults renders the visible result rows, each as "<list> › <title>",
// with the cursor highlighted. Only m.visible rows render; a longer list is
// windowed around the cursor so it never leaves the screen.
func (m Model) renderResults() string {
	if len(m.results) == 0 {
		dim := lipgloss.NewStyle().Foreground(appstyles.Active.TextDim)
		if m.query == "" {
			return dim.Render("Type to search across every list")
		}
		return dim.Render("No matches")
	}

	start := 0
	shown := m.results
	if len(m.results) > m.visible {
		start = max(0, min(m.cursor-(m.visible/2), len(m.results)-m.visible))
		shown = m.results[start : start+m.visible]
	}

	lines := make([]string, 0, len(shown))
	for i, r := range shown {
		selected := i+start == m.cursor
		lines = append(lines, renderResult(r, selected))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderResult renders one candidate row. The selected row is lifted onto the
// modal background so it reads as an opaque highlight (same mechanism as the
// tree's selected row).
func renderResult(r Result, selected bool) string {
	label := r.ListName + " › " + r.Title
	if selected {
		return lipgloss.NewStyle().
			Background(appstyles.Active.ModalBg).
			Foreground(appstyles.Active.TextPrimary).
			Render(label)
	}
	return lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Render(label)
}

func (m Model) renderError() string {
	if m.errMsg == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(appstyles.Active.StatusOverdue).Render(m.errMsg)
}

func (m Model) renderHints() string {
	return chrome.RenderKeyHints([]chrome.KeyHint{
		chrome.HintFor(keys.Overlay.Submit),
		{Key: "↑/↓", Desc: "navigate"},
		chrome.HintFor(keys.Overlay.Cancel),
	}, appstyles.Active.TextMuted)
}