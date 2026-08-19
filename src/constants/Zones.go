package constants

// Component ids. A component compares the id it was built with against
// cmds.SetFocusMsg to decide whether it is focused, so these values are part
// of the focus protocol and must stay stable.
const (
	COMPONENT_LISTS_PANEL   = 0
	COMPONENT_TASK_TREE     = 1
	COMPONENT_ADD_INPUT     = 2
	COMPONENT_DETAILS_PANEL = 3
	// COMPONENT_ARCHIVE_PAGE is the Archived Lists page (docs/DESIGN.md §5). Like
	// Details it is never in the tab/shift+tab cycle: it is a full-body takeover
	// entered and left by explicit open/close transitions, not a panel Tasks
	// shares focus with.
	COMPONENT_ARCHIVE_PAGE = 4
)

// FocusableComponents are the component ids tab / shift+tab cycle
// through, in order: the task tree always, the lists panel only while it is
// visible. Inline creation lives inside the tree, so there is no separate
// add-input zone to focus. COMPONENT_ADD_INPUT is retained above as a stable
// id — the addinput package itself has been removed.
var FocusableComponents = []int{
	COMPONENT_TASK_TREE,
	COMPONENT_LISTS_PANEL,
}
