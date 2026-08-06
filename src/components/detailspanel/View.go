package detailspanel

import (
	"fmt"
	"image/color"
	"strconv"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
)

// View renders the Task details modal: the shared chrome.ModalSurface (accent
// border, sealed background) wrapping a fixed-width content column. The modal is
// as wide as its outer box (about 90% of the terminal) and as tall as its
// content, capped at that box — so it takes most of the screen without leaving a
// blank gap when a task has little to show (docs/DESIGN.md §5).
func (m *Model) View() tea.View {
	if m.width <= 0 || m.height <= 0 {
		// Before the first SetDetailsLayoutMsg there is no box to render into.
		return tea.NewView("")
	}

	bg := appstyles.Active.ModalBg
	m.applyTextareaStyles(bg)
	m.applyInputStyles(bg)

	body := lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(),
		m.fieldLabel("Title", focusTitle),
		m.titleInput.View(),
		"",
		m.fieldLabel("Notes", focusNotes),
		m.notes.View(),
		"",
		m.fieldLabel("Progress", focusProgress),
		m.renderProgressZone(),
		m.renderStatusLine(),
		"",
		m.renderComments(),
		"",
		m.renderFooter(),
	)

	innerW := m.innerWidth()
	// Fix the column to the inner width so every row seals to the same right
	// edge; MaxWidth clips a line too long for a narrow modal (e.g. the footer
	// hints or the flush-right task id) rather than letting it push the surface
	// wider, and MaxHeight clips (never pads) the height so a tall thread loses
	// its overflow rather than growing the modal past the screen.
	body = lipgloss.NewStyle().
		Width(innerW).
		MaxWidth(innerW).
		MaxHeight(m.innerHeight()).
		Render(body)

	return tea.NewView(chrome.ModalSurface(bg, body))
}

// fieldLabel renders a section label, bolded onto the primary ink while its
// field holds keyboard focus so the focused zone is always obvious. The compose
// card owns focus while composing, so no zone label is bolded then.
func (m *Model) fieldLabel(text string, focus int) string {
	if m.focus == focus && !m.composing {
		return lipgloss.NewStyle().
			Bold(true).
			Foreground(appstyles.Active.TextPrimary).
			Render(text)
	}
	return lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted).Render(text)
}

// renderHeader renders the modal's heading row: the "Task details" accent
// chip on the left, and the task id pinned flush-right on the same line so it
// reads at a glance. The ctrl+y copy shortcut, advertised in the footer, is
// the same key from every zone.
func (m *Model) renderHeader() string {
	left := chrome.ModalTitle("Task details")
	right := m.renderTaskID()
	return modalTitleRow(m.innerWidth(), left, right)
}

// renderTaskID renders the task id flush-right in the header, dimmed like the
// rest of the header chrome. The ctrl+y copy shortcut (in the footer) lets the
// user grab the id to the clipboard from any zone.
func (m *Model) renderTaskID() string {
	return lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Render("ID " + m.taskID)
}

// modalTitleRow lays left on the line's left edge and right on its right edge
// within a width-w box, mirroring chrome.titleRow for the modal header. When
// right is too wide to fit alongside left it is truncated so the heading never
// pushes the surface wider than its box (docs/DESIGN.md §5).
func modalTitleRow(width int, left, right string) string {
	if right == "" {
		return left
	}
	leftW := lipgloss.Width(left)
	budget := width - leftW
	if budget <= 0 {
		return left
	}
	trunc := right
	if w := lipgloss.Width(right); w > budget {
		trunc = chrome.Truncate(right, budget)
	}
	gap := width - leftW - lipgloss.Width(trunc)
	if gap < 1 {
		gap = 1
	}
	spacer := lipgloss.NewStyle().Width(gap).Render("")
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, trunc)
}

// applyTextareaStyles seals the textarea onto the modal background. Rebuilt every
// render so a theme switch while the modal is closed cannot leave a stale
// background behind.
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

