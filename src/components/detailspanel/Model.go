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
	"strconv"

	"charm.land/bubbles/v2/textarea"
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

	focus             int
	errMsg            string
	confirmingDiscard bool
}

// New builds an unloaded Details panel. It does not fetch a task — AppModel
// sends a RefreshDetailsMsg once it opens the panel, and that hydrates it.
func New(s *store.Store) tea.Model {
	return &Model{
		store: s,
		notes: textarea.New(),
		focus: focusNotes,
	}
}

func (m *Model) Init() tea.Cmd { return textarea.Blink }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmds.SetBodyLayoutMsg:
		m.body = msg
		m.notes.SetWidth(chrome.PanelBodyWidth(msg.DetailsWidth))
		m.notes.SetHeight(max(0, chrome.PanelBodyHeight(msg.Height)-detailsChromeRows))
		return m, nil

	case cmds.SetFocusMsg:
		m.focused = int(msg) == focusedZoneID
		return m, nil

	case cmds.RefreshDetailsMsg:
		return m.handleRefresh(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Non-key messages (blink, etc.) keep the textarea live while Notes has
	// focus, mirroring the modal editor.
	if m.focus == focusNotes {
		var cmd tea.Cmd
		m.notes, cmd = m.notes.Update(msg)
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

	if m.notes.Value() != msg.Task.Notes {
		m.notes.SetValue(msg.Task.Notes)
	}
	m.origNotes = msg.Task.Notes

	if resetFocus {
		m.focus = focusNotes
		m.notes.Focus()
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

	case "tab":
		m.confirmingDiscard = false
		if m.focus == focusNotes {
			m.notes.Blur()
			m.focus = focusProgress
		} else {
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
