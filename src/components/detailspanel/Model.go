// Package detailspanel is the Task details surface: a centered modal that
// takes most of the screen (docs/DESIGN.md §5). It edits a task's title, notes
// and progress and shows its comment thread as selectable cards. It holds its
// display state as apptypes (not store rows), sizes itself from a
// SetDetailsLayoutMsg (the modal's outer box, computed by AppModel from the
// terminal), and closes by emitting cmds.CloseDetailsSideMsg rather than a
// modal close of its own.
//
// It began as the modal editor, was briefly a thin side panel, and is a modal
// again: the side surface could never be made wide enough to show notes and
// comments without crushing the Tasks list beside it.
package detailspanel

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/constants"
	"github.com/filipemolina/chore-crusher/src/store"
)

// focusedZoneID is the zone id this component answers to. Details is focused
// only while it is visible, entered by AppModel's explicit open transition.
const focusedZoneID = constants.COMPONENT_DETAILS_PANEL

const (
	// focusTitle is the single-line title editor; it is the first field so the
	// tab cycle reads top-to-bottom (Title → Notes → Progress → Comments).
	focusTitle = iota
	focusNotes
	focusProgress
	// focusComments is the comment thread: the cards are selectable and ↑/↓
	// move the highlight, y copies the highlighted comment's id, and c opens the
	// inline compose card. Unlike the other zones it is always in the cycle even
	// while the thread is empty — c adds the first comment from here.
	focusComments
	// focusCount is the number of tab-cycle zones; cycleFocus wraps modulo it.
	focusCount
)

const (
	// minNotesRows is the smallest the notes textarea shrinks to — a short note
	// still gets a couple of rows so the field reads as editable, but nothing
	// like the whole modal (the "notes shouldn't occupy so much space" ask).
	minNotesRows = 3
	// commentCardRows is the nominal height of one comment card used for the
	// notes-cap budget: a header line (author · timestamp), a blank spacer, and
	// one note line. A card with a wrapping note renders taller; renderComments
	// measures real heights when windowing, so this is only the reservation
	// estimate. The compose card is the same height.
	commentCardRows = 3
	// commentCardGap is the blank line inserted between consecutive comment
	// cards so they read as distinct blocks rather than a solid strip. It is
	// added to each card's measured height (except the last shown card, which
	// has nothing beneath it) so the windowing and notes-cap budget already
	// account for the gap.
	commentCardGap = 1
	// reservedCommentCards is how many cards the layout keeps room for when
	// capping the notes textarea, so notes can expand with content but never
	// past the point where at least this many comment cards stay visible.
	reservedCommentCards = 2
	// cardInset is the card chrome before its content: the ▌ bar column plus the
	// card's one-column right padding, mirroring the task row card.
	cardInset = 2

	// modalChromeW and modalChromeH are the rows/columns chrome.ModalSurface
	// spends on its rounded border (1 each edge) and Padding(1,2): 6 columns
	// and 4 rows in total. The content box is the modal's outer box minus these.
	modalChromeW = 6
	modalChromeH = 4
	// detailsFixedRows is every content row the modal spends on something other
	// than the notes textarea and the comment cards: the "Task details" heading
	// with its margin and the flush-right task id on one line (2), the Title
	// label and value (2), a blank, the Notes label, a blank after the textarea,
	// the Progress label and value (2), the error/flash line, a blank, the
	// Comments label, a blank, and the footer — 14 rows. The textarea and the
	// comment cards share whatever height is left (flexRows).
	detailsFixedRows = 14
)

// Model is the Task details modal. It starts unloaded; a RefreshDetailsMsg
// hydrates it. store is retained only to write title/notes/progress and post
// comments.
type Model struct {
	store *store.Store

	loaded bool
	taskID string
	listID string

	focused bool
	// width and height are the modal's outer box, set by SetDetailsLayoutMsg.
	width  int
	height int

	titleInput textinput.Model
	origTitle  string

	notes     textarea.Model
	origNotes string

	progressKind     apptypes.ProgressKind
	origProgressKind apptypes.ProgressKind
	percentInput     string
	origPercentInput string
	derivedPct       int
	displayAsSimple  bool

	// comments is the task's comment thread, oldest first, loaded from
	// RefreshDetails. A posted comment is appended in the submit handler so it
	// appears immediately without a poll round-trip.
	comments []apptypes.Comment
	// selectedComment is the index of the highlighted comment card while the
	// comments zone is focused; y copies this comment's id.
	selectedComment int
	// commentInput is the single-line compose field for a new comment. It is
	// live only while composing is true (the inline compose card, entered with
	// c from the comments zone) and submitted with enter.
	commentInput textinput.Model
	// composing reports whether the inline new-comment card is open. While it is,
	// the compose input owns the keyboard: enter posts, esc cancels.
	composing bool

	focus             int
	errMsg            string
	flash             string
	confirmingDiscard bool
}

