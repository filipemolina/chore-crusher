package listspanel

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/components/chrome"
	"github.com/filipemolina/farol/src/constants"
	"github.com/filipemolina/farol/src/keys"
)

// focusedZoneID is the zone id this component answers to.
const focusedZoneID = constants.COMPONENT_LISTS_PANEL

// Model is the lists-panel zone. It renders the store's lists as a bubbles
// list with the same card-style rows as stack-stitcher's groups list
// (phase B step 1).
type Model struct {
	focused      bool
	body         cmds.SetBodyLayoutMsg
	list         list.Model
	listDelegate listDelegate
	work         map[string]apptypes.AgentActivity
	claimedLists map[string]bool
	animFrame    int
}

func (m Model) Init() tea.Cmd { return nil }

// New builds the lists panel.
func New() tea.Model {
	l := list.New(nil, listDelegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowFilter(true)
	l.SetFilteringEnabled(true)
	// The bubbles list ships dots at the bottom when there are multiple
	// pages; this app renders its own "N below" overflow indicator instead,
	// so the raw dots would read as debris (docs/DESIGN.md §12). The
	// paginator's key handling (G/g/pgup/pgdown) still works — only the
	// visual pagination view is suppressed.
	l.SetShowPagination(false)
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
			m.claimedLists = msg.ClaimedLists
			m.listDelegate.work = m.work
			m.listDelegate.claimedLists = m.claimedLists
			m.listDelegate.animFrame = m.animFrame
			m.list.SetDelegate(m.listDelegate)

			items := make([]list.Item, len(msg.Lists))
			for i, l := range msg.Lists {
				items[i] = l
			}
			prevSelected := m.SelectedListID()
			// SetItems resets filteredItems and, while a filter is active, returns
			// a command that re-runs the filter against the new items. That command
			// must be returned (not discarded) so the narrowed view survives the
			// refresh — otherwise the poll's periodic RefreshListsMsg wipes the
			// filter results and the panel snaps back to unfiltered mid-typing
			// (bug: the lists filter blanks/discards the query on refresh).
			cmd = m.list.SetItems(items)
			if len(items) > 0 && m.SelectedListID() != prevSelected && prevSelected != "" {
				// The refresh moved the highlight — the selected list was
				// deleted ahead of the cursor. bubbles' SetItems clamps a
				// stale cursor to the new last item, but silently: the
				// highlight moves without the SelectListMsg that keeps
				// AppModel's active list in step, leaving highlight and
				// active list out of sync until the next keypress. Re-broadcast
				// the new selection so they stay together.
				cmd = tea.Batch(cmd, m.selectList())
				return m, cmd
			}
			if len(items) > 0 && m.list.Index() < 0 {
				// First load: seed a valid highlight on the first list so the
				// panel never renders with nothing selected. Do NOT broadcast
				// SelectListMsg here — AppModel has already chosen the
				// active list (the saved last-active list, docs/DESIGN.md §7)
				// and aligns the panel to it via SelectListInPanelMsg. A
				// broadcast would pick index 0 and clobber that choice back to
				// the first list on every startup.
				m.list.Select(0)
			}
		}

	case cmds.SelectListInPanelMsg:
		// One-way startup alignment: highlight the active list AppModel
		// reopened from the Setting table. No broadcast — the panel is
		// following AppModel here, not driving it, so echoing back would loop.
		if msg.ListID != "" {
			for i, it := range m.list.Items() {
				if ls, ok := it.(apptypes.ListSummary); ok && ls.List.ID == msg.ListID {
					m.list.Select(i)
					break
				}
			}
		}

	case cmds.AnimFrameMsg:
		m.animFrame = msg.Frame
		m.listDelegate.animFrame = msg.Frame
		m.list.SetDelegate(m.listDelegate)

	case cmds.ActivateListFilterMsg:
		m.list.SetFilterState(list.Filtering)

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

// listsBelow returns the number of lists on pages after the current one —
// the count to advertise in the "N below" overflow footer. It is the total
// minus what the paginator has already placed before (and including) the
// current page's visible slice.
func (m Model) listsBelow() int {
	total := len(m.list.Items())
	first := m.list.Paginator.Page * m.list.Paginator.PerPage
	onPage := m.list.Paginator.ItemsOnPage(total)
	return max(0, total-first-onPage)
}

// FilterActive reports whether the panel's own filter is open or applied,
// for AppModel-level tests.
func (m Model) FilterActive() bool { return m.list.FilterState() != list.Unfiltered }

// FilterValue returns the current filter query text, for AppModel-level
// tests that need to prove a keystroke landed in the filter input rather
// than being swallowed or triggering a shortcut.
func (m Model) FilterValue() string { return m.list.FilterValue() }

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

// KeepsEsc reports whether the list needs esc for itself: typing in
// the filter or an applied filter both claim esc so the ladder
// doesn't steal it before the list can handle it.
func (m Model) KeepsEsc() bool {
	return m.focused && m.list.FilterState() != list.Unfiltered
}
