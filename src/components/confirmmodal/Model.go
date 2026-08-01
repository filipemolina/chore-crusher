package confirmmodal

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-completer/src/appstyles"
	"github.com/filipemolina/chore-completer/src/cmds"
	"github.com/filipemolina/chore-completer/src/components/chrome"
	"github.com/filipemolina/chore-completer/src/store"
)

// Model is a confirmation modal for destructive operations.
type Model struct {
	title      string
	message    string
	listID     string // ID of the list to delete (for now, only for delete list)
	store      *store.Store
	yesHover   bool   // whether "yes" is highlighted
}

// New creates a new confirmation modal.
func New(title, message, listID string, s *store.Store) tea.Model {
	return Model{
		title:    title,
		message:  message,
		listID:   listID,
		store:    s,
		yesHover: true, // default focus on "no" (safer default)
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "tab", "right", "l":
			m.yesHover = !m.yesHover
			return m, nil
		case "shift+tab", "left", "h":
			m.yesHover = !m.yesHover
			return m, nil
		case "y", "Y":
			m.yesHover = true
			return m, m.confirmCmd()
		case "n", "N":
			m.yesHover = false
			return m, cmds.CloseModal(nil)
		case "enter":
			if m.yesHover {
				return m, m.confirmCmd()
			}
			return m, cmds.CloseModal(nil)
		case "esc":
			return m, cmds.CloseModal(nil)
		}
	}
	return m, nil
}

// confirmCmd creates a command that deletes the list.
func (m Model) confirmCmd() tea.Cmd {
	return cmds.CloseModal(func() tea.Msg {
		if m.store == nil || m.listID == "" {
			return nil
		}
		err := m.store.DeleteList(m.listID)
		if err != nil {
			// TODO: surface error to user
			return nil
		}
		// Refresh lists after deleting
		return cmds.RefreshLists(m.store)()
	})
}

func (m Model) View() tea.View {
	yesStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)
	noStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)

	if m.yesHover {
		yesStyle = yesStyle.Foreground(appstyles.Active.Accent)
	} else {
		noStyle = noStyle.Foreground(appstyles.Active.Accent)
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		yesStyle.Render("[ y yes ]"),
		"  ",
		noStyle.Render("[ n no ]"),
	)

	body := lipgloss.JoinVertical(lipgloss.Left,
		chrome.ModalTitle(m.title),
		m.message,
		"",
		buttons,
	)

	return tea.NewView(chrome.ModalSurface(appstyles.Active.ModalBg, body))
}
