package cmds

import (
	tea "charm.land/bubbletea/v2"
)

// SetTaskTreeViewMsg is the task tree's current Pending/Complete/All view
// mode, emitted whenever it changes so the header can render it (mirrors
// ../pulso's cmds.TableStateMsg). It carries the mode as a string, not
// tasktree.ViewMode, so mainmenu can read it without importing tasktree —
// the tree owns the mode; the header is the only other reader.
type SetTaskTreeViewMsg struct {
	View string
}

// SetTaskTreeView builds the message the tree sends on a view-mode key.
func SetTaskTreeView(view string) tea.Cmd {
	return func() tea.Msg {
		return SetTaskTreeViewMsg{View: view}
	}
}
