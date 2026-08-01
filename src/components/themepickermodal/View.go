package themepickermodal

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-completer/src/appstyles"
	"github.com/filipemolina/chore-completer/src/apptypes"
	"github.com/filipemolina/chore-completer/src/components/chrome"
	"github.com/filipemolina/chore-completer/src/keys"
)

// themePickerDelegate renders one theme name per row, marking the active
// theme and highlighting the cursor.
type themePickerDelegate struct{}

func (d themePickerDelegate) Height() int                             { return 1 }
func (d themePickerDelegate) Spacing() int                            { return 0 }
func (d themePickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d themePickerDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(apptypes.ThemeItem)
	if !ok {
		return
	}

	style := lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted)
	if index == m.Index() {
		style = style.Foreground(appstyles.Active.TextPrimary).Bold(true)
	} else if item.Active {
		style = style.Foreground(appstyles.Active.Accent)
	}

	fmt.Fprint(w, style.Render(item.Title()))
}

// themePickerHints is the modal's own help line. Two rows: navigation on
// the first, confirm/cancel on the second.
func themePickerHints() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		chrome.RenderKeyHints([]chrome.KeyHint{
			chrome.HintFor(keys.Overlay.Navigation),
		}, appstyles.Active.TextMuted),
		chrome.RenderKeyHints([]chrome.KeyHint{
			chrome.HintAs(keys.Overlay.Submit, "apply"),
			chrome.HintFor(keys.Overlay.Cancel),
		}, appstyles.Active.TextMuted),
	)
}

func (m Model) View() tea.View {
	content := lipgloss.JoinVertical(lipgloss.Left, chrome.ModalTitle("Choose theme"), m.list.View(), "", themePickerHints())

	return tea.NewView(chrome.ModalSurface(appstyles.Active.ModalBg, content))
}
