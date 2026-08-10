package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
)

// TestUnassignKeyReleasesAnotherAgentsTask pins u end to end. The release is
// deliberately forced: a stale assignment is by definition held by an agent
// that is not the person at the keyboard, and it has no TTL and no sweeper, so
// a release that refused a foreign holder would leave abandoned work stuck
// forever (decision 2).
func TestUnassignKeyReleasesAnotherAgentsTask(t *testing.T) {
	m := seedOneList(t)
	taskID := seededTaskID(t, m)
	if err := m.store.AssignTask(taskID, "claude", false); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	m = refresh(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = refresh(t, m, cmds.RefreshTasks(m.store, m.activeListID, apptypes.SortManual)())

	m = refresh(t, m, tea.KeyPressMsg{Text: "u", Code: 'u'})

	task, err := m.store.GetTask(taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Assignee != "" {
		t.Errorf("assignee after u = %q, want \"\"", task.Assignee)
	}
	if task.AssignedAt != nil {
		t.Errorf("assigned_at after u = %v, want nil", *task.AssignedAt)
	}
	if m.lastError != "" {
		t.Errorf("u reported an error: %q", m.lastError)
	}
}

// TestReleaseListConfirmsBeforeClearing pins U. Unlike u it can free work
// several agents hold at once, so it goes through the same confirm modal every
// other bulk TUI action uses (docs/DESIGN.md §9) and the dialog counts what is
// about to be released. Nothing is written until the user answers yes.
func TestReleaseListConfirmsBeforeClearing(t *testing.T) {
	m := seedOneList(t)
	listID := m.activeListID
	first := seededTaskID(t, m)
	second, err := m.store.CreateTask(listID, "two", nil, "")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := m.store.AssignTask(first, "claude", false); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	if err := m.store.AssignTask(second, "pi", false); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	m = refresh(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = refresh(t, m, cmds.RefreshTasks(m.store, listID, apptypes.SortManual)())

	m = refresh(t, m, tea.KeyPressMsg{Text: "U", Code: 'U'})

	if m.activeModal == nil {
		t.Fatal("U must confirm before releasing a whole list")
	}
	body := ansi.Strip(m.activeModal.View().Content)
	if !strings.Contains(body, "2 assignments") {
		t.Errorf("the dialog must count what it is about to release, got: %q", body)
	}
	if held, _ := m.store.GetTask(first); held.Assignee != "claude" {
		t.Errorf("assignment cleared before the user answered: %q", held.Assignee)
	}

	m = refresh(t, m, tea.KeyPressMsg{Text: "y", Code: 'y'})

	if m.activeModal != nil {
		t.Error("confirm modal should close after y")
	}
	for _, id := range []string{first, second} {
		task, err := m.store.GetTask(id)
		if err != nil {
			t.Fatalf("GetTask %s: %v", id, err)
		}
		if task.Assignee != "" {
			t.Errorf("task %s still assigned to %q after releasing the list", id, task.Assignee)
		}
	}
}

// TestReleaseListCancelKeepsEveryAssignment is the other half of the confirm:
// answering no leaves the board exactly as it was.
func TestReleaseListCancelKeepsEveryAssignment(t *testing.T) {
	m := seedOneList(t)
	taskID := seededTaskID(t, m)
	if err := m.store.AssignTask(taskID, "claude", false); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	m = refresh(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = refresh(t, m, cmds.RefreshTasks(m.store, m.activeListID, apptypes.SortManual)())

	m = refresh(t, m, tea.KeyPressMsg{Text: "U", Code: 'U'})
	if m.activeModal == nil {
		t.Fatal("precondition: U should have opened a confirm modal")
	}
	m = refresh(t, m, tea.KeyPressMsg{Text: "n", Code: 'n'})

	task, err := m.store.GetTask(taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Assignee != "claude" {
		t.Errorf("assignee after cancelling = %q, want it untouched", task.Assignee)
	}
}
