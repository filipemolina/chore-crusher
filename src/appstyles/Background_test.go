package appstyles

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// forEachTheme runs fn once per registered theme, with Active set to that
// theme for the duration, in sorted order for stable test output.
//
// FillBackground/HasBackgroundBleed operate on ANSI escape sequences, not on
// what a color actually resolves to, so this loop does not exercise a
// different code path per theme the way src/model/background_test.go's does
// - that file's parameterization is the one with real teeth, since it
// renders whole frames built from Theme-driven components. This one is
// cheap insurance and keeps this file's fixtures tied to the registry
// instead of an arbitrary color, per docs/ROADMAP.md's Theme phase.
func forEachTheme(t *testing.T, fn func(t *testing.T)) {
	t.Helper()

	original := Active
	t.Cleanup(func() { Active = original })

	for _, name := range slices.Sorted(maps.Keys(Themes)) {
		t.Run(name, func(t *testing.T) {
			Active = Themes[name]
			fn(t)
		})
	}
}

func TestFillBackgroundPaintsSpacesAfterAReset(t *testing.T) {
	forEachTheme(t, func(t *testing.T) {
		pill := lipgloss.NewStyle().Background(Active.Accent).Render("Details")

		// This is the shape JoinVertical produces: a styled run, then bare
		// padding out to the width of a wider sibling block.
		block := pill + strings.Repeat(" ", 20)

		if !HasBackgroundBleed(block) {
			t.Fatalf("precondition: expected raw block to bleed, got %q", block)
		}

		filled := FillBackground(Active.BackgroundPanel, block)

		if HasBackgroundBleed(filled) {
			t.Errorf("FillBackground left unpainted spaces: %q", filled)
		}

		if got, want := lipgloss.Width(filled), lipgloss.Width(block); got != want {
			t.Errorf("FillBackground changed width: got %d, want %d", got, want)
		}
	})
}

func TestFillBackgroundKeepsInnerBackgrounds(t *testing.T) {
	forEachTheme(t, func(t *testing.T) {
		pill := lipgloss.NewStyle().Background(Active.Accent).Render("Details")
		filled := FillBackground(Active.BackgroundPanel, pill+"   ")

		accentSeq := backgroundSeq(Active.Accent)
		if !strings.Contains(filled, accentSeq) {
			t.Errorf("FillBackground dropped the inner accent background %q from %q", accentSeq, filled)
		}
	})
}

func TestFillBackgroundPreservesPlainText(t *testing.T) {
	forEachTheme(t, func(t *testing.T) {
		filled := FillBackground(Active.BackgroundPanel, "Name: web\nImage: nginx")

		if got := ansi.Strip(filled); got != "Name: web\nImage: nginx" {
			t.Errorf("FillBackground altered the text: got %q", got)
		}

		if got, want := strings.Count(filled, "\n"), 1; got != want {
			t.Errorf("FillBackground changed the line count: got %d newlines, want %d", got, want)
		}
	})
}

func TestFillBackgroundNilAndEmpty(t *testing.T) {
	if got := FillBackground(nil, "hello"); got != "hello" {
		t.Errorf("nil background should be a no-op, got %q", got)
	}

	forEachTheme(t, func(t *testing.T) {
		if got := FillBackground(Active.BackgroundPanel, ""); got != "" {
			t.Errorf("empty block should stay empty, got %q", got)
		}
	})
}
