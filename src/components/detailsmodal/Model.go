package detailsmodal

import (
	"fmt"
	"strconv"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/store"
)

const (
	focusNotes = iota
	focusProgress
)

// Model is the details screen modal for editing task notes and progress.
type Model struct {
	taskID            string
	title             string
	listID            string
	store             *store.Store
	notes             textarea.Model
	origNotes         string
	progressKind      store.ProgressKind
	origProgressKind  store.ProgressKind
	percentInput      string
	origPercentInput  string
	derivedPct        int
	displayAsSimple   bool
	focus             int
	errMsg            string
	confirmingDiscard bool
}

// New creates a new details modal for the given task. It loads the task from
// the store and hydrates all fields.
func New(taskID, listID string, s *store.Store) tea.Model {
	t, err := s.GetTask(taskID)
	if err != nil {
		// Graceful fallback: modal with just the id, empty fields, error shown
		// on first render. In practice this shouldn't happen (AppModel opens
		// details only on a selected task that exists in the tree).
		return &Model{
			taskID: taskID,
			listID: listID,
			store:  s,
			notes:  textarea.New(),
			errMsg: fmt.Sprintf("failed to load task: %v", err),
		}
	}

	notes := textarea.New()
	notes.SetValue(t.Notes)
	notes.Focus()

	// Normalize ProgressNone (pending task) to simple for the UI.
	kind := store.ProgressKind(t.ProgressKind)
	if kind == store.ProgressNone {
		kind = store.ProgressSimple
	}

	// Fetch derived progress for subtasks mode display.
	derivedKind, derivedPct, displayAsSimple, _ := s.DerivedProgress(taskID)
	_ = derivedKind

	// Initialize percent buffer from stored value if percentage mode.
	percentInput := ""
	if kind == store.ProgressPercentage && t.ProgressPct != nil {
		percentInput = strconv.Itoa(*t.ProgressPct)
	}

	m := &Model{
		taskID:           taskID,
		title:            t.Title,
		listID:           listID,
		store:            s,
		notes:            notes,
		origNotes:        t.Notes,
		progressKind:     kind,
		origProgressKind: kind,
		percentInput:     percentInput,
		origPercentInput: percentInput,
		derivedPct:       derivedPct,
		displayAsSimple:  displayAsSimple,
		focus:            focusNotes,
	}

	// Set textarea styling per bubble tea v2 API.
	styles := textarea.DefaultDarkStyles()
	styles.Focused.Base = lipgloss.NewStyle().Background(appstyles.Active.ModalBg)
	styles.Blurred.Base = lipgloss.NewStyle().Background(appstyles.Active.ModalBg)
	m.notes.SetStyles(styles)

	return m
}

func (m *Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
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
			return m, cmds.CloseModal(nil)

		case "y", "Y":
			if m.confirmingDiscard {
				m.confirmingDiscard = false
				return m, cmds.CloseModal(nil)
			}
			// Pass through to textarea if in notes mode.
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
			// Pass through to textarea if in notes mode.
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

		// Notes zone: pass through to textarea, but don't intercept enter
		// (it should insert a newline).
		if m.focus == focusNotes {
			var cmd tea.Cmd
			m.notes, cmd = m.notes.Update(msg)
			return m, cmd
		}
	}

	// Non-key messages: always pass to textarea to handle focus blink, etc.
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
		if m.progressKind != store.ProgressPercentage {
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
		if m.progressKind != store.ProgressPercentage {
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
	modes := []store.ProgressKind{
		store.ProgressSimple,
		store.ProgressSubtasks,
		store.ProgressPercentage,
	}
	idx := -1
	for i, mode := range modes {
		if mode == m.progressKind {
			idx = i
			break
		}
	}
	if idx >= 0 {
		idx = (idx + int(delta) + len(modes)) % len(modes)
		m.progressKind = modes[idx]
		// Clear percent input when switching away from percentage mode.
		if m.progressKind != store.ProgressPercentage {
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

func (m *Model) save() (tea.Model, tea.Cmd) {
	// Save notes if dirty.
	if m.notes.Value() != m.origNotes {
		if err := m.store.SetNotes(m.taskID, m.notes.Value()); err != nil {
			m.errMsg = fmt.Sprintf("failed to save notes: %v", err)
			return m, nil
		}
	}

	// Save progress if dirty (and only if it was actually changed).
	if m.progressKind != m.origProgressKind || m.percentInput != m.origPercentInput {
		var pct *int
		if m.progressKind == store.ProgressPercentage {
			if m.percentInput == "" {
				pct = nil
			} else {
				val, _ := strconv.Atoi(m.percentInput)
				pct = &val
			}
		}
		if err := m.store.SetProgress(m.taskID, m.progressKind, pct); err != nil {
			m.errMsg = fmt.Sprintf("failed to save progress: %v", err)
			return m, nil
		}
	}

	// Close and refresh.
	return m, cmds.CloseModal(cmds.RefreshTasks(m.store, m.listID))
}

func (m *Model) View() tea.View {
	body := lipgloss.JoinVertical(lipgloss.Left,
		chrome.ModalTitle(m.title),
		"",
		chrome.ModalTitle("Notes"),
		m.renderNotesZone(),
		"",
		chrome.ModalTitle("Progress"),
		m.renderProgressZone(),
		m.renderErrorLine(),
		"",
		m.renderFooter(),
	)

	return tea.NewView(chrome.ModalSurface(appstyles.Active.ModalBg, body))
}

func (m *Model) renderNotesZone() string {
	return m.notes.View()
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
	case store.ProgressSimple:
		valueDisplay = ""
	case store.ProgressSubtasks:
		if m.displayAsSimple {
			valueDisplay = "(no children)"
		} else {
			valueDisplay = fmt.Sprintf("(%d%%)", m.derivedPct)
		}
	case store.ProgressPercentage:
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

func (m *Model) renderFooter() string {
	if m.confirmingDiscard {
		return lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted).Render("Discard changes? (y/n)")
	}

	var hint string
	if m.focus == focusNotes {
		hint = "tab to progress  ctrl+s save  esc cancel"
	} else {
		hint = "tab to notes  ←/→ cycle mode  ctrl+s save  esc cancel"
	}
	return lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted).Render(hint)
}
