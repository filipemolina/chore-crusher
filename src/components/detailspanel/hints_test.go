package detailspanel

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
	"github.com/filipemolina/chore-crusher/src/keys"
)

// zoneFor tabs the panel to a named focus zone so a test can name the zone it
// means rather than counting tab presses.
func zoneFor(t *testing.T, m *Model, zone int) *Model {
	t.Helper()
	for i := 0; i < focusCount && m.focus != zone; i++ {
		m.cycleFocus(1)
	}
	if m.focus != zone {
		t.Fatalf("could not reach focus zone %d", zone)
	}
	return m
}

// The modal is the only hint line on screen once the global footer goes blank,
// so every Details binding declared in src/keys has to be advertised in the
// zone where it is live — CommentNew included, which the old hardcoded lines
// only ever showed as the invented word "comment".
func TestEveryDetailsBindingIsAdvertisedInSomeZone(t *testing.T) {
	m, _, _ := loaded(t, "")

	// Collect the hint text from every zone, every progress mode, plus the
	// compose card — some hints are mode-specific, not just zone-specific.
	var all []chrome.KeyHint
	for _, zone := range []int{focusTitle, focusNotes, focusProgress, focusComments} {
		m = zoneFor(t, m, zone)
		for _, kind := range []apptypes.ProgressKind{
			apptypes.ProgressSimple, apptypes.ProgressSubtasks, apptypes.ProgressPercentage,
		} {
			m.progressKind = kind
			all = append(all, m.zoneHints()...)
		}
	}
	m.composing = true
	all = append(all, m.zoneHints()...)
	m.composing = false
	m.confirmingDiscard = true
	all = append(all, m.zoneHints()...)
	m.confirmingDiscard = false

	declared := []struct {
		name    string
		binding key.Binding
	}{
		{"Save", keys.Details.Save},
		{"NextField", keys.Details.NextField},
		{"CycleMode", keys.Details.CycleMode},
		{"CycleModeBack", keys.Details.CycleModeBack},
		{"PercentNudge", keys.Details.PercentNudge},
		{"PercentType", keys.Details.PercentType},
		{"DiscardPrompt", keys.Details.DiscardPrompt},
		{"CopyTaskID", keys.Details.CopyTaskID},
		{"CommentNew", keys.Details.CommentNew},
		{"CommentSubmit", keys.Details.CommentSubmit},
		{"CopyCommentID", keys.Details.CopyCommentID},
	}
	for _, d := range declared {
		want := chrome.HintFor(d.binding)
		found := false
		for _, h := range all {
			if h == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("keys.Details.%s (%q %q) is declared but advertised in no zone", d.name, want.Key, want.Desc)
		}
	}
}

// Every hint the modal shows must come from a binding in src/keys, so the modal
// can never describe a key in words the help overlay does not use. This is what
// stops "esc close" / "esc cancel" and "copy task ID" / "copy task id" from
// drifting apart again.
func TestModalHintWordingComesFromTheBindings(t *testing.T) {
	m, _, _ := loaded(t, "")

	known := map[chrome.KeyHint]bool{}
	for _, b := range []key.Binding{
		keys.Details.Save, keys.Details.NextField, keys.Details.CycleMode,
		keys.Details.CycleModeBack, keys.Details.PercentNudge, keys.Details.PercentType,
		keys.Details.DiscardPrompt,
		keys.Details.CopyTaskID, keys.Details.CommentNew,
		keys.Details.CommentSubmit, keys.Details.CopyCommentID,
		keys.Overlay.Cancel, keys.Overlay.Navigation, keys.Overlay.Submit,
	} {
		known[chrome.HintFor(b)] = true
	}

	for _, zone := range []int{focusTitle, focusNotes, focusProgress, focusComments} {
		m = zoneFor(t, m, zone)
		for _, kind := range []apptypes.ProgressKind{
			apptypes.ProgressSimple, apptypes.ProgressSubtasks, apptypes.ProgressPercentage,
		} {
			m.progressKind = kind
			for _, h := range m.zoneHints() {
				if !known[h] {
					t.Errorf("zone %d advertises %q %q, which no src/keys binding declares", zone, h.Key, h.Desc)
				}
			}
		}
	}
}

