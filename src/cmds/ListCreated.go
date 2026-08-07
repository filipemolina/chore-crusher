package cmds

// ListCreatedMsg reports that a new list was created and should become the
// active one. listnamemodal's ModeNew follow returns this instead of a plain
// RefreshListsMsg so AppModel can select the new list specifically — the
// generic lists refresh has no way to tell "just created, land here" apart
// from any other refresh.
type ListCreatedMsg struct {
	ID string
}
