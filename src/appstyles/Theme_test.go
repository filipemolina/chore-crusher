package appstyles

import (
	"image/color"
	"reflect"
	"testing"

	"charm.land/lipgloss/v2"
)

// colorFields returns the name and value of every color.Color field on t, by
// reflection - so a new field added to Theme is covered here without anyone
// remembering to add it by hand.
func colorFields(t Theme) map[string]color.Color {
	fields := make(map[string]color.Color)

	v := reflect.ValueOf(t)
	colorType := reflect.TypeOf((*color.Color)(nil)).Elem()

	for i := range v.NumField() {
		field := v.Type().Field(i)
		if !field.Type.Implements(colorType) {
			continue
		}

		fields[field.Name] = v.Field(i).Interface().(color.Color)
	}

	return fields
}

// A theme built by anything other than newTheme - a hand-written struct
// literal that forgets a field - leaves that field nil. A nil color.Color
// sets no SGR at all, which is a background-bleed bug wearing a different
// hat: see appstyles.HasBackgroundBleed. This is the property the roadmap
// calls "a theme that leaves an unpainted cell fails CI" for the fields
// themselves, one level before rendering.
func TestNoThemeLeavesAFieldZeroValued(t *testing.T) {
	for name, theme := range Themes {
		t.Run(name, func(t *testing.T) {
			for field, c := range colorFields(theme) {
				if c == nil {
					t.Errorf("%s.%s is nil", name, field)
				}
			}
		})
	}
}

// The registry key and the theme's own idea of its name have to agree, or a
// theme picker keyed on one and displaying the other would show the wrong
// label.
func TestThemeNameMatchesItsRegistryKey(t *testing.T) {
	for key, theme := range Themes {
		if theme.Name != key {
			t.Errorf("Themes[%q].Name = %q", key, theme.Name)
		}
	}
}

func TestDefaultThemeIsRegistered(t *testing.T) {
	if _, ok := Themes[DefaultTheme]; !ok {
		t.Errorf("DefaultTheme %q is not in Themes", DefaultTheme)
	}
}

// newTheme's whole promise: the tiers derive from Panel by the same deltas
// in both directions, Lighten for a dark theme and Darken for a light one.
// This pins the formula itself, independent of any concrete palette.
func TestNewThemeDerivesTiersByDirection(t *testing.T) {
	base := lipgloss.Color("#334455")

	dark := newTheme(themeParams{Dark: true, Panel: base, Text: base})
	light := newTheme(themeParams{Dark: false, Panel: base, Text: base})

	tests := []struct {
		name      string
		darkGot   color.Color
		lightGot  color.Color
		darkWant  color.Color
		lightWant color.Color
	}{
		{"BackgroundContent", dark.BackgroundContent, light.BackgroundContent, lipgloss.Lighten(base, 0.04), lipgloss.Darken(base, 0.04)},
		{"BackgroundPanel", dark.BackgroundPanel, light.BackgroundPanel, lipgloss.Lighten(base, 0.08), lipgloss.Darken(base, 0.08)},
		{"BackgroundElevated", dark.BackgroundElevated, light.BackgroundElevated, lipgloss.Lighten(base, 0.12), lipgloss.Darken(base, 0.12)},
		{"BorderDefault", dark.BorderDefault, light.BorderDefault, lipgloss.Darken(base, 0.3), lipgloss.Lighten(base, 0.3)},
		{"BorderCard", dark.BorderCard, light.BorderCard, lipgloss.Lighten(base, 0.18), lipgloss.Darken(base, 0.18)},
	}

	for _, tc := range tests {
		if tc.darkGot != tc.darkWant {
			t.Errorf("dark %s = %v, want %v", tc.name, tc.darkGot, tc.darkWant)
		}
		if tc.lightGot != tc.lightWant {
			t.Errorf("light %s = %v, want %v", tc.name, tc.lightGot, tc.lightWant)
		}
	}

	// BackgroundRecessed is always the un-raised base, in both directions.
	if dark.BackgroundRecessed != base || light.BackgroundRecessed != base {
		t.Errorf("BackgroundRecessed should equal Panel unmodified in both themes")
	}
}

// InkOn picks whichever of the two fixed inks has better contrast on the
// given fill — this is the whole point of the helper. Run over every
// registered theme to catch future regressions.
func TestInkOnPicksTheLegibleInk(t *testing.T) {
	fills := []struct {
		name string
		c    color.Color
	}{
		{"Accent", Active.Accent},
		{"Danger", Active.Danger},
		{"StatusComplete", Active.StatusComplete},
		{"StatusPending", Active.StatusPending},
		{"StatusInProgress", Active.StatusInProgress},
		{"StatusOverdue", Active.StatusOverdue},
	}

	for name, theme := range Themes {
		t.Run(name, func(t *testing.T) {
			for _, fill := range fills {
				chosen := InkOn(fill.c)
				chosenRatio := Contrast(chosen, fill.c)
				lightRatio := Contrast(theme.InkOnLight, fill.c)
				darkRatio := Contrast(theme.InkOnDark, fill.c)

				// The chosen ink must score at least as well as the better of
				// the two fixed inks — anything else is a bug in InkOn.
				if chosenRatio < lightRatio && chosenRatio < darkRatio {
					t.Errorf("%s on %s: InkOn chose %v (ratio %.2f), but InkOnLight gives %.2f and InkOnDark gives %.2f",
						name, fill.name, chosen, chosenRatio, lightRatio, darkRatio)
				}
			}
		})
	}
}

// InkOnLight/InkOnDark are the one deliberate exception to "derived from
// base colors": they exist to stay legible on a status pill whose fill does
// not itself vary with the app's theme, so they must not vary either.
func TestInkTokensDoNotVaryWithDark(t *testing.T) {
	dark := newTheme(themeParams{Dark: true})
	light := newTheme(themeParams{Dark: false})

	if dark.InkOnLight != light.InkOnLight {
		t.Error("InkOnLight differs between a dark and a light theme")
	}
	if dark.InkOnDark != light.InkOnDark {
		t.Error("InkOnDark differs between a dark and a light theme")
	}
}

// SetTheme should fall back to DefaultTheme when the name is not registered,
// so a config file naming a theme that was removed doesn't crash the app.
func TestUnknownSavedThemeFallsBackToDefault(t *testing.T) {
	defer SetTheme(DefaultTheme) // restore

	if ok := SetTheme("made-up-theme-name"); ok {
		t.Error("SetTheme should return false for an unknown name")
	}
	if Active.Name != DefaultTheme {
		t.Errorf("after failed SetTheme, Active = %q, want %q", Active.Name, DefaultTheme)
	}
}
