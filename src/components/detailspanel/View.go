package detailspanel

import (
	"fmt"
	"image/color"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/components/chrome"
	"github.com/filipemolina/farol/src/keys"
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
		m.fieldLabel("Priority", focusPriority),
		m.renderPriorityZone(),
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

// applyTextareaStyles seals the textarea onto the modal background and themes
// its cursor line. Rebuilt every render so a theme switch while the modal is
// closed cannot leave a stale background behind.
//
// textarea.DefaultDarkStyles() leaves uncustomized fields at bubbles' own
// hardcoded ANSI-256 greys, unrelated to the active Theme — their brightness
// relative to Text (explicitly set to TextPrimary below) was whatever that
// fixed palette happened to produce, which is why filler (empty) lines could
// read brighter than real text (bug: accent color on the notes field). Every
// tier here comes from the active Theme instead: EndOfBuffer uses TextDim so
// empty lines read quieter than filled ones, and the cursor line gets
// BackgroundElevated — the same "this is where the keyboard is" lift the
// percentage field and the highlighted comment card in this modal use
// (docs/DESIGN.md §12) — so the current line is marked rather than blending in.
func (m *Model) applyTextareaStyles(bg color.Color) {
	styles := textarea.DefaultDarkStyles()
	base := lipgloss.NewStyle().Background(bg)
	text := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Background(bg)
	dim := lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Background(bg)

	cursorText := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextPrimary).
		Background(appstyles.Active.BackgroundElevated)

	styles.Focused.Base = base
	styles.Blurred.Base = base
	styles.Focused.Text = text
	styles.Blurred.Text = text
	// Lit only while Notes actually holds the keyboard. Setting the blurred
	// state to the same lift made every field look focused at once, which is
	// what left the bold section label as the only real focus signal.
	styles.Focused.CursorLine = cursorText
	styles.Blurred.CursorLine = text
	styles.Focused.EndOfBuffer = dim
	styles.Blurred.EndOfBuffer = dim
	m.notes.SetStyles(styles)
}

// applyInputStyles seals the title and compose textinputs onto whatever surface
// they sit on, so their text rows do not bleed the terminal default behind
// them, matching the notes textarea. Rebuilt every render for the same
// theme-switch reason as applyTextareaStyles. The sealing itself is the
// shared chrome.SealInput helper — the two inputs sit on different surfaces,
// so each passes its own pair (docs/DESIGN.md §12).
//
// The Title field sits directly on the modal and lifts to BackgroundElevated
// across its full width while focused, the app's one way of showing focus
// (docs/DESIGN.md §12) — before that it had no edge and no frame of its own,
// so the only thing marking it as active was its section label going bold,
// easy to miss on a modal with four zones. The widgets pick Focused vs
// Blurred from their own state, which cycleFocus already drives.
func (m *Model) applyInputStyles(bg color.Color) {
	chrome.SealInput(&m.titleInput, appstyles.Active.BackgroundElevated, bg)
	// The compose input only ever renders INSIDE the compose card, so it takes
	// the card's tier in both states — it has no bare-modal row to seal onto.
	// Sealing it onto the modal background cut a modal-coloured stripe through
	// the middle of the card and made the card read as broken.
	chrome.SealInput(&m.commentInput, composeCardBg(), composeCardBg())
}

// progressModeLabel is the user-facing name for a progress mode. The stored
// ProgressKind values ("simple", "subtasks", "percentage") are a public
// contract — the column value, the CLI's `crush progress --mode`, and the MCP
// tool's parameter all speak them (docs/DESIGN.md §9) — so they are never
// renamed. This maps them to language a reader recognises, for display only:
// "simple" in particular describes nothing on its own, where §3's actual
// meaning is "being worked on, with no number attached".
func progressModeLabel(kind apptypes.ProgressKind) string {
	switch kind {
	case apptypes.ProgressSimple:
		return "in progress (flag)"
	case apptypes.ProgressSubtasks:
		return "from subtasks"
	case apptypes.ProgressPercentage:
		return "percentage"
	}
	return string(kind)
}

