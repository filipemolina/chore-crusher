package model

import (
	"charm.land/lipgloss/v2"
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-completer/src/appstyles"
	"github.com/filipemolina/chore-completer/src/constants"
)

// View renders the whole screen. The zones render through their own
// chrome.PanelFrame calls; this function composes them, seals the tier-2
// background, and composites the modal on top.
func (m AppModel) View() tea.View {
	layout := m.renderBody()

	// Seal the frame against tier 2. JoinVertical/JoinHorizontal pad the
	// narrower pieces out to the body width with unstyled spaces, and an
	// outer Background() style cannot fix that — it only paints the padding
	// it adds itself. This is the outermost tier, so it must run last: every
	// inner tier (each panel's PanelFrame) has already sealed its own
	// region, which leaves no unpainted cell inside a panel for this pass to
	// reach (see appstyles.FillBackground).
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

// renderBody renders the zones: the main panel (task tree over the add
// input) and, while the lists panel is visible, the sidebar with a thin
// tier-2 gutter between them. The gutter's width is the same constant the
// layout subtracted from the row before sizing the panels, so the three
// pieces add up to the terminal width exactly.
func (m AppModel) renderBody() string {
	layout := m.bodyLayout

	main := lipgloss.JoinVertical(lipgloss.Left,
		m.components.TaskTree.View().Content,
		m.components.AddInput.View().Content,
	)

	if !m.listsPanelVisible {
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
		m.components.ListsPanel.View().Content,
		gutter,
		main,
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
