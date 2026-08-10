package taskspanel

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/constants"
)

// fgSeq returns the bare foreground SGR token lipgloss emits for c (e.g.
// "38;2;203;166;247"), so a test can assert a rendered frame paints a glyph in
// that color whether the frame keeps the escape standalone or merges it with a
// background into one "\x1b[<fg>;<bg>m" run.
func fgSeq(c color.Color) string {
	probe := lipgloss.NewStyle().Foreground(c).Render("X")
	inner := strings.SplitN(probe, "X", 2)[0]
	inner = strings.TrimPrefix(inner, "\x1b[")
	return strings.TrimSuffix(inner, "m")
}

// The panel opens in its initial-load state: it renders the shared Tasks frame
// with an animated "Loading" body, and a spinner tick advances that animation
// (and reschedules the next tick) while loading is still outstanding.
func TestLoadingViewAnimatesWhileLoading(t *testing.T) {
	panel := layoutModel(t, constants.COMPONENT_TASK_TREE)

	before := ansi.Strip(panel.View().Content)
	if !strings.Contains(before, "Loading") {
		t.Fatalf("initial view missing the Loading label: %q", before)
	}
	if got := strings.Count(before, "Tasks"); got != 1 {
		t.Errorf("loading view should still use the Tasks panel frame once: count = %d", got)
	}

	// Init starts the spinner; its command yields a spinner.TickMsg.
	tick := panel.Init()()
	if _, ok := tick.(spinner.TickMsg); !ok {
		t.Fatalf("Init did not start a spinner tick, got %T", tick)
	}
	updated, cmd := panel.Update(tick)
	if cmd == nil {
		t.Error("spinner tick did not reschedule the next frame while loading")
	}
	// The Ellipsis spinner advances "" -> "." so the rendered body changes.
	after := ansi.Strip(updated.(Model).View().Content)
	if after == before {
		t.Errorf("spinner tick did not advance the loading frame:\nbefore %q\nafter  %q", before, after)
	}
}

// The first RefreshListsMsg — success or error — leaves the initial-load state
// for good; a later poll refresh or a stray spinner tick never restores it.
func TestFirstRefreshClearsLoadingPermanently(t *testing.T) {
	panel := layoutModel(t, constants.COMPONENT_TASK_TREE)
	if !strings.Contains(ansi.Strip(panel.View().Content), "Loading") {
		t.Fatal("precondition: panel should be loading before the first refresh")
	}

	updated, _ := panel.Update(cmds.RefreshListsMsg{})
	loaded := updated.(Model)
	if strings.Contains(ansi.Strip(loaded.View().Content), "Loading") {
		t.Errorf("first RefreshListsMsg did not clear the loading view")
	}

	// A later poll refresh and a leftover spinner tick must not re-enable it.
	afterPoll, _ := loaded.Update(cmds.RefreshListsMsg{})
	afterTick, cmd := afterPoll.(Model).Update(spinner.TickMsg{})
	if strings.Contains(ansi.Strip(afterTick.(Model).View().Content), "Loading") {
		t.Errorf("a later refresh/tick restored the loading view")
	}
	if cmd != nil {
		t.Errorf("a spinner tick after loading should not reschedule, got a command")
	}
}

// The ellipsis accent is read from appstyles.Active at draw time, so a live
// theme switch while loading repaints it — proving no themed color is cached
// on the spinner (the plan's render-time-color rule).
func TestLoadingEllipsisAccentFollowsLiveTheme(t *testing.T) {
	orig := appstyles.Active
	t.Cleanup(func() { appstyles.Active = orig })

	appstyles.Active = appstyles.Themes["catppuccin-mocha"]
	panel := layoutModel(t, constants.COMPONENT_TASK_TREE)
	// Advance to a non-empty ellipsis frame so the accent has a glyph to paint.
	updated, _ := panel.Update(panel.Init()())
	panel = updated.(Model)

	accentA := fgSeq(appstyles.Active.Accent)
	if !strings.Contains(panel.View().Content, accentA) {
		t.Fatalf("loading ellipsis missing theme A accent foreground")
	}

	// Switch themes live; the same model repaints with the new accent.
	appstyles.Active = appstyles.Themes["dracula"]
	accentB := fgSeq(appstyles.Active.Accent)
	frameB := panel.View().Content
	if !strings.Contains(frameB, accentB) {
		t.Errorf("loading ellipsis did not follow the live theme switch to the new accent")
	}
	if accentA != accentB && strings.Contains(frameB, accentA) {
		t.Errorf("loading ellipsis kept a cached theme-A accent after a live theme switch")
	}
}