func (m *Model) renderProgressZone() string {
	modeName := progressModeLabel(m.progressKind)
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

	// Derived and unavailable values are annotations — parenthesised and muted,
	// because the user cannot edit them here. The percentage is the one value
	// they can, so it renders as a field instead (renderPercentField).
	annotate := func(s string) string {
		return " " + lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted).Render(s)
	}

	var valueDisplay string
	switch m.progressKind {
	case apptypes.ProgressSimple:
		valueDisplay = ""
	case apptypes.ProgressSubtasks:
		if m.displayAsSimple {
			valueDisplay = annotate("(no children)")
		} else {
			valueDisplay = annotate(fmt.Sprintf("(%d%%)", m.derivedPct))
		}
	case apptypes.ProgressPercentage:
		valueDisplay = " " + m.renderPercentField()
	}

	return modeDisplay + valueDisplay
}

// renderPercentField renders the percentage as something that looks editable.
// It used to render as a dim parenthetical — "(60%)", or "(—)" when unset —
// which read as a status annotation, so nothing on screen suggested the field
// took input at all. Now the value is TextPrimary, an unset value shows "0%"
// rather than an em dash, and while the Progress field has focus the value
// sits on BackgroundElevated: the same lift the notes textarea uses for its
// cursor line, which is how this app shows focus (docs/DESIGN.md §12) rather
// than inventing a caret glyph.
func (m *Model) renderPercentField() string {
	value := m.percentInput
	if value == "" {
		value = "0"
	}
	style := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)
	if m.focus == focusProgress {
		style = style.Background(appstyles.Active.BackgroundElevated)
	}
	return style.Render(" " + value + "% ")
}

// renderPriorityZone renders the task's rank as an editable field, the same
// shape renderPercentField uses: TextPrimary text that lifts onto
// BackgroundElevated while the zone holds the keyboard, which is how this app
// says "this value takes input" (docs/DESIGN.md §12) — there is no caret
// glyph to invent. `none` is spelled out here, unlike the task row's badge,
// which renders nothing for it: a field the user is editing has to show the
// value it currently holds, where a row badge reading "NONE" on most rows
// would be noise.
func (m *Model) renderPriorityZone() string {
	style := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)
	if m.focus == focusPriority {
		style = style.Background(appstyles.Active.BackgroundElevated)
	}
	value := style.Render(" " + string(m.priority) + " ")
	if m.focus != focusPriority {
		return value
	}
	return value + " " +
		lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted).Render("←/→ cycle")
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

// composeCardBg is the tier the inline compose card paints on. Both the card
// fill and the input sealed inside it need it, and they must not decide it
// separately: when they did, the input painted its own row with the modal
// background instead and the card looked broken.
func composeCardBg() color.Color { return appstyles.Active.BackgroundElevated }

// renderComposeCard renders the inline new-comment card: the author (the OS
// user) and a single-line text input, styled like the selected comment card so
// it reads as an active draft — the same "fake card while adding" shape the task
// tree uses for inline creation.
func (m *Model) renderComposeCard(innerW int) string {
	bg := composeCardBg()
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
//
// Since the global footer goes blank while this modal is open, this is the only
// hint line on screen — so every key live in the current zone has to be here.
// The wording is never written out here: each hint comes from the binding in
// src/keys, through chrome.HintFor, so the modal and the help overlay cannot
// drift into describing the same key two ways.
func (m *Model) renderFooter() string {
	if m.confirmingDiscard {
		question := lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted).Render("Discard changes?")
		return question + "  " + m.fitHints(m.zoneHints(), lipgloss.Width(question)+2)
	}
	return m.fitHints(m.zoneHints(), 0)
}

