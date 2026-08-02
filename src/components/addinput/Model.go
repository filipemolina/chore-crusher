package addinput

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/store"
)

// focusedZoneID is the zone id this component answers to
// (constants.COMPONENT_ADD_INPUT).
const focusedZoneID = 2

// Model is the add-input zone, pinned to the bottom of the main panel and
// always visible (docs/DESIGN.md §5). Phase 5 implements the full text input
// with level-selection behavior per docs/plans/phase-5-add-input.md.
type Model struct {
	focused       bool
	body          cmds.SetBodyLayoutMsg
	textinput     textinput.Model // bubbles/textinput v2
	levelOffset   int             // {-1, 0, +1}: relative to selected task depth
	selectedID    string          // ID of currently selected task
	selectedDepth int             // Depth of selected task (for validating shift+tab)
	store         *store.Store
	activeListID  string
}

func (m Model) Init() tea.Cmd { return nil }

// New builds the add input with an embedded textinput component.
func New(st *store.Store, activeListID string) tea.Model {
	ti := textinput.New()
	sty := textinput.DefaultDarkStyles()
	ti.SetStyles(sty)
	ti.Placeholder = "new task"

	return &Model{
		textinput:    ti,
		levelOffset:  0,
		store:        st,
		activeListID: activeListID,
	}
}

// nextOffset computes the new level offset after a keystroke, implementing
// the exact table from docs/plans/phase-5-add-input.md §2.
func nextOffset(current int, key string, selectedDepth int) int {
	if key == "tab" {
		return min(current+1, 1) // clamp to +1
	}
	if key == "shift+tab" {
		// shift+tab is a no-op when selectedDepth == 0 and current == 0
		// (already at root with default offset).
		if selectedDepth == 0 && current == 0 {
			return 0
		}
		// Otherwise, only decrement if the resulting level would be valid.
		// The "resulting level" is selectedDepth + current - 1 (after decrement).
		// This must be >= -1 (the root's parent level).
		resultingDepth := selectedDepth + current - 1
		if resultingDepth >= -1 {
			return max(current-1, -1) // clamp to -1
		}
		return current // no-op: would go invalid
	}
	return current // any other key: no change
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmds.SetBodyLayoutMsg:
		m.body = msg
		m.textinput.SetWidth(chrome.PanelBodyWidth(msg.MainWidth) - 4) // account for glyph + space + indent

	case cmds.SetFocusMsg:
		m.focused = int(msg) == focusedZoneID
		if m.focused {
			return m, m.textinput.Focus()
		}
		m.textinput.Blur()

	case cmds.SetSelectionMsg:
		m.selectedID = msg.TaskID
		m.selectedDepth = msg.Depth
		m.levelOffset = 0 // reset level on selection change (§4 spec)

	case tea.KeyPressMsg:
		if !m.focused {
			break
		}

		// Intercept tab/shift+tab for level offset logic
		if key.Matches(msg, key.NewBinding(key.WithKeys("tab"))) {
			m.levelOffset = nextOffset(m.levelOffset, "tab", m.selectedDepth)
			return m, nil
		}
		if key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))) {
			m.levelOffset = nextOffset(m.levelOffset, "shift+tab", m.selectedDepth)
			return m, nil
		}

		// Intercept enter: create task if text is non-empty
		if key.Matches(msg, key.NewBinding(key.WithKeys("enter"))) {
			title := m.textinput.Value()
			if title == "" {
				return m, nil // no-op on empty input
			}
			return m, m.createTaskCmd()
		}

		// Intercept esc: clear if text present, otherwise fall through to esc ladder
		if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
			if m.textinput.Value() != "" {
				m.textinput.Reset()
				m.levelOffset = 0
				return m, nil
			}
			// Fall through: don't consume esc if input is empty
			break
		}

		// Forward other keys to textinput
		var cmd tea.Cmd
		m.textinput, cmd = m.textinput.Update(msg)
		return m, cmd
	}

	// Forward non-key messages to textinput
	var cmd tea.Cmd
	m.textinput, cmd = m.textinput.Update(msg)
	return m, cmd
}

