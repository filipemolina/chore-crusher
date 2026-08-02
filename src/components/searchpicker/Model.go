// Package searchpicker is the cross-list search modal opened by the global
// binding F. A text input at the top live-searches every list; the results
// below show each match's list context ("list › title"). Enter jumps to the
// selected task (switching the active list if needed), esc cancels. See
// docs/plans/phase-8-search.md step 2.
package searchpicker

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-completer/src/store"
)

// modalChrome is how many rows the picker's chrome (border, title, input,
// hint lines) needs beyond its result list. It is the same number
// chrome.modalListChrome reserves internally; the picker sizes its own list
// so it re-derives it rather than importing the chrome helper.
const modalChrome = 10

// Result is one candidate: the task plus the name of the list it lives in,
// enough to render "<list> › <title>" and to jump to it.
type Result struct {
	TaskID   string
	Title    string
	ListID   string
	ListName string
}

// Model is the cross-list search picker: a focused text input, a live result
// list, and a cursor over it.
type Model struct {
	input   textinput.Model
	query   string
	results []Result
	cursor  int
	errMsg  string
	store   *store.Store
	visible int // max result rows the terminal can show
}

func (m Model) Init() tea.Cmd { return textinput.Blink }

// New builds the picker. termHeight sizes the result list so the modal fits
// the terminal. The input starts focused so the user can type immediately.
func New(s *store.Store, termHeight int) tea.Model {
	input := textinput.New()
	input.Focus()
	input.Placeholder = "search all lists"

	return Model{
		input:   input,
		store:   s,
		visible: max(3, termHeight-modalChrome),
	}
}

// runSearch re-runs the store search for the input's current value and
// replaces the results, keeping the cursor in range.
func (m *Model) runSearch() {
	m.query = m.input.Value()
	m.results = rank(m.store, m.query)
	m.errMsg = ""
	m.clampCursor()
}

// clampCursor keeps the cursor within the result list (or at -1 when there
// is nothing to pick).
func (m *Model) clampCursor() {
	if len(m.results) == 0 {
		m.cursor = -1
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.results) {
		m.cursor = len(m.results) - 1
	}
}