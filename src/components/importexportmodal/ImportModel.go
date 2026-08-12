package importexportmodal

import (
	"encoding/json"
	"fmt"
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

// ImportModel is the import modal: a single source file path. On confirm it
// parses the versioned document and recreates each list via
// store.ImportList (additive, fresh ids — docs/DESIGN.md §9, export/import).
// It imports the whole document (every list in the file), matching the CLI's
// default `farol import <file>` with no --list.
type ImportModel struct {
	store *store.Store
	input textinput.Model
}

// NewImport creates the import modal focused on its path input.
func NewImport(s *store.Store) tea.Model {
	input := textinput.New()
	input.Focus()
	// Same default-prompt leak guard as the other modals: SealInput (called
	// in View) strips the foreground so it reads on a light theme's modal.
	input.Prompt = ""
	input.Placeholder = "path/to/file.json"

	return ImportModel{store: s, input: input}
}

func (m ImportModel) Init() tea.Cmd { return textinput.Blink }

func (m ImportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keys.Overlay.Submit):
			path := strings.TrimSpace(m.input.Value())
			// No path, nothing to read — same as an empty create name.
			if path == "" {
				return m, nil
			}
			return m, cmds.CloseModal(m.importFollowCmd(path))
		case key.Matches(msg, keys.Overlay.Cancel):
			return m, cmds.CloseModal(nil)
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// importFollowCmd reads and parses the export file, then recreates each list.
// Failures route through lastError via RefreshListsMsg{Err: ...}, the same
// channel a failed delete uses (docs/DESIGN.md §9).
func (m ImportModel) importFollowCmd(path string) tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return cmds.RefreshListsMsg{Err: err}
		}
		var doc store.ExportDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			return cmds.RefreshListsMsg{Err: fmt.Errorf("parse export file: %w", err)}
		}
		if doc.Version != store.ExportVersion {
			return cmds.RefreshListsMsg{Err: fmt.Errorf("unsupported export version %d (want %d)", doc.Version, store.ExportVersion)}
		}
		for _, el := range doc.Lists {
			if err := m.store.ImportList(el); err != nil {
				return cmds.RefreshListsMsg{Err: err}
			}
		}
		// Success: refresh so the new lists appear in the Lists panel. Import
		// only appends, so no list becomes active — RefreshTasks is not needed.
		return cmds.RefreshLists(m.store)()
	}
}

func (m ImportModel) View() tea.View {
	// Seal the input onto the modal surface every render (see ExportModel /
	// listnamemodal for why).
	chrome.SealInput(&m.input, appstyles.Active.ModalBg, appstyles.Active.ModalBg)

	lines := []string{
		chrome.ModalTitle("Import"),
		m.input.View(),
		"",
		lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted).Render("new lists are created with fresh ids — existing lists are untouched"),
	}

	hints := []chrome.KeyHint{
		chrome.HintFor(keys.Overlay.Submit),
		chrome.HintFor(keys.Overlay.Cancel),
	}
	lines = append(lines, "", chrome.RenderKeyHints(hints, appstyles.Active.TextMuted))

	body := lipgloss.JoinVertical(lipgloss.Left, lines...)

	return tea.NewView(chrome.ModalSurface(appstyles.Active.ModalBg, body))
}
