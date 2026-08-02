package listnamemodal

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/store"
)

// Mode is whether the modal is for creating a new list or renaming an existing one.
type Mode int

const (
	ModeNew Mode = iota
	ModeRename
)

// Model is the list name input modal (create or rename).
type Model struct {
	mode   Mode
	listID string
	input  textinput.Model
	store  *store.Store
}

// New creates a new list name modal.
func New(mode Mode, listID string, s *store.Store) tea.Model {
	input := textinput.New()
	input.Focus()
	if mode == ModeRename {
		input.Placeholder = "new name"
	}
	return Model{mode: mode, listID: listID, input: input, store: s}
}

func (m Model) Init() tea.Cmd { return textinput.Blink }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			name := strings.TrimSpace(m.input.Value())
			if name == "" {
				return m, nil
			}
			followCmd := m.createFollowCmd(name)
			return m, cmds.CloseModal(followCmd)
		case "esc":
			return m, cmds.CloseModal(nil)
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// createFollowCmd creates a command that creates or renames the list.
func (m Model) createFollowCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return nil
		}

		if m.mode == ModeNew {
			_, err := m.store.CreateList(name)
			if err != nil {
				// TODO: surface error to user
				return nil
			}
		} else {
			err := m.store.RenameList(m.listID, name)
			if err != nil {
				// TODO: surface error to user
				return nil
			}
		}

		// Refresh lists after creating/renaming
		return cmds.RefreshLists(m.store)()
	}
}

func (m Model) View() tea.View {
	title := "New list"
	if m.mode == ModeRename {
		title = "Rename list"
	}

	hintStyle := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextMuted)

	body := lipgloss.JoinVertical(lipgloss.Left,
		chrome.ModalTitle(title),
		m.input.View(),
		"",
		hintStyle.Render("↵ submit  esc cancel"),
	)

	return tea.NewView(chrome.ModalSurface(appstyles.Active.ModalBg, body))
}
