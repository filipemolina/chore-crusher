package addinput

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-completer/src/cmds"
	"github.com/filipemolina/chore-completer/src/components/chrome"
)

// focusedZoneID is the zone id this component answers to
// (constants.COMPONENT_ADD_INPUT).
const focusedZoneID = 2

// Model is the add-input zone, pinned to the bottom of the main panel and
// always visible (docs/DESIGN.md §5). Phase 3 renders the placeholder body
// inside the shared frame; phase 5 (docs/plans/phase-5-add-input.md) puts
// the real text input and its keys in here. The AddInput group of keys is
// declared in src/keys already, waiting for it.
type Model struct {
	focused bool
	body    cmds.SetBodyLayoutMsg
}

func (m Model) Init() tea.Cmd { return nil }

// New builds the placeholder add input.
func New() tea.Model { return Model{} }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmds.SetBodyLayoutMsg:
		m.body = msg

	case cmds.SetFocusMsg:
		m.focused = int(msg) == focusedZoneID
	}

	return m, nil
}

// View renders the placeholder. The body is a single line, so the zone reads
// as one input row inside its frame — the fixed ADD_INPUT_HEIGHT rows come
// from the layout, not from any padding here.
func (m Model) View() tea.View {
	width := chrome.PanelBodyWidth(m.body.MainWidth)
	height := chrome.PanelBodyHeight(m.body.InputHeight)

	body := "(add input — phase 5)"

	return tea.NewView(chrome.PanelFrame(m.focused, width, height, body))
}
