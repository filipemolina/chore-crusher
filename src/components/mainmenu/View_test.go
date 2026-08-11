package mainmenu

import (
	"strings"
	"testing"

	"github.com/filipemolina/farol/src/cmds"
)

func TestHeaderRendersWordmark(t *testing.T) {
	m := New()
	updated, _ := m.(Model).Update(cmds.SetBodyLayoutMsg{TerminalWidth: 80})

	out := updated.(Model).View().Content
	if !strings.Contains(out, "Farol") {
		t.Errorf("header output does not contain wordmark:\n%s", out)
	}
}
