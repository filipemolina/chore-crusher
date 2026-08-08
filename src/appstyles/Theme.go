// Package appstyles holds the Theme type and the 14-theme registry, ported
// from stack-stitcher (github.com/filipemolina/stack-stitcher/src/appstyles)
// with only the status-color field names changed to fit this app's domain
// (docs/DESIGN.md §11). Do not redesign the derivation math; import it.
package appstyles

import (
	"image/color"
	"math"

	"charm.land/lipgloss/v2"
)

// Theme is every color the app draws with, resolved to concrete values. One
// field per semantic token, so a registered Theme is the app's complete
// visual vocabulary - no component builds a color of its own.
//
// A Theme is inert data, not a service: nothing here reads the terminal or
// does I/O. Active (below) is the one Theme in effect, and everything that
// draws reads it fresh on every render rather than caching a color at
// package init - see the package-level styles in styles.go for why that
// distinction matters. That is the whole of what lets a later switch repaint
// the app: assign a different registered Theme to Active and the next frame
// draws it.
type Theme struct {
	Name string
	// Dark says which way the tiers derive: a dark theme raises a surface's
	// attention by lightening it, a light theme by darkening it. See
	// newTheme.
	Dark bool

	// Accent is the brand color: focus, the wordmark, the active tab, title
	// chips. It does not vary with Dark - see newTheme.
	Accent color.Color

	// Text tiers, most to least emphasis.
	TextPrimary color.Color
	TextMuted   color.Color
	TextDim     color.Color

	// PanelBg is the base surface color every background tier below derives
	// from, and is also BackgroundRecessed's value directly.
	PanelBg color.Color
	// BackgroundContent/Panel/Elevated are tiers 2/3/4 in docs/DESIGN.md's
	// "Background tiers, and sealing them": the frame, an unfocused panel,
	// and a focused panel.
	BackgroundContent  color.Color
	BackgroundPanel    color.Color
	BackgroundElevated color.Color
	// BackgroundRecessed sits *below* the panel tier, for insets like the
	// empty-state cards - see docs/DESIGN.md. Equal to PanelBg by
	// construction: both are the tier ladder's un-raised base.
	BackgroundRecessed color.Color
	// ModalBg is the surface every modal - and an active list row - is drawn
	// on: a distinct register from the panel tiers, not derived from
	// PanelBg.
	ModalBg color.Color

	// BorderDefault rims an ordinary panel; in a dark theme it is darker
	// than PanelBg so it all but disappears against a *recessed* fill.
	// BorderCard rims a recessed surface, so it has to go the other way -
	// lighter than PanelBg in a dark theme - or the rim vanishes into what
	// it is meant to outline. See docs/DESIGN.md.
	BorderDefault color.Color
	BorderCard    color.Color

	// Status tiers reflect one task's own state, and do not vary with Dark:
	// a "complete" checkbox is the same green whichever theme is active.
	// StatusOverdue is reserved - a due-date feature does not exist yet (see
	// docs/ROADMAP.md) - but its field and palette entries are carried
	// forward so the registry does not churn when it lands. See
	// InkOnLight/InkOnDark for the text that sits on top of one.
	StatusComplete   color.Color
	StatusPending    color.Color
	StatusInProgress color.Color
	StatusOverdue    color.Color

	// Danger is app-level alert chrome - the error banner, an inline
	// validation message - a different concept from StatusOverdue (one
	// task's own state) even though earlier code used one hex for both
	// by coincidence.
	Danger color.Color

	// InkOnLight and InkOnDark are deliberately theme-invariant: a status
	// pill's fill (StatusComplete green, StatusInProgress amber,
	// StatusOverdue red/pink) is the same hue whichever theme is active, so
	// the text that reads legibly on it can't follow Dark either - a bright
	// pill needs dark ink and a dark pill needs light ink regardless of the
	// *app's* theme.
	InkOnLight color.Color
	InkOnDark  color.Color
}

