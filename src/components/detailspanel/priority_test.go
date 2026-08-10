package detailspanel

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/store"
)

// The Priority zone cycles through exactly the four values decision 6 locks,
// in rank order, wrapping both ways — → always means "more important" until it
// wraps. Alphabetical order would put high next to low, which is what makes a
// text column the wrong thing to sort on in the first place.
func TestPriorityCyclesThroughFourValuesBothWays(t *testing.T) {
	m, _, _ := loaded(t, "")
	m = zoneFor(t, m, focusPriority)

	if m.Priority() != apptypes.PriorityNone {
		t.Fatalf("a fresh task opens at %q, want none", m.Priority())
	}

	forward := []apptypes.Priority{
		apptypes.PriorityLow, apptypes.PriorityMedium,
		apptypes.PriorityHigh, apptypes.PriorityNone,
	}
	for _, want := range forward {
		m, _ = updateModel(m, tea.KeyPressMsg{Text: "right"})
		if m.Priority() != want {
			t.Fatalf("→ gave %q, want %q", m.Priority(), want)
		}
	}

	backward := []apptypes.Priority{
		apptypes.PriorityHigh, apptypes.PriorityMedium,
		apptypes.PriorityLow, apptypes.PriorityNone,
	}
	for _, want := range backward {
		m, _ = updateModel(m, tea.KeyPressMsg{Text: "left"})
		if m.Priority() != want {
			t.Fatalf("← gave %q, want %q", m.Priority(), want)
		}
	}
}

// The zone renders the value it holds — including none, unlike the task row's
// badge, which renders nothing for it. A field the user is editing has to show
// what it currently holds, and while it has the keyboard it lifts onto
// BackgroundElevated, which is how this app says "this takes input"
// (docs/DESIGN.md §12).
func TestPriorityZoneRendersItsValueAndLifts(t *testing.T) {
	m, _, _ := loaded(t, "")
	m, _ = updateModel(m, cmds.SetDetailsLayout(80, 30)())

	m = zoneFor(t, m, focusProgress)
	unfocused := m.renderPriorityZone()
	if got := ansi.Strip(unfocused); !strings.Contains(got, "none") {
		t.Errorf("the Priority zone must show its value, got: %q", got)
	}

	m = zoneFor(t, m, focusPriority)
	focused := m.renderPriorityZone()
	lift := lipgloss.NewStyle().
		Foreground(appstyles.Active.TextPrimary).
		Background(appstyles.Active.BackgroundElevated).
		Render("x")
	if !strings.Contains(focused, lift[:strings.Index(lift, "x")]) {
		t.Errorf("the focused Priority value must lift onto BackgroundElevated, got: %q", focused)
	}
	if strings.Contains(unfocused, lift[:strings.Index(lift, "x")]) {
		t.Errorf("an unfocused Priority value must not be lit, got: %q", unfocused)
	}

	// The zone label is part of the modal body, so the whole surface names it.
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Priority") {
		t.Errorf("the modal must label the Priority zone:\n%s", view)
	}
}

// Cycling the rank dirties the draft, so esc raises the discard prompt rather
// than throwing the change away silently — the same contract the other three
// fields have.
func TestCyclingPriorityDirtiesTheDraft(t *testing.T) {
	m, _, _ := loaded(t, "")
	m = zoneFor(t, m, focusPriority)
	if m.hasDirtyFields() {
		t.Fatal("a freshly hydrated modal must be clean")
	}
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "right"})
	if !m.hasDirtyFields() {
		t.Error("cycling the priority must dirty the draft")
	}

	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "esc"})
	if msg := runCmd(cmd); msg != nil {
		t.Fatalf("esc on a dirty modal closed it with %T instead of prompting", msg)
	}
	if !m.confirmingDiscard {
		t.Error("esc on a modal dirtied only by a priority change must prompt before discarding")
	}
}

// ctrl+s persists the rank through store.SetPriority, so the value the TUI
// shows and the value the CLI and MCP surfaces read are the same one.
func TestPrioritySavesThroughTheStore(t *testing.T) {
	m, s, taskID := loaded(t, "")
	m = zoneFor(t, m, focusPriority)
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "right"}) // low
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "right"}) // medium
	m, _ = updateModel(m, tea.KeyPressMsg{Text: "right"}) // high

	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "ctrl+s"})
	if _, ok := runCmd(cmd).(cmds.CloseDetailsSideMsg); !ok {
		t.Fatalf("ctrl+s: got %T, want CloseDetailsSideMsg", runCmd(cmd))
	}
	if m.errMsg != "" {
		t.Fatalf("save reported %q", m.errMsg)
	}

	task, err := s.GetTask(taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Priority != store.PriorityHigh {
		t.Errorf("stored priority = %q, want high", task.Priority)
	}
}

// The trap, on this surface: store.SetPriority rejects the zero value and
// bumps updated_at, so a save that touched only the title must not write the
// priority at all. A rename silently clearing (or merely re-stamping) a rank
// someone set is exactly the failure that section exists to prevent.
func TestTitleOnlySaveLeavesThePriorityAlone(t *testing.T) {
	m, s, taskID := loaded(t, "")
	if err := s.SetPriority(taskID, store.PriorityHigh); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}
	// Re-hydrate so the modal opens on the stored rank.
	m, _ = updateModel(m, cmds.RefreshDetails(s, taskID)())
	if m.Priority() != apptypes.PriorityHigh {
		t.Fatalf("modal hydrated at %q, want high", m.Priority())
	}
	before, err := s.GetTask(taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	// Rename only.
	m = typeRune(t, m, '!')
	m, cmd := updateModel(m, tea.KeyPressMsg{Text: "ctrl+s"})
	if _, ok := runCmd(cmd).(cmds.CloseDetailsSideMsg); !ok {
		t.Fatalf("ctrl+s: got %T, want CloseDetailsSideMsg", runCmd(cmd))
	}

	after, err := s.GetTask(taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if after.Priority != store.PriorityHigh {
		t.Errorf("priority after a title-only save = %q, want high", after.Priority)
	}
	if after.Title == before.Title {
		t.Fatal("precondition: the title should have changed")
	}
}
