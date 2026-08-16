package aboutmodal

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/banner"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/components/chrome"
	"github.com/filipemolina/farol/src/constants"
)

// Model is the about modal: it displays the figlet banner and basic app info.
type Model struct {
	width int
}

// New creates a new about modal.
func New(width int) tea.Model {
	return Model{width: width}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyPressMsg:
		// Any key closes the about modal
		return m, cmds.CloseModal(nil)
	}
	return m, nil
}

func (m Model) View() tea.View {
	// Render the figlet banner
	figlet := banner.Banner(appstyles.Active)

	// App info
	version := constants.Version()
	info := fmt.Sprintf("Version: %s\n\nA to-do list manager with agent support.\n\nPress any key to close.", version)

	infoStyle := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextPrimary).
		Align(lipgloss.Center)

	body := lipgloss.JoinVertical(lipgloss.Center,
		figlet,
		"",
		infoStyle.Render(info),
	)

	return tea.NewView(chrome.ModalSurface(appstyles.Active.ModalBg, body))
}
