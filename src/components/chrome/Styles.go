package chrome

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/appstyles"
)

// WrapperStyle is the frame every zone renders inside: Padding(1, 2), fixed,
// matching Stack Stitcher's PanelFrame exactly (docs/DESIGN.md §12 "Two
// shared frames"). No component sets its own padding value - the shared
// frame is what keeps a section header's left edge, a checkbox's left edge,
// and a list row's left edge all landing on the same inset regardless of
// which surface they're in. Its frame size is subtracted from the zone box
// when inner content is sized.
var WrapperStyle = lipgloss.NewStyle().
	Padding(1, 2)

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

// ModalSurface wraps a modal's content in the shared modal chrome: an accent
// rounded border, padding, and a background sealed against `bg` so the modal
// reads as one opaque surface over the page it is composited onto. Matches
// Stack Stitcher's ModalSurface exactly — panels go borderless (elevation is
// the only focus signal, docs/DESIGN.md §12), but a modal floats over the
// page rather than sitting structurally within it, and needs its own edge to
// read as a distinct layer. Modals in particular cannot afford an unpainted
// cell - the page shows through it.
//
// BorderBackground is set explicitly because lipgloss leaves border cells on
// the default background otherwise, which outlines the modal in the
// terminal's color.
func ModalSurface(bg color.Color, content string) string {
	style := lipgloss.NewStyle().
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(appstyles.Active.Accent).
		BorderBackground(bg).
		Background(bg)

	return appstyles.FillBackground(bg, style.Render(content))
}

// Scrim dims the whole page behind a modal to one flat muted tier, so no
// styled fragment of the page underneath it — a status pill's tail, an
// empty-state card's border — can still read as distinct text once the
// modal is composited on top (docs/DESIGN.md §12 "Modal scrim"; the bug
// this fixes: overlays leave orphan fragments of the rows underneath).
//
// lipgloss's compositor has no alpha blending — a layer only ever draws
// opaquely (see lipgloss.Layer) — so a translucent-looking veil is not
// available, and wrapping the already-styled page in an outer Background()
// style does not work either: it only paints cells the page left unstyled
// (see appstyles.FillBackground's doc comment), leaving every glyph's own
// embedded color untouched underneath. Scrim instead strips every SGR the
// page carries and re-renders the plain text in one TextDim-on-
// BackgroundContent tier, which recolors every cell uniformly because
// nothing embedded is left to compete with it.
func Scrim(page string) string {
	return lipgloss.NewStyle().
		Foreground(appstyles.Active.TextDim).
		Background(appstyles.Active.BackgroundContent).
		Render(ansi.Strip(page))
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

// ListRowBg is the background a list row renders on. The active row of
// a FOCUSED panel is lifted to the surface tier; every other row sits flush
// on its panel's tier. An active row in an UNFOCUSED panel reads as
// "remembered" (BackgroundElevated) rather than "live" (ModalBg), so only
// one panel ever shows a live selected row at a time. Rows need an explicit
// background (rather than inheriting the panel's) because each row is
// rendered and sealed on its own - see appstyles.FillBackground.
func ListRowBg(isActive bool, isParentFocused bool) color.Color {
	if isActive && isParentFocused {
		return appstyles.Active.ModalBg
	}
	return appstyles.Active.BackgroundElevated
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

// SealInput themes a bubbles textinput onto the surfaces it renders on, so
// its typed text never inherits the terminal's default foreground. The
// default style a textinput.New() ships carries NO foreground on its
// focused Text (and a hardcoded ANSI-256 white on the blurred one), which
// reads on the dark panels but disappears on a light theme's panel
// (crush-day: white text on warm off-white). Every tier here comes from the
// active theme: the typed text is TextPrimary in both focus states, the
// placeholder is TextDim (§12's inert-text tier), and the two states sit on
// the caller's own surfaces. Rebuilt every render, the same way detailspanel's
// applyTextareaStyles does it, so a theme switch while the input was last
// drawn cannot leave a stale palette behind.
//
// focusedBg is the surface the input lifts to while it owns the keyboard;
// blurredBg is the one it sits on otherwise — a parked input on the same
// surface as its card passes the card's color twice (docs/DESIGN.md §12,
// "Focus is shown by lifting a tier").
func SealInput(in *textinput.Model, focusedBg, blurredBg color.Color) {
	st := in.Styles()
	st.Focused.Text = lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Background(focusedBg)
	st.Blurred.Text = lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Background(blurredBg)
	st.Focused.Placeholder = lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Background(focusedBg)
	st.Blurred.Placeholder = lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Background(blurredBg)
	// The bubbles default prompt is a hardcoded ANSI white "> ", so even an
	// input whose caller cleared nothing still leaks default color through
	// its prompt glyph. Style it as an inline hint instead: TextMuted, no
	// background, so a sealed input never carries a terminal-default glyph.
	promptStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted)
	st.Focused.Prompt = promptStyle
	st.Blurred.Prompt = promptStyle
	st.Cursor.Color = appstyles.Active.TextPrimary
	in.SetStyles(st)
}

// SealListFilter themes a bubbles list's built-in filter input onto the
// panel it lives in. list.New() ships DefaultStyles(true) — hardcoded dark
// assumptions — so the filter bar carries the same default-color text as an
// unsealed textinput, and its "Filter: " prompt the hardcoded white of
// textinput's defaults. Every tier comes from the active theme, matching
// SealInput's; the prompt glyph keeps its "Filter: " wording (the lists
// panel's filter announces itself that way, unlike the task tree's bare
// slash bar) but draws in TextMuted on the panel's own surface. Rebuilt
// every render by the caller so a theme switch cannot leave a stale palette
// on the bar (docs/DESIGN.md §12).
func SealListFilter(l *list.Model, bg color.Color) {
	// The filter input only ever draws while the panel is in Filtering
	// state, so one surface serves both focus states.
	st := l.FilterInput.Styles()
	st.Focused.Text = lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Background(bg)
	st.Blurred.Text = lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Background(bg)
	st.Focused.Placeholder = lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Background(bg)
	st.Blurred.Placeholder = lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Background(bg)
	promptStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted).Background(bg)
	st.Focused.Prompt = promptStyle
	st.Blurred.Prompt = promptStyle
	st.Cursor.Color = appstyles.Active.TextPrimary
	l.FilterInput.SetStyles(st)

	// The list's own text tiers ride the same defaults: the "No items."
	// empty-state line and the pagination glyphs draw in subdued greys that
	// are fine on dark but read as terminal noise on a light panel. Point
	// them at the dim tier instead.
	styles := l.Styles
	styles.NoItems = lipgloss.NewStyle().Foreground(appstyles.Active.TextDim).Background(bg)
	l.Styles = styles
}
