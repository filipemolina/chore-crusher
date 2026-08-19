// Package archivepage is the Archived Lists page: a full-body surface that
// temporarily replaces Tasks and Lists, the way detailsPanelVisible already
// does for Details but without a modal's centered/scrimmed treatment
// (docs/DESIGN.md §5 — see the parent task's notes on why this is a page,
// not a modal). AppModel routes every keypress here while the page is open
// and renders its View in place of the normal body.
//
// This is the plumbing skeleton only: the list-plus-preview content lands in
// a follow-up task (@0000380RE8636NZFYQ0WX26Y6C). For now it shows a
// placeholder so the open/close wiring can be built and tested on its own.
package archivepage

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/components/chrome"
	"github.com/filipemolina/farol/src/constants"
	"github.com/filipemolina/farol/src/store"
)

// focusedZoneID is the zone id this component answers to. Like Details, the
// Archive page is focused only while it is visible, entered by AppModel's
// explicit open transition — it is never in the tab/shift+tab cycle.
const focusedZoneID = constants.COMPONENT_ARCHIVE_PAGE

// Model is the Archived Lists page. store is retained for the follow-up
// task's queries (ListArchivedLists, UnarchiveList, DeleteList); the
// skeleton itself does not read it yet.
type Model struct {
	store *store.Store

	focused bool
	body    cmds.SetBodyLayoutMsg
}

// New builds the Archive page.
func New(s *store.Store) tea.Model {
	return Model{store: s}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmds.SetBodyLayoutMsg:
		m.body = msg
		return m, nil

	case cmds.SetFocusMsg:
		m.focused = int(msg) == focusedZoneID
		return m, nil

	case tea.KeyPressMsg:
		// Only act while focused: AppModel routes every keypress here
		// exclusively while the page is visible, but a hidden page must not
		// react to a stray keystroke either (mirrors detailspanel.Update).
		if !m.focused {
			return m, nil
		}
		if msg.String() == "esc" {
			return m, cmds.CloseArchivePage(nil)
		}
		return m, nil
	}
	return m, nil
}

// View renders the page filling the whole body — the terminal width, not
// just a Tasks-sized column, since the Archive page replaces the
// Tasks/Lists split entirely rather than sharing the row with it
// (docs/DESIGN.md §5).
func (m Model) View() tea.View {
	width := m.body.TerminalWidth
	height := m.body.Height

	inner := chrome.EmptyStateCard(
		"Archived lists\n\nComing soon.",
		chrome.PanelBodyWidth(width),
		chrome.PanelBodyHeight(height),
		chrome.PanelBg(m.focused),
	)

	return tea.NewView(chrome.PanelFrame("Archived Lists", m.focused, width, height, inner))
}
