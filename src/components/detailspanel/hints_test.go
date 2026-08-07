package detailspanel

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/apptypes"
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
