package cmds

import tea "charm.land/bubbletea/v2"

// DeleteCommentMsg requests that the store delete the given comment. Note is
// carried along so AppModel can quote the comment's text in the confirm
// dialog without a store round-trip — detailspanel already holds it.
type DeleteCommentMsg struct {
	TaskID    string
	CommentID string
	Note      string
}

// DeleteComment returns a command that asks AppModel to delete the given
// comment, mirroring DeleteTask.
func DeleteComment(taskID, commentID, note string) tea.Cmd {
	return func() tea.Msg {
		return DeleteCommentMsg{TaskID: taskID, CommentID: commentID, Note: note}
	}
}
