package detailspanel

import (
	"fmt"
	"image/color"
	"strconv"
	"time"

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
		m.renderComments(),
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

// renderComments renders the comment thread and the compose control below the
// progress zone. Each comment is a one-line card (author, note, timestamp) —
// comments are short status/handoff notes per the spec, so a single line
// per card is sufficient (docs/plan/task-comments.md §6, Commit 5). The
// thread is oldest-first, mirroring store.ListComments' ORDER BY created_at
// ASC. An empty thread renders only the compose input, so the section is
// always available to write the first comment.
func (m *Model) renderComments() string {
	var parts []string

	if len(m.comments) > 0 {
		label := lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted)
		parts = append(parts, label.Render("Comments"))
		for _, c := range m.comments {
			parts = append(parts, m.renderCommentCard(c))
		}
		parts = append(parts, "") // blank between the last card and the compose
	}

	parts = append(parts, m.renderCommentCompose())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderCommentCard renders one comment as a single-line card: the author and
// timestamp in TextDim, then the note in TextPrimary — all within one line so
// the thread stays compact. The note is truncated to the panel body width by
// chrome.Truncate, and the prefix is kept short so a long note still leads
// with readable content (docs/DESIGN.md §12 truncation contract).
func (m *Model) renderCommentCard(c apptypes.Comment) string {
	bodyWidth := chrome.PanelBodyWidth(m.body.DetailsWidth)
	muted := lipgloss.NewStyle().Foreground(appstyles.Active.TextDim)
	primary := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)

	ts := time.Unix(c.CreatedAt, 0).Format("2006-01-02 15:04")
	prefix := fmt.Sprintf("%s · %s · ", c.Author, ts)
	note := chrome.Truncate(c.Note, max(0, bodyWidth-lipgloss.Width(prefix)))
	return muted.Render(prefix) + primary.Render(note)
}

// renderCommentCompose renders the single-line compose input with its label.
// The input is a textinput (not the notes textarea) since comments are short
// one-liners; it is focused only while focus == focusComments.
func (m *Model) renderCommentCompose() string {
	label := lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted)
	bodyWidth := chrome.PanelBodyWidth(m.body.DetailsWidth)
	// textinput doesn't set its own width from the style, so budget the
	// prefix glyph against the panel body width.
	m.commentInput.SetWidth(max(0, bodyWidth-len([]rune(commentPrompt))))
	input := m.commentInput.View()
	return label.Render(commentPrompt) + " " + input
}

// commentPrompt is the single-column label that prefixes the comment compose
// input in the Details panel footer area.
const commentPrompt = "Comment:"

// renderFooter advertises exactly the keys that work inside the panel. It
// swaps to the discard prompt while a dirty Esc is pending.
func (m *Model) renderFooter() string {
	muted := lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted)
	if m.confirmingDiscard {
		return muted.Render("Discard changes? (y/n)")
	}

	var hint string
	switch m.focus {
	case focusNotes:
		hint = "tab to progress  ctrl+s save  esc cancel"
	case focusComments:
		hint = "tab to notes  ctrl+enter post  esc cancel"
	default:
		hint = "tab to notes  ←/→ cycle mode  ctrl+s save  esc cancel"
	}
	return muted.Render(hint)
}