// New builds an unloaded Details modal. It does not fetch a task — AppModel
// sends a RefreshDetailsMsg once it opens the modal, and that hydrates it.
func New(s *store.Store) tea.Model {
	ci := textinput.New()
	ci.Prompt = ""
	ci.Placeholder = "Write a comment…"

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "Task title"

	notes := textarea.New()
	// No line numbers on a to-do note. The gutter is a code-editor
	// affordance: notes here are a few lines of prose, never something a
	// reader needs to cite by line, so the column only spent width and made
	// the field look like an editor pane. bubbles requires this be set before
	// SetWidth (textarea.go's SetWidth comment), which the layout message does
	// later — setting it at construction is what keeps that order.
	notes.ShowLineNumbers = false
	// bubbles/textarea's default Prompt is "┃ ". With the gutter gone the
	// prompt is the field's only left edge, so it keeps the single trailing
	// space as the gap between the edge and the text.
	notes.Prompt = "┃ "

	return &Model{
		store:        s,
		notes:        notes,
		titleInput:   ti,
		commentInput: ci,
		focus:        focusTitle,
	}
}

func (m *Model) Init() tea.Cmd { return tea.Batch(textarea.Blink, textinput.Blink) }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmds.SetDetailsLayoutMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeNotes()
		return m, nil

	case cmds.SetFocusMsg:
		m.focused = int(msg) == focusedZoneID
		return m, nil

	case cmds.RefreshDetailsMsg:
		return m.handleRefresh(msg)

	case tea.KeyPressMsg:
		// Only act on keys while focused. AppModel routes keys here exclusively
		// while the modal is visible, and a hidden modal must not let a stray
		// tree keystroke mutate its draft — that would later read as a dirty
		// same-task editor and swallow the reopen refresh.
		if !m.focused {
			return m, nil
		}
		return m.handleKey(msg)
	}

	// Non-key messages (blink, paste, etc.) keep the active editor live. Gated
	// on focus so a hidden modal never absorbs a paste into its draft.
	switch {
	case m.focused && m.composing:
		var cmd tea.Cmd
		m.commentInput, cmd = m.commentInput.Update(msg)
		return m, cmd
	case m.focused && m.focus == focusTitle:
		var cmd tea.Cmd
		m.titleInput, cmd = m.titleInput.Update(msg)
		return m, cmd
	case m.focused && m.focus == focusNotes:
		var cmd tea.Cmd
		m.notes, cmd = m.notes.Update(msg)
		return m, cmd
	}
	return m, nil
}

// handleRefresh loads a details response. A fresh task (a newly opened modal)
// always hydrates and resets focus to Notes. A response for the task already
// displayed hydrates only while the editor is clean — a dirty draft keeps its
// edits and ignores the external update until it is saved or discarded
// (docs/DESIGN.md §5). An error is recorded for rendering without destroying a
// clean prior view.
func (m *Model) handleRefresh(msg cmds.RefreshDetailsMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.errMsg = fmt.Sprintf("failed to load task: %v", msg.Err)
		return m, nil
	}

	newTask := msg.TaskID != m.taskID || !m.loaded
	if !newTask && m.hasDirtyFields() {
		return m, nil
	}
	m.hydrate(msg, newTask)
	return m, nil
}

