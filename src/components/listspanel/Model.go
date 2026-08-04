package listspanel

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/constants"
	"github.com/filipemolina/chore-crusher/src/keys"
)

// focusedZoneID is the zone id this component answers to.
const focusedZoneID = constants.COMPONENT_LISTS_PANEL

// Model is the lists-panel zone. It renders the store's lists as a bubbles
// list with the same card-style rows as stack-stitcher's groups list
// (docs/plans/stack-stitcher-sister-tui.md, phase B step 1).
type Model struct {
	focused       bool
	body          cmds.SetBodyLayoutMsg
	list          list.Model
	listDelegate  listDelegate
	work          map[string]apptypes.AgentActivity
	animFrame     int
}

func (m Model) Init() tea.Cmd { return nil }

// New builds the lists panel.
func New() tea.Model {
	l := list.New(nil, listDelegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowFilter(false)
	l.KeyMap = keys.ListKeyMap()
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
		m.listDelegate.isParentFocused = m.focused
		m.list.SetDelegate(m.listDelegate)

	case cmds.RefreshListsMsg:
		if msg.Err == nil {
			// Build the work map so the delegate can render spinners.
			m.work = make(map[string]apptypes.AgentActivity, len(msg.Activities))
			for _, a := range msg.Activities {
				m.work[a.EntityID] = a
			}
			m.listDelegate.work = m.work
			m.listDelegate.animFrame = m.animFrame
			m.list.SetDelegate(m.listDelegate)

			items := make([]list.Item, len(msg.Lists))
			for i, l := range msg.Lists {
				items[i] = l
			}
			m.list.SetItems(items)
			if len(items) > 0 && m.list.Index() < 0 {
				m.list.Select(0)
				return m, m.selectList()
			}
		}

	case cmds.AnimFrameMsg:
		m.animFrame = msg.Frame
		m.listDelegate.animFrame = msg.Frame
		m.list.SetDelegate(m.listDelegate)

	case tea.KeyPressMsg:
		if !m.focused {
			// An unfocused panel must not react to keys: letting the inner
			// list consume them would navigate the lists while the user
			// types j/k into the task tree's create input.
			return m, nil
		}
		previousIndex := m.list.Index()
		m.list, cmd = m.list.Update(msg)
		if m.list.Index() != previousIndex {
			if selCmd := m.selectList(); selCmd != nil {
				cmd = tea.Batch(cmd, selCmd)
			}
		}
		return m, cmd
	}

	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// SelectedListID returns the id of the list currently selected in the panel,
// for model-level behavior tests.
func (m Model) SelectedListID() string {
	if sel, ok := m.list.SelectedItem().(apptypes.ListSummary); ok {
		return sel.List.ID
	}
	return ""
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

// OwnsKeyboard reports whether the list is taking every keystroke for itself,
// which it does while the user is typing a filter: n, d and q are letters then,
// not commands. Only while typing - once a filter is applied and the cursor is
// back in the rows, the panel keys mean what they always mean, and esc clears
// the filter. See AppModel.keyboardOwned.
func (m Model) OwnsKeyboard() bool {
	return m.list.FilterState() == list.Filtering
}

// KeepsEsc reports whether the list needs esc for itself: an applied filter
// is cleared by esc alone, and the key only reaches the list while the list
// is focused. AppModel's "back" checks this before it takes focus away.
func (m Model) KeepsEsc() bool {
	return m.focused && m.list.FilterState() == list.FilterApplied
}
