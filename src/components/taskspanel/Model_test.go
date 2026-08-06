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

func TestTasksSurfaceRendersSinglePanel(t *testing.T) {
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
	// The add input is gone: inline creation now lives in the tree, so the old
	// bottom-pinned footer placeholder ("new task") must not render. The tree's
	// own empty-state text ("Add a task to get started") is expected here for
	// a nil-store panel and is unrelated to the evicted footer.
	if strings.Contains(stripped, "new task") {
		t.Errorf("add-input footer placeholder still rendered: %q", stripped)
	}
}

func TestTasksHeaderShowsActiveListName(t *testing.T) {
	panel := layoutModel(t, constants.COMPONENT_TASK_TREE)
	updated, _ := panel.Update(cmds.RefreshTasksMsg{ListID: "L1", ListName: "Groceries"})
	view := updated.(Model).View().Content
	stripped := ansi.Strip(view)

	if !strings.Contains(stripped, "Tasks") {
		t.Errorf("header lost the Tasks label: %q", stripped)
	}
	if !strings.Contains(stripped, "Groceries") {
		t.Errorf("header missing the active list name: %q", stripped)
	}
	// The name is on the title line, to the right of the Tasks chip.
	titleLine := lineContaining(stripped, "Tasks")
	if !strings.Contains(titleLine, "Groceries") {
		t.Errorf("list name not on the title line: %q", titleLine)
	}
	if idx, name := strings.Index(titleLine, "Tasks"), strings.Index(titleLine, "Groceries"); name <= idx {
		t.Errorf("list name should sit right of Tasks: %q", titleLine)
	}
	if got, want := lipgloss.Width(view), 40; got != want {
		t.Errorf("header widened the panel: width = %d, want %d", got, want)
	}
}

func TestTasksHeaderPlainWithoutActiveList(t *testing.T) {
	panel := layoutModel(t, constants.COMPONENT_TASK_TREE)
	// No RefreshTasksMsg yet: the header is just "Tasks", no trailing name.
	titleLine := lineContaining(ansi.Strip(panel.View().Content), "Tasks")
	if !strings.Contains(titleLine, "Tasks") {
		t.Errorf("title line missing Tasks: %q", titleLine)
	}
}

// lineContaining returns the first line of block that holds sub, or "".
func lineContaining(block, sub string) string {
	for _, line := range strings.Split(block, "\n") {
		if strings.Contains(line, sub) {
			return line
		}
	}
	return ""
}

func TestTasksHeaderTruncatesLongListName(t *testing.T) {
	panel := layoutModel(t, constants.COMPONENT_TASK_TREE)
	long := strings.Repeat("verylongname ", 10)
	updated, _ := panel.Update(cmds.RefreshTasksMsg{ListID: "L1", ListName: long})
	view := updated.(Model).View().Content
	if got, want := lipgloss.Width(view), 40; got != want {
		t.Errorf("long list name overflowed the panel: width = %d, want %d", got, want)
	}
}

func TestTasksSurfaceRendersOnePanelWhetherFocused(t *testing.T) {
	// The task tree is the only focus zone left in the Tasks surface; a
	// non-task-tree focus (lists) simply leaves the panel unfocused. In both
	// cases exactly one "Tasks" panel renders.
	for _, focus := range []int{constants.COMPONENT_TASK_TREE, constants.COMPONENT_LISTS_PANEL} {
		panel := layoutModel(t, focus)
		stripped := ansi.Strip(panel.View().Content)
		if got := strings.Count(stripped, "Tasks"); got != 1 {
			t.Errorf("focus %d: Tasks title count = %d, want 1", focus, got)
		}
	}
}

var _ tea.Model = Model{}
