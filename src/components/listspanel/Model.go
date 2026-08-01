package listspanel

import (
	"fmt"
	"io"
	"strconv"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-completer/src/appstyles"
	"github.com/filipemolina/chore-completer/src/apptypes"
	"github.com/filipemolina/chore-completer/src/cmds"
	"github.com/filipemolina/chore-completer/src/components/chrome"
	"github.com/filipemolina/chore-completer/src/constants"
	"github.com/filipemolina/chore-completer/src/keys"
)

// focusedZoneID is the zone id this component answers to.
const focusedZoneID = constants.COMPONENT_LISTS_PANEL

// Model is the lists-panel zone. Phase 3 renders the lists the store holds —
// enough to verify the TUI and CLI share one store (docs/plans/phase-3-tui-shell.md
// "Killing and restarting the TUI against the same database file shows the
// same lists and counts phase 2's CLI created") — as plain rows inside the
// shared frame. Phase 6 (docs/plans/phase-6-lists-panel.md) replaces the
// body with the real bubbles list; the frame and focus handling below are
// what it keeps.
type Model struct {
	focused bool
	body    cmds.SetBodyLayoutMsg
	list    list.Model
}

func (m Model) Init() tea.Cmd { return nil }

// New builds the lists panel.
func New() tea.Model {
	l := list.New(nil, listDelegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowFilter(false)
	return Model{list: l}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case cmds.SetBodyLayoutMsg:
		m.body = msg
		m.list.SetSize(chrome.PanelBodyWidth(msg.ListsWidth), chrome.PanelBodyHeight(msg.Height))

	case cmds.SetFocusMsg:
		m.focused = int(msg) == focusedZoneID

	case cmds.RefreshListsMsg:
		if msg.Err == nil {
			items := make([]list.Item, len(msg.Lists))
			for i, l := range msg.Lists {
				items[i] = l
			}
			m.list.SetItems(items)
			if len(items) > 0 && m.list.Index() == 0 {
				m.selectList()
			}
		}

	case tea.KeyMsg:
		if m.focused && len(m.list.Items()) > 0 {
			if m.matchesKey(msg, keys.Lists.Navigate) {
				m.list, cmd = m.list.Update(msg)
				m.selectList()
				return m, cmd
			}
		}
	}

	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// selectList broadcasts which list is selected to AppModel.
func (m Model) selectList() tea.Cmd {
	if len(m.list.Items()) == 0 {
		return nil
	}
	if sel, ok := m.list.SelectedItem().(apptypes.ListSummary); ok {
		return cmds.SelectList(sel.List.ID)
	}
	return nil
}

// matchesKey reports whether msg matches the binding.
func (m Model) matchesKey(msg tea.KeyMsg, binding key.Binding) bool {
	for _, k := range binding.Keys() {
		if msg.String() == k {
			return true
		}
	}
	return false
}

// listDelegate renders each list as one row: name, then the pending/complete counts.
type listDelegate struct{}

func (d listDelegate) Height() int                             { return 1 }
func (d listDelegate) Spacing() int                            { return 0 }
func (d listDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d listDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	l, ok := listItem.(apptypes.ListSummary)
	if !ok {
		return
	}

	nameStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)
	countStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextDim)

	if index == m.Index() {
		nameStyle = nameStyle.Foreground(appstyles.Active.Accent)
	}

	fmt.Fprint(w, nameStyle.Render(l.List.Name)+"  "+countStyle.Render(
		strconv.Itoa(l.PendingCount)+" pending · "+strconv.Itoa(l.CompleteCount)+" done"))
}

// View renders each list as one row.
func (m Model) View() tea.View {
	width := chrome.PanelBodyWidth(m.body.ListsWidth)
	height := chrome.PanelBodyHeight(m.body.Height)

	var body string
	if len(m.list.Items()) == 0 {
		body = chrome.EmptyStateCard("no lists yet", width, height)
	} else {
		body = m.list.View()
	}

	return tea.NewView(chrome.PanelFrame(m.focused, width, height, body))
}