// hydrate copies a details response into the draft fields. When resetFocus is
// set (a freshly opened task) it also moves focus into Notes and clears any
// transient error/prompt/compose state. Text values are rewritten only when
// they actually differ, so a clean same-task poll refresh does not jump a
// cursor. An open compose draft survives the refresh — the input is cleared
// only when not composing — so a poll tick can no longer wipe a half-typed
// comment (the "input goes blank after a few characters" bug).
func (m *Model) hydrate(msg cmds.RefreshDetailsMsg, resetFocus bool) {
	m.loaded = true
	m.taskID = msg.TaskID
	m.listID = msg.Task.ListID

	if m.titleInput.Value() != msg.Task.Title {
		m.titleInput.SetValue(msg.Task.Title)
	}
	m.origTitle = msg.Task.Title

	kind := msg.Task.ProgressKind
	if kind == apptypes.ProgressNone {
		kind = apptypes.ProgressSimple
	}
	m.progressKind = kind
	m.origProgressKind = kind

	percentInput := ""
	if kind == apptypes.ProgressPercentage && msg.Task.ProgressPct != nil {
		percentInput = strconv.Itoa(*msg.Task.ProgressPct)
	}
	m.percentInput = percentInput
	m.origPercentInput = percentInput

	m.derivedPct = msg.DerivedPct
	m.displayAsSimple = msg.DisplayAsSimple

	// Replace the comment thread from the refresh. A posted comment (see
	// postComment) is appended in-handler for immediate feedback; the next
	// refresh reconciles any ordering/timestamp drift. The compose input is
	// reset only when the compose card is closed, so a live draft outlives a
	// poll refresh.
	m.comments = msg.Comments
	m.clampSelectedComment()
	if !m.composing {
		m.commentInput.SetValue("")
	}
	// Rebalance the textarea height against the (possibly changed) comment
	// section so the modal never overflows its box.
	m.resizeNotes()

	if m.notes.Value() != msg.Task.Notes {
		m.notes.SetValue(msg.Task.Notes)
	}
	m.origNotes = msg.Task.Notes

	if resetFocus {
		// A freshly opened task hydrates and resets focus to Title, the first
		// field so the tab cycle reads top-to-bottom (Title → Notes → Progress →
		// Comments).
		m.focus = focusTitle
		m.titleInput.Focus()
		m.notes.Blur()
		m.commentInput.Blur()
		m.composing = false
		m.selectedComment = 0
		m.errMsg = ""
		m.flash = ""
		m.confirmingDiscard = false
	}
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Any key clears the transient copy flash; the copy handler re-sets it
	// after this, so it survives exactly until the next keystroke.
	m.flash = ""

	// Copying the task id works from anywhere in the modal, including while a
	// text field or the compose card owns the keyboard — ctrl+y is bound to no
	// input widget. It is the only way to get the id now that it is not shown in
	// the header.
	if msg.String() == "ctrl+y" {
		m.flash = fmt.Sprintf("copied task id %s", m.taskID)
		return m, tea.SetClipboard(m.taskID)
	}

	// While the inline compose card is open it owns the keyboard: enter posts,
	// esc cancels, everything else types into the new comment.
	if m.composing {
		return m.handleComposeKey(msg)
	}

	switch msg.String() {
	case "ctrl+s":
		if m.confirmingDiscard {
			m.confirmingDiscard = false
			return m, nil
		}
		return m.save()

	case "tab":
		m.confirmingDiscard = false
		m.cycleFocus(1)
		return m, nil

	case "shift+tab":
		m.confirmingDiscard = false
		m.cycleFocus(-1)
		return m, nil

	case "esc":
		m.confirmingDiscard = false
		if m.hasDirtyFields() {
			m.confirmingDiscard = true
			return m, nil
		}
		return m, cmds.CloseDetailsSide(nil)
	}

	// While the discard prompt is up it owns the keyboard: only y/n resolve it,
	// every other key is swallowed so a stray keystroke can't leak into a draft
	// the user is about to throw away.
	if m.confirmingDiscard {
		switch msg.String() {
		case "y", "Y":
			m.confirmingDiscard = false
			return m, cmds.CloseDetailsSide(nil)
		case "n", "N":
			m.confirmingDiscard = false
		}
		return m, nil
	}

	// Zone dispatch. y/n are ordinary characters here (they type into the title
	// or notes input, and y copies inside the comments zone) — they only mean
	// "discard yes/no" while the prompt above is up.
	switch m.focus {
	case focusTitle:
		var cmd tea.Cmd
		m.titleInput, cmd = m.titleInput.Update(msg)
		return m, cmd
	case focusProgress:
		return m.handleProgressKey(msg)
	case focusComments:
		return m.handleCommentsKey(msg)
	case focusNotes:
		var cmd tea.Cmd
		m.notes, cmd = m.notes.Update(msg)
		return m, cmd
	}
	return m, nil
}

