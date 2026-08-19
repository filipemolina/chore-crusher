// Package archivepage is the Archived Lists page: a full-body surface that
// temporarily replaces Tasks and Lists, the way detailsPanelVisible already
// does for Details but without a modal's centered/scrimmed treatment
// (docs/DESIGN.md §5 — see the parent task's notes on why this is a page,
// not a modal). AppModel routes every keypress here while the page is open
// and renders its View in place of the normal body.
//
// The page is the sole component on its "surface", mirroring
// ../cais/src/components/backuppage's shape: a left column of archived lists
// (newest-archived first, name-filterable) beside a right-side read-only
// preview of the selected list's tasks. This is the list+preview shell —
// unarchive and permanent-delete actions land in follow-up tasks.
package archivepage

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/components/chrome"
	"github.com/filipemolina/farol/src/constants"
	"github.com/filipemolina/farol/src/keys"
	"github.com/filipemolina/farol/src/store"
)

// focusedZoneID is the zone id this component answers to. Like Details, the
// Archive page is focused only while it is visible, entered by AppModel's
// explicit open transition — it is never in the tab/shift+tab cycle.
const focusedZoneID = constants.COMPONENT_ARCHIVE_PAGE

// Model is the Archived Lists page.
type Model struct {
	store *store.Store

	focused bool
	body    cmds.SetBodyLayoutMsg
	// listWidth and previewWidth split the body row between the two columns,
	// mirroring backuppage's own split.
	listWidth    int
	previewWidth int

	loading bool
	loadErr error
	// entries is the full archived set, unfiltered — filtering narrows it at
	// render/selection time (visibleEntries) rather than re-querying the
	// store on every keystroke.
	entries []apptypes.ListSummary
	// selectedIdx indexes into visibleEntries(), not entries — it is reset to
	// 0 whenever the filtered set changes shape, so it never points past the
	// end of what is actually on screen.
	selectedIdx int

	// filterInput is the name filter row at the top of the list column.
	// filtering reports whether it currently owns the keyboard (typing); the
	// query itself is always filterInput.Value(), live-applied whether or not
	// the input has focus right now. enter commits without clearing (blurs
	// the input, leaves the filtered view active); esc clears the query in
	// one step whether typing or already applied, mirroring the task tree's
	// own /-filter exactly (docs/DESIGN.md §5) — see handleFilterKey and
	// handleKey's Global.Back case.
	filterInput textinput.Model
	filtering   bool

	// previewListID is the archived list the current preview rows belong to,
	// so a slow RefreshArchivedListPreviewMsg racing a newer selection can be
	// told apart from the response the user is actually looking at and
	// dropped rather than clobbering a fresher selection.
	previewListID  string
	previewRows    []apptypes.Row
	previewLoading bool
	previewErr     error

	// previewFocused reports which of the page's two scrollable columns tab
	// and shift+tab (ArchivePage.FocusPreview) currently route Navigate/
	// GoToStart/GoToEnd to: false is the archived-list column (the default,
	// and where selection-driven actions like Unarchive/Delete always read
	// from regardless of which column has this focus), true is the read-only
	// task preview. Exactly two zones, so tab and shift+tab both just toggle
	// it — there is no third state to cycle through.
	previewFocused bool
	// listScroll is the index of the first archived-list entry currently
	// rendered, kept just far enough from 0 to keep selectedIdx inside the
	// viewport (see listViewportRows/clampWindowStart) — the list column's
	// own scroll, driven indirectly by moving the selection.
	listScroll int
	// previewScroll is the index of the first preview task row currently
	// rendered. Unlike listScroll it is not tied to a selection — it is a
	// plain viewport offset the user drives directly with Navigate/
	// GoToStart/GoToEnd while previewFocused is true, clamped to
	// previewViewportRows() after every Update (see the exported Update
	// wrapper below).
	previewScroll int
}

// New builds the Archive page.
func New(s *store.Store) tea.Model {
	fi := textinput.New()
	fi.Prompt = "/"
	fi.Placeholder = "filter by name"

	return Model{store: s, filterInput: fi}
}

func (m Model) Init() tea.Cmd { return nil }

