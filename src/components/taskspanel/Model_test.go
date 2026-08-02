package taskspanel

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/constants"
)

func layoutModel(t *testing.T, focus int) Model {
	t.Helper()
	model := New(nil, "")
	updated, _ := model.Update(cmds.SetBodyLayoutMsg{Height: 12, MainWidth: 40})
	updated, _ = updated.Update(cmds.SetFocusMsg(focus))
	panel, ok := updated.(Model)
	if !ok {
		t.Fatalf("TaskPanel is %T, want taskspanel.Model", updated)
	}
	return panel
}

func TestTasksSurfaceHasOneTitleAndPinnedInput(t *testing.T) {
	panel := layoutModel(t, constants.COMPONENT_TASK_TREE)
	view := panel.View().Content
	stripped := ansi.Strip(view)

	if got := strings.Count(stripped, "Tasks"); got != 1 {
		t.Errorf("Tasks title count = %d, want 1: %q", got, stripped)
	}
	if got, want := lipgloss.Width(view), 40; got != want {
		t.Errorf("Tasks surface width = %d, want %d", got, want)
	}
	if got, want := lipgloss.Height(view), 12; got != want {
		t.Errorf("Tasks surface height = %d, want %d", got, want)
	}
	if strings.LastIndex(stripped, "new task") < strings.LastIndex(stripped, "Add a task to get started") {
		t.Errorf("input is not below task content: %q", stripped)
	}
}

func TestTreeAndInputFocusKeepOneTasksSurface(t *testing.T) {
	for _, focus := range []int{constants.COMPONENT_TASK_TREE, constants.COMPONENT_ADD_INPUT} {
		panel := layoutModel(t, focus)
		view := ansi.Strip(panel.View().Content)
		if got := strings.Count(view, "Tasks"); got != 1 {
			t.Errorf("focus %d: Tasks title count = %d, want 1", focus, got)
		}
	}
}

var _ tea.Model = Model{}