// handleComposeKey drives the inline compose card. enter (or ctrl+enter, for
// terminals that distinguish it) posts the comment; esc cancels and closes the
// card; every other key types into the single-line input. ctrl+s is
// deliberately not save here — while composing, the card owns the keyboard the
// way the task tree's inline create row does.
func (m *Model) handleComposeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "ctrl+enter":
		return m.postComment()
	case "esc":
		m.cancelComposing()
		return m, nil
	case "ctrl+s", "tab":
		// Swallowed while the compose card owns the keyboard: neither save nor
		// field-cycling apply mid-comment, and they must never leak into the
		// draft as literal text.
		return m, nil
	}
	var cmd tea.Cmd
	m.commentInput, cmd = m.commentInput.Update(msg)
	return m, cmd
}

// cycleFocus moves focus one step in dir (+1 tab, -1 shift+tab) through
// Title → Notes → Progress → Comments, blurring the outgoing text input and
// focusing the incoming one so the cursor is never live in two inputs at once.
// Every zone is always in the cycle: the comments zone stays reachable even
// with an empty thread so c can add the first comment.
func (m *Model) cycleFocus(dir int) {
	switch m.focus {
	case focusTitle:
		m.titleInput.Blur()
	case focusNotes:
		m.notes.Blur()
	}
	m.focus = (m.focus + dir + focusCount) % focusCount
	switch m.focus {
	case focusTitle:
		m.titleInput.Focus()
	case focusNotes:
		m.notes.Focus()
	}
}

// innerWidth and innerHeight are the modal's content box: the ModalSurface
// border and padding taken off its outer box (SetDetailsLayoutMsg).
func (m *Model) innerWidth() int  { return max(0, m.width-modalChromeW) }
func (m *Model) innerHeight() int { return max(0, m.height-modalChromeH) }

// flexRows is the height the notes textarea and the comment cards share: the
// modal content height minus every fixed row (title, labels, progress, footer,
// and the blanks between them).
func (m *Model) flexRows() int { return max(0, m.innerHeight()-detailsFixedRows) }

// handleCommentsKey moves the comment highlight, opens the compose card, and
// copies the selected comment's id. ↑/↓ (and k/j) move within the thread; c
// opens the inline compose card; y writes the highlighted comment's id to the
// system clipboard (OSC-52 via tea.SetClipboard) and flashes a confirmation.
func (m *Model) handleCommentsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "c", "C":
		return m.startComposing()
	case "up", "k":
		if m.selectedComment > 0 {
			m.selectedComment--
		}
	case "down", "j":
		if m.selectedComment < len(m.comments)-1 {
			m.selectedComment++
		}
	case "y", "Y":
		if len(m.comments) == 0 {
			return m, nil
		}
		id := m.comments[m.selectedComment].ID
		m.flash = fmt.Sprintf("copied comment id %s", id)
		return m, tea.SetClipboard(id)
	}
	return m, nil
}

// startComposing opens the inline compose card and focuses its input. It is
// reachable only from the comments zone (c), so it never steals a keystroke
// from the title or notes editor.
func (m *Model) startComposing() (tea.Model, tea.Cmd) {
	m.composing = true
	m.commentInput.SetValue("")
	m.commentInput.Focus()
	m.errMsg = ""
	m.resizeNotes()
	return m, textinput.Blink
}

// cancelComposing closes the compose card, discarding its draft.
func (m *Model) cancelComposing() {
	m.composing = false
	m.commentInput.Blur()
	m.commentInput.SetValue("")
	m.resizeNotes()
}

// NotesValue returns the notes textarea's current value, for tests.
func (m *Model) NotesValue() string { return m.notes.Value() }

// TitleValue returns the title input's current value, for tests.
func (m *Model) TitleValue() string { return m.titleInput.Value() }