// createTaskCmd creates a command that creates a new task based on levelOffset
// and the selected task, then refreshes the tree and moves selection to the new task.
func (m *Model) createTaskCmd() tea.Cmd {
	return func() tea.Msg {
		if m.store == nil || m.activeListID == "" {
			return nil // can't create without store or list
		}

		title := m.textinput.Value()
		if title == "" {
			return nil
		}

		// Resolve the parent ID based on levelOffset and selectedID
		parentID, afterID, err := m.resolveParentAndAfter()
		if err != nil {
			// TODO: surface error to user
			return nil
		}

		// Create the task
		newID, err := m.store.CreateTaskAfter(m.activeListID, title, parentID, "", afterID)
		if err != nil {
			// TODO: surface error to user
			return nil
		}

		// Clear the input and reset level
		m.textinput.Reset()
		m.levelOffset = 0

		// Return a batch of commands: refresh and set selection
		return cmds.CreateTaskMsg{
			NewID: newID,
			Depth: m.selectedDepth + m.levelOffset,
		}
	}
}

// resolveParentAndAfter computes the parentID and afterID based on levelOffset
// and the current selected task (docs/plans/phase-5-add-input.md §3).
func (m *Model) resolveParentAndAfter() (*string, string, error) {
	if m.selectedID == "" {
		// No task selected; create at root level
		return nil, "", nil
	}

	selectedTask, err := m.store.GetTask(m.selectedID)
	if err != nil {
		return nil, "", err
	}

	switch m.levelOffset {
	case 0:
		// Default: sibling of selected task
		// New task's parent = selected task's parent
		// Insert immediately after selected task
		return selectedTask.ParentID, m.selectedID, nil

	case 1:
		// Child of selected task
		// New task's parent = selected task's id
		// Insert as last child (empty afterID means append)
		return &selectedTask.ID, "", nil

	case -1:
		// Sibling of selected task's parent
		// Get the grandparent (parent of parent)
		var grandparentID *string
		var afterID string
		if selectedTask.ParentID != nil {
			parentTask, err := m.store.GetTask(*selectedTask.ParentID)
			if err != nil {
				return nil, "", err
			}
			grandparentID = parentTask.ParentID
			afterID = *selectedTask.ParentID
		}
		// Insert immediately after the selected task's parent
		return grandparentID, afterID, nil
	}

	return nil, "", nil
}

// glyphForOffset returns the leading glyph for the current level offset
// (docs/DESIGN.md §4 and docs/plans/phase-5-add-input.md §4).
func glyphForOffset(offset int) string {
	switch offset {
	case -1:
		return "^"
	case 0:
		return "-"
	case 1:
		return "+"
	default:
		return "-"
	}
}

// View renders the input with glyph, indentation, and textinput.
func (m Model) View() tea.View {
	width := chrome.PanelBodyWidth(m.body.MainWidth)
	height := chrome.PanelBodyHeight(m.body.InputHeight)

	indentDepth := m.selectedDepth + m.levelOffset
	indentWidth := 2 * indentDepth // 2 spaces per level
	indent := ""
	for i := 0; i < indentWidth; i++ {
		indent += " "
	}

	// Render the glyph and input together
	glyph := glyphForOffset(m.levelOffset)
	tiView := m.textinput.View()

	// Construct the body: indent + glyph + space + textinput
	// The textinput already includes its own prompt if set; we prepend the glyph instead
	body := indent + glyph + " " + tiView

	return tea.NewView(chrome.PanelFrame(m.focused, width, height, body))
}

// OwnsKeyboard reports whether this component claims esc, used by AppModel's
// esc ladder (docs/DESIGN.md §5) to determine if the input should swallow esc
// or let it propagate (docs/plans/phase-5-add-input.md §6).
func (m Model) OwnsKeyboard() bool {
	return m.textinput.Value() != ""
}
