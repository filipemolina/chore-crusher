package listnamemodal

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/keys"
	"github.com/filipemolina/chore-crusher/src/store"
)

// Mode is whether the modal is for creating a new list or renaming an existing one.
type Mode int

const (
	ModeNew Mode = iota
	ModeRename
)

// Model is the list name input modal (create or rename). Both modes support
// the collaborative toggle (docs/DESIGN.md §9, "Tag a list as collaborative")
// — the human-only way to set it; there is no MCP tool. ModeRename seeds the
// toggle from the store at construction so opening rename never silently
// resets an existing list's flag.
type Model struct {
	mode   Mode
	listID string
	input  textinput.Model
	store  *store.Store

	// collaborative and origCollaborative track the toggle state. origCollaborative
	// is only meaningful for ModeRename (to detect changes); for ModeNew it's
	// always false. toggleFocused reports whether tab has moved focus off the
	// name field onto the toggle; space flips it there.
	collaborative     bool
	origCollaborative bool
	toggleFocused     bool
}

// New creates a new list name modal. For ModeRename it reads the list's
// current collaborative flag so the toggle starts at its real value; a read
// failure leaves it at the zero value (false) — the same "TODO: surface
// error to user" laxness the rest of this file already has.
func New(mode Mode, listID string, s *store.Store) tea.Model {
	input := textinput.New()
	input.Focus()
	// The bubbles default prompt is a hardcoded ANSI-white "> ", which would
	// leak a default-colored glyph into the modal body. This modal shows the
	// field with its ModalTitle line alone, like detailspanel's inputs.
	input.Prompt = ""

	m := Model{mode: mode, listID: listID, input: input, store: s}
	if mode == ModeRename {
		input.Placeholder = "new name"
		if s != nil {
			if l, err := s.GetList(listID); err == nil {
				m.collaborative = l.Collaborative
				m.origCollaborative = l.Collaborative
			}
		}
	}
	return m
}

func (m Model) Init() tea.Cmd { return textinput.Blink }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keys.Overlay.Submit):
			name := strings.TrimSpace(m.input.Value())
			// ModeNew requires a name — nothing to fall back to. ModeRename
			// allows submitting with the name field untouched (empty) so a
			// collaborative-only toggle can be saved without retyping the
			// name; createFollowCmd only renames when name is non-empty.
			if name == "" && m.mode == ModeNew {
				return m, nil
			}
			followCmd := m.createFollowCmd(name)
			return m, cmds.CloseModal(followCmd)
		case key.Matches(msg, keys.Overlay.Cancel):
			return m, cmds.CloseModal(nil)
		case key.Matches(msg, keys.ListNameModal.NextField):
			m.toggleFocused = !m.toggleFocused
			if m.toggleFocused {
				m.input.Blur()
			} else {
				m.input.Focus()
			}
			return m, nil
		case key.Matches(msg, keys.ListNameModal.ToggleCollaborative):
			if m.toggleFocused {
				m.collaborative = !m.collaborative
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

// createFollowCmd creates a command that creates or renames the list, and
// writes the collaborative flag when it changed (or when creating a new list
// with collaborative enabled).
func (m Model) createFollowCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return nil
		}

		if m.mode == ModeNew {
			id, err := m.store.CreateList(name, "")
			if err != nil {
				// TODO: surface error to user
				return nil
			}
			// Set collaborative flag if enabled
			if m.collaborative {
				if err := m.store.SetCollaborative(id, true); err != nil {
					// TODO: surface error to user
					return nil
				}
			}
			// A new list is the one worth landing on: AppModel selects it,
			// refreshes, and closes the transient Lists picker (docs/DESIGN.md
			// §5) — the plain RefreshLists a rename uses below has no way to
			// say "land here specifically".
			return cmds.ListCreatedMsg{ID: id}
		}

		if name != "" {
			if err := m.store.RenameList(m.listID, name); err != nil {
				// TODO: surface error to user
				return nil
			}
		}
		if m.collaborative != m.origCollaborative {
			if err := m.store.SetCollaborative(m.listID, m.collaborative); err != nil {
				// TODO: surface error to user
				return nil
			}
		}

		// Refresh lists after renaming/toggling.
		return cmds.RefreshLists(m.store)()
	}
}

func (m Model) View() tea.View {
	title := "New list"
	if m.mode == ModeRename {
		title = "Rename list"
	}

	// Seal the input onto the modal surface every render: the bubbles
	// textinput default carries no foreground on focused text, which
	// vanishes on a light theme's modal (crush-day). Same per-render
	// discipline as detailspanel's inputs (docs/DESIGN.md §12).
	chrome.SealInput(&m.input, appstyles.Active.ModalBg, appstyles.Active.ModalBg)

	lines := []string{chrome.ModalTitle(title), m.input.View()}

	// Reuses the task row's own checkbox glyphs (◻ unchecked, ◼ checked —
	// docs/DESIGN.md §12) rather than inventing a new one for this toggle.
	box := "◻"
	if m.collaborative {
		box = "◼"
	}
	toggleStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)
	if m.toggleFocused {
		toggleStyle = toggleStyle.Foreground(appstyles.Active.Accent)
	}
	lines = append(lines, "", toggleStyle.Render(box+" collaborative — any agent may restructure this list"))

	hints := []chrome.KeyHint{chrome.HintFor(keys.Overlay.Submit), chrome.HintFor(keys.Overlay.Cancel)}
	hints = append(hints, chrome.HintFor(keys.ListNameModal.NextField), chrome.HintFor(keys.ListNameModal.ToggleCollaborative))
	lines = append(lines, "", chrome.RenderKeyHints(hints, appstyles.Active.TextMuted))

	body := lipgloss.JoinVertical(lipgloss.Left, lines...)

	return tea.NewView(chrome.ModalSurface(appstyles.Active.ModalBg, body))
}