// CommentInputValue returns the comment compose input's current value, for
// tests.
func (m *Model) CommentInputValue() string { return m.commentInput.Value() }

// Composing reports whether the inline compose card is open, for tests.
func (m *Model) Composing() bool { return m.composing }

// Comments returns the panel's current comment thread, for tests.
func (m *Model) Comments() []apptypes.Comment { return m.comments }

// SelectedCommentIndex returns the highlighted comment index, for tests.
func (m *Model) SelectedCommentIndex() int { return m.selectedComment }

// clampSelectedComment keeps the highlight within the thread after it grows or
// shrinks (a refresh replacing the comments, or a post appending one).
func (m *Model) clampSelectedComment() {
	if m.selectedComment < 0 {
		m.selectedComment = 0
	}
	if m.selectedComment > len(m.comments)-1 {
		m.selectedComment = max(0, len(m.comments)-1)
	}
}

// resizeNotes budgets the notes textarea against the modal's flexible height.
// The textarea grows with its content (m.notes.LineCount) but is capped so at
// least reservedCommentCards worth of comment cards stay visible — notes never
// swallow the whole modal (docs/DESIGN.md §5, the "notes shouldn't occupy so
// much space" contract). The title and compose inputs are kept to the modal
// body width so they never widen the surface.
func (m *Model) resizeNotes() {
	innerW := m.innerWidth()
	m.notes.SetWidth(innerW)
	m.notes.SetHeight(m.notesRows())
	m.titleInput.SetWidth(max(0, innerW))
	m.commentInput.SetWidth(max(0, innerW-cardInset))
}

// notesRows is the height the notes textarea renders at: at least minNotesRows,
// growing with the note's line count, but never past the cap that keeps the
// comment thread (and, while composing, the compose card) visible.
func (m *Model) notesRows() int {
	flex := m.flexRows()
	if flex <= 0 {
		return 0
	}
	reserved := m.commentsReservedRows()
	maxNotes := max(1, flex-reserved)

	desired := max(minNotesRows, m.notes.LineCount())
	desired = min(desired, flex)
	return min(desired, maxNotes)
}

// commentsReservedRows is the height the layout keeps for the comment thread
// when capping notes: room for the compose card while composing, else up to
// reservedCommentCards cards (plus the gap between them) when there are
// comments, or one line for the "no comments yet" placeholder when there aren't.
func (m *Model) commentsReservedRows() int {
	if m.composing {
		// The compose card plus one card of surrounding context.
		return 2 * commentCardRows
	}
	if len(m.comments) == 0 {
		return 1
	}
	cards := min(len(m.comments), reservedCommentCards)
	// cards cards with commentCardGap between each adjacent pair.
	return cards*commentCardRows + (cards-1)*commentCardGap
}

func (m *Model) handleProgressKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "right", "l":
		m.cycleMode(1)
		return m, nil
	case "left", "h":
		m.cycleMode(-1)
		return m, nil
	case "up":
		return m.nudgePercent(5)
	case "down":
		return m.nudgePercent(-5)
	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if m.progressKind != apptypes.ProgressPercentage {
			return m, nil
		}
		newBuf := m.percentInput + msg.String()
		val, err := strconv.Atoi(newBuf)
		if err != nil || val > 100 {
			m.errMsg = fmt.Sprintf("percent out of range: %s", newBuf)
			return m, nil
		}
		m.errMsg = ""
		m.percentInput = newBuf
		return m, nil
	case "backspace":
		if m.progressKind != apptypes.ProgressPercentage {
			return m, nil
		}
		if len(m.percentInput) > 0 {
			m.percentInput = m.percentInput[:len(m.percentInput)-1]
			m.errMsg = ""
		}
		return m, nil
	}
	return m, nil
}

// nudgePercent steps the percentage-mode value by delta, clamped to 0–100.
// An unset value counts as 0, so the first ↑ on a fresh percentage task gives
// 5 rather than doing nothing. Clamping (rather than refusing) is what makes
// holding ↑ settle at 100 instead of erroring: the out-of-range message is for
// typed input, where the user can see what they typed.
func (m *Model) nudgePercent(delta int) (tea.Model, tea.Cmd) {
	if m.progressKind != apptypes.ProgressPercentage {
		return m, nil
	}
	current := 0
	if m.percentInput != "" {
		current, _ = strconv.Atoi(m.percentInput)
	}
	m.percentInput = strconv.Itoa(min(100, max(0, current+delta)))
	m.errMsg = ""
	return m, nil
}

