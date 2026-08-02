package tasktree

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-completer/src/apptypes"
	"github.com/filipemolina/chore-completer/src/cmds"
	"github.com/filipemolina/chore-completer/src/components/chrome"
	"github.com/filipemolina/chore-completer/src/keys"
	"github.com/sahilm/fuzzy"
)

// Model is the task-tree zone with hierarchical rendering, navigation,
// and collapse state. Selection is preserved across refreshes by id
// (docs/DESIGN.md §7), collapsed state is view-only and not persisted
// (docs/plans/phase-4-task-tree.md step 1). Phase 8 adds the local `/`
// fuzzy filter: while a filter is active the visible rows are narrowed to
// each match plus its ancestor chain, and the collapse state stops driving
// visibility in favour of the filter set (docs/plans/phase-8-search.md).
type Model struct {
	focused    bool
	body       cmds.SetBodyLayoutMsg
	rows       []apptypes.Row
	selectedID string
	activeList bool
	collapsed  map[string]bool // view-only collapse state, taskID -> is collapsed

	// Local /-filter state. filterTyping is true while the filter input is
	// open and receiving characters; filterApplied is true once enter has
	// locked a non-empty query in. Either being set makes the filter active
	// (displayedRows narrows to the query). esc clears filterTyping while
	// typing (unfiltered), or clears an applied filter afterwards.
	filterTyping  bool
	filterApplied bool
	filterQuery   string
	filterInput   textinput.Model
	// pendingSelect is a SelectTaskMsg for a task not yet in rows (the
	// picker jumped to a list that has not loaded); applyRows honours it on
	// the first refresh that contains the id.
	pendingSelect string
}

func (m Model) Init() tea.Cmd { return nil }

// New builds the task tree.
func New() tea.Model {
	return Model{
		collapsed:   make(map[string]bool),
		filterInput: textinput.New(),
	}
}

// Rows returns the tree's current (unfiltered) rows. The model's tests read
// it to check the poll cycle end to end; phase 4's tree reads the same field
// internally.
func (m Model) Rows() []apptypes.Row { return m.rows }

// SelectedID returns the currently selected task id, for tests and the
// cross-list picker's jump verification.
func (m Model) SelectedID() string { return m.selectedID }

