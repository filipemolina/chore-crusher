package helpoverlay

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/keys"
)

// helpOverlayMaxWidth caps the content column so hint runs wrap in a few
// places on wide terminals rather than stretching into one unreadable line.
const helpOverlayMaxWidth = 64

// Model is the ? overlay: every key in the app, grouped by scope and
// rendered from keys.Catalog, so what it says is what the handlers do.
// Rows the user could not press in the screen it was opened from are dimmed.
type Model struct {
	catalog   []keys.Scope
	termWidth int
}

func (m Model) Init() tea.Cmd { return nil }

// New builds the help overlay for the screen described by ctx (which keys
// are pressable) and the terminal width for wrapping.
func New(ctx keys.Context, termWidth int) tea.Model {
	return Model{
		catalog:   keys.Catalog(ctx),
		termWidth: termWidth,
	}
}
