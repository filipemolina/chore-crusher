package importexportmodal

import (
	"encoding/json"
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/components/chrome"
	"github.com/filipemolina/farol/src/keys"
	"github.com/filipemolina/farol/src/store"
)

// Model is the export modal: a destination file path plus a whole-store /
// this-list toggle. On confirm it marshals store.Export's document and writes
// the file (docs/DESIGN.md §9, export/import). It reuses the list name
// modal's toggle discipline (tab moves focus onto the toggle, space flips it
// there) so the two modals behave identically.
type Model struct {
	store         *store.Store
	listID        *string // highlighted list to export; nil = whole store default
	wholeStore    bool    // toggle: true exports everything, false exports listID
	input         textinput.Model
	toggleFocused bool // tab moves focus off the path onto the toggle
	termWidth     int  // terminal width, to size the path input (docs/DESIGN.md §12)
}

// NewExport creates the export modal. wholeStore seeds to true when no list
// is highlighted (the only sensible default is "everything"); with a list
// highlighted the natural default is to export just that list. termWidth
// sizes the path input so its placeholder renders in full rather than the
// bubbles v2 textinput's first-character-only bug at Width 0.
func NewExport(s *store.Store, listID *string, termWidth int) tea.Model {
	input := textinput.New()
	input.Focus()
	// The bubbles default prompt is a hardcoded ANSI-white "> ", which would
	// leak a default-colored glyph into the modal body. SealInput (called in
	// View) strips the foreground so it reads on a light theme's modal.
	input.Prompt = ""
	input.Placeholder = "path/to/file.json"

	wholeStore := listID == nil
	return Model{store: s, listID: listID, wholeStore: wholeStore, input: input, termWidth: termWidth}
}

func (m Model) Init() tea.Cmd { return textinput.Blink }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keys.Overlay.Submit):
			path := strings.TrimSpace(m.input.Value())
			// No path, nothing to write — same as ModeNew requiring a name.
			if path == "" {
				return m, nil
			}
			return m, cmds.CloseModal(m.exportFollowCmd(path))
		case key.Matches(msg, keys.Overlay.Cancel):
			return m, cmds.CloseModal(nil)
		case key.Matches(msg, keys.ExportModal.NextField):
			m.toggleFocused = !m.toggleFocused
			if m.toggleFocused {
				m.input.Blur()
			} else {
				m.input.Focus()
			}
			return m, nil
		case key.Matches(msg, keys.ListNameModal.ToggleCollaborative):
			// Reuse the collaborative toggle's space binding for our toggle:
			// space flips the whole-store flag only while the toggle has focus,
			// exactly like listnamemodal flips collaborative.
			if m.toggleFocused {
				m.wholeStore = !m.wholeStore
				return m, nil
			}
		}
	}

	// While the toggle (not the text field) has focus, every other key is
	// swallowed rather than typed into a blurred input.
	if m.toggleFocused {
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// exportFollowCmd writes store.Export's document to the path the user typed.
// Failures route through lastError via RefreshListsMsg{Err: ...}, the same
// channel a failed delete uses (docs/DESIGN.md §9).
func (m Model) exportFollowCmd(path string) tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return nil
		}

		target := m.listID
		if m.wholeStore {
			target = nil
		}
		doc, err := m.store.Export(target)
		if err != nil {
			return cmds.RefreshListsMsg{Err: err}
		}
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return cmds.RefreshListsMsg{Err: err}
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return cmds.RefreshListsMsg{Err: err}
		}
		// Success: just refresh + close. The write needs no confirmation —
		// the user named the file, so they know where it landed.
		return cmds.RefreshLists(m.store)()
	}
}

func (m Model) View() tea.View {
	// Seal the input onto the modal surface every render: the bubbles
	// textinput default carries no foreground on focused text, which
	// vanishes on a light theme's modal (farol-day). Same per-render
	// discipline as listnamemodal and detailspanel (docs/DESIGN.md §12).
	chrome.SealInput(&m.input, appstyles.Active.ModalBg, appstyles.Active.ModalBg)
	// Size the input from the terminal width so the placeholder renders
	// in full. bubbles v2 textinput's placeholderView truncates to the
	// first character when Width is 0, leaving a stray 'p' (first rune of
	// "path/to/file.json") as the cursor char. ModalSurface spends 6 cols
	// on border+padding; cap the field at 50 so it never grows comically
	// wide on a large terminal.
	m.input.SetWidth(max(10, min(m.termWidth-6, 50)))

	lines := []string{chrome.ModalTitle("Export"), m.input.View()}

	// Reuses the task row's own checkbox glyphs (◻ unchecked, ◼ checked —
	// docs/DESIGN.md §12) rather than inventing a new one for this toggle.
	box := "◻"
	label := "export this list"
	if m.wholeStore {
		box = "◼"
		label = "export whole store"
	}
	toggleStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)
	if m.toggleFocused {
		toggleStyle = toggleStyle.Foreground(appstyles.Active.Accent)
	}
	lines = append(lines, "", toggleStyle.Render(box+" "+label))

	hints := []chrome.KeyHint{
		chrome.HintFor(keys.Overlay.Submit),
		chrome.HintFor(keys.Overlay.Cancel),
		chrome.HintFor(keys.ExportModal.NextField),
		chrome.HintAs(keys.ListNameModal.ToggleCollaborative, "flip whole/list"),
	}
	lines = append(lines, "", chrome.RenderKeyHints(hints, appstyles.Active.TextMuted))

	body := lipgloss.JoinVertical(lipgloss.Left, lines...)

	return tea.NewView(chrome.ModalSurface(appstyles.Active.ModalBg, body))
}
