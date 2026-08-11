package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/constants"
)

// resize drives a window-size message through the model the way the runtime
// would, so the layout and the too-small predicate both see the new size.
func resize(t *testing.T, m AppModel, w, h int) AppModel {
	t.Helper()
	return refresh(t, m, tea.WindowSizeMsg{Width: w, Height: h})
}

// TestTerminalTooSmallReplacesTheWholeFrame covers defect 4: below the
// minimum supported size the app renders one centred line and nothing else,
// rather than a layout whose panels silently clip their own content.
func TestTerminalTooSmallReplacesTheWholeFrame(t *testing.T) {
	tooSmall := []struct{ w, h int }{
		{constants.MIN_TERMINAL_WIDTH - 1, constants.MIN_TERMINAL_HEIGHT},     // 39 columns
		{constants.MIN_TERMINAL_WIDTH, constants.MIN_TERMINAL_HEIGHT - 1},     // 9 rows
		{constants.MIN_TERMINAL_WIDTH - 1, constants.MIN_TERMINAL_HEIGHT - 1}, // both
		{20, 6},
	}

	for _, size := range tooSmall {
		m := resize(t, seedOneList(t), size.w, size.h)
		out := ansi.Strip(m.View().Content)

		if !strings.Contains(out, tooSmallMessage) {
			t.Errorf("%dx%d: expected %q, got:\n%s", size.w, size.h, tooSmallMessage, out)
		}
		// Nothing else from the normal frame may render: not the header
		// wordmark, not the panel title, not a footer hint.
		for _, forbidden := range []string{"Farol", "Tasks", "tab next"} {
			if strings.Contains(out, forbidden) {
				t.Errorf("%dx%d: frame still rendered %q alongside the message:\n%s",
					size.w, size.h, forbidden, out)
			}
		}
		// The message fills the terminal exactly — no unpainted cells around it.
		if got := lipgloss.Height(out); got != size.h {
			t.Errorf("%dx%d: rendered %d rows, want %d", size.w, size.h, got, size.h)
		}
		if got := lipgloss.Width(out); got > size.w {
			t.Errorf("%dx%d: rendered %d columns, want at most %d", size.w, size.h, got, size.w)
		}
	}
}

// TestMinimumSizeRendersTheRealLayout pins the other side of the threshold:
// exactly 40x10 is supported, so the normal frame renders there.
func TestMinimumSizeRendersTheRealLayout(t *testing.T) {
	m := resize(t, seedOneList(t), constants.MIN_TERMINAL_WIDTH, constants.MIN_TERMINAL_HEIGHT)
	out := ansi.Strip(m.View().Content)

	if strings.Contains(out, tooSmallMessage) {
		t.Errorf("%dx%d is the supported minimum and must render the layout, got:\n%s",
			constants.MIN_TERMINAL_WIDTH, constants.MIN_TERMINAL_HEIGHT, out)
	}
	if !strings.Contains(out, "Tasks") {
		t.Errorf("expected the Tasks panel at the minimum size, got:\n%s", out)
	}
}

// TestTooSmallRecoversOnResize checks the message is not sticky: growing the
// terminal back over the threshold restores the real layout.
func TestTooSmallRecoversOnResize(t *testing.T) {
	m := resize(t, seedOneList(t), 30, 8)
	if !strings.Contains(ansi.Strip(m.View().Content), tooSmallMessage) {
		t.Fatal("30x8 did not render the too-small message")
	}

	m = resize(t, m, 100, 40)
	out := ansi.Strip(m.View().Content)
	if strings.Contains(out, tooSmallMessage) {
		t.Errorf("100x40 still shows the too-small message:\n%s", out)
	}
	if !strings.Contains(out, "Tasks") {
		t.Errorf("expected the Tasks panel after growing the terminal, got:\n%s", out)
	}
}

// TestTooSmallHidesAnOpenModal: the message replaces the entire frame, so a
// modal that was open when the terminal shrank does not render over it.
func TestTooSmallHidesAnOpenModal(t *testing.T) {
	m := refresh(t, seedOneList(t), tea.KeyPressMsg{Text: "?", Code: '?'})
	if m.activeModal == nil {
		t.Fatal("? did not open the help overlay")
	}

	m = resize(t, m, 30, 8)
	out := ansi.Strip(m.View().Content)
	if strings.Contains(out, "Keyboard shortcuts") {
		t.Errorf("the help overlay rendered over the too-small message:\n%s", out)
	}
	if !strings.Contains(out, tooSmallMessage) {
		t.Errorf("expected the too-small message, got:\n%s", out)
	}
}

// TestPreLayoutFrameIsNotTooSmall guards the startup case: before the first
// WindowSizeMsg the model's width and height are still 0, and that must not be
// mistaken for a tiny terminal.
func TestPreLayoutFrameIsNotTooSmall(t *testing.T) {
	m := newTestModel(t, t.TempDir())
	m.layoutInitialized = false
	m.terminalWidth, m.terminalHeight = 0, 0

	if m.terminalTooSmall() {
		t.Error("a model that has not seen a WindowSizeMsg must not report too small")
	}
	// And once a real size arrives, the predicate answers from it.
	m = refresh(t, m, cmds.SetBodyLayout(0, 0, 0, 0)())
	m = resize(t, m, 20, 6)
	if !m.terminalTooSmall() {
		t.Error("20x6 must report too small")
	}
}
