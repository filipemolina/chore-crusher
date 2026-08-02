package keybindingbar

import (
	"strings"
	"testing"

	"github.com/filipemolina/chore-crusher/src/cmds"
	"github.com/filipemolina/chore-crusher/src/constants"
)

func TestFooterRendersContextAndGlobalKeys(t *testing.T) {
	m := New()
	m, _ = m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 120})
	m, _ = m.(Model).Update(cmds.SetFooterContextMsg{
		Focused:       constants.COMPONENT_TASK_TREE,
		HasActiveList: true,
		TaskTreeEmpty: false,
	})

	out := m.(Model).View().Content
	for _, label := range []string{"navigate", "lists", "theme", "help", "quit"} {
		if !strings.Contains(out, label) {
			t.Errorf("footer output missing %q:\n%s", label, out)
		}
	}
}

func TestFooterShedsHintsOnNarrowTerminal(t *testing.T) {
	m := New()
	m, _ = m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 20})
	m, _ = m.(Model).Update(cmds.SetFooterContextMsg{
		Focused:       constants.COMPONENT_TASK_TREE,
		HasActiveList: true,
		TaskTreeEmpty: false,
	})

	out := m.(Model).View().Content
	// The bar should never wrap onto multiple physical lines; with only 20
	// columns it must have shed the long context hints.
	if strings.Count(out, "\n") > 1 {
		t.Errorf("narrow footer wrapped onto multiple lines:\n%s", out)
	}
	if strings.Contains(out, "navigate") {
		t.Errorf("narrow footer unexpectedly contains long hint:\n%s", out)
	}
}
