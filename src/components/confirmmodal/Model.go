package confirmmodal

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/components/chrome"
)

// Model is a confirmation modal for destructive operations. The onConfirm
// callback runs (after the modal closes) when the user accepts; keeping the
// action outside the modal lets one type serve every destructive verb —
// list delete, task delete, etc. — instead of growing a field per domain.
type Model struct {
	title     string
	message   string
	onConfirm func() tea.Msg
	yesHover  bool // whether "yes" is highlighted
}

// New creates a new confirmation modal. onConfirm runs inside a
// CloseModal follow-up, so callers build the delete + refresh as one
// func() tea.Msg (e.g. store.DeleteTask then RefreshTasks).
func New(title, message string, onConfirm func() tea.Msg) tea.Model {
	return Model{
		title:     title,
		message:   message,
		onConfirm: onConfirm,
		yesHover:  false, // default focus on "yes"; confirm modals are opt-in
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

// confirmCmd closes the modal and then runs the confirm callback,
// which performs the destructive action and returns the refresh message.
func (m Model) confirmCmd() tea.Cmd {
	return cmds.CloseModal(func() tea.Msg {
		if m.onConfirm != nil {
			return m.onConfirm()
		}
		return nil
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