// themeParams are the handful of base colors newTheme builds a Theme from.
// Everything else is derived - see newTheme.
type themeParams struct {
	Name string
	Dark bool

	Accent color.Color
	Text   color.Color
	Panel  color.Color
	Modal  color.Color
	Danger color.Color

	Complete, Pending, InProgress, Overdue color.Color
}

// newTheme derives a full Theme from a handful of base colors, so adding a
// theme is picking a handful of hex values rather than hand-tuning thirty.
//
// raise/lower pick Lighten or Darken based on Dark: a dark theme raises a
// surface's attention by lightening it (further from the near-black base,
// toward the light text sitting on it); a light theme raises attention by
// darkening it (further from the near-white base, toward the dark text
// sitting on it). Both themes apply the same deltas, so the tiers stay
// proportional to each other; only the direction flips.
func newTheme(p themeParams) Theme {
	raise, lower := lipgloss.Lighten, lipgloss.Darken
	if !p.Dark {
		raise, lower = lower, raise
	}

	return Theme{
		Name: p.Name,
		Dark: p.Dark,

		Accent: p.Accent,

		TextPrimary: p.Text,
		TextMuted:   lower(p.Text, 0.2),
		TextDim:     lower(p.Text, 0.3),

		PanelBg:            p.Panel,
		BackgroundContent:  raise(p.Panel, 0.04),
		BackgroundPanel:    raise(p.Panel, 0.08),
		BackgroundElevated: raise(p.Panel, 0.12),
		BackgroundRecessed: p.Panel,
		ModalBg:            p.Modal,

		BorderDefault: lower(p.Panel, 0.3),
		BorderCard:    raise(p.Panel, 0.18),

		StatusComplete:   p.Complete,
		StatusPending:    p.Pending,
		StatusInProgress: p.InProgress,
		StatusOverdue:    p.Overdue,

		Danger: p.Danger,

		// Fixed regardless of p.Dark - see the Theme field comment.
		InkOnLight: lipgloss.Color("#151520"),
		InkOnDark:  lipgloss.Color("#FAFAFA"),
	}
}

// InkOn returns whichever of the theme's two fixed inks reads better on fill.
//
// InkOnLight/InkOnDark do not vary with the theme (see the Theme field
// comment) because the fills they sit on - a status pill, the accent title
// chip - are not derived from the surface tiers. What *does* vary is which of
// the two is correct, and that is a property of the fill, not of the call
// site. Hard-coding it worked while one dark theme existed; with a light
// theme and ten imported palettes in the registry, the same call site draws
// on a #BC3FBC magenta in one theme and a #A7C080 sage in another.
func InkOn(fill color.Color) color.Color {
	if Contrast(Active.InkOnLight, fill) >= Contrast(Active.InkOnDark, fill) {
		return Active.InkOnLight
	}
	return Active.InkOnDark
}

