package cmds

// CreateTaskMsg is sent by add-input after successfully creating a task,
// carrying the new task's id and depth so AppModel can refresh and update selection.
type CreateTaskMsg struct {
	NewID string
	Depth int
}
