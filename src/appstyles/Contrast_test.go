package appstyles

import (
	"image/color"
	"testing"
)

// channelDiff returns the maximum per-channel 8-bit difference between two
// opaque colors, used to assert elevation separation between tiers.
func channelDiff(a, b color.Color) int {
	ra, ga, ba, _ := a.RGBA()
	rb, gb, bb, _ := b.RGBA()
	// RGBA() returns premultiplied 16-bit values; shift down to 8-bit.
	diff := func(x, y uint32) int {
		d := int(x>>8) - int(y>>8)
		if d < 0 {
			return -d
		}
		return d
	}
	return max(diff(ra, rb), diff(ga, gb), diff(ba, bb))
}

// Elevation separation floors (maximum per-channel distance).
//
//	recessed → content     8
//	content  → panel       8
//	panel    → elevated    8
//	elevated ↔ modal      14
//	panel    ↔ borderDef  12
//	recessed ↔ borderCard 12
func TestElevationSeparation(t *testing.T) {
	for name, theme := range Themes {
		t.Run(name, func(t *testing.T) {
			tests := []struct {
				label string
				a, b  color.Color
				floor int
			}{
				{"recessed → content", theme.BackgroundRecessed, theme.BackgroundContent, 8},
				{"content → panel", theme.BackgroundContent, theme.BackgroundPanel, 8},
				{"panel → elevated", theme.BackgroundPanel, theme.BackgroundElevated, 8},
				{"elevated ↔ modal", theme.BackgroundElevated, theme.ModalBg, 14},
				{"panel ↔ borderDefault", theme.BackgroundPanel, theme.BorderDefault, 12},
				{"recessed ↔ borderCard", theme.BackgroundRecessed, theme.BorderCard, 12},
			}

			for _, tc := range tests {
				got := channelDiff(tc.a, tc.b)
				if got < tc.floor {
					t.Errorf("%s: channelDiff = %d, want ≥ %d", tc.label, got, tc.floor)
				}
			}
		})
	}
}

// TestFocusStepContrast guards the panel-focus contrast between the focused
// tier (BackgroundElevated) and the unfocused tier (BackgroundPanel). It was
// added with the focus bug, whose root cause was NOT this step but
// ListRowBg ignoring focus (both panels showed a live selected row) — fixed
// separately. This test still pins the step so a future change can't collapse
// elevated and panel onto the same fill.
//
// Measured floor: ~1.10 (farol-day, the lone light theme). The panel-surface
// step is inherently bounded at ~1.1-1.2 for BOTH light and dark themes,
// because both tiers derive from the same near-white (light) or near-black
// (dark) base by Lighten/Darken: in a light theme a larger coefficient
// darkens elevated toward the (lighter) panel, shrinking the ratio, so the
// additive raise ladder cannot exceed ~1.2 on farol-day without also moving
// the base palette. (A geometric ladder was prototyped and hit the same
// ~1.1-1.2 cap — the base, not the step function, is the binding variable.)
// The genuinely perceptible, theme-independent focus signal is therefore the
// SELECTED-ROW contrast (ModalBg focused vs BackgroundElevated unfocused,
// fixed in Step 1), which measures ~9.5:1 on farol-day — three orders of
// magnitude more legible than the panel-surface step. This floor of 1.10
// only catches a regression that makes the two tiers identical; it is not, and
// cannot be under the current elevation math, a perceptibility guarantee.
func TestFocusStepContrast(t *testing.T) {
	for name, theme := range Themes {
		t.Run(name, func(t *testing.T) {
			ratio := Contrast(theme.BackgroundElevated, theme.BackgroundPanel)
			if ratio < 1.10 {
				t.Errorf("focus step (elevated vs panel) contrast = %.2f, want ≥ 1.10", ratio)
			}
		})
	}
}

