package cmds

import tea "charm.land/bubbletea/v2"

// CreateTaskFromInputMsg is sent by the task tree when the inline creation
// input is submitted with non-empty text. AppModel resolves parent/after ids
// from the refreshed rows and creates the task via the store.
type CreateTaskFromInputMsg struct {
	Title       string
	BeforeID    string
	LevelOffset int
}

// CreateTaskFromInput returns a command that asks AppModel to create a task
// from the inline creation input.
func CreateTaskFromInput(title, beforeID string, levelOffset int) tea.Cmd {
	return func() tea.Msg {
		return CreateTaskFromInputMsg{
			Title:       title,
			BeforeID:    beforeID,
			LevelOffset: levelOffset,
		}
	}
}
