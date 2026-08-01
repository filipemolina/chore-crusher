package chrome

import (
	"fmt"
	"image/color"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/filipemolina/chore-completer/src/appstyles"
)

// KeyHint represents a single keybinding for display in a help line.
// Key is the literal key (e.g. "n", "space", "←/→"). Desc is a short
// verb describing what the key does.
type KeyHint struct {
	Key  string
	Desc string
}

// HintFor is one binding as a hint. HintAs overrides the description for the
// places where a shared key does something more specific than its general help
// text says - Enter is "confirm" everywhere, but "create group" in the
// checklist that creates a group.
func HintFor(binding key.Binding) KeyHint {
	help := binding.Help()

	return KeyHint{help.Key, help.Desc}
}

func HintAs(binding key.Binding, desc string) KeyHint {
	return KeyHint{binding.Help().Key, desc}
}

// RenderKeyHints renders hints as "key desc · key desc": the key bold in the
// primary text color, the description in descColor. Modals render their own
// help lines through this so they read the same as the footer bar, passing a
// lighter descColor when they sit on a lighter surface than the bar does.
func RenderKeyHints(hints []KeyHint, descColor color.Color) string {
	descStyle := lipgloss.NewStyle().Foreground(descColor)
	sepStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextDim)
	keyStyle := lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary).Bold(true)

	parts := make([]string, 0, len(hints)*2)
	for i, h := range hints {
		if i > 0 {
			parts = append(parts, sepStyle.Render(" · "))
		}
		parts = append(parts, fmt.Sprintf("%s %s", keyStyle.Render(h.Key), descStyle.Render(h.Desc)))
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}
