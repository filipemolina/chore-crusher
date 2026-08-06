package model

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/constants"
)

// View renders the whole screen. The header, body zones, and footer compose
// on a tier-2 background; the modal, if any, is layered on top.
func (m AppModel) View() tea.View {
	header := m.components.MainMenu.View().Content
	body := m.renderBody()
	footer := m.components.KeybindingBar.View().Content

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

	if m.activeModal != nil {
		rendered = m.renderWithModal(rendered)
	}

	// AltScreen is set unconditionally so the app never drops back to the
	// terminal's normal buffer mid-run (the zero tea.View leaves AltScreen
	// false, which reads as a crash).
	v := tea.NewView(rendered)
	v.AltScreen = true

	return v
}

// renderBody renders the Tasks surface and, while visible, the Lists surface
// separated by a sealed tier-2 gutter. Before the first WindowSizeMsg the body
// height is 0 and the components have not yet been sized; their natural render
// still leaves the header and footer as the frame boundary.
func (m AppModel) renderBody() string {
	layout := m.bodyLayout
	main := m.components.TaskPanel.View().Content

	// Details, when the terminal is too narrow for a side surface, is the only
	// body surface: Tasks is not rendered until it closes (docs/DESIGN.md §5).
	if m.detailsPanelVisible && layout.MainWidth == 0 {
		return m.components.DetailsPanel.View().Content
	}

	// Exactly one side surface may accompany Tasks. Pick it, or render Tasks
	// alone when neither is in the layout (Lists yields at a narrow width by
	// getting ListsWidth == 0, which lands here too).
	var side string
	switch {
	case m.detailsPanelVisible && layout.DetailsWidth > 0:
		side = m.components.DetailsPanel.View().Content
	case m.listsPanelVisible && layout.ListsWidth > 0:
		side = m.components.ListsPanel.View().Content
	default:
		return main
	}

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

// renderWithModal composites the active modal as a centered layer on top of
// the rest of the screen (stack-stitcher's pattern: clamp y at 0 so a modal
// taller than the terminal loses its bottom edge rather than scrolling).
func (m AppModel) renderWithModal(base string) string {
	modalContent := m.activeModal.View().Content

	x := max(0, (m.terminalWidth-lipgloss.Width(modalContent))/2)
	y := max(0, (m.terminalHeight-lipgloss.Height(modalContent))/2)

	baseLayer := lipgloss.NewLayer(base)
	modalLayer := lipgloss.NewLayer(modalContent).X(x).Y(y).Z(1)

	return lipgloss.NewCompositor(baseLayer, modalLayer).Render()
}