// c is live only in the comments zone (handleCommentsKey owns it), so no other
// zone may offer it. Advertising a dead key is the lie the blank global footer
// exists to avoid.
func TestCommentKeyAdvertisedOnlyWhereItWorks(t *testing.T) {
	m, _, _ := loaded(t, "")
	newComment := chrome.HintFor(keys.Details.CommentNew)

	for _, tc := range []struct {
		zone int
		want bool
	}{
		{focusTitle, false},
		{focusNotes, false},
		{focusProgress, false},
		{focusComments, true},
	} {
		m = zoneFor(t, m, tc.zone)
		got := false
		for _, h := range m.zoneHints() {
			if h == newComment {
				got = true
			}
		}
		if got != tc.want {
			t.Errorf("zone %d advertises c=%v, want %v", tc.zone, got, tc.want)
		}
	}
}

// The rendered footer actually carries the zone's hints (guards against
// zoneHints being correct while renderFooter drops them).
func TestRenderedFooterShowsTheZoneHints(t *testing.T) {
	m, _, _ := loaded(t, "")
	m = zoneFor(t, m, focusComments)

	footer := ansi.Strip(m.renderFooter())
	for _, want := range []string{"add comment", "copy task id", "next field", "cancel"} {
		if !strings.Contains(footer, want) {
			t.Errorf("comments-zone footer missing %q, got: %q", want, footer)
		}
	}
	// The old invented wordings must be gone.
	for _, gone := range []string{"copy task ID", "esc close"} {
		if strings.Contains(footer, gone) {
			t.Errorf("footer still uses the invented wording %q: %q", gone, footer)
		}
	}
}

// The discard prompt states its own keys, from the binding, and does not
// promise enter. enter is unbound there on purpose: the confirm modal has a
// visible yes/no selection for enter to act on, this prompt has none, so
// binding it would put unsaved edits one stray keystroke from gone.
func TestDiscardPromptAdvertisesYesNoAndNotEnter(t *testing.T) {
	m, _, _ := loaded(t, "")
	m.confirmingDiscard = true

	footer := ansi.Strip(m.renderFooter())
	if !strings.Contains(footer, "Discard changes?") {
		t.Errorf("the discard prompt must still ask its question, got: %q", footer)
	}
	for _, want := range []string{"y/n", "discard or keep edits"} {
		if !strings.Contains(footer, want) {
			t.Errorf("the discard prompt must advertise %q, got: %q", want, footer)
		}
	}
	if strings.Contains(footer, "enter") {
		t.Errorf("the discard prompt must not promise enter, got: %q", footer)
	}
}

// Pressing enter at the discard prompt does nothing at all — it neither
// discards nor dismisses — so the prompt cannot be resolved by accident.
func TestEnterAtDiscardPromptIsInert(t *testing.T) {
	m, _, _ := loaded(t, "")
	m = typeRune(t, m, 'x') // dirty the title
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "esc"})
	if !m.confirmingDiscard {
		t.Fatal("precondition: a dirty esc should raise the discard prompt")
	}

	m, cmd := updateModel(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.confirmingDiscard {
		t.Error("enter must leave the discard prompt up, not resolve it")
	}
	if runCmd(cmd) != nil {
		t.Errorf("enter at the discard prompt must emit nothing, got %T", runCmd(cmd))
	}

	// y still discards, n still keeps — the prompt itself is unchanged.
	m, cmd = updateModel(m, tea.KeyPressMsg{Text: "y", Code: 'y'})
	if _, ok := runCmd(cmd).(cmds.CloseDetailsSideMsg); !ok {
		t.Errorf("y must still discard and close, got %T", runCmd(cmd))
	}
}