// WCAG contrast floors.
//
//	TextPrimary on panel / elevated     4.5
//	TextPrimary on modal                4.2  (everforest measures 4.30)
//	TextMuted on panel                  3.0
//	TextDim on panel                    2.2
//	Accent on panel and on modal        3.0
//	InkOn(Accent) on Accent             4.2  (complete-dark/day measure 4.40)
//	InkOn(fill) on each status / Danger 4.2  (pills are bold uppercase; WCAG
//	                                        large-text threshold is 3.0)
//	each status color as text on panel  2.6
func TestWCAGContrastAgainstSurfaces(t *testing.T) {
	for name, theme := range Themes {
		t.Run(name, func(t *testing.T) {
			// Surfaces.
			panel := theme.BackgroundPanel
			elevated := theme.BackgroundElevated
			modal := theme.ModalBg

			// TextPrimary on panel and elevated.
			ratio := Contrast(theme.TextPrimary, panel)
			if ratio < 4.5 {
				t.Errorf("TextPrimary on panel: ratio = %.2f, want ≥ 4.5", ratio)
			}
			ratio = Contrast(theme.TextPrimary, elevated)
			if ratio < 4.5 {
				t.Errorf("TextPrimary on elevated: ratio = %.2f, want ≥ 4.5", ratio)
			}

			// TextPrimary on modal (everforest dips to 4.30).
			ratio = Contrast(theme.TextPrimary, modal)
			if ratio < 4.2 {
				t.Errorf("TextPrimary on modal: ratio = %.2f, want ≥ 4.2", ratio)
			}

			// TextMuted on panel.
			ratio = Contrast(theme.TextMuted, panel)
			if ratio < 3.0 {
				t.Errorf("TextMuted on panel: ratio = %.2f, want ≥ 3.0", ratio)
			}

			// TextDim on panel.
			ratio = Contrast(theme.TextDim, panel)
			if ratio < 2.2 {
				t.Errorf("TextDim on panel: ratio = %.2f, want ≥ 2.2", ratio)
			}

			// Accent on panel.
			ratio = Contrast(theme.Accent, panel)
			if ratio < 3.0 {
				t.Errorf("Accent on panel: ratio = %.2f, want ≥ 3.0", ratio)
			}

			// Accent on modal.
			ratio = Contrast(theme.Accent, modal)
			if ratio < 3.0 {
				t.Errorf("Accent on modal: ratio = %.2f, want ≥ 3.0", ratio)
			}

			// InkOn(Accent) on Accent — the title chip.
			inkOnAccent := InkOn(theme.Accent)
			ratio = Contrast(inkOnAccent, theme.Accent)
			if ratio < 4.2 {
				t.Errorf("InkOn(Accent) on Accent: ratio = %.2f (ink=%v), want ≥ 4.2", ratio, inkOnAccent)
			}

			// InkOn(fill) on each status fill and on Danger — pills.
			type pillCase struct {
				label string
				fill  color.Color
			}
			pills := []pillCase{
				{"StatusComplete", theme.StatusComplete},
				{"StatusPending", theme.StatusPending},
				{"StatusInProgress", theme.StatusInProgress},
				{"StatusOverdue", theme.StatusOverdue},
				{"Danger", theme.Danger},
			}
			for _, p := range pills {
				ink := InkOn(p.fill)
				ratio := Contrast(ink, p.fill)
				if ratio < 4.2 {
					t.Errorf("InkOn(%s) on %s: ratio = %.2f (ink=%v), want ≥ 4.2", p.label, p.label, ratio, ink)
				}
			}

			// Each status color as text on panel — small text like status
			// dots or labels that appear on the panel background.
			type statusCase struct {
				label string
				c     color.Color
			}
			statuses := []statusCase{
				{"StatusComplete", theme.StatusComplete},
				{"StatusPending", theme.StatusPending},
				{"StatusInProgress", theme.StatusInProgress},
				{"StatusOverdue", theme.StatusOverdue},
			}
			for _, s := range statuses {
				ratio := Contrast(s.c, panel)
				if ratio < 2.6 {
					t.Errorf("%s as text on panel: ratio = %.2f, want ≥ 2.6", s.label, ratio)
				}
			}

			// The status colors that actually render as text on the modal
			// tier — the selected task row's background (and the inline
			// create row's). Only the in-progress and complete tokens draw
			// there: pending's label is TextMuted, not StatusPending, and
			// StatusOverdue is reserved for a feature that does not exist
			// yet.
			modalStatuses := []statusCase{
				{"StatusInProgress", theme.StatusInProgress},
				{"StatusComplete", theme.StatusComplete},
			}
			for _, s := range modalStatuses {
				ratio := Contrast(s.c, modal)
				if ratio < 2.6 {
					t.Errorf("%s as text on modal: ratio = %.2f, want ≥ 2.6", s.label, ratio)
				}
			}

			// The recessed tier carries text of its own: the empty-state
			// cards' guidance sits on it, and its ink is the tiers below.
			// Recessed is the darkest surface in a dark theme but the
			// lightest in a light one, so it is the surface where a
			// light-theme text tier would fail first - it needs its own
			// floors rather than inheriting the panel's by assumption.
			recessed := theme.BackgroundRecessed

			type inkCase struct {
				label string
				c     color.Color
				floor float64
			}
			inks := []inkCase{
				{"TextPrimary", theme.TextPrimary, 4.5},
				{"TextDim", theme.TextDim, 2.2},
				{"Accent", theme.Accent, 3.0},
				{"StatusOverdue", theme.StatusOverdue, 2.6},
			}
			for _, ink := range inks {
				ratio := Contrast(ink.c, recessed)
				if ratio < ink.floor {
					t.Errorf("%s on recessed: ratio = %.2f, want ≥ %.1f", ink.label, ratio, ink.floor)
				}
			}

			// Spinner glyph contrast. The spinner draws Accent when the row is
			// focused/selected (ModalBg) and TextDim otherwise (BackgroundPanel).
			// Floors match the existing Accent/modal (3.0) and TextDim/panel (2.2)
			// tiers — the spinner is a small decorative glyph, not body text.
			// (docs/DESIGN.md §12, glyph vocabulary.)
			spinnerCases := []struct {
				label   string
				ink     color.Color
				surface color.Color
				floor   float64
			}{
				{"Accent on modal (active row)", theme.Accent, modal, 3.0},
				{"TextDim on panel (unfocused row)", theme.TextDim, panel, 2.2},
			}
			for _, sc := range spinnerCases {
				ratio := Contrast(sc.ink, sc.surface)
				if ratio < sc.floor {
					t.Errorf("spinner %s: ratio = %.2f, want ≥ %.1f", sc.label, ratio, sc.floor)
				}
			}
		})
	}
}
