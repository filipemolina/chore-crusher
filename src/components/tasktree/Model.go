package tasktree

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/keys"
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
	// structureInFlight is true between the moment the tree emits a
	// structural change (indent/outdent/move/toggle/delete) and the moment
	// the resulting RefreshTasksMsg applies those rows. In that window
	// m.rows is stale relative to the store, so the row-derived gestures
	// defer instead of computing a target from data that no longer matches
	// what the user sees (bug 4: a second ] right after nesting a task
	// picked the just-moved task as its previous sibling and nested the
	// wrong task under it).
	structureInFlight bool
	// pendingGesture is the newest row-derived gesture (indent/outdent/
	// move) pressed while structureInFlight was true; the RefreshTasksMsg
	// handler replays it against the fresh rows. The task id is captured at
	// press time so a cursor move before the refresh cannot redirect the
	// gesture.
	pendingGesture *deferredGesture

	// Inline creation state. While creating is true the tree takes every
	// keystroke for itself and renders a special "new task" row at the
	// computed insertion point (task-row redesign + inline creation,
	// docs/plans/task-row-redesign-and-inline-creation.md).
	creating bool
	// createBeforeID is the data-insertion anchor: the task the new task is
	// created as a sibling of (at createLevelOffset's relationship). The
	// create row's visual position is computed by createRenderAnchorID,
	// which places it after the anchor's last visible descendant so the
	// card never splits a task from its children ("""") = append at end.
	createBeforeID    string
	createInput       textinput.Model
	createLevelOffset int // -1 = parent, 0 = sibling, +1 = child
	// createSuppressed remembers that the user esc-cancelled creating, so
	// the next refresh of the same empty list does not silently re-open the
	// input. Cleared when creating starts again (n) or the active list
	// changes (docs/plan/task-row-cards-and-status.md).
	createSuppressed bool
	activeListID     string // id of the list the rows belong to; a change clears createSuppressed

	// Agent presence: the live claim set and the current spinner frame.
	// work is keyed by entity_id for EntityType=="task" claims.
	work      map[string]apptypes.AgentActivity
	animFrame int

	// scrollOffset is the index of the first rendered line of the task-tree
	// line plan (see View.go). It is selection-driven: a Bubble Tea update
	// recomputes it to keep the selected row (or the inline create row) within
	// the visible window, never rendering — so long lists stay reachable with
	// the existing navigation keys (docs/DESIGN.md §§5, 6). There is no mouse
	// or page-key scrolling and no horizontal scroll.
	scrollOffset int
}

func (m Model) Init() tea.Cmd { return nil }

// New builds the task tree.
func New() tea.Model {
	return Model{
		collapsed:   make(map[string]bool),
		filterInput: textinput.New(),
	}
}

// OwnsKeyboard reports whether the tree is taking every keystroke for itself,
// which it does while the user is typing a filter: T, F and L are letters then,
// not commands. Only while typing - once a filter is applied and the cursor is
// back in the rows, the panel keys mean what they always mean, and esc clears
// the filter. See AppModel.keyboardOwned.
func (m Model) OwnsKeyboard() bool {
	return m.filterTyping || m.creating
}

// KeepsEsc reports whether the tree needs esc for itself: typing in the
// filter, an applied filter, or inline creating all claim esc so the
// ladder doesn't steal it before the tree can handle it.
func (m Model) KeepsEsc() bool {
	return m.focused && (m.filterTyping || m.filterApplied || m.creating)
}

// Rows returns the tree's current (unfiltered) rows. The model's tests read
// it to check the poll cycle end to end; phase 4's tree reads the same field
// internally.
func (m Model) Rows() []apptypes.Row { return m.rows }

// FilterActive reports whether the /-filter is narrowing rows (typing or
// applied), for AppModel-level tests.
func (m Model) FilterActive() bool { return m.filterActive() }

// SelectedID returns the currently selected task id, for tests and the
// cross-list picker's jump verification.
func (m Model) SelectedID() string { return m.selectedID }

// IsEmpty reports whether the tree has no rows right now. AppModel reads
// this for the keybinding bar, so the footer only advertises keys that make
// sense in the current state.
func (m Model) IsEmpty() bool {
	return !m.activeList || len(m.rows) == 0
}

