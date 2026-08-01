package tasktree

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-completer/src/apptypes"
	"github.com/filipemolina/chore-completer/src/cmds"
	"github.com/filipemolina/chore-completer/src/keys"
)

// Model is the task-tree zone with hierarchical rendering, navigation,
// and collapse state. Selection is preserved across refreshes by id
// (docs/DESIGN.md §7), collapsed state is view-only and not persisted
// (docs/plans/phase-4-task-tree.md step 1).
type Model struct {
	focused    bool
	body       cmds.SetBodyLayoutMsg
	rows       []apptypes.Row
	selectedID string
	activeList bool
	collapsed  map[string]bool // view-only collapse state, taskID -> is collapsed
}

func (m Model) Init() tea.Cmd { return nil }

// New builds the task tree.
func New() tea.Model { return Model{collapsed: make(map[string]bool)} }

// Rows returns the tree's current rows. The model's tests read it to check
// the poll cycle end to end; phase 4's tree reads the same field internally.
func (m Model) Rows() []apptypes.Row { return m.rows }

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

	case cmds.SetFocusMsg:
		m.focused = int(msg) == focusedZoneID

	case cmds.RefreshTasksMsg:
		if msg.Err != nil {
			return m, nil
		}
		m.activeList = msg.ListID != ""
		m.applyRows(msg.Rows)

	case tea.KeyPressMsg:
		if !m.focused || len(m.rows) == 0 {
			return m, nil
		}

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
		}
	}

	return m, nil
}

// moveSelection moves the cursor by delta visible rows.
func (m *Model) moveSelection(delta int) {
	current := m.visibleIndex(m.selectedID)
	if current < 0 {
		current = 0
	}
	next := current + delta
	visible := m.visibleRows()
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

// visibleIndex returns the index of a task in the visible rows list.
func (m *Model) visibleIndex(taskID string) int {
	for i, r := range m.visibleRows() {
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

// toggleComplete marks the selected task complete/pending and returns a command.
// TODO(phase 4): Integrate with store to persist changes.
func (m *Model) toggleComplete() tea.Cmd {
	row := m.findRow(m.selectedID)
	if row == nil {
		return nil
	}
	// For now, just return nil. Phase 4 needs to call store.Complete/Reopen
	// and immediately refresh the tree (not wait for the poll tick).
	return nil
}
