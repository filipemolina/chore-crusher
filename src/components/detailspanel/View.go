package detailspanel

import (
	"fmt"
	"image/color"
	"strconv"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
)

// View renders the Details panel inside the shared chrome.PanelFrame. Focus is
// shown only by the frame's background tier (docs/DESIGN.md §12); the textarea
// colors are applied here, at render time, from the current theme and focus so
// no background from a previous theme is retained.
func (m *Model) View() tea.View {
	bg := chrome.PanelBg(m.focused)
	m.applyTextareaStyles(bg)

	bodyWidth := chrome.PanelBodyWidth(m.body.DetailsWidth)
	label := lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted)
	title := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextPrimary).
		Render(chrome.Truncate(m.title, bodyWidth))

	body := lipgloss.JoinVertical(lipgloss.Left,
		label.Render("Title"),
		title,
		"",
		label.Render("Notes"),
		m.notes.View(),
		"",
		label.Render("Progress"),
		m.renderProgressZone(),
		m.renderErrorLine(),
		"",
		m.renderFooter(),
	)

	return tea.NewView(chrome.PanelFrame("Details", m.focused, m.body.DetailsWidth, m.body.Height, body))
}

// applyTextareaStyles seals the textarea onto the panel background for the
// current focus tier. Rebuilt every render so a theme switch while the panel
// is closed cannot leave a stale background behind.
func (m *Model) applyTextareaStyles(bg color.Color) {
	styles := textarea.DefaultDarkStyles()
	base := lipgloss.NewStyle().Background(bg)
	text := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Background(bg)
	styles.Focused.Base = base
	styles.Blurred.Base = base
	styles.Focused.Text = text
	styles.Blurred.Text = text
	styles.Focused.CursorLine = lipgloss.NewStyle().Background(bg)
	styles.Blurred.CursorLine = lipgloss.NewStyle().Background(bg)
	m.notes.SetStyles(styles)
}

func (m *Model) renderProgressZone() string {
	modeName := string(m.progressKind)
	modeStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)
	if m.focus == focusProgress {
		modeStyle = modeStyle.Foreground(appstyles.Active.Accent)
	}

	var modeDisplay string
	if m.focus == focusProgress {
		modeDisplay = modeStyle.Render(modeName) + " " +
			lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted).Render("←/→ cycle")
	} else {
		modeDisplay = modeStyle.Render(modeName)
	}

	var valueDisplay string
	switch m.progressKind {
	case apptypes.ProgressSimple:
		valueDisplay = ""
	case apptypes.ProgressSubtasks:
		if m.displayAsSimple {
			valueDisplay = "(no children)"
		} else {
			valueDisplay = fmt.Sprintf("(%d%%)", m.derivedPct)
		}
	case apptypes.ProgressPercentage:
		if m.focus == focusProgress && m.percentInput != "" {
			valueDisplay = fmt.Sprintf("(%s%%)", m.percentInput)
		} else if m.percentInput != "" {
			val, _ := strconv.Atoi(m.percentInput)
			valueDisplay = fmt.Sprintf("(%d%%)", val)
		} else {
			valueDisplay = "(—)"
		}
	}

	if valueDisplay != "" {
		valueDisplay = " " + lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted).Render(valueDisplay)
	}

	return modeDisplay + valueDisplay
}

func (m *Model) renderErrorLine() string {
	if m.errMsg == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(appstyles.Active.StatusOverdue).Render(m.errMsg)
}

// renderFooter advertises exactly the keys that work inside the panel. It
// swaps to the discard prompt while a dirty Esc is pending.
func (m *Model) renderFooter() string {
	muted := lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted)
	if m.confirmingDiscard {
		return muted.Render("Discard changes? (y/n)")
	}

	var hint string
	if m.focus == focusNotes {
		hint = "tab to progress  ctrl+s save  esc cancel"
	} else {
		hint = "tab to notes  ←/→ cycle mode  ctrl+s save  esc cancel"
	}
	return muted.Render(hint)
}
