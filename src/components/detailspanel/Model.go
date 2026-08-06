// Package detailspanel is the Details side surface: an editing panel for a
// task's notes and progress that replaces the Lists panel on the right
// (docs/DESIGN.md §5). It is the modal editor's state machine moved into a
// chrome.PanelFrame — same notes/progress drafts, ctrl+s save, tab field
// cycle, mode cycling, dirty detection, and inline discard prompt — but it
// holds its display state as apptypes (not store rows) and closes by emitting
// cmds.CloseDetailsSideMsg rather than a modal close.
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
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/constants"
	"github.com/filipemolina/chore-crusher/src/store"
)

// focusedZoneID is the zone id this component answers to. Details is focused
// only while it is visible, entered by AppModel's explicit open transition.
const focusedZoneID = constants.COMPONENT_DETAILS_PANEL

const (
	focusNotes = iota
	focusProgress
	// focusComments is the comment compose input; tab cycles notes ->
	// progress -> comments -> notes (docs/plan/task-comments.md §6, Commit 5).
	// ctrl+s (save) is never bound to this zone — posting a comment is
	// ctrl+enter, so the two write paths never collide.
	focusComments
)

// detailsChromeRows is every body row the panel spends on something other
// than the notes textarea: the Title label and its value, a blank, the Notes
// label, a blank after the textarea, the Progress label and its value, the
// error line, a blank, and the footer. The textarea gets whatever body height
// is left after these, so it never forces the frame taller.
const detailsChromeRows = 10

// Model is the Details side panel. It starts unloaded; a RefreshDetailsMsg
// hydrates it. store is retained only to write notes/progress on save.
type Model struct {
	store *store.Store

	loaded bool
	taskID string
	listID string
	title  string

	focused bool
	body    cmds.SetBodyLayoutMsg

	notes     textarea.Model
	origNotes string

	progressKind     apptypes.ProgressKind
	origProgressKind apptypes.ProgressKind
	percentInput     string
	origPercentInput string
	derivedPct       int
	displayAsSimple  bool

	// Comments is the task's comment thread, oldest first, loaded from
	// RefreshDetails. A posted comment is appended to this in the submit
	// handler so it appears immediately without a poll round-trip.
	comments []apptypes.Comment
	// commentInput is the single-line compose field for a new comment;
	// submitted with ctrl+enter (docs/plan/task-comments.md §6, Commit 5).
	commentInput textinput.Model

	focus             int
	errMsg            string
	confirmingDiscard bool
}

// New builds an unloaded Details panel. It does not fetch a task — AppModel
// sends a RefreshDetailsMsg once it opens the panel, and that hydrates it.
func New(s *store.Store) tea.Model {
	ci := textinput.New()
	ci.Placeholder = "Write a comment…"
	return &Model{
		store:        s,
		notes:        textarea.New(),
		commentInput: ci,
		focus:        focusNotes,
	}
}