// applyRows replaces the tree's rows, preserving the selection across the
// refresh: the selected task is matched by id, not by row index, and when
// the selected id is gone (deleted, or the active list changed) the
// selection falls back to the nearest surviving row by position
// (docs/DESIGN.md §7). Getting this right against a placeholder list is
// easier than retrofitting it once phase 4's real tree exists.
func (m *Model) applyRows(rows []apptypes.Row) {
	if len(rows) == 0 {
		hadRows := len(m.rows) > 0
		m.rows = nil
		m.selectedID = ""
		// Deleting every remaining task re-opens the empty list's input even
		// after an esc cancel: createSuppressed means "a refresh must not
		// undo my esc", not "never show the input on this list again" — the
		// list becoming empty is one of the two ways the input comes back, n
		// being the other (docs/plan/task-row-cards-and-status.md).
		if hadRows {
			m.createSuppressed = false
		}
		// An empty active list auto-shows the inline input unless the user
		// just esc-cancelled it (createSuppressed): a refresh must not undo
		// the user's cancel.
		if m.activeList && !m.creating && !m.createSuppressed {
			m.startCreating("")
		}
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

// Update handles a message and then keeps the vertical scroll offset in step
// with the current selection. All the real work is in updateInner; this wrapper
// only recomputes scrollOffset so the selected row (or the inline create row)
// stays visible after navigation, refresh, filter, collapse/expand, create
// start/cancel/confirm, and layout changes — the one place scroll state
// changes, per docs/DESIGN.md §§5/6 (a Bubble Tea update, never rendering).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.updateInner(msg)
	var tm Model
	switch v := updated.(type) {
	case Model:
		tm = v
	case *Model:
		tm = *v
	default:
		return updated, cmd
	}
	tm.scrollOffset = tm.scrollFor(tm.scrollOffset)
	return tm, cmd
}

func (m Model) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		// Build the work map for spinner rendering.
		m.work = make(map[string]apptypes.AgentActivity, len(msg.Activities))
		for _, a := range msg.Activities {
			if a.EntityType == "task" {
				m.work[a.EntityID] = a
			}
		}
		if m.activeListID != msg.ListID {
			// A list switch ends any esc-suppression: the next empty list
			// auto-shows its input again.
			m.activeListID = msg.ListID
			m.createSuppressed = false
			// A list switch also closes any inline create input left open
			// from the previous list — e.g. the auto-open on an empty list
			// carrying over — and drops its draft. Clearing happens before
			// applyRows below re-evaluates the empty-list auto-open, so an
			// empty destination list still gets its input back; a non-empty
			// one does not inherit a stale input anchored to a task id that
			// belongs to the list just left (bug: create-mode persists when
			// you switch to another list, which also explains the reported
			// viewport pinned near the bottom — clampScroll follows the
			// stale create row's position, not the new list's top).
			if m.creating {
				m.creating = false
				m.createBeforeID = ""
				m.createLevelOffset = 0
				m.createInput.Blur()
				m.createInput.Reset()
			}
		}
		m.activeList = msg.ListID != ""
		m.applyRows(msg.Rows)
		// The store caught up with any structural change in flight: a
		// gesture deferred while the rows were stale is replayed against
		// the fresh rows now, so a second ] inside the refresh window
		// lands on the post-change state instead of the stale one (bug 4).
		if cmd := m.replayDeferred(); cmd != nil {
			return m, cmd
		}
		// Broadcast the current selection's depth to add-input
		if row := m.findRow(m.selectedID); row != nil {
			return m, cmds.SetSelection(row.Task.ID, row.Depth)
		}

	case cmds.AnimFrameMsg:
		m.animFrame = msg.Frame

	case cmds.StartCreatingMsg:
		m.StartCreating(m.selectedID)
		return m, m.createInput.Focus()

	case cmds.CreateTaskConfirmedMsg:
		// The store created a task from the inline input. Keep creating for
		// rapid entry: drop the draft's title, anchor the next create row on
		// the new task, and move the cursor onto it. SetSelection is a no-op
		// until the new task is present in rows (it arrives on the refresh
		// triggered by applyCreateDraft).
		m.ResetCreateInput(msg.NewID)
		m.selectedID = msg.NewID
		if row := m.findRow(msg.NewID); row != nil {
			return m, cmds.SetSelection(row.Task.ID, row.Depth)
		}
		return m, nil

	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}

		// While creating, every keystroke goes to the textinput except
		// the creation-specific shortcuts.
		if m.creating {
			return m.handleCreatingKey(msg)
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

		// n starts creating even when the tree is empty: esc can leave an
		// empty list's surface bare, and n is the only way back in
		// (docs/plan/task-row-cards-and-status.md).
		if key.Matches(msg, keys.Tree.New) {
			m.StartCreating(m.selectedID)
			return m, m.createInput.Focus()
		}

		// Tree navigation shortcuts are only relevant while there are
		// rows; an empty tree falls through so the bar can show only
		// the keys that make sense.
		if len(m.rows) == 0 {
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
			return m, m.beginStructure(m.toggleComplete())
		case key.Matches(msg, keys.Tree.OpenDetails):
			if m.selectedID != "" {
				return m, cmds.OpenDetails(m.selectedID)
			}
		case key.Matches(msg, keys.Tree.Delete):
			if m.selectedID != "" {
				return m, m.beginStructure(cmds.DeleteTask(m.selectedID))
			}
		case key.Matches(msg, keys.Tree.Outdent):
			// [ moves the selected task one level shallower; the same key
			// picks the new task's level while creating (§4).
			return m, m.handleStructural(gestureOutdent)
		case key.Matches(msg, keys.Tree.Indent):
			return m, m.handleStructural(gestureIndent)
		case key.Matches(msg, keys.Tree.MoveUp):
			return m, m.handleStructural(gestureMoveUp)
		case key.Matches(msg, keys.Tree.MoveDown):
			return m, m.handleStructural(gestureMoveDown)
		}

		// If selection changed, broadcast it to add-input
		if m.selectedID != oldSelection {
			if row := m.findRow(m.selectedID); row != nil {
				return m, cmds.SetSelection(row.Task.ID, row.Depth)
			}
		}

	case tea.PasteMsg:
		if m.filterTyping {
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			// Same live sync as handleFilterKey: a pasted query narrows the
			// tree immediately, exactly as a typed one does.
			m.filterQuery = m.filterInput.Value()
			return m, cmd
		}
		if m.creating {
			var cmd tea.Cmd
			m.createInput, cmd = m.createInput.Update(msg)
			return m, cmd
		}

	case cmds.CloseModalMsg:
		// A modal closing returns the tree to an unfiltered view so a
		// leftover query cannot surprise the user after the picker closes
		// (docs/plans/phase-8-search.md's independence case).
		m.clearFilter()
	}

	return m, nil
}