// Update runs the message through update and then reclamps both columns'
// scroll offsets against whatever state came out of it — a resize, a
// selection change, a filter keystroke, or a fresh preview load can all
// shrink the valid range out from under a scroll position set before it, and
// this is the one place that has to know that, rather than every branch of
// update remembering to call it (docs/DESIGN.md §12's viewport pattern,
// mirroring the task tree's own scrollFor).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	out := next.(Model)
	out.listScroll = clampWindowStart(len(out.visibleEntries()), out.selectedIdx, out.listViewportRows(), out.listScroll)
	out.previewScroll = clampScrollOffset(len(out.previewRows), out.previewViewportRows(), out.previewScroll)
	return out, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmds.SetBodyLayoutMsg:
		m.body = msg
		m.setColumnWidths()
		return m, nil

	case cmds.SetFocusMsg:
		m.focused = int(msg) == focusedZoneID
		return m, nil

	// AppModel issues this on the A keypress and reissues it isn't needed
	// again — but resetting state here (rather than relying on whatever the
	// page last showed) means a page left mid-filter, closed, and reopened
	// starts clean rather than resuming a stale query and stale selection.
	case cmds.OpenArchivePageMsg:
		m.loading = true
		m.loadErr = nil
		m.entries = nil
		m.selectedIdx = 0
		m.filtering = false
		m.filterInput.SetValue("")
		m.filterInput.Blur()
		m.previewListID = ""
		m.previewRows = nil
		m.previewLoading = false
		m.previewErr = nil
		m.previewFocused = false
		m.listScroll = 0
		m.previewScroll = 0
		return m, nil

	case cmds.RefreshArchivedListsMsg:
		return m.handleRefreshArchivedLists(msg)

	case cmds.RefreshArchivedListPreviewMsg:
		// Drop a response that no longer matches the current selection — the
		// user moved on before it arrived.
		if msg.ListID != m.previewListID {
			return m, nil
		}
		m.previewLoading = false
		m.previewErr = msg.Err
		if msg.Err == nil {
			m.previewRows = msg.Rows
		}
		return m, nil

	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}
		return m.handleKey(msg)
	}
	return m, nil
}

// handleRefreshArchivedLists hydrates the entries list and, if the
// effectively selected entry changed as a result (a fresh load, or the
// previously selected list vanishing from the set), kicks off a preview load
// for whatever is selected now.
func (m Model) handleRefreshArchivedLists(msg cmds.RefreshArchivedListsMsg) (tea.Model, tea.Cmd) {
	m.loading = false
	m.loadErr = msg.Err
	if msg.Err != nil {
		return m, nil
	}
	m.entries = msg.Lists
	m.clampSelection()
	return m, m.loadPreviewIfSelectionChanged()
}

