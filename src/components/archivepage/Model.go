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
	"fmt"
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
	// the input has focus right now — committing (esc/enter) only stops
	// typing, it does not clear the query (docs/DESIGN.md §5's esc-ladder
	// idiom: type, commit, then a further esc clears).
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

	// actionErr is a failed unarchive's message, shown inline near the hint
	// line without disturbing the loaded list — distinct from loadErr, which
	// replaces the whole page (there is nothing wrong with the page itself,
	// only with one write against it). Cleared on the next keypress or
	// successful refresh, the same "any key clears the flash" idiom
	// detailspanel's own errMsg/flash fields use.
	actionErr string
}

// New builds the Archive page.
func New(s *store.Store) tea.Model {
	fi := textinput.New()
	fi.Prompt = "/"
	fi.Placeholder = "filter by name"

	return Model{store: s, filterInput: fi}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	case archiveActionErrMsg:
		m.actionErr = msg.text
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
	m.actionErr = ""

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

	case key.Matches(msg, keys.ArchivePage.Filter):
		m.filtering = true
		m.filterInput.Focus()
		return m, textinput.Blink

	case key.Matches(msg, keys.ArchivePage.GoToStart):
		return m.setSelection(0)
	case key.Matches(msg, keys.ArchivePage.GoToEnd):
		return m.setSelection(len(m.visibleEntries()) - 1)
	case key.Matches(msg, keys.ArchivePage.Navigate):
		switch msg.String() {
		case "up", "k":
			return m.moveSelection(-1)
		case "down", "j":
			return m.moveSelection(1)
		}
		return m, nil

	case key.Matches(msg, keys.ArchivePage.Unarchive):
		return m, m.unarchiveSelectedCmd()

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

// unarchiveSelectedCmd restores the selected list to normal discovery
// (store.UnarchiveList) and reloads the archived set — no confirmation, the
// reversible direction (see keys.ArchivePageKeys.Unarchive). A write failure
// surfaces as actionErr rather than replacing the loaded list with a
// full-page error.
func (m Model) unarchiveSelectedCmd() tea.Cmd {
	sel, ok := m.selectedEntry()
	if !ok {
		return nil
	}
	s := m.store
	id, name := sel.List.ID, sel.List.Name
	return func() tea.Msg {
		if err := s.UnarchiveList(id); err != nil {
			return archiveActionErrMsg{fmt.Sprintf("failed to unarchive %q: %v", name, err)}
		}
		return cmds.RefreshArchivedLists(s)()
	}
}

// archiveActionErrMsg carries a failed write's message into actionErr,
// without going through the full-page loadErr path (see its doc comment).
type archiveActionErrMsg struct{ text string }

// handleFilterKey drives the inline filter input. esc and enter both commit
// (stop typing, keep the query); every other key edits the query and
// re-narrows the visible set live.
func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.filtering = false
		m.filterInput.Blur()
		return m, nil
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
