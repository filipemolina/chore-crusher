package model

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/components/keybindingbar"
	"github.com/filipemolina/chore-crusher/src/constants"
)

// View renders the whole screen. The header, body zones, and footer compose
// on a tier-2 background; the modal, if any, is layered on top.
func (m AppModel) View() tea.View {
	header := m.components.MainMenu.View().Content
	body := m.renderBody()
	footer := m.footerView()

	layout := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)

	// Seal the frame against tier 2. JoinVertical pads the narrower pieces
	// out to the terminal width with unstyled spaces, and an outer
	// Background() style cannot fix that — it only paints the padding it adds
	// itself. This is the outermost tier, so it must run last: every inner
	// tier (header, footer, and each panel's PanelFrame) has already sealed
	// its own region, which leaves no unpainted cell for this pass to reach.
	layout = appstyles.FillBackground(appstyles.Active.BackgroundContent, layout)

	// Wrap the full layout in a style that fills the terminal width with
	// the tier-2 background. MaxWidth/MaxHeight are the backstop: Width()
	// pads but never truncates, so anything rendered wider than the terminal
	// would otherwise be wrapped by the terminal itself.
	rendered := lipgloss.NewStyle().
		Background(appstyles.Active.BackgroundContent).
		Width(m.terminalWidth).
		Height(m.terminalHeight).
		MaxWidth(m.terminalWidth).
		MaxHeight(m.terminalHeight).
		Render(layout)

	// The Task details modal and any other modal are both centered overlays.
	// Details is layered first so a confirm/error modal (were one ever opened
	// over it) would sit on top; in practice they are mutually exclusive.
	if m.detailsPanelVisible {
		rendered = m.overlayModal(rendered, m.components.DetailsPanel.View().Content)
	}
	if m.activeModal != nil {
		rendered = m.overlayModal(rendered, m.activeModal.View().Content)
	}

	// AltScreen is set unconditionally so the app never drops back to the
	// terminal's normal buffer mid-run (the zero tea.View leaves AltScreen
	// false, which reads as a crash).
	v := tea.NewView(rendered)
	v.AltScreen = true

	return v
}

// footerView renders the keybinding bar for the frame being drawn right now.
// The bar's own Update only learns of context changes (creating, filtering,
// focus) via SetFooterContextMsg, a command whose message is not delivered
// until the NEXT Bubble Tea cycle — rendering the bar's stored ctx here would
// always show the context computed one keystroke ago. Handing it this
// frame's helpContext() directly, the same way SetBodyLayoutMsg's width is
// otherwise threaded through, keeps the footer in lockstep with what the
// user just pressed (bug: footer key hints lag the current mode by one
// keystroke).
func (m AppModel) footerView() string {
	bar, ok := m.components.KeybindingBar.(keybindingbar.Model)
	if !ok {
		return m.components.KeybindingBar.View().Content
	}
	return bar.WithContext(m.helpContext()).View().Content
}

// renderBody renders the Tasks surface and, while visible, the Lists surface
// separated by a sealed tier-2 gutter. Before the first WindowSizeMsg the body
// height is 0 and the components have not yet been sized; their natural render
// still leaves the header and footer as the frame boundary. The Details modal
// is not a body surface — it is composited over this in View.
func (m AppModel) renderBody() string {
	layout := m.bodyLayout
	main := m.components.TaskPanel.View().Content

	// Lists is the only side surface; render Tasks alone when it is not in the
	// layout (it yields at a narrow width by getting ListsWidth == 0).
	if !m.listsPanelRendered() {
		return main
	}
	side := m.components.ListsPanel.View().Content

	// Before the first WindowSizeMsg the broadcast height is 0; fall back to
	// the tallest rendered piece so the gutter still spans the body.
	bodyHeight := layout.Height
	if bodyHeight == 0 {
		bodyHeight = lipgloss.Height(main)
	}

	gutter := lipgloss.NewStyle().
		Background(appstyles.Active.BackgroundContent).
		Width(constants.BODY_GUTTER_WIDTH).
		Height(bodyHeight).
		Render("")

	return lipgloss.JoinHorizontal(lipgloss.Top,
		main,
		gutter,
		side,
	)
}

// overlayModal composites modalContent as a centered layer on top of the rest
// of the screen (stack-stitcher's pattern: clamp y at 0 so a modal taller than
// the terminal loses its bottom edge rather than scrolling). Both the Task
// details modal and the activeModal overlays go through here, so scrimming
// base here — rather than in each modal — dims the page behind every one of
// them (confirm, help, theme picker, search picker, details) the same way.
func (m AppModel) overlayModal(base, modalContent string) string {
	x := max(0, (m.terminalWidth-lipgloss.Width(modalContent))/2)
	y := max(0, (m.terminalHeight-lipgloss.Height(modalContent))/2)

	baseLayer := lipgloss.NewLayer(chrome.Scrim(base))
	modalLayer := lipgloss.NewLayer(modalContent).X(x).Y(y).Z(1)

	return lipgloss.NewCompositor(baseLayer, modalLayer).Render()
}
