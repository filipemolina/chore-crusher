package constants

// Component ids. A component compares the id it was built with against
// cmds.SetFocusMsg to decide whether it is focused, so these values are part
// of the focus protocol and must stay stable.
const (
	COMPONENT_LISTS_PANEL = 0
	COMPONENT_TASK_TREE   = 1
	COMPONENT_ADD_INPUT   = 2
)

// FocusableComponents are the component ids Tab cycles through, in order —
// but unlike stack-stitcher's fixed slice, this app's cycle is computed at
// call time by AppModel.focusableZones (docs/plans/phase-3-tui-shell.md step
// 4): the lists panel enters and leaves the cycle at runtime as L is
// pressed, so a static slice would leave a focusable-but-invisible stop in
// the cycle (docs/DESIGN.md §5). The order below is the one the method
// preserves: task tree first — the app's premise is "spend your time in one
// list" — then the lists panel when visible, then the add input.
var FocusableComponents = []int{
	COMPONENT_TASK_TREE,
	COMPONENT_LISTS_PANEL,
	COMPONENT_ADD_INPUT,
}
