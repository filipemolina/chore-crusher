package listspanel

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/components/chrome"
)

// TestListSpinnerFgRule pins the list-row spinner color rule: Accent on the
// selected row, TextDim otherwise (docs/plan/mcp-server-enhancement.md §3.7).
func TestListSpinnerFgRule(t *testing.T) {
	if got := spinnerFg(true); got != appstyles.Active.Accent {
		t.Errorf("selected spinner fg = %v, want Accent", got)
	}
	if got := spinnerFg(false); got != appstyles.Active.TextDim {
		t.Errorf("unselected spinner fg = %v, want TextDim", got)
	}
}

// TestRenderListRowShowsSpinnerWhenClaimed pins the agent-presence render for
// list rows (docs/plan/mcp-server-enhancement.md §3.7): a claimed list
// appends the animated spinner glyph for the delegate's animFrame plus the
// short agent id after the pending/done count, and an unclaimed list renders
// none.
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
