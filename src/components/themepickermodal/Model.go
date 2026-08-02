package themepickermodal

import (
	"slices"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
)

// Model is the theme picker: a list of registered themes with live preview
// on cursor movement. Enter applies and persists; Esc restores the theme
// that was active when the modal opened.
type Model struct {
	// originalTheme is the theme name to restore on cancel. Captured once
	// at construction time so Esc always goes back to what the user started
	// with, even after several preview cursor movements.
	originalTheme string
	list          list.Model
}

func (m Model) Init() tea.Cmd {
	return nil
}

// New builds the theme picker overlay. It lists every registered theme,
// marks the one currently active, and starts the cursor on it. Moving the
// cursor previews that theme live; Enter applies and persists; Esc restores
// the original.
//
// termHeight is the terminal height in rows — used to size the list so it
// never overflows the modal chrome (borders, title, hints).
func New(termHeight int) tea.Model {
	currentTheme := appstyles.Active.Name

	// Collect and sort theme names for a stable order.
	names := make([]string, 0, len(appstyles.Themes))
	for name := range appstyles.Themes {
		names = append(names, name)
	}
	slices.Sort(names)

	items := make([]list.Item, 0, len(names))
	activeIndex := 0
	for i, name := range names {
		items = append(items, apptypes.ThemeItem{Name: name, Active: name == currentTheme})
		if name == currentTheme {
			activeIndex = i
		}
	}

	visible := chrome.ModalListHeight(len(items), termHeight)

	picker := list.New(items, themePickerDelegate{}, 40, visible)
	picker.SetShowTitle(false)
	picker.SetShowHelp(false)
	picker.SetShowStatusBar(false)
	picker.SetShowPagination(visible < len(items))
	picker.SetShowFilter(false)
	picker.Select(activeIndex)

	return Model{
		originalTheme: currentTheme,
		list:          picker,
	}
}
