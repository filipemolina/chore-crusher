package helpoverlay

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/components/chrome"
	"github.com/filipemolina/farol/src/keys"
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

	lines := []string{titleStyle.Render(scope.Title), body}
	if scope.Note != "" {
		// Behaviour a key/description pair cannot carry — see keys.Scope.Note.
		lines = append(lines, dimStyle.Width(width).Render(scope.Note))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// contentLines renders every scope and flattens the result to one list of
// already-wrapped lines, with a blank line between scopes. Flattening before
// windowing is what lets a scope taller than the window still be read: the
// window cuts between lines, not between scopes.
func (m Model) contentLines() []string {
	width := m.contentWidth()

	var lines []string
	for i, scope := range m.catalog {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, strings.Split(renderScope(scope, width), "\n")...)
	}
	return lines
}

// maxOffset is the furthest the content can scroll: enough to bring the last
// line into view, never past it.
func (m Model) maxOffset() int {
	return max(0, len(m.contentLines())-m.contentRows())
}

// assemble wraps a body of content lines in the overlay's chrome: the title
// above, then the overflow counts, the dimming legend and the hint line below.
// contentRows measures itself against this, so the two can never disagree
// about what the chrome costs.
func (m Model) assemble(body, overflow string) string {
	windowed := overflow != ""
	width := m.contentWidth()
	dim := lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Width(width)

	sections := []string{
		chrome.ModalTitle("Keyboard shortcuts"),
		body,
	}

	if windowed {
		// Hidden-line counts, in the same "N above / N below" shape the task
		// tree's pinned section headers use for the same problem
		// (docs/DESIGN.md §12). Without it a windowed catalog looks complete.
		sections = append(sections, dim.Render(overflow))
	}

	// The overlay lists every key in the app on every screen, so it has to say
	// what the dimming means — otherwise a dimmed row reads as "removed" or
	// "broken" rather than "not on this screen".
	sections = append(sections, dim.Render("Dimmed keys exist but are not available on the screen you opened this from."))

	// The overlay's own keys, built from the same bindings as everything
	// else: it owns the keyboard while open, and an overlay advertises its
	// own keys because there is no footer beneath it. Scrolling is only
	// advertised when there is something to scroll to.
	var hints []chrome.KeyHint
	if windowed {
		hints = append(hints, chrome.HintAs(keys.Overlay.Navigation, "scroll"))
	}
	hints = append(hints,
		chrome.HintAs(keys.Global.Help, "close"),
		chrome.HintAs(keys.Overlay.Cancel, "close"),
	)
	sections = append(sections, chrome.RenderKeyHints(hints, appstyles.Active.TextMuted))

	return chrome.ModalSurface(appstyles.Active.ModalBg, strings.Join(sections, "\n\n"))
}

// overflowLabel reports how many content lines are hidden above and below the
// current window.
func (m Model) overflowLabel() string {
	lines := m.contentLines()
	offset := min(m.offset, m.maxOffset())
	end := min(offset+m.contentRows(), len(lines))

	var parts []string
	if offset > 0 {
		parts = append(parts, fmt.Sprintf("%d above", offset))
	}
	if below := len(lines) - end; below > 0 {
		parts = append(parts, fmt.Sprintf("%d below", below))
	}
	return strings.Join(parts, " · ")
}

func (m Model) View() tea.View {
	lines := m.contentLines()
	windowed := m.maxOffset() > 0
	offset := min(m.offset, m.maxOffset())
	end := min(offset+m.contentRows(), len(lines))

	overflow := ""
	if windowed {
		overflow = m.overflowLabel()
	}
	return tea.NewView(m.assemble(strings.Join(lines[offset:end], "\n"), overflow))
}