// handleKey mirrors detailspanel's compose-vs-modal split: while the filter
// input owns the keyboard it gets first refusal, then esc, then navigation
// and actions — matched against the bindings keys.ArchivePage declares
// (docs/DESIGN.md §5's rule that a key is declared in keys.go exactly once
// and components match against it, not against ad hoc strings).
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		return m.handleFilterKey(msg)
	}

	switch {
	case key.Matches(msg, keys.Global.Back):
		// The esc-ladder idiom the tree and Lists panel already establish
		// (docs/DESIGN.md §5): a non-empty applied filter is cleared first;
		// only a second esc, with nothing left to clear, leaves the page.
		if m.filterInput.Value() != "" {
			m.filterInput.SetValue("")
			m.clampSelection()
			return m, m.loadPreviewIfSelectionChanged()
		}
		return m, cmds.CloseArchivePage(nil)

	// PageActive (1) is the page-tab way off this page, alongside esc — it
	// only reaches here (rather than the filter input) because m.filtering
	// is checked above this switch, so a "1" typed into the name filter
	// lands there instead. No filter-clearing ladder: unlike esc, 1 always
	// jumps straight to the Active page in one press.
	case key.Matches(msg, keys.Global.PageActive):
		return m, cmds.CloseArchivePage(nil)

	case key.Matches(msg, keys.ArchivePage.Filter):
		// The filter row lives in the archived-list column, so typing into
		// it always brings that column's focus back — the same way pressing
		// / elsewhere in the app moves the cursor to whatever it is about to
		// filter.
		m.previewFocused = false
		m.filtering = true
		m.filterInput.Focus()
		return m, textinput.Blink

	case key.Matches(msg, keys.ArchivePage.FocusPreview):
		// Exactly two columns, so tab and shift+tab both just toggle which
		// one Navigate/GoToStart/GoToEnd act on next.
		m.previewFocused = !m.previewFocused
		return m, nil

	case key.Matches(msg, keys.ArchivePage.GoToStart):
		if m.previewFocused {
			m.previewScroll = 0
			return m, nil
		}
		return m.setSelection(0)
	case key.Matches(msg, keys.ArchivePage.GoToEnd):
		if m.previewFocused {
			// Any value at or past the last valid offset works: Update's
			// wrapper reclamps it to previewViewportRows()'s actual max right
			// after this returns.
			m.previewScroll = len(m.previewRows)
			return m, nil
		}
		return m.setSelection(len(m.visibleEntries()) - 1)
	case key.Matches(msg, keys.ArchivePage.Navigate):
		delta := 0
		switch msg.String() {
		case "up", "k":
			delta = -1
		case "down", "j":
			delta = 1
		default:
			return m, nil
		}
		if m.previewFocused {
			m.previewScroll += delta
			return m, nil
		}
		return m.moveSelection(delta)

	case key.Matches(msg, keys.ArchivePage.Unarchive):
		// Routes through AppModel's confirmmodal the same way Delete does
		// (docs/DESIGN.md §9): restoring a list to normal discovery acts on
		// the whole list with one keystroke and no visible undo in the
		// moment, so it warrants a confirmation even though it is
		// reversible from the Lists panel's own Archive. The component only
		// requests it; only AppModel opens a modal.
		if sel, ok := m.selectedEntry(); ok {
			return m, cmds.UnarchiveArchivedList(sel.List.ID, sel.List.Name)
		}

	case key.Matches(msg, keys.ArchivePage.Delete):
		// Irreversible, unlike Unarchive — routes through AppModel's
		// confirmmodal the same way Tree.Delete and Lists.Delete do
		// (docs/DESIGN.md §9). The component only requests it; only AppModel
		// opens a modal.
		if sel, ok := m.selectedEntry(); ok {
			return m, cmds.DeleteArchivedList(sel.List.ID, sel.List.Name, sel.PendingCount+sel.CompleteCount)
		}
	}
	return m, nil
}

// handleFilterKey drives the inline filter input. esc and enter both commit
// (stop typing, keep the query); every other key edits the query and
// re-narrows the visible set live.
func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Commits: blur the input, keep the query applied — the tree's own
		// /-filter does the same (docs/DESIGN.md §5, "enter... blurs the
		// input and leaves the filtered view active").
		m.filtering = false
		m.filterInput.Blur()
		return m, nil
	case "esc":
		// Clears in one step, whether typing or (via handleKey's own esc
		// case below) already applied — mirrors the tree's own /-filter
		// exactly ("esc clears it", docs/DESIGN.md §5), not a "commit, then
		// a second esc clears" ladder of its own.
		m.filtering = false
		m.filterInput.Blur()
		m.filterInput.SetValue("")
		m.clampSelection()
		return m, m.loadPreviewIfSelectionChanged()
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.clampSelection()
	return m, tea.Batch(cmd, m.loadPreviewIfSelectionChanged())
}

// moveSelection steps the selection by delta within the filtered set,
// clamped to its bounds (no wraparound — matches the task tree's own
// GoToStart/GoToEnd-at-the-edge behavior).
func (m Model) moveSelection(delta int) (tea.Model, tea.Cmd) {
	return m.setSelection(m.selectedIdx + delta)
}

func (m Model) setSelection(idx int) (tea.Model, tea.Cmd) {
	n := len(m.visibleEntries())
	if n == 0 {
		m.selectedIdx = 0
		return m, nil
	}
	if idx < 0 {
		idx = 0
	}
	if idx > n-1 {
		idx = n - 1
	}
	m.selectedIdx = idx
	return m, m.loadPreviewIfSelectionChanged()
}

// clampSelection keeps selectedIdx within the current filtered set after it
// changes shape (a refresh, or a filter keystroke narrowing/widening it).
func (m *Model) clampSelection() {
	n := len(m.visibleEntries())
	if n == 0 {
		m.selectedIdx = 0
		return
	}
	if m.selectedIdx > n-1 {
		m.selectedIdx = n - 1
	}
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
}

