package chrome

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
)

// WrapperStyle is the frame every zone renders inside: Padding(0).
// No component sets its own padding value - the shared frame is what keeps a
// section header's left edge, a checkbox's left edge, and the add input's
// left edge all landing on the same column when the zones stack vertically
// in the main panel (docs/DESIGN.md §12 "One shared frame"). Its frame size
// is subtracted from the zone box when inner content is sized.
var WrapperStyle = lipgloss.NewStyle()

// FitBox constrains a style to an exact w x h box: Width/Height pad it out,
// Max* clip anything that would otherwise overflow (Width alone pads but
// never truncates, which is how a too-wide panel ends up wrapped by the
// terminal). Non-positive dimensions are left unset so a component still
// renders naturally before the first SetBodyLayoutMsg arrives.
func FitBox(s lipgloss.Style, w, h int) lipgloss.Style {
	if w > 0 {
		s = s.Width(w).MaxWidth(w)
	}

	if h > 0 {
		s = s.Height(h).MaxHeight(h)
	}

	return s
}

// PanelBg is the background tier a zone renders on: tier 4 when focused,
// tier 3 otherwise. Focus lifts the whole panel rather than adding a border,
// so the zone's box stays exactly the same size either way
// (docs/DESIGN.md §12 "Focus is shown by lifting a tier"). This is the only
// place the focused/unfocused decision is made.
func PanelBg(isFocused bool) color.Color {
	if isFocused {
		return appstyles.Active.BackgroundElevated
	}

	return appstyles.Active.BackgroundPanel
}

// ModalSurface wraps a modal's content in the shared modal chrome: padding
// and a background sealed against `bg` so the modal reads as one opaque
// surface over the page it is composited onto. Modals in particular cannot
// afford an unpainted cell - the page shows through it.
func ModalSurface(bg color.Color, content string) string {
	style := lipgloss.NewStyle().
		Padding(1, 2).
		Background(bg)

	return appstyles.FillBackground(bg, style.Render(content))
}

// ModalTitle renders a modal's heading. Every modal names itself, so a user
// who lands on one mid-flow can tell what it is about to do without having to
// infer it from the fields. It is the shared accent chip - appstyles.NormalTitle
// - stood off the body by its own margin, so a style or theme change to the
// chip lands on modals and panes alike.
//
// The margin replaces the blank line each caller would otherwise have to add -
// it matches the blank row the hint line sits above: the heading and the
// footer are the modal's chrome, and both stand off the body.
func ModalTitle(text string) string {
	return appstyles.NormalTitle().
		MarginBottom(1).
		Render(text)
}

// modalListChrome is the rows a list-in-a-modal spends on everything that is
// not a list row: ModalSurface's border (2) and padding (2), the ModalTitle
// and the blank row its margin leaves (2), the blank row above the hints (1),
// and the two hint lines (2).
const modalListChrome = 9

// ModalListHeight is the height to build a modal's list with so the modal
// fits a terminal termHeight rows tall.
//
// renderWithModal (src/model/View.go) centers a modal by clamping y to 0, so
// a modal taller than the terminal does not scroll or shrink - it loses its
// hint line and bottom border off the bottom of the screen. A list sized to
// len(items) is therefore a latent overflow on any project big enough, and
// the caller pairs this with SetShowPagination(height < len(items)) so the
// rows that no longer fit stay reachable and say so.
//
// The floor of 3 is deliberate: below about 12 rows there is no honest answer,
// and a terminal that short cannot show the modal's own chrome either.
func ModalListHeight(items, termHeight int) int {
	return min(items, max(3, termHeight-modalListChrome))
}

// ListWrapperStyle is the frame around the body lists. Its padding is
// what separates the list content from the panel edges, and its frame size is
// subtracted from the panel box when the inner list is sized.
var ListWrapperStyle = lipgloss.NewStyle().
	Padding(1, 2, 2, 2)

// ListRowBg is the background a list row renders on. The active row is lifted
// to the surface tier; every other row sits flush on its panel's tier. Rows
// need an explicit background (rather than inheriting the panel's) because each
// row is rendered and sealed on its own - see appstyles.FillBackground.
func ListRowBg(isActive bool, isParentFocused bool) color.Color {
	if isActive {
		return appstyles.Active.ModalBg
	}
	return PanelBg(isParentFocused)
}

// BarColumn renders the nav's ▌ indicator once per line of content, so the
// bar spans a multi-line row's full height instead of a sliver at its top.
// bg may be nil to leave the cell background unset.
func BarColumn(fg color.Color, bg color.Color, content string) string {
	style := lipgloss.NewStyle().Foreground(fg)
	if bg != nil {
		style = style.Background(bg)
	}
	lines := max(1, strings.Count(content, "\n")+1)
	bar := style.Render("▌")
	return strings.Repeat(bar+"\n", lines-1) + bar
}