// bodyHeight and bodyWidth are the inner dimensions the tree renders into,
// derived from the last layout the panel broadcast — the same values taskspanel
// passes to ViewInPanel, so the scroll math and the render agree on the window.
func (m Model) bodyHeight() int { return chrome.PanelBodyHeight(m.body.Height) }
func (m Model) bodyWidth() int  { return chrome.PanelBodyWidth(m.body.MainWidth) }

// scrollTargetID is the id of the line the window must keep visible: the inline
// create row while creating (the cursor is in its input), otherwise the
// selected task. Empty means there is nothing to keep visible (an empty or
// no-active-list state), which resets the offset to the top.
func (m Model) scrollTargetID() string {
	if m.creating {
		return createLineID
	}
	return m.selectedID
}

// selectedLineIndex returns the plan index of the scroll target line, or -1
// when the target is absent (nothing selected, or an empty/error state). It
// reads the same line plan the renderer uses, so no header/spacing counts are
// duplicated here (Commit 5 step 2).
func (m Model) selectedLineIndex(plan []panelLine) int {
	target := m.scrollTargetID()
	if target == "" {
		return -1
	}
	for i, ln := range plan {
		if ln.taskID == target {
			return i
		}
	}
	return -1
}

// scrollFor returns the scroll offset that keeps the current selection visible,
// shifting prev only as far as needed. It is 0 for the states that never scroll
// (no active list, or an empty list that is not mid-create) and clamps to the
// plan against the current body height otherwise.
func (m Model) scrollFor(prev int) int {
	height := m.bodyHeight()
	if height <= 0 || !m.activeList || (len(m.rows) == 0 && !m.creating) {
		return 0
	}
	plan := m.linePlan(m.bodyWidth(), chrome.PanelBg(m.focused))
	return clampScroll(len(plan), m.selectedLineIndex(plan), height, prev)
}

