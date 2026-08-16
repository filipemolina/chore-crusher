package cmds

import tea "charm.land/bubbletea/v2"

// DeleteAttachmentMsg requests that the store delete the given attachment.
// Path is carried along so AppModel can quote the attachment's path in the
// confirm dialog without a store round-trip — detailspanel already holds it.
type DeleteAttachmentMsg struct {
	TaskID       string
	AttachmentID string
	Path         string
}

// DeleteAttachment returns a command that asks AppModel to delete the given
// attachment, mirroring DeleteComment.
func DeleteAttachment(taskID, attachmentID, path string) tea.Cmd {
	return func() tea.Msg {
		return DeleteAttachmentMsg{TaskID: taskID, AttachmentID: attachmentID, Path: path}
	}
}