// applyRows replaces the tree's rows, preserving the selection across the
// refresh: the selected task is matched by id, not by row index, and when
// the selected id is gone (deleted, or the active list changed) the
// selection falls back to the nearest surviving row by position
// (docs/DESIGN.md §7). Getting this right against a placeholder list is
// easier than retrofitting it once phase 4's real tree exists.
func (m *Model) applyRows(rows []apptypes.Row) {
	if len(rows) == 0 {
		m.rows = nil
		m.selectedID = ""
		return
	}

	oldIndex := slices.IndexFunc(m.rows, func(r apptypes.Row) bool {
		return r.Task.ID == m.selectedID
	})

	m.rows = rows

	// A pending select (the picker's jump) outranks selection-preservation:
	// if the new rows contain the requested id, land on it and clear the
	// pending request.
	if m.pendingSelect != "" {
		if idx := slices.IndexFunc(rows, func(r apptypes.Row) bool {
			return r.Task.ID == m.pendingSelect
		}); idx >= 0 {
			m.selectedID = m.pendingSelect
			m.pendingSelect = ""
			return
		}
	}

	if slices.IndexFunc(rows, func(r apptypes.Row) bool {
		return r.Task.ID == m.selectedID
	}) >= 0 {
		return // selection survived by id
	}

	// Nearest surviving row: clamp the old position into the new list.
	if oldIndex < 0 {
		oldIndex = 0
	}
	idx := min(oldIndex, len(rows)-1)
	m.selectedID = rows[idx].Task.ID
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmds.SetBodyLayoutMsg:
		m.body = msg
		m.filterInput.SetWidth(gutterFilterWidth(m.body.MainWidth))

	case cmds.SetFocusMsg:
		m.focused = int(msg) == focusedZoneID

	case cmds.ActivateFilterMsg:
		// / from anywhere: enter the filter input, carrying any applied query
		// forward so the user refines rather than starts over.
		if len(m.rows) > 0 {
			m.filterTyping = true
			m.filterApplied = false
			m.filterInput.SetValue(m.filterQuery)
			m.filterInput.CursorEnd()
			return m, m.filterInput.Focus()
		}

	case cmds.SelectTaskMsg:
		// The global picker's jump. Select immediately when the task is
		// already in rows; otherwise remember it and let the next refresh
		// (after the list switch) land on it via applyRows.
		m.pendingSelect = msg.TaskID
		if row := m.findRow(msg.TaskID); row != nil {
			m.pendingSelect = ""
			if m.selectedID != msg.TaskID {
				m.selectedID = msg.TaskID
				return m, cmds.SetSelection(row.Task.ID, row.Depth)
			}
		}

	case cmds.RefreshTasksMsg:
		if msg.Err != nil {
			return m, nil
		}
		m.activeList = msg.ListID != ""
		m.applyRows(msg.Rows)
		// Broadcast the current selection's depth to add-input
		if row := m.findRow(m.selectedID); row != nil {
			return m, cmds.SetSelection(row.Task.ID, row.Depth)
		}

	case tea.KeyPressMsg:
		if !m.focused || len(m.rows) == 0 {
			return m, nil
		}

		// While the filter input is open it claims the keyboard.
		if m.filterTyping {
			return m.handleFilterKey(msg)
		}

		// When a filter is applied (not typing), esc clears it back to the
		// unfiltered view; navigation moves through the filtered rows.
		if m.filterApplied && key.Matches(msg, keys.Overlay.Cancel) {
			m.clearFilter()
			return m, nil
		}

		oldSelection := m.selectedID
		switch {
		case key.Matches(msg, keys.Tree.Navigate):
			// Determine direction based on key
			if msg.String() == "up" || msg.String() == "k" {
				m.moveSelection(-1)
			} else if msg.String() == "down" || msg.String() == "j" {
				m.moveSelection(1)
			}
		case key.Matches(msg, keys.Tree.Collapse):
			m.toggleCollapse(false)
		case key.Matches(msg, keys.Tree.Expand):
			m.toggleCollapse(true)
		case key.Matches(msg, keys.Tree.Toggle):
			return m, m.toggleComplete()
		case key.Matches(msg, keys.Tree.OpenDetails):
			if m.selectedID != "" {
				return m, cmds.OpenDetails(m.selectedID)
			}
		}

		// If selection changed, broadcast it to add-input
		if m.selectedID != oldSelection {
			if row := m.findRow(m.selectedID); row != nil {
				return m, cmds.SetSelection(row.Task.ID, row.Depth)
			}
		}

	case cmds.CloseModalMsg:
		// A modal closing returns the tree to an unfiltered view so a
		// leftover query cannot surprise the user after the picker closes
		// (docs/plans/phase-8-search.md's independence case).
		m.clearFilter()
	}

	return m, nil
}

// filterActive reports whether the /-filter is narrowing the visible rows:
// while typing (live preview) or after enter locked a query in.
func (m Model) filterActive() bool {
	return m.filterTyping || (m.filterApplied && m.filterQuery != "")
}

// handleFilterKey handles a keystroke while the filter input is open: esc
// cancels back to the unfiltered view, enter applies the query and leaves
// the filtered view active, anything else types into the input.
func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// enter applies a non-empty query and leaves the filtered view active,
	// with esc now clearing it.
	if key.Matches(msg, keys.Overlay.Submit) {
		if strings.TrimSpace(m.filterInput.Value()) == "" {
			// Empty apply: keep typing rather than locking in nothing.
			return m, nil
		}
		m.filterQuery = m.filterInput.Value()
		m.filterApplied = true
		m.filterTyping = false
		m.filterInput.Blur()
		return m, nil
	}

	// esc while typing cancels and restores the unfiltered view.
	if key.Matches(msg, keys.Overlay.Cancel) {
		m.clearFilter()
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	return m, cmd
}

// clearFilter returns the tree to its unfiltered view, used by esc while
// typing, esc on an applied filter, and when the modal layers close.
func (m *Model) clearFilter() {
	m.filterTyping = false
	m.filterApplied = false
	m.filterQuery = ""
	m.filterInput.Blur()
	m.filterInput.Reset()
}

// moveSelection moves the cursor by delta visible rows. When a filter is
// active the cursor moves through the filtered set, not the full tree.
func (m *Model) moveSelection(delta int) {
	visible := m.displayedRows()
	current := m.visibleIndex(m.selectedID)
	if current < 0 {
		current = 0
	}
	next := current + delta
	if next < 0 {
		next = 0
	}
	if next >= len(visible) {
		next = len(visible) - 1
	}
	if next >= 0 && next < len(visible) {
		m.selectedID = visible[next].Task.ID
	}
}

// displayedRows returns the rows the cursor and renderer should operate on:
// the filter set while a /-filter is active (typing or applied), otherwise
// the normal collapse-aware visible rows.
func (m *Model) displayedRows() []apptypes.Row {
	if m.filterActive() {
		return filterMatches(m.rows, m.filterQuery)
	}
	return m.visibleRows()
}