func (m *Model) Init() tea.Cmd { return tea.Batch(textarea.Blink, textinput.Blink) }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmds.SetBodyLayoutMsg:
		m.body = msg
		m.resizeNotes()
		return m, nil

	case cmds.SetFocusMsg:
		m.focused = int(msg) == focusedZoneID
		return m, nil

	case cmds.RefreshDetailsMsg:
		return m.handleRefresh(msg)

	case tea.KeyPressMsg:
		// Only act on keys while focused. AppModel routes keys here exclusively
		// while the panel is visible, and a hidden panel must not let a stray
		// tree keystroke mutate its draft — that would later read as a dirty
		// same-task editor and swallow the reopen refresh.
		if !m.focused {
			return m, nil
		}
		return m.handleKey(msg)
	}

	// Non-key messages (blink, paste, etc.) keep the active editor live, mirroring
	// the modal editor. Gated on focus so a hidden panel never absorbs a paste into
	// its draft (same reasoning as the key gate).
	switch {
	case m.focused && m.focus == focusNotes:
		var cmd tea.Cmd
		m.notes, cmd = m.notes.Update(msg)
		return m, cmd
	case m.focused && m.focus == focusComments:
		var cmd tea.Cmd
		m.commentInput, cmd = m.commentInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// handleRefresh loads a details response. A fresh task (a newly opened panel)
// always hydrates and resets focus to Notes. A response for the task already
// displayed hydrates only while the editor is clean — a dirty draft keeps its
// edits and ignores the external update until it is saved or discarded
// (docs/DESIGN.md §5). An error is recorded for rendering without destroying
// a clean prior view.
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
// transient error/prompt state. The notes value is rewritten only when it
// actually differs, so a clean same-task poll refresh does not jump the
// textarea cursor.
func (m *Model) hydrate(msg cmds.RefreshDetailsMsg, resetFocus bool) {
	m.loaded = true
	m.taskID = msg.TaskID
	m.listID = msg.Task.ListID
	m.title = msg.Task.Title

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
	// reset here so a stale draft never survives a refresh.
	m.comments = msg.Comments
	m.commentInput.SetValue("")
	// Rebalance the textarea height against the (possibly changed) comment
	// section so the panel never overflows its frame.
	m.resizeNotes()

	if m.notes.Value() != msg.Task.Notes {
		m.notes.SetValue(msg.Task.Notes)
	}
	m.origNotes = msg.Task.Notes

	if resetFocus {
		m.focus = focusNotes
		m.notes.Focus()
		m.commentInput.Blur()
		m.errMsg = ""
		m.confirmingDiscard = false
	}
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+s":
		if m.confirmingDiscard {
			m.confirmingDiscard = false
			return m, nil
		}
		return m.save()

	case "ctrl+enter":
		// Post the comment in the compose input — only wired while the focus
		// is on the comment zone, so ctrl+s (save) is never stolen
		// (docs/plan/task-comments.md §6, Commit 5).
		if m.focus == focusComments {
			return m.postComment()
		}
		return m, nil

	case "tab":
		m.confirmingDiscard = false
		// Cycle notes -> progress -> comments -> notes. Blurring the
		// outgoing zone and focusing the incoming one keeps the cursor out of
		// two text inputs at once, and gives the textinput a focused state
		// so it actually accepts keystrokes (docs/DESIGN.md §5 focus contract).
		switch m.focus {
		case focusNotes:
			m.notes.Blur()
			m.commentInput.Blur()
			m.focus = focusProgress
		case focusProgress:
			m.commentInput.Focus()
			m.focus = focusComments
		case focusComments:
			m.commentInput.Blur()
			m.notes.Focus()
			m.focus = focusNotes
		}
		return m, nil

	case "esc":
		m.confirmingDiscard = false
		if m.hasDirtyFields() {
			m.confirmingDiscard = true
			return m, nil
		}
		return m, cmds.CloseDetailsSide(nil)

	case "y", "Y":
		if m.confirmingDiscard {
			m.confirmingDiscard = false
			return m, cmds.CloseDetailsSide(nil)
		}
		if m.focus == focusNotes {
			var cmd tea.Cmd
			m.notes, cmd = m.notes.Update(msg)
			return m, cmd
		}
		return m, nil

	case "n", "N":
		if m.confirmingDiscard {
			m.confirmingDiscard = false
			return m, nil
		}
		if m.focus == focusNotes {
			var cmd tea.Cmd
			m.notes, cmd = m.notes.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Progress zone: mode cycling and percent entry.
	if m.focus == focusProgress {
		return m.handleProgressKey(msg)
	}

	// Comments zone: pass typing through to the single-line compose input
	// (ctrl+enter is matched above as a submit; other keys type into it).
	if m.focus == focusComments {
		var cmd tea.Cmd
		m.commentInput, cmd = m.commentInput.Update(msg)
		return m, cmd
	}

	// Notes zone: pass through to the textarea (enter inserts a newline).
	if m.focus == focusNotes {
		var cmd tea.Cmd
		m.notes, cmd = m.notes.Update(msg)
		return m, cmd
	}
	return m, nil
}

// NotesValue returns the notes textarea's current value, for tests.
func (m *Model) NotesValue() string { return m.notes.Value() }

// CommentInputValue returns the comment compose input's current value, for
// tests.
func (m *Model) CommentInputValue() string { return m.commentInput.Value() }

// Comments returns the panel's current comment thread, for tests.
func (m *Model) Comments() []apptypes.Comment { return m.comments }

// commentSectionHeight is the number of body rows the comment thread + compose
// control occupy: a "Comments" label and one row per comment when there are
// any (a label is shown only when there is at least one), plus the single-line
// compose input. Used to budget the notes textarea so the comments section
// never pushes the panel past its allotted height.
func (m *Model) commentSectionHeight() int {
	h := 0
	if len(m.comments) > 0 {
		h += 1 // "Comments" label
		h += len(m.comments)
	}
	h += 1 // compose input row
	return h
}

// resizeNotes budgets the notes textarea against the panel body height: the
// textarea fills whatever the fixed chrome (detailsChromeRows) and the
// comment section do not claim, so the panel never grows past its frame. The
// input width is kept to the panel body width so the compose field never
// widens the surface either.
func (m *Model) resizeNotes() {
	bodyH := chrome.PanelBodyHeight(m.body.Height)
	m.notes.SetWidth(chrome.PanelBodyWidth(m.body.DetailsWidth))
	m.notes.SetHeight(max(0, bodyH-detailsChromeRows-m.commentSectionHeight()))
	m.commentInput.SetWidth(max(0, chrome.PanelBodyWidth(m.body.DetailsWidth)))
}

func (m *Model) handleProgressKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "right", "l":
		m.cycleMode(1)
		return m, nil
	case "left", "h":
		m.cycleMode(-1)
		return m, nil
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

// save writes any dirty notes/progress, then closes the panel and refreshes
// the task tree. store.ProgressKind is derived only here, at the write
// boundary, so the component itself never retains a store row type.
func (m *Model) save() (tea.Model, tea.Cmd) {
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
// OS user, then appends it to the live thread so it renders immediately
// without waiting for the next poll refresh (docs/plan/task-comments.md §6,
// Commit 5). Comments are append-only — there is no edit or delete path — and
// the compose input is cleared on success. An empty (whitespace-only) note is a
// no-op so ctrl+enter can't insert a blank comment row.
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
	// Append the just-written comment to the in-memory thread for immediate
	// feedback; the next RefreshDetails (via the poll) reconciles the
	// authoritative ordering and any drift.
	now := time.Now().Unix()
	m.comments = append(m.comments, apptypes.Comment{
		ID:        id,
		TaskID:    m.taskID,
		Author:    osUser(),
		Note:      note,
		CreatedAt: now,
	})
	m.commentInput.SetValue("")
	m.errMsg = ""
	return m, nil
}

// osUser returns the current OS username for human-authored writes (comments),
// falling back to $USER/$LOGNAME when os/user.Current fails — some minimal
// containers lack /etc/passwd (docs/plan/task-comments.md §1). Mirrors the
// CLI's osUser so human attribution is consistent across surfaces.
func osUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return os.Getenv("LOGNAME")
}
