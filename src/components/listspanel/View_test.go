package listspanel

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/components/chrome"
)

// TestListSpinnerFgRule pins the list-row spinner color rule: Accent on the
// selected row, TextDim otherwise.
func TestListSpinnerFgRule(t *testing.T) {
	if got := spinnerFg(true); got != appstyles.Active.Accent {
		t.Errorf("selected spinner fg = %v, want Accent", got)
	}
	if got := spinnerFg(false); got != appstyles.Active.TextDim {
		t.Errorf("unselected spinner fg = %v, want TextDim", got)
	}
}

// TestRenderListRowShowsSpinnerWhenClaimed pins the agent-presence render for
// list rows: a claimed list appends the animated spinner glyph for
// the delegate's animFrame plus the short agent id after the pending/done
// count, and an unclaimed list renders none.
func TestRenderListRowShowsSpinnerWhenClaimed(t *testing.T) {
	item := apptypes.ListSummary{
		List:          apptypes.List{ID: "L1", Name: "Groceries"},
		PendingCount:  2,
		CompleteCount: 1,
	}

	claimed := listDelegate{
		work: map[string]apptypes.AgentActivity{
			"L1": {EntityType: "list", EntityID: "L1", AgentID: "claude", Kind: "working"},
		},
		animFrame: 2,
	}
	var buf strings.Builder
	claimed.Render(&buf, list.New([]list.Item{item}, claimed, 30, 10), 0, item)

	rendered := ansi.Strip(buf.String())
	if !strings.Contains(rendered, "2 pending · 1 done") {
		t.Errorf("claimed row must keep its count line, got: %q", rendered)
	}
	if !strings.Contains(rendered, chrome.Spinner(2)+" claude") {
		t.Errorf("claimed row must render spinner %q + agent id, got: %q", chrome.Spinner(2), rendered)
	}

	unclaimed := listDelegate{work: map[string]apptypes.AgentActivity{}}
	buf.Reset()
	unclaimed.Render(&buf, list.New([]list.Item{item}, unclaimed, 30, 10), 0, item)

	free := ansi.Strip(buf.String())
	if strings.Contains(free, chrome.Spinner(2)) {
		t.Errorf("unclaimed row must render no spinner, got: %q", free)
	}
}

// TestRenderListRowShowsSpinnerWhenTaskClaimed pins the task-claim aggregate
// a list with any live task claim renders the spinner without an
// agent id, and a simultaneous list-level claim wins (spinner + agent id).
func TestRenderListRowShowsSpinnerWhenTaskClaimed(t *testing.T) {
	item := apptypes.ListSummary{
		List:          apptypes.List{ID: "L1", Name: "Groceries"},
		PendingCount:  2,
		CompleteCount: 1,
	}

	taskClaimed := listDelegate{
		claimedLists: map[string]bool{"L1": true},
		animFrame:    3,
	}
	var buf strings.Builder
	taskClaimed.Render(&buf, list.New([]list.Item{item}, taskClaimed, 30, 10), 0, item)

	rendered := ansi.Strip(buf.String())
	if !strings.Contains(rendered, chrome.Spinner(3)) {
		t.Errorf("task-claimed row must render the spinner, got: %q", rendered)
	}
	if strings.Contains(rendered, "claude") {
		t.Errorf("task-claimed row is an aggregate and must not name an agent, got: %q", rendered)
	}

	// A list-level claim wins over a task-level one: spinner + agent id.
	both := taskClaimed
	both.work = map[string]apptypes.AgentActivity{
		"L1": {EntityType: "list", EntityID: "L1", AgentID: "claude", Kind: "working"},
	}
	buf.Reset()
	both.Render(&buf, list.New([]list.Item{item}, both, 30, 10), 0, item)

	bothRendered := ansi.Strip(buf.String())
	if !strings.Contains(bothRendered, chrome.Spinner(3)+" claude") {
		t.Errorf("list-claim must win over task-claim, got: %q", bothRendered)
	}
}

// TestRenderListRowShowsCollaborativeMarker pins the "Tag a list as
// collaborative" visual contract (docs/DESIGN.md §12, "List is
// collaborative"): a collaborative row appends " · shared" to its count
// line, and a non-collaborative row renders no such marker.
func TestRenderListRowShowsCollaborativeMarker(t *testing.T) {
	shared := apptypes.ListSummary{
		List:          apptypes.List{ID: "L1", Name: "Team backlog", Collaborative: true},
		PendingCount:  2,
		CompleteCount: 1,
	}
	private := apptypes.ListSummary{
		List:          apptypes.List{ID: "L2", Name: "Personal", Collaborative: false},
		PendingCount:  1,
		CompleteCount: 0,
	}

	d := listDelegate{}
	var buf strings.Builder
	d.Render(&buf, list.New([]list.Item{shared}, d, 30, 10), 0, shared)
	sharedRendered := ansi.Strip(buf.String())
	if !strings.Contains(sharedRendered, "2 pending · 1 done · shared") {
		t.Errorf("collaborative row must append ' · shared' to the count line, got: %q", sharedRendered)
	}

	buf.Reset()
	d.Render(&buf, list.New([]list.Item{private}, d, 30, 10), 0, private)
	privateRendered := ansi.Strip(buf.String())
	if strings.Contains(privateRendered, "shared") {
		t.Errorf("non-collaborative row must not render the shared marker, got: %q", privateRendered)
	}
}
