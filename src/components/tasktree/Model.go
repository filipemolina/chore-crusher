package tasktree

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-completer/src/apptypes"
	"github.com/filipemolina/chore-completer/src/cmds"
)

// Model is the task-tree zone. Phase 3's job is the shell around it: the
// frame, the focus state, and the cursor-preservation rule from
// docs/DESIGN.md §7 are all implemented now, against a placeholder body —
// phase 4 (docs/plans/phase-4-task-tree.md) replaces the body with the real
// custom tree renderer, and the selection logic below is exactly what it
// will keep.
type Model struct {
	focused    bool
	body       cmds.SetBodyLayoutMsg
	rows       []apptypes.Row
	selectedID string
	activeList bool
}

func (m Model) Init() tea.Cmd { return nil }

// New builds the placeholder task tree.
func New() tea.Model { return Model{} }

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
	}

	return m, nil
}