// selectedEntry returns the currently selected visible entry, or the zero
// value with ok false when the filtered set is empty.
func (m Model) selectedEntry() (apptypes.ListSummary, bool) {
	visible := m.visibleEntries()
	if m.selectedIdx < 0 || m.selectedIdx >= len(visible) {
		return apptypes.ListSummary{}, false
	}
	return visible[m.selectedIdx], true
}

// loadPreviewIfSelectionChanged issues a preview load only when the
// effectively selected list actually changed — a filter keystroke that
// leaves the same top entry selected, or a poll refresh that reorders
// nothing, must not restart the preview and lose its scroll/flash state for
// no reason.
func (m *Model) loadPreviewIfSelectionChanged() tea.Cmd {
	sel, ok := m.selectedEntry()
	if !ok {
		m.previewListID = ""
		m.previewRows = nil
		m.previewLoading = false
		m.previewErr = nil
		return nil
	}
	if sel.List.ID == m.previewListID {
		return nil
	}
	m.previewListID = sel.List.ID
	m.previewRows = nil
	m.previewErr = nil
	m.previewLoading = true
	m.previewScroll = 0
	return cmds.RefreshArchivedListPreview(m.store, sel.List.ID)
}

// visibleEntries returns entries narrowed by the filter query — a plain
// case-insensitive substring match against the list name, the same
// simplicity the store's own ListArchivedLists nameFilter uses server-side
// (this is client-side purely so filtering feels live with no round trip).
func (m Model) visibleEntries() []apptypes.ListSummary {
	q := strings.ToLower(strings.TrimSpace(m.filterInput.Value()))
	if q == "" {
		return m.entries
	}
	out := make([]apptypes.ListSummary, 0, len(m.entries))
	for _, e := range m.entries {
		if strings.Contains(strings.ToLower(e.List.Name), q) {
			out = append(out, e)
		}
	}
	return out
}

// setColumnWidths splits the body row into a list column and a preview
// column, mirroring backuppage's own split.
func (m *Model) setColumnWidths() {
	bodyW := max(1, chrome.PanelBodyWidth(m.body.TerminalWidth))
	half := bodyW / 2
	m.listWidth = max(1, half)
	m.previewWidth = max(1, bodyW-m.listWidth-1)
}

// columnContentHeight is the vertical space either column has for its own
// content, below the shared frame chrome and above the docked hint line —
// the same quantity View computes as contentH. The hint is always exactly
// one line (chrome.RenderKeyHints joins its parts horizontally, never
// wrapping), so the "+1 for the blank spacer above it" View's own comment
// describes is a constant 2 rather than something that has to be rendered
// here just to measure.
func (m Model) columnContentHeight() int {
	return max(0, max(1, chrome.PanelBodyHeight(m.body.Height))-2)
}

// listViewportRows is how many archived-list entries (each a fixed two
// display lines: name, then metadata) fit in the list column at once, below
// its filter row and the rule under it.
func (m Model) listViewportRows() int {
	return max(0, m.columnContentHeight()-2) / 2
}

// previewViewportRows is how many preview task lines fit in the preview
// column at once, below its one-line "preview · <list>" header.
func (m Model) previewViewportRows() int {
	return max(0, m.columnContentHeight()-1)
}

// clampWindowStart returns the minimal-shift window start that keeps idx
// inside [start, start+visible) of an n-long collection, moving prev the
// least distance necessary rather than re-centering on every call — the same
// idea as the task tree's own clampScroll, adapted from lines to fixed-height
// rows since every entry in this column is exactly two lines tall.
func clampWindowStart(n, idx, visible, prev int) int {
	if visible <= 0 || n == 0 || idx < 0 {
		return 0
	}
	maxStart := max(0, n-visible)
	start := min(max(prev, 0), maxStart)
	switch {
	case idx < start:
		start = idx
	case idx >= start+visible:
		start = idx - visible + 1
	}
	return min(max(start, 0), maxStart)
}

// clampScrollOffset keeps a plain viewport offset (not tied to a selection)
// within the valid range for an n-long collection shown visible rows at a
// time — used for the preview column, which the user scrolls directly rather
// than via a moving selection.
func clampScrollOffset(n, visible, offset int) int {
	maxStart := max(0, n-visible)
	return min(max(offset, 0), maxStart)
}