// clampScroll keeps the selected plan line inside the visible window
// [offset, offset+height). It shifts the previous offset the minimum distance
// needed, then clamps to the valid range; with no selected line (selIdx < 0) or
// a plan that fits, it returns the top (0).
func clampScroll(planLen, selIdx, height, prev int) int {
	if height <= 0 {
		return 0
	}
	maxOffset := max(0, planLen-height)
	if selIdx < 0 {
		return 0
	}
	off := min(max(prev, 0), maxOffset)
	if selIdx < off {
		off = selIdx
	} else if selIdx >= off+height {
		off = selIdx - height + 1
	}
	return min(max(off, 0), maxOffset)
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
	// The tree filters live, the way F already does: every keystroke lands in
	// filterQuery, so displayedRows and the filter bar narrow as the user
	// types instead of waiting for enter. enter above still matters — it is
	// what flips filterTyping to filterApplied so the input blurs and the
	// tree stays filtered while the cursor moves.
	m.filterQuery = m.filterInput.Value()
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

// moveSelection moves the cursor by delta visible rows. The cursor walks the
// Pending and Complete sections as one sequence — Pending first, then
// Complete — so pressing ↓ past the last pending row lands on the first
// complete row, and ↑ from the first complete row returns to the last pending
// row. Each section keeps its own ordering (its rows never interleave by
// store position), which is the "two lists with their own indexes" contract
// (docs/DESIGN.md §6). While a /-filter is active the sections do not exist,
// so the cursor moves through the flat filtered set instead.
func (m *Model) moveSelection(delta int) {
	var order []apptypes.Row
	if m.filterActive() {
		order = m.displayedRows()
	} else {
		pending, complete := m.splitSections()
		order = make([]apptypes.Row, 0, len(pending)+len(complete))
		order = append(order, pending...)
		order = append(order, complete...)
	}

	current := -1
	for i, r := range order {
		if r.Task.ID == m.selectedID {
			current = i
			break
		}
	}
	if current < 0 {
		current = 0
	}
	next := current + delta
	if next < 0 {
		next = 0
	}
	if next >= len(order) {
		next = len(order) - 1
	}
	if next >= 0 && next < len(order) {
		m.selectedID = order[next].Task.ID
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

// outdentSelected moves the selected task one level shallower: it becomes a
// sibling of its parent, positioned immediately after it, so its line stays
// in place. A root task has nothing above it — no-op (docs/DESIGN.md §5).
func (m *Model) outdentSelected() tea.Cmd {
	if m.selectedID == "" {
		return nil
	}
	row := m.findRow(m.selectedID)
	if row == nil || row.Task.ParentID == nil {
		return nil
	}
	return cmds.MoveTask(m.selectedID, *row.Task.ParentID)
}

// gestureKind identifies one of the row-derived restructuring gestures.
// Their targets are computed from m.rows, which is why they are the ones
// that defer while a previous change's refresh is in flight.
type gestureKind int

const (
	gestureIndent gestureKind = iota
	gestureOutdent
	gestureMoveUp
	gestureMoveDown
)

// deferredGesture is a restructuring keypress held back while the tree's
// rows are stale, carrying the task it was pressed on.
type deferredGesture struct {
	kind   gestureKind
	taskID string
}

// beginStructure marks a structural change as in flight — rows will be
// stale until its refresh lands — and returns the command unchanged.
func (m *Model) beginStructure(cmd tea.Cmd) tea.Cmd {
	if cmd != nil {
		m.structureInFlight = true
	}
	return cmd
}

// handleStructural runs one of the four row-derived restructuring gestures
// (indent, outdent, move up, move down). While a previous change's refresh
// has not yet landed the computed target is wrong: the task just nested
// elsewhere is still picked as a sibling (bug 4's repro — ] on task 2, then
// ] on task 3 inside the refresh window nested 3 under 2 instead of 1). In
// that window the gesture is deferred and replayed against the fresh rows
// by replayDeferred, so the key always acts on the state the user is about
// to see.
func (m *Model) handleStructural(kind gestureKind) tea.Cmd {
	if m.structureInFlight {
		m.pendingGesture = &deferredGesture{kind: kind, taskID: m.selectedID}
		return nil
	}
	return m.beginStructure(m.gestureCmd(kind))
}

// gestureCmd computes the command for a structural gesture against the
// tree's current rows.
func (m *Model) gestureCmd(kind gestureKind) tea.Cmd {
	switch kind {
	case gestureIndent:
		return m.indentSelected()
	case gestureOutdent:
		return m.outdentSelected()
	case gestureMoveUp:
		return m.moveSelected(-1)
	case gestureMoveDown:
		return m.moveSelected(1)
	}
	return nil
}

// replayDeferred applies the newest deferred gesture (if any) against rows
// that just caught up with the store, keeping the gesture's original target
// task even if the cursor moved while it waited. Clears the in-flight flag:
// a replayed gesture that emits a command re-arms it for its own refresh.
func (m *Model) replayDeferred() tea.Cmd {
	m.structureInFlight = false
	if m.pendingGesture == nil {
		return nil
	}
	g := *m.pendingGesture
	m.pendingGesture = nil
	saved := m.selectedID
	m.selectedID = g.taskID
	cmd := m.beginStructure(m.gestureCmd(g.kind))
	m.selectedID = saved
	return cmd
}

// indentSelected moves the selected task one level deeper: it becomes the
// last child of its previous sibling, so its line stays in place. A task
// with no previous sibling stays put, and a pending task never moves under a
// complete sibling (§3 forbids a complete ancestor over a pending
// descendant) — both are silent no-ops (docs/DESIGN.md §5).
func (m *Model) indentSelected() tea.Cmd {
	if m.selectedID == "" {
		return nil
	}
	row := m.findRow(m.selectedID)
	if row == nil {
		return nil
	}
	run := siblingRun(m.rows, row.Task.ParentID)
	idx := slices.IndexFunc(run, func(r apptypes.Row) bool { return r.Task.ID == m.selectedID })
	if idx <= 0 {
		return nil
	}
	prev := run[idx-1]
	if prev.Task.Status == apptypes.StatusComplete && row.Task.Status != apptypes.StatusComplete {
		return nil // §3
	}
	return cmds.ReparentTask(m.selectedID, &prev.Task.ID)
}

// moveSelected moves the selected task up (delta -1) or down (+1) within its
// own status run: a pending task swaps with the previous/next pending
// sibling and a complete one with the previous/next complete sibling, so the
// two sections never mix — a task at its run's boundary stays put
// (docs/DESIGN.md §6). The gesture resolves to a concrete after-id that
// AppModel executes through store.MoveTask.
func (m *Model) moveSelected(delta int) tea.Cmd {
	if m.selectedID == "" {
		return nil
	}
	row := m.findRow(m.selectedID)
	if row == nil {
		return nil
	}
	run := siblingRun(m.rows, row.Task.ParentID)
	idx := slices.IndexFunc(run, func(r apptypes.Row) bool { return r.Task.ID == m.selectedID })
	if idx < 0 {
		return nil
	}

	// Walk in the direction to the nearest same-status sibling, skipping
	// opposite-status rows that sit between (pending and complete siblings
	// can interleave by store position even though they render separately).
	next := idx + delta
	for next >= 0 && next < len(run) && run[next].Task.Status != row.Task.Status {
		next += delta
	}
	if next < 0 || next >= len(run) {
		return nil // run boundary
	}

	target := run[next]
	if delta > 0 {
		// Swap with the next same-status sibling: land right after it.
		return cmds.MoveTask(m.selectedID, target.Task.ID)
	}
	// Swap with the previous same-status sibling: land after whatever
	// precedes it, or at the front of the run when it has no predecessor.
	if next > 0 {
		return cmds.MoveTask(m.selectedID, run[next-1].Task.ID)
	}
	return cmds.MoveTask(m.selectedID, "")
}

// siblingRun returns the rows sharing one parent (nil = the list root) in
// tree order — the sibling set the indent and move gestures reorder within.
func siblingRun(rows []apptypes.Row, parentID *string) []apptypes.Row {
	out := make([]apptypes.Row, 0, 4)
	for _, r := range rows {
		if (r.Task.ParentID == nil) == (parentID == nil) &&
			(parentID == nil || *r.Task.ParentID == *parentID) {
			out = append(out, r)
		}
	}
	return out
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

// selectedDepth returns the depth of the currently selected task, or 0 when
// nothing is selected (used to validate the level offset at the root).
func (m Model) selectedDepth() int {
	if row := m.findRow(m.selectedID); row != nil {
		return row.Depth
	}
	return 0
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

// matchVisible returns the rows a /-filter keeps visible plus, for each
// directly-matched row, the byte offsets in its title that the query matched
// (sahilm/fuzzy's MatchedIndexes, from the same Find call that decides the
// match — the renderer highlights those offsets rather than re-running its
// own search). Membership in that map is what marks a row as a direct match;
// an ancestor that is visible only to anchor a match is absent from it and
// gets dimmed. An empty query shows every row with an empty direct set.
func matchVisible(rows []apptypes.Row, query string) ([]apptypes.Row, map[string][]int) {
	q := strings.TrimSpace(query)
	if q == "" {
		return rows, make(map[string][]int)
	}

	direct := make(map[string][]int, len(rows))
	titles := make([]string, len(rows))
	for i, r := range rows {
		titles[i] = r.Task.Title
	}
	for _, m := range fuzzy.Find(q, titles) {
		direct[rows[m.Index].Task.ID] = m.MatchedIndexes
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

// StartCreating enters inline creation mode, placing the input row after
// the given task id (empty string appends at the end).
func (m *Model) StartCreating(beforeID string) {
	m.startCreating(beforeID)
}

func (m *Model) startCreating(beforeID string) {
	m.creating = true
	m.createSuppressed = false

	// When the selection is a complete task, place the create row after the
	// last pending task (at that task's depth) rather than splicing it under
	// the complete row. When no pending rows exist at that depth, create at
	// root depth so the Pending section shows only the input (phase B step 4).
	m.createBeforeID = beforeID
	if beforeID != "" {
		if row := m.findRow(beforeID); row != nil && row.Task.Status == apptypes.StatusComplete {
			lastPendingID := m.lastPendingIDAtDepth(row.Depth)
			if lastPendingID != "" {
				m.createBeforeID = lastPendingID
			} else {
				m.createBeforeID = ""
				m.createLevelOffset = 0
			}
		}
	}

	m.createLevelOffset = 0
	m.createInput = textinput.New()
	m.createInput.Prompt = ""
	if m.createLevelOffset > 0 {
		m.createInput.Placeholder = "Add a follow-up"
	} else {
		m.createInput.Placeholder = "Add a task"
	}
	m.createInput.Focus()
}

// lastPendingIDAtDepth returns the id of the last pending (non-complete) row
// at the given depth in display order, or "" if none exists at that depth.
func (m *Model) lastPendingIDAtDepth(depth int) string {
	var lastID string
	for _, row := range m.displayedRows() {
		if row.Depth == depth && row.Task.Status != apptypes.StatusComplete {
			lastID = row.Task.ID
		}
	}
	return lastID
}

// CancelCreating exits inline creation mode and resets the input. It marks
// the session as esc-suppressed so the next refresh of the same empty list
// does not re-open the input under the user (docs/plan/task-row-cards-and-status.md).
func (m *Model) CancelCreating() {
	m.creating = false
	m.createSuppressed = true
	m.createBeforeID = ""
	m.createLevelOffset = 0
	m.createInput.Blur()
	m.createInput.Reset()
}

// CreateDraft returns the current input value as a draft task, or false
// when the input is empty.
func (m Model) CreateDraft() (title string, beforeID string, levelOffset int, ok bool) {
	if !m.creating || strings.TrimSpace(m.createInput.Value()) == "" {
		return "", "", 0, false
	}
	return m.createInput.Value(), m.createBeforeID, m.createLevelOffset, true
}

// ResetCreateInput clears the input and moves the insertion point to the
// next after-id, keeping creating mode active for rapid entry.
func (m *Model) ResetCreateInput(nextBeforeID string) {
	m.createInput.Reset()
	m.createBeforeID = nextBeforeID
	m.createLevelOffset = 0
}

// IsCreating reports whether the tree is in inline creation mode.
func (m Model) IsCreating() bool {
	return m.creating
}

// handleCreatingKey processes keystrokes while the inline input is active.
// The create row owns the keyboard: only an allowlist of keys is accepted
// (typing, backspace, [ / ] for level, enter, esc). Every other key is
// swallowed so it cannot trigger an app shortcut or navigate away mid-entry
// (docs/DESIGN.md §5, phase A step 3).
func (m *Model) handleCreatingKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, keys.Overlay.Submit) {
		if title, beforeID, levelOffset, ok := m.CreateDraft(); ok {
			return m, cmds.CreateTaskFromInput(title, beforeID, levelOffset)
		}
		return m, nil
	}

	if key.Matches(msg, keys.Overlay.Cancel) {
		// Single press always cancels: discard any typed text and remove the
		// create row, from every entry path (manual n or the empty list's
		// auto-input). The refresh that follows must not re-open it — see
		// createSuppressed (docs/plan/task-row-cards-and-status.md).
		m.CancelCreating()
		return m, nil
	}

	if key.Matches(msg, keys.Tree.Outdent) {
		// A new task can never sit above root: once the create row is at
		// depth 0 (a root selection, or a deeper selection already
		// outdented to its root), [ is a no-op and the ^ glyph never
		// renders for a root-level row (docs/DESIGN.md §4).
		if m.selectedDepth()+m.createLevelOffset-1 < 0 {
			return m, nil
		}
		m.createLevelOffset = max(m.createLevelOffset-1, -1)
		return m, nil
	}

	if key.Matches(msg, keys.Tree.Indent) {
		m.createLevelOffset = min(m.createLevelOffset+1, 1)
		return m, nil
	}

	// Hard allowlist: only typing (printable characters) and backspace reach
	// the text input. Everything else — arrows, F-keys, ctrl combos — is
	// swallowed so it cannot navigate the tree while the create row owns the
	// keyboard. tab/shift+tab never arrive here as characters: AppModel
	// routes them to the focus cycle before this handler runs.
	if msg.Text != "" || msg.Code == tea.KeyBackspace {
		var cmd tea.Cmd
		m.createInput, cmd = m.createInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

// createRenderAnchorID returns the task id after which the create row should
// be rendered: the last visible descendant of createBeforeID (when the anchor
// has visible children), or createBeforeID itself when the anchor has no
// visible descendants, or "" when createBeforeID is empty (append at end).
//
// This decouples the create row's visual position from its data insertion
// point. The row always renders after the selected task's entire visible
// subtree, while the new task is still inserted as a sibling at the selected
// task's depth (docs/DESIGN.md §4). A collapsed subtree yields no visible
// descendants, so the row renders right after the anchor — where the sibling
// will land once created.
func (m Model) createRenderAnchorID() string {
	if m.createBeforeID == "" {
		return ""
	}
	visible := m.displayedRows()
	anchorIdx := -1
	for i, r := range visible {
		if r.Task.ID == m.createBeforeID {
			anchorIdx = i
			break
		}
	}
	if anchorIdx < 0 {
		return m.createBeforeID
	}
	anchorDepth := visible[anchorIdx].Depth
	// In depth-first preorder, descendants form a contiguous run after the
	// anchor with depth > anchorDepth. The last one is the render anchor;
	// if none exist (collapsed subtree or no children), the anchor itself
	// is the render point.
	lastDesc := anchorIdx
	for i := anchorIdx + 1; i < len(visible); i++ {
		if visible[i].Depth <= anchorDepth {
			break
		}
		lastDesc = i
	}
	return visible[lastDesc].Task.ID
}