// applyInputStyles seals the title and compose textinputs onto the modal
// background so their text rows do not bleed the terminal default behind them,
// matching the notes textarea. Rebuilt every render for the same theme-switch
// reason as applyTextareaStyles.
func (m *Model) applyInputStyles(bg color.Color) {
	seal := func(in *textinput.Model) {
		st := in.Styles()
		text := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Background(bg)
		placeholder := lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Background(bg)
		st.Focused.Text = text
		st.Blurred.Text = text
		st.Focused.Placeholder = placeholder
		st.Blurred.Placeholder = placeholder
		in.SetStyles(st)
	}
	seal(&m.titleInput)
	seal(&m.commentInput)
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

// renderStatusLine is the single reserved row below the progress zone: an error
// (StatusOverdue) when one is pending, else the transient copy-confirmation
// flash (Accent), else blank. Reserving the row keeps the layout height stable
// whether or not there is anything to say.
func (m *Model) renderStatusLine() string {
	if m.errMsg != "" {
		return lipgloss.NewStyle().Foreground(appstyles.Active.StatusOverdue).Render(m.errMsg)
	}
	if m.flash != "" {
		return lipgloss.NewStyle().Foreground(appstyles.Active.Accent).Render(m.flash)
	}
	return ""
}

// renderComments renders the "Comments" label and the thread as selectable
// cards, windowed to the height the layout leaves after the notes textarea so
// the highlighted card (and the compose card, while composing) is always on
// screen. An empty thread renders a muted placeholder so the section still names
// itself.
func (m *Model) renderComments() string {
	label := m.fieldLabel("Comments", focusComments)
	innerW := m.innerWidth()

	// Rows the cards may use: whatever the notes textarea did not take.
	avail := max(0, m.flexRows()-m.notesRows())

	// The compose card, when open, is rendered first and always kept on screen;
	// the real comment cards window into the rows it leaves.
	var composeCard string
	composeRows := 0
	if m.composing {
		composeCard = m.renderComposeCard(innerW)
		composeRows = lipgloss.Height(composeCard)
	}

	rows := []string{label}

	if len(m.comments) == 0 {
		if composeCard != "" {
			// Gap between the label and the compose card.
			rows = append(rows, "", composeCard)
		} else {
			rows = append(rows, "", lipgloss.NewStyle().
				Foreground(appstyles.Active.TextDim).
				Render("No comments yet."))
		}
		return lipgloss.JoinVertical(lipgloss.Left, rows...)
	}

	// Render every card so the window can measure real (wrapped) heights, then
	// pick a contiguous window that contains the anchor and fits the budget.
	// Each card's measured height includes one commentCardGap so the windowing
	// budget reserves the gap that separates adjacent cards; the gap between the
	// label and the first card, and between the last card and the compose card,
	// is a single-sided margin counted separately below.
	cards := make([]string, len(m.comments))
	heights := make([]int, len(m.comments))
	for i, c := range m.comments {
		cards[i] = m.renderCommentCard(c, i == m.selectedComment && !m.composing, innerW)
		// Add the inter-card gap so the windowing budget already reserves it.
		heights[i] = lipgloss.Height(cards[i]) + commentCardGap
	}

	// While composing, anchor on the newest comment so the freshest context sits
	// next to the compose card; otherwise anchor on the selection.
	anchor := m.selectedComment
	if m.composing {
		anchor = len(m.comments) - 1
	}
	// Budget accounts for the single-sided label margin and the compose margin
	// (each one commentCardGap) in addition to the compose card itself.
	composeBudget := avail - composeRows
	if composeRows > 0 {
		composeBudget -= commentCardGap
	}
	start, end := commentWindow(heights, anchor, max(0, composeBudget))
	// Flatten the window: a blank line before the first card (label margin),
	// cards separated by one blank line (commentCardGap), with the gap only
	// between adjacent cards, never trailing, then a blank line before the
	// compose card (compose margin) when present.
	rows = append(rows, "")
	for i := start; i < end; i++ {
		rows = append(rows, cards[i])
		if i < end-1 {
			rows = append(rows, "")
		}
	}
	if composeCard != "" {
		rows = append(rows, "", composeCard)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// commentWindow returns the [start,end) slice of cards to show: a contiguous
// run that always contains anchor and whose measured heights fit budget rows,
// grown downward first (toward newer comments) then upward. The anchor card is
// always included even when it alone exceeds the budget — MaxHeight then clips
// its overflow rather than dropping it.
func commentWindow(heights []int, anchor, budget int) (int, int) {
	n := len(heights)
	if n == 0 {
		return 0, 0
	}
	if anchor < 0 {
		anchor = 0
	}
	if anchor > n-1 {
		anchor = n - 1
	}
	start, end := anchor, anchor+1
	used := heights[anchor]
	for {
		grew := false
		if end < n && used+heights[end] <= budget {
			used += heights[end]
			end++
			grew = true
		}
		if start > 0 && used+heights[start-1] <= budget {
			used += heights[start-1]
			start--
			grew = true
		}
		if !grew {
			break
		}
	}
	return start, end
}

// renderCommentCard renders one comment as a card in the shared row-card chrome
// — a ▌ bar column, the card background, and Padding(0,1,0,0) — mirroring the
// task row card (docs/DESIGN.md §12). The highlighted card is lifted onto the
// elevated tier with an accent bar; the rest sit flush on the modal background
// with a dim bar. Line one is the author and timestamp (dim), then a blank
// spacer, then the note (primary) wrapped to the card's inner width.
func (m *Model) renderCommentCard(c apptypes.Comment, selected bool, innerW int) string {
	bg := appstyles.Active.ModalBg
	barFg := appstyles.Active.TextDim
	if selected {
		bg = appstyles.Active.BackgroundElevated
		barFg = appstyles.Active.Accent
	}

	contentWidth := max(1, innerW-cardInset)
	ts := time.Unix(c.CreatedAt, 0).Format("2006-01-02 15:04")
	header := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextDim).
		Render(chrome.Truncate(fmt.Sprintf("%s · %s", c.Author, ts), contentWidth))
	note := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextPrimary).
		Width(contentWidth).
		Render(c.Note)
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", note)

	return m.cardChrome(bg, barFg, content, innerW)
}

// renderComposeCard renders the inline new-comment card: the author (the OS
// user) and a single-line text input, styled like the selected comment card so
// it reads as an active draft — the same "fake card while adding" shape the task
// tree uses for inline creation.
func (m *Model) renderComposeCard(innerW int) string {
	bg := appstyles.Active.BackgroundElevated
	barFg := appstyles.Active.Accent

	contentWidth := max(1, innerW-cardInset)
	m.commentInput.SetWidth(contentWidth)
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(appstyles.Active.Accent).
		Render(chrome.Truncate(osUser(), contentWidth))
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", m.commentInput.View())

	return m.cardChrome(bg, barFg, content, innerW)
}

// cardChrome wraps card content in the shared row-card chrome: a background,
// one column of right padding, and the ▌ bar column down the left, then seals
// the whole block onto bg (docs/DESIGN.md §12).
func (m *Model) cardChrome(bg, barFg color.Color, content string, innerW int) string {
	wrapper := lipgloss.NewStyle().
		Width(max(0, innerW-1)).
		Padding(0, 1, 0, 0).
		Background(bg).
		Render(content)
	return appstyles.FillBackground(bg,
		lipgloss.JoinHorizontal(lipgloss.Left, chrome.BarColumn(barFg, bg, wrapper), wrapper))
}

// renderFooter advertises exactly the keys that work in the current zone, each
// key bolded ahead of its muted description. It swaps to the discard prompt
// while a dirty Esc is pending, and to the compose hints while the compose card
// is open.
func (m *Model) renderFooter() string {
	if m.confirmingDiscard {
		return lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted).Render("Discard changes? (y/n)")
	}

	var hints [][2]string
	switch {
	case m.composing:
		hints = [][2]string{{"enter", "post"}, {"esc", "cancel"}, {"ctrl+y", "copy task ID"}}
	case m.focus == focusProgress:
		hints = [][2]string{{"tab", "next"}, {"←/→", "mode"}, {"ctrl+s", "save"}, {"ctrl+y", "copy task ID"}, {"esc", "close"}}
	case m.focus == focusComments:
		hints = [][2]string{{"tab", "next"}, {"↑/↓", "select"}, {"c", "comment"}, {"y", "copy id"}, {"ctrl+y", "copy task ID"}, {"esc", "close"}}
	default: // focusTitle, focusNotes
		hints = [][2]string{{"tab", "next"}, {"ctrl+s", "save"}, {"ctrl+y", "copy task ID"}, {"esc", "close"}}
	}
	return m.renderHints(hints)
}

// renderHints joins key/description pairs with each key bolded onto the primary
// ink and each description muted, separated by two spaces.
func (m *Model) renderHints(hints [][2]string) string {
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(appstyles.Active.TextPrimary)
	descStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted)
	out := ""
	for i, h := range hints {
		if i > 0 {
			out += descStyle.Render("  ")
		}
		out += keyStyle.Render(h[0]) + " " + descStyle.Render(h[1])
	}
	return out
}
