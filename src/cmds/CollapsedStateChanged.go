package cmds

// CollapsedStateChangedMsg is emitted when the task tree's collapse/expand
// state changes. AppModel handles this to persist the state per list.
type CollapsedStateChangedMsg struct {
	ListID    string
	Collapsed map[string]bool
}