// fitHints renders hints on exactly one line, shedding whole hints from the
// tail until the line fits the modal's inner width (less used, e.g. by the
// discard prompt's question). The modal's content column is fixed-width, so a
// hint line too long to fit would otherwise wrap and grow the modal by a row —
// the footer bar sheds for the same reason. Each zone lists its core keys
// first (save, next field, cancel) so shedding takes the extras.
func (m *Model) fitHints(hints []chrome.KeyHint, used int) string {
	line := chrome.RenderKeyHints(hints, appstyles.Active.TextMuted)
	// Before the first layout message there is no box to fit, so nothing sheds
	// — View() renders nothing at that point anyway.
	if m.innerWidth() <= 0 {
		return line
	}
	avail := max(0, m.innerWidth()-used)
	for len(hints) > 1 && lipgloss.Width(line) > avail {
		hints = hints[:len(hints)-1]
		line = chrome.RenderKeyHints(hints, appstyles.Active.TextMuted)
	}
	return line
}

// zoneHints is the set of keys live in the current zone, in the order they are
// advertised. A key absent from a zone is genuinely dead there — c only opens
// the compose card from the comments zone, so it is not offered elsewhere,
// the same honesty the footer bar applies to the globals it drops.
func (m *Model) zoneHints() []chrome.KeyHint {
	switch {
	case m.confirmingDiscard:
		// The prompt owns the keyboard and answers to y/n alone. enter is not
		// bound here on purpose: unlike the confirm modal, which has a visible
		// yes/no selection for enter to act on, this prompt has no default —
		// so enter would be one stray keystroke away from losing unsaved edits.
		return []chrome.KeyHint{chrome.HintFor(keys.Details.DiscardPrompt)}
	case m.composing:
		return []chrome.KeyHint{
			chrome.HintFor(keys.Details.CommentSubmit),
			chrome.HintFor(keys.Overlay.Cancel),
			chrome.HintFor(keys.Details.CopyTaskID),
		}
	case m.focus == focusProgress:
		// Ways out of the zone come first, then how to commit, then the input
		// methods for the value, then mode cycling, then the copy extra. A
		// narrow modal sheds from the tail, so what goes first is what a user
		// stuck in this zone would need last.
		hints := []chrome.KeyHint{
			chrome.HintFor(keys.Details.NextField),
			chrome.HintFor(keys.Overlay.Cancel),
			chrome.HintFor(keys.Details.Save),
		}
		// The percentage value is the only editable one, so its input methods
		// are advertised only in that mode — typing digits and ↑/↓ do nothing
		// in the other two. In that mode they outrank mode cycling: not
		// knowing the field takes typing is the bug this line exists to fix.
		if m.progressKind == apptypes.ProgressPercentage {
			hints = append(hints,
				chrome.HintFor(keys.Details.PercentType),
				chrome.HintFor(keys.Details.PercentNudge),
			)
		}
		return append(hints,
			chrome.HintFor(keys.Details.CycleMode),
			chrome.HintFor(keys.Details.CycleModeBack),
			chrome.HintFor(keys.Details.CopyTaskID),
		)
	case m.focus == focusPriority:
		return []chrome.KeyHint{
			chrome.HintFor(keys.Details.NextField),
			chrome.HintFor(keys.Overlay.Cancel),
			chrome.HintFor(keys.Details.Save),
			chrome.HintFor(keys.Details.CyclePriority),
			chrome.HintFor(keys.Details.CopyTaskID),
		}
	case m.focus == focusComments:
		return []chrome.KeyHint{
			chrome.HintFor(keys.Details.NextField),
			chrome.HintFor(keys.Overlay.Cancel),
			chrome.HintFor(keys.Details.CommentNew),
			chrome.HintFor(keys.Overlay.Navigation),
			chrome.HintFor(keys.Details.CopyCommentID),
			chrome.HintFor(keys.Details.CommentDelete),
			chrome.HintFor(keys.Details.CopyTaskID),
		}
	default: // focusTitle, focusNotes
		return []chrome.KeyHint{
			chrome.HintFor(keys.Details.NextField),
			chrome.HintFor(keys.Overlay.Cancel),
			chrome.HintFor(keys.Details.Save),
			chrome.HintFor(keys.Details.CopyTaskID),
		}
	}
}
