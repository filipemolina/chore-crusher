package helpoverlay

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/keys"
)

// contentWidth is the column the overlay's hints wrap to: the terminal minus
// the modal chrome and a margin, capped.
func (m Model) contentWidth() int {
	return max(24, min(helpOverlayMaxWidth, m.termWidth-16))
}

// renderScope renders one scope as a title line over its hint runs, wrapped
// to width. Unavailable rows are dimmed whole; available rows get the
// footer's treatment (key bold, description muted).
func renderScope(scope keys.Scope, width int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(appstyles.Active.Accent)
	dimStyle := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextDim)
	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(appstyles.Active.TextPrimary)
	descStyle := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextMuted)

	runs := make([]string, 0, len(scope.Entries)*2)
	for i, entry := range scope.Entries {
		if i > 0 {
			runs = append(runs, dimStyle.Render(" · "))
		}

		help := entry.Binding.Help()
		if entry.Available {
			runs = append(runs, keyStyle.Render(help.Key)+descStyle.Render(" "+help.Desc))
		} else {
			runs = append(runs, dimStyle.Render(help.Key+" "+help.Desc))
		}
	}

	body := lipgloss.NewStyle().
		Width(width).
		Render(lipgloss.JoinHorizontal(lipgloss.Left, runs...))

	return lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render(scope.Title), body)
}

func (m Model) View() tea.View {
	width := m.contentWidth()

	sections := []string{
		chrome.ModalTitle("Keyboard shortcuts"),
	}

	for _, scope := range m.catalog {
		sections = append(sections, renderScope(scope, width))
	}

	// The overlay's own keys, built from the same bindings as everything
	// else: it owns the keyboard while open, and an overlay advertises its
	// own keys because there is no footer beneath it.
	hint := chrome.RenderKeyHints([]chrome.KeyHint{
		chrome.HintAs(keys.Global.Help, "close"),
		chrome.HintAs(keys.Overlay.Cancel, "close"),
	}, appstyles.Active.TextMuted)
	sections = append(sections, hint)

	return tea.NewView(chrome.ModalSurface(
		appstyles.Active.ModalBg,
		strings.Join(sections, "\n\n"),
	))
}