// visibleRows returns rows that should be rendered (collapsed nodes' children hidden).
func (m *Model) visibleRows() []apptypes.Row {
	var visible []apptypes.Row
	for _, row := range m.rows {
		if row.Depth == 0 || !m.isParentCollapsed(row) {
			visible = append(visible, row)
		}
	}
	return visible
}

// isParentCollapsed returns true if any ancestor of this row is collapsed.
func (m *Model) isParentCollapsed(row apptypes.Row) bool {
	if row.Task.ParentID == nil {
		return false
	}
	for _, r := range m.rows {
		if r.Task.ID == *row.Task.ParentID && m.collapsed[r.Task.ID] {
			return true
		}
	}
	return false
}

// visibleIndex returns the index of a task in the currently displayed rows
// (filtered set while a filter is active, else the collapse-aware rows).
func (m *Model) visibleIndex(taskID string) int {
	for i, r := range m.displayedRows() {
		if r.Task.ID == taskID {
			return i
		}
	}
	return -1
}

// toggleCollapse toggles or sets the collapse state of the selected row.
func (m *Model) toggleCollapse(expand bool) {
	row := m.findRow(m.selectedID)
	if row == nil || !row.HasChildren {
		return
	}
	if expand {
		delete(m.collapsed, m.selectedID)
	} else if row.Depth == 0 || !m.isParentCollapsed(*row) {
		m.collapsed[m.selectedID] = true
	}
}

// findRow returns the Row for the given task ID.
func (m *Model) findRow(taskID string) *apptypes.Row {
	for i, r := range m.rows {
		if r.Task.ID == taskID {
			return &m.rows[i]
		}
	}
	return nil
}

// toggleComplete asks AppModel to toggle the selected task. The actual
// store.Toggle call lives in AppModel so the tree stays decoupled from the
// store; AppModel refreshes the rows immediately after a successful toggle
// (docs/DESIGN.md §5, §9).
func (m *Model) toggleComplete() tea.Cmd {
	if m.selectedID == "" {
		return nil
	}
	return cmds.ToggleTask(m.selectedID)
}

// filterMatches returns the rows the /-filter keeps visible for a query: every
// row whose title fuzzy-matches plus every ancestor of every such row, in the
// tree's original order. An ancestor that does not itself match stays visible,
// so a matched leaf never floats without a visible parent anchor
// (docs/plans/phase-8-search.md step 1). An empty query shows everything.
func filterMatches(rows []apptypes.Row, query string) []apptypes.Row {
	visible, _ := matchVisible(rows, query)
	return visible
}

// matchVisible returns the rows a /-filter keeps visible plus the set of
// directly-matched row ids. The direct set is what a renderer highlights; an
// ancestor that is visible only to anchor a match is outside it and gets
// dimmed. An empty query shows every row with an empty direct set.
func matchVisible(rows []apptypes.Row, query string) ([]apptypes.Row, map[string]bool) {
	q := strings.TrimSpace(query)
	if q == "" {
		return rows, make(map[string]bool)
	}

	direct := make(map[string]bool, len(rows))
	titles := make([]string, len(rows))
	for i, r := range rows {
		titles[i] = r.Task.Title
	}
	for _, m := range fuzzy.Find(q, titles) {
		direct[rows[m.Index].Task.ID] = true
	}

	// visible = direct plus every ancestor of every direct match, so a matched
	// leaf never loses its parent chain.
	visible := make(map[string]bool, len(direct))
	for id := range direct {
		visible[id] = true
	}
	parentOf := make(map[string]string, len(rows))
	for _, r := range rows {
		if r.Task.ParentID != nil {
			parentOf[r.Task.ID] = *r.Task.ParentID
		}
	}
	for id := range direct {
		for pid, ok := parentOf[id]; ok && pid != ""; pid, ok = parentOf[pid] {
			visible[pid] = true
		}
	}

	out := make([]apptypes.Row, 0, len(visible))
	for _, r := range rows {
		if visible[r.Task.ID] {
			out = append(out, r)
		}
	}
	return out, direct
}

// gutterFilterWidth is the width the /-filter input renders at inside the
// tree's frame width: the whole body width minus the glyph/space prefix it
// shares with the task rows (docs/DESIGN.md §12's shared frame columns).
func gutterFilterWidth(mainWidth int) int {
	w := chrome.PanelBodyWidth(mainWidth)
	return max(0, w-6)
}