// Contrast is the WCAG 2.x contrast ratio between two opaque colors.
func Contrast(a, b color.Color) float64 {
	la := relativeLuminance(a)
	lb := relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// relativeLuminance is WCAG 2.x relative luminance.
func relativeLuminance(c color.Color) float64 {
	r, g, b := extractRGB(c)
	r = srgbLinearize(r)
	g = srgbLinearize(g)
	b = srgbLinearize(b)
	return 0.2126*r + 0.7152*g + 0.0722*b
}

func extractRGB(c color.Color) (float64, float64, float64) {
	rgba := color.RGBAModel.Convert(c).(color.RGBA)
	return float64(rgba.R) / 255.0, float64(rgba.G) / 255.0, float64(rgba.B) / 255.0
}

func srgbLinearize(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// DefaultTheme is the theme a fresh AppModel starts with. Gruvbox Dark
// (github.com/morhetz/gruvbox) is the app's default palette; catppuccin-mocha
// and the rest remain selectable through the theme picker (docs/DESIGN.md §11).
const DefaultTheme = "gruvbox-dark"

// Themes is the registry the theme picker chooses from. Every entry is built
// through newTheme rather than a bare struct literal, so a registered theme
// can't leave a field zero-valued the way a hand-written literal could -
// which matters here because a nil color.Color renders as no SGR at all, i.e.
// a background-bleed bug. See src/appstyles/Theme_test.go and
// src/appstyles/Background_test.go.
var Themes = map[string]Theme{
	"crush-dark": newTheme(themeParams{
		Name:   "crush-dark",
		Dark:   true,
		Accent: lipgloss.Color("#BC3FBC"),
		Text:   lipgloss.Color("#FAFAFA"),
		Panel:  lipgloss.Color("#151520"),
		Modal:  lipgloss.Color("#282828"),
		Danger: lipgloss.Color("#D9534F"),

		Complete:   lipgloss.Color("#67C58A"),
		Pending:    lipgloss.Color("#858392"),
		InProgress: lipgloss.Color("#E8C547"),
		Overdue:    lipgloss.Color("#EB4268"),
	}),

	// crush-ember is a dark theme with a warm brown-black base and an
	// amber accent. The same shared status/danger colors keep the task-state
	// vocabulary consistent across themes.
	"crush-ember": newTheme(themeParams{
		Name:   "crush-ember",
		Dark:   true,
		Accent: lipgloss.Color("#E8A44A"),
		Text:   lipgloss.Color("#F5EDE4"),
		Panel:  lipgloss.Color("#1E1612"),
		Modal:  lipgloss.Color("#52413A"),
		Danger: lipgloss.Color("#D9534F"),

		Complete:   lipgloss.Color("#67C58A"),
		Pending:    lipgloss.Color("#8A8078"),
		InProgress: lipgloss.Color("#E8C547"),
		Overdue:    lipgloss.Color("#EB4268"),
	}),

	// crush-slate is a refined dark theme with golden accents on a blue-
	// black base - understated elegance with a warm metallic shimmer.
	"crush-slate": newTheme(themeParams{
		Name:   "crush-slate",
		Dark:   true,
		Accent: lipgloss.Color("#cca43b"),
		Text:   lipgloss.Color("#e5e5e5"),
		Panel:  lipgloss.Color("#1D2634"),
		Modal:  lipgloss.Color("#363636"),
		Danger: lipgloss.Color("#EB4268"),

		Complete:   lipgloss.Color("#67C58A"),
		Pending:    lipgloss.Color("#858392"),
		InProgress: lipgloss.Color("#E8C547"),
		Overdue:    lipgloss.Color("#EB4268"),
	}),

	// crush-day is crush-dark inverted: the same #BC3FBC magenta on a
	// warm off-white rather than a violet near-black. The neutral carries a
	// faint magenta bias so the greys read as chosen rather than as default
	// terminal grey, and the status colors are darkened from their dark-theme
	// values because a #67C58A green that reads on a near-black panel washes
	// out entirely on a near-white one.
	"crush-day": newTheme(themeParams{
		Name:   "crush-day",
		Dark:   false,
		Accent: lipgloss.Color("#BC3FBC"),
		Text:   lipgloss.Color("#241F2B"),
		Panel:  lipgloss.Color("#F6F2F7"),
		Modal:  lipgloss.Color("#FCF8FD"),
		Danger: lipgloss.Color("#B33A3A"),

		Complete:   lipgloss.Color("#1E7F4E"),
		Pending:    lipgloss.Color("#6B6878"),
		InProgress: lipgloss.Color("#A87409"),
		Overdue:    lipgloss.Color("#C0243F"),
	}),

	// catppuccin-mocha — Catppuccin Mocha (github.com/catppuccin/catppuccin)
	"catppuccin-mocha": newTheme(themeParams{
		Name:   "catppuccin-mocha",
		Dark:   true,
		Accent: lipgloss.Color("#CBA6F7"),
		Text:   lipgloss.Color("#CDD6F4"),
		Panel:  lipgloss.Color("#11111B"),
		Modal:  lipgloss.Color("#45475A"),
		Danger: lipgloss.Color("#EBA0AC"),

		Complete:   lipgloss.Color("#A6E3A1"),
		Pending:    lipgloss.Color("#7F849C"),
		InProgress: lipgloss.Color("#F9E2AF"),
		Overdue:    lipgloss.Color("#F38BA8"),
	}),

	// gruvbox-dark — Gruvbox dark (github.com/morhetz/gruvbox)
	"gruvbox-dark": newTheme(themeParams{
		Name:   "gruvbox-dark",
		Dark:   true,
		Accent: lipgloss.Color("#FE8019"),
		Text:   lipgloss.Color("#EBDBB2"),
		Panel:  lipgloss.Color("#1D2021"),
		Modal:  lipgloss.Color("#504945"),
		Danger: lipgloss.Color("#FB4934"),

		Complete:   lipgloss.Color("#B8BB26"),
		Pending:    lipgloss.Color("#928374"),
		InProgress: lipgloss.Color("#FABD2F"),
		Overdue:    lipgloss.Color("#FB4934"),
	}),

	// tokyo-night — Tokyo Night (github.com/folke/tokyonight.nvim)
	"tokyo-night": newTheme(themeParams{
		Name:   "tokyo-night",
		Dark:   true,
		Accent: lipgloss.Color("#7AA2F7"),
		Text:   lipgloss.Color("#C0CAF5"),
		Panel:  lipgloss.Color("#16161E"),
		Modal:  lipgloss.Color("#394B70"),
		Danger: lipgloss.Color("#DB4B4B"),

		Complete:   lipgloss.Color("#9ECE6A"),
		Pending:    lipgloss.Color("#737AA2"),
		InProgress: lipgloss.Color("#E0AF68"),
		Overdue:    lipgloss.Color("#F7768E"),
	}),

	// nord — Nord (nordtheme.com)
	"nord": newTheme(themeParams{
		Name:   "nord",
		Dark:   true,
		Accent: lipgloss.Color("#88C0D0"),
		Text:   lipgloss.Color("#D8DEE9"),
		Panel:  lipgloss.Color("#242933"),
		Modal:  lipgloss.Color("#4C566A"),
		Danger: lipgloss.Color("#BF616A"),

		Complete:   lipgloss.Color("#A3BE8C"),
		Pending:    lipgloss.Color("#7B88A1"),
		InProgress: lipgloss.Color("#EBCB8B"),
		Overdue:    lipgloss.Color("#BF616A"),
	}),

	// dracula — Dracula (draculatheme.com)
	"dracula": newTheme(themeParams{
		Name:   "dracula",
		Dark:   true,
		Accent: lipgloss.Color("#BD93F9"),
		Text:   lipgloss.Color("#F8F8F2"),
		Panel:  lipgloss.Color("#1E1F29"),
		Modal:  lipgloss.Color("#44475A"),
		Danger: lipgloss.Color("#FF5555"),

		Complete:   lipgloss.Color("#50FA7B"),
		Pending:    lipgloss.Color("#6272A4"),
		InProgress: lipgloss.Color("#FFB86C"),
		Overdue:    lipgloss.Color("#FF5555"),
	}),

	// solarized-dark — Solarized Dark (ethanschoonover.com/solarized)
	"solarized-dark": newTheme(themeParams{
		Name:   "solarized-dark",
		Dark:   true,
		Accent: lipgloss.Color("#268BD2"),
		Text:   lipgloss.Color("#BCC4CF"),
		Panel:  lipgloss.Color("#001A21"),
		Modal:  lipgloss.Color("#073642"),
		Danger: lipgloss.Color("#DC322F"),

		Complete:   lipgloss.Color("#859900"),
		Pending:    lipgloss.Color("#657B83"),
		InProgress: lipgloss.Color("#B58900"),
		Overdue:    lipgloss.Color("#DC322F"),
	}),

	// one-dark — One Dark (github.com/joshdick/onedark.vim)
	"one-dark": newTheme(themeParams{
		Name:   "one-dark",
		Dark:   true,
		Accent: lipgloss.Color("#61AFEF"),
		Text:   lipgloss.Color("#C6CDD7"),
		Panel:  lipgloss.Color("#21252B"),
		Modal:  lipgloss.Color("#2C323C"),
		Danger: lipgloss.Color("#E06C75"),

		Complete:   lipgloss.Color("#98C379"),
		Pending:    lipgloss.Color("#828997"),
		InProgress: lipgloss.Color("#E5C07B"),
		Overdue:    lipgloss.Color("#E06C75"),
	}),

	// everforest-dark — Everforest Dark (github.com/sainnhe/everforest)
	"everforest-dark": newTheme(themeParams{
		Name:   "everforest-dark",
		Dark:   true,
		Accent: lipgloss.Color("#A7C080"),
		Text:   lipgloss.Color("#E4D9C0"),
		Panel:  lipgloss.Color("#232A2E"),
		Modal:  lipgloss.Color("#4F585E"),
		Danger: lipgloss.Color("#E67E80"),

		Complete:   lipgloss.Color("#83C092"),
		Pending:    lipgloss.Color("#859289"),
		InProgress: lipgloss.Color("#DBBC7F"),
		Overdue:    lipgloss.Color("#E67E80"),
	}),

	// rose-pine — Rosé Pine (rosepinetheme.com)
	"rose-pine": newTheme(themeParams{
		Name:   "rose-pine",
		Dark:   true,
		Accent: lipgloss.Color("#C4A7E7"),
		Text:   lipgloss.Color("#E0DEF4"),
		Panel:  lipgloss.Color("#191724"),
		Modal:  lipgloss.Color("#403D52"),
		Danger: lipgloss.Color("#EB6F92"),

		Complete:   lipgloss.Color("#9CCFD8"),
		Pending:    lipgloss.Color("#908CAA"),
		InProgress: lipgloss.Color("#F6C177"),
		Overdue:    lipgloss.Color("#EB6F92"),
	}),

	// kanagawa-wave — Kanagawa Wave (github.com/rebelot/kanagawa.nvim)
	"kanagawa-wave": newTheme(themeParams{
		Name:   "kanagawa-wave",
		Dark:   true,
		Accent: lipgloss.Color("#7E9CD8"),
		Text:   lipgloss.Color("#DCD7BA"),
		Panel:  lipgloss.Color("#16161D"),
		Modal:  lipgloss.Color("#223249"),
		Danger: lipgloss.Color("#E82424"),

		Complete:   lipgloss.Color("#98BB6C"),
		Pending:    lipgloss.Color("#727169"),
		InProgress: lipgloss.Color("#FF9E3B"),
		Overdue:    lipgloss.Color("#E82424"),
	}),
}

// Active is the one Theme in effect. Everything that draws reads it fresh
// each render - see the Theme doc comment - so assigning a different
// registered Theme here and re-rendering is the whole of what a theme switch
// needs to do.
var Active = Themes[DefaultTheme]

// SetTheme assigns a new active theme by name. Everything that draws reads
// Active fresh on each render, so the next frame repaints in the new
// palette. Exported so the theme picker modal can apply themes live as the
// cursor moves.
//
// Returns false if name is not in the registry, so the caller can report
// the error. The theme picker's confirm step persists the chosen name via
// config.SaveConfig (src/cmds/ApplyTheme.go).
func SetTheme(name string) bool {
	if t, ok := Themes[name]; ok {
		Active = t
		return true
	}
	return false
}