func (m *Model) cycleMode(delta int) {
	modes := []apptypes.ProgressKind{
		apptypes.ProgressSimple,
		apptypes.ProgressSubtasks,
		apptypes.ProgressPercentage,
	}
	idx := -1
	for i, mode := range modes {
		if mode == m.progressKind {
			idx = i
			break
		}
	}
	if idx >= 0 {
		idx = (idx + delta + len(modes)) % len(modes)
		m.progressKind = modes[idx]
		if m.progressKind != apptypes.ProgressPercentage {
			m.percentInput = ""
		}
		m.errMsg = ""
	}
}

func (m *Model) hasDirtyFields() bool {
	if m.titleInput.Value() != m.origTitle {
		return true
	}
	if m.notes.Value() != m.origNotes {
		return true
	}
	if m.progressKind != m.origProgressKind {
		return true
	}
	if m.percentInput != m.origPercentInput {
		return true
	}
	return false
}

// save writes any dirty title/notes/progress, then closes the modal and
// refreshes the task tree. store.ProgressKind is derived only here, at the
// write boundary, so the component itself never retains a store row type. A
// title cleared to whitespace is refused in place (the store forbids an empty
// title) rather than closing the modal on a write that would fail.
func (m *Model) save() (tea.Model, tea.Cmd) {
	if m.titleInput.Value() != m.origTitle {
		if strings.TrimSpace(m.titleInput.Value()) == "" {
			m.errMsg = "title must not be empty"
			return m, nil
		}
		if err := m.store.RenameTask(m.taskID, m.titleInput.Value()); err != nil {
			m.errMsg = fmt.Sprintf("failed to save title: %v", err)
			return m, nil
		}
	}

	if m.notes.Value() != m.origNotes {
		if err := m.store.SetNotes(m.taskID, m.notes.Value()); err != nil {
			m.errMsg = fmt.Sprintf("failed to save notes: %v", err)
			return m, nil
		}
	}

	if m.progressKind != m.origProgressKind || m.percentInput != m.origPercentInput {
		var pct *int
		if m.progressKind == apptypes.ProgressPercentage && m.percentInput != "" {
			val, _ := strconv.Atoi(m.percentInput)
			pct = &val
		}
		if err := m.store.SetProgress(m.taskID, store.ProgressKind(m.progressKind), pct); err != nil {
			m.errMsg = fmt.Sprintf("failed to save progress: %v", err)
			return m, nil
		}
	}

	return m, cmds.CloseDetailsSide(cmds.RefreshTasks(m.store, m.listID))
}

// postComment writes the compose input's text as a comment attributed to the
// OS user, then appends it to the live thread so it renders immediately without
// waiting for the next poll refresh. Comments are append-only — there is no
// edit or delete path — and the compose card is closed on success. An empty
// (whitespace-only) note is a no-op so enter can't insert a blank comment.
func (m *Model) postComment() (tea.Model, tea.Cmd) {
	note := strings.TrimSpace(m.commentInput.Value())
	if note == "" {
		return m, nil
	}
	id, err := m.store.AddComment(m.taskID, osUser(), note)
	if err != nil {
		m.errMsg = fmt.Sprintf("failed to post comment: %v", err)
		return m, nil
	}
	now := time.Now().Unix()
	m.comments = append(m.comments, apptypes.Comment{
		ID:        id,
		TaskID:    m.taskID,
		Author:    osUser(),
		Note:      note,
		CreatedAt: now,
	})
	m.selectedComment = len(m.comments) - 1
	m.composing = false
	m.commentInput.Blur()
	m.commentInput.SetValue("")
	m.errMsg = ""
	m.resizeNotes()
	return m, nil
}

// osUser returns the current OS username for human-authored writes (comments),
// falling back to $USER/$LOGNAME when os/user.Current fails — some minimal
// containers lack /etc/passwd. Mirrors the CLI's osUser so human attribution is
// consistent across surfaces.
func osUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return os.Getenv("LOGNAME")
}
