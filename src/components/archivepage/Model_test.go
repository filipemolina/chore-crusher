package archivepage

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/cmds"
	"github.com/filipemolina/farol/src/constants"
)

func layoutModel(t *testing.T, focus int) Model {
	t.Helper()
	model := New(nil)
	updated, _ := model.Update(cmds.SetBodyLayoutMsg{Height: 12, TerminalWidth: 60})
	updated, _ = updated.Update(cmds.SetFocusMsg(focus))
	page, ok := updated.(Model)
	if !ok {
		t.Fatalf("ArchivePage is %T, want archivepage.Model", updated)
	}
	return page
}

func TestArchivePageRendersAtTerminalWidth(t *testing.T) {
	page := layoutModel(t, constants.COMPONENT_ARCHIVE_PAGE)
	view := page.View().Content

	if got, want := lipgloss.Width(view), 60; got != want {
		t.Errorf("Archive page width = %d, want terminal width %d", got, want)
	}
	if got, want := lipgloss.Height(view), 12; got != want {
		t.Errorf("Archive page height = %d, want %d", got, want)
	}
	if !strings.Contains(ansi.Strip(view), "Archived Lists") {
		t.Errorf("Archive page missing its title: %q", ansi.Strip(view))
	}
	// The keybinding bar goes blank while the page owns the keyboard
	// (mirroring Details), so the page must render its own "esc back" hint —
	// otherwise there is nothing on screen telling the user how to leave.
	if !strings.Contains(ansi.Strip(view), "back") {
		t.Errorf("Archive page missing its own esc-back hint: %q", ansi.Strip(view))
	}
}

func TestArchivePageEscEmitsCloseOnlyWhenFocused(t *testing.T) {
	unfocused := layoutModel(t, constants.COMPONENT_TASK_TREE)
	_, cmd := unfocused.Update(tea.KeyPressMsg{Text: "esc"})
	if cmd != nil {
		t.Fatal("esc closed the Archive page while it was not focused")
	}

	focused := layoutModel(t, constants.COMPONENT_ARCHIVE_PAGE)
	_, cmd = focused.Update(tea.KeyPressMsg{Text: "esc"})
	if cmd == nil {
		t.Fatal("esc produced no command while the Archive page was focused")
	}
	if _, ok := cmd().(cmds.CloseArchivePageMsg); !ok {
		t.Errorf("esc command = %T, want cmds.CloseArchivePageMsg", cmd())
	}
}

var _ tea.Model = Model{}
