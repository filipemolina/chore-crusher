package tasktree

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/cmds"
)

// TestPriorityLabelRendersRankAndNothingForNone pins the badge text: the three
// real ranks render as colored dot plus all-caps labels, and none
// renders NOTHING rather than a badge reading "NONE" — most tasks are none,
// and a badge on every row is noise (docs/DESIGN.md §12).
func TestPriorityLabelRendersRankAndNothingForNone(t *testing.T) {
	for _, tc := range []struct {
		p    apptypes.Priority
		want string
	}{
		{apptypes.PriorityHigh, "● HIGH"},
		{apptypes.PriorityMedium, "● MED"},
		{apptypes.PriorityLow, "● LOW"},
		{apptypes.PriorityNone, ""},
		{apptypes.Priority(""), ""},
	} {
		if got := priorityLabel(tc.p); got != tc.want {
			t.Errorf("priorityLabel(%q) = %q, want %q", tc.p, got, tc.want)
		}
	}

	m := &Model{}
	row := apptypes.Row{Task: apptypes.Task{ID: "1", Title: "Paint the fence", Priority: apptypes.PriorityHigh}}
	m.rows = []apptypes.Row{row}
	if got := ansi.Strip(m.renderRow(row, 80, testBg, nil)); !strings.Contains(got, "● HIGH") {
		t.Errorf("a high-priority row must show its badge, got: %q", got)
	}

	row.Task.Priority = apptypes.PriorityNone
	m.rows = []apptypes.Row{row}
	got := ansi.Strip(m.renderRow(row, 80, testBg, nil))
	if strings.Contains(got, "NONE") || strings.Contains(got, "none") {
		t.Errorf("a none-priority row must render no badge, got: %q", got)
	}
}

// TestPriorityFgIsAStatusColorLadder pins the rank ladder: high uses
// StatusOverdue (red), medium uses StatusInProgress (amber), low uses
// StatusPending (grey). Every value is an active-theme status token rather
// than a text tier, creating a more distinctive visual hierarchy.
func TestPriorityFgIsAStatusColorLadder(t *testing.T) {
	if got := priorityFg(apptypes.PriorityHigh); got != appstyles.Active.StatusOverdue {
		t.Errorf("high priority fg = %v, want StatusOverdue", got)
	}
	if got := priorityFg(apptypes.PriorityMedium); got != appstyles.Active.StatusInProgress {
		t.Errorf("medium priority fg = %v, want StatusInProgress", got)
	}
	if got := priorityFg(apptypes.PriorityLow); got != appstyles.Active.StatusPending {
		t.Errorf("low priority fg = %v, want StatusPending", got)
	}
}

// TestAssigneeBadgeRendersTagOrNothing pins the badge itself: an assigned task
// shows @tag, an unassigned one shows nothing at all, and a long tag is
// truncated through chrome.Truncate rather than pushing the right block across
// the row (docs/UI_INSTRUCTIONS.md rule 3).
func TestAssigneeBadgeRendersTagOrNothing(t *testing.T) {
	if got := assigneeBadge("", ""); got != "" {
		t.Errorf("assigneeBadge(\"\") = %q, want \"\"", got)
	}
	if got := assigneeBadge("pi", ""); got != "@pi" {
		t.Errorf("assigneeBadge(\"pi\") = %q, want \"@pi\"", got)
	}
	long := assigneeBadge("an-extremely-long-agent-identity", "")
	if w := lipgloss.Width(long); w > assigneeTagWidth+1 {
		t.Errorf("assigneeBadge clipped to %d cells (%q), want at most %d", w, long, assigneeTagWidth+1)
	}
	if !strings.HasSuffix(long, "…") {
		t.Errorf("a clipped tag must end in the truncation ellipsis, got %q", long)
	}

	m := &Model{}
	row := apptypes.Row{Task: apptypes.Task{ID: "1", Title: "Ship it", Assignee: "claude"}}
	m.rows = []apptypes.Row{row}
	if got := ansi.Strip(m.renderRow(row, 80, testBg, nil)); !strings.Contains(got, "@claude") {
		t.Errorf("an assigned row must show its @tag, got: %q", got)
	}

	row.Task.Assignee = ""
	m.rows = []apptypes.Row{row}
	if got := ansi.Strip(m.renderRow(row, 80, testBg, nil)); strings.Contains(got, "@") {
		t.Errorf("an unassigned row must show no assignee badge, got: %q", got)
	}
}

// TestAssigneeFgIsTheStaleTier pins the one signal the whole assignment model
// hangs on: assignment has no TTL, so an assignee whose agent holds no live
// presence claim is the only thing on screen that says the work was abandoned
// rather than merely owned (docs/DESIGN.md §3). Live is ordinary row chrome;
// stale is the warning token.
func TestAssigneeFgIsTheStaleTier(t *testing.T) {
	if got := assigneeFg(true); got != appstyles.Active.TextMuted {
		t.Errorf("live assignee fg = %v, want TextMuted", got)
	}
	if got := assigneeFg(false); got != appstyles.Active.StatusOverdue {
		t.Errorf("stale assignee fg = %v, want StatusOverdue (the warning tier)", got)
	}
}

// TestStaleAssignmentRendersDifferentlyFromLiveOne drives the tier through the
// real render path, so the test fails if the badge is drawn without consulting
// the live-agent set at all — which is exactly the wiring a colour-only unit
// test cannot catch. The live set arrives with the refresh, so the test feeds
// one rather than poking the field.
func TestStaleAssignmentRendersDifferentlyFromLiveOne(t *testing.T) {
	row := apptypes.Row{Task: apptypes.Task{ID: "1", Title: "Ship it", Assignee: "claude"}}

	tree := New()
	updated, _ := tree.Update(cmds.RefreshTasksMsg{
		ListID: "L",
		Rows:   []apptypes.Row{row},
		Activities: []apptypes.AgentActivity{
			{EntityType: "task", EntityID: "1", AgentID: "claude", Kind: "working"},
		},
	})
	liveModel := updated.(Model)
	live := liveModel.renderRow(row, 80, testBg, nil)

	updated, _ = tree.Update(cmds.RefreshTasksMsg{
		ListID: "L",
		Rows:   []apptypes.Row{row},
		// The agent that holds the task is gone: no claim of its own, and
		// another agent's claim must not light this row up.
		Activities: []apptypes.AgentActivity{
			{EntityType: "task", EntityID: "9", AgentID: "someone-else", Kind: "working"},
		},
	})
	staleModel := updated.(Model)
	stale := staleModel.renderRow(row, 80, testBg, nil)

	if live == stale {
		t.Fatal("an abandoned assignment renders identically to a live one: the stale tier is not wired to the live-agent set")
	}
	if !strings.Contains(stale, sgrPrefix(appstyles.Active.StatusOverdue)) {
		t.Errorf("a stale assignee badge must draw in the warning tier, got: %q", stale)
	}
	if strings.Contains(live, sgrPrefix(appstyles.Active.StatusOverdue)) {
		t.Errorf("a live assignee badge must not draw in the warning tier, got: %q", live)
	}
	// The live row de-dupes: the assignee is the agent present on the row, so
	// the presence unit already names it and the @tag is suppressed (backlog:
	// "hermes @hermes" must not print the name twice). The stale row has no
	// live claim from the assignee, so the @tag is the only thing naming the
	// owner — and it draws in the warning tier.
	if strings.Contains(ansi.Strip(live), "@claude") {
		t.Errorf("live row must de-dupe the @tag when the assignee is present, got: %q", ansi.Strip(live))
	}
	if !strings.Contains(ansi.Strip(live), "claude") {
		t.Errorf("live row must still name the present agent via the presence unit, got: %q", ansi.Strip(live))
	}
	if !strings.Contains(ansi.Strip(stale), "@claude") {
		t.Errorf("stale row must show the @tag naming the owner, got: %q", ansi.Strip(stale))
	}
}

// sgrPrefix is the escape sequence lipgloss emits for one foreground colour,
// probed from a render rather than assembled by hand so it cannot drift from
// what the row renderer actually writes.
func sgrPrefix(fg color.Color) string {
	probe := lipgloss.NewStyle().Foreground(fg).Render("x")
	return probe[:strings.Index(probe, "x")]
}

// TestBadgeDropOrder pins the width budget with both new cells present:
// every unit is atomic (full width or zero, never a fragment) and they shed in
// the order docs/DESIGN.md §12 records — icon+priority block, agent spinner,
// assignee, progress. Priority outliving the assignee is the point: at 40
// columns "what should I pick up next" outlives "who has it".
func TestBadgeDropOrder(t *testing.T) {
	const checkbox = 1
	progress := "42%"
	agent := "claude"
	assignee := assigneeBadge("claude", "")
	priority := priorityLabel(apptypes.PriorityHigh)

	detailsFull := detailsColWidth + 1
	progressFull := len(progress) + 1
	agentFull := lipgloss.Width(agent) + 1
	assigneeFull := lipgloss.Width(assignee) + 1
	priorityFull := lipgloss.Width(priority) + 1

	for width := 1; width <= 140; width++ {
		cols := computeTaskRowCols(width, checkbox, progress, agent, assignee, priority)

		if cols.assignee != 0 && cols.assignee != assigneeFull {
			t.Fatalf("width %d: assignee = %d, want 0 or %d", width, cols.assignee, assigneeFull)
		}
		if cols.priority != 0 && cols.priority != priorityFull {
			t.Fatalf("width %d: priority = %d, want 0 or %d", width, cols.priority, priorityFull)
		}
		if cols.details != 0 && cols.details != detailsFull {
			t.Fatalf("width %d: details = %d, want 0 or %d", width, cols.details, detailsFull)
		}
		if cols.agentPresence != 0 && cols.agentPresence != agentFull {
			t.Fatalf("width %d: agent-spinner = %d, want 0 or %d", width, cols.agentPresence, agentFull)
		}
		if cols.progress != 0 && cols.progress != progressFull {
			t.Fatalf("width %d: progress = %d, want 0 or %d", width, cols.progress, progressFull)
		}

		// Drop order, each link of the chain.
		// Order: details+priority shed together -> assignee -> agentPresence
		// -> progress. Priority is the icon column's adjacent state group, so
		// it sheds with the icon column, not after the assignee.
		if (cols.details == 0) != (cols.priority == 0) {
			t.Fatalf("width %d: details=%d and priority=%d must shed together", width, cols.details, cols.priority)
		}
		if cols.details != 0 && cols.assignee == 0 {
			t.Fatalf("width %d: icon+priority kept but assignee shed", width)
		}
		if cols.assignee != 0 && cols.agentPresence == 0 {
			t.Fatalf("width %d: assignee kept but agent-spinner shed", width)
		}
		if cols.agentPresence != 0 && cols.progress == 0 {
			t.Fatalf("width %d: agent-spinner kept but progress shed", width)
		}

		sum := cols.title + cols.details + cols.progress +
			cols.agentPresence + cols.assignee + cols.priority
		if sum > width {
			t.Fatalf("width %d: cols sum %d > table width %d (overflow)", width, sum, width)
		}
		if cols.title < 1 {
			t.Fatalf("width %d: title = %d, want >= 1", width, cols.title)
		}
	}
}

// TestRowWithBothBadgesNeverOverflows is the mechanical backstop for the two
// new cells: a row carrying every passenger at once still spans its panel
// exactly at every width, from below the 40-column minimum upward.
func TestRowWithBothBadgesNeverOverflows(t *testing.T) {
	m := &Model{}
	row := apptypes.Row{Task: apptypes.Task{
		ID: "1", Title: "Water the ferns before the weekend meeting",
		Status: apptypes.StatusInProgress, ProgressKind: apptypes.ProgressPercentage,
		ProgressPct: intPtr(63), Assignee: "pi", Priority: apptypes.PriorityHigh,
	}}
	m.rows = []apptypes.Row{row}
	m.selectedID = "1"
	m.work = map[string]apptypes.AgentActivity{
		"1": {EntityType: "task", EntityID: "1", AgentID: "claude", Kind: "working"},
	}
	m.liveAgents = map[string]bool{"claude": true}

	for width := 20; width <= 200; width++ {
		rendered := m.renderRow(row, width, testBg, nil)
		if got := lipgloss.Width(rendered); got != width {
			t.Fatalf("width %d: rendered row = %d columns, want exactly %d", width, got, width)
		}
		if width >= 140 {
			stripped := ansi.Strip(rendered)
			for _, want := range []string{"HIGH", "@pi", "claude", "63%"} {
				if !strings.Contains(stripped, want) {
					t.Fatalf("width %d: expected %q in row: %q", width, want, stripped)
				}
			}
		}
	}
}

// TestPrioritySitsNextToIconColumn pins the render order of the right-aligned
// block on a row that carries every badge: progress, agent spinner, assignee,
// priority, then the trailing icon column. Priority sits immediately left of
// the icon column (the ask "put task priority closer to the task status"
// that motivated the position originally still holds now that the icon
// column, not a status label, is what follows it), so the row reads as "how
// much work, who is working, who owns it, what priority" rather than
// scattering the priority badge in the middle (docs/DESIGN.md §12).
func TestPrioritySitsNextToIconColumn(t *testing.T) {
	m := &Model{}
	row := apptypes.Row{Task: apptypes.Task{
		ID: "1", Title: "Paint the fence", Notes: "n",
		Status: apptypes.StatusInProgress, ProgressKind: apptypes.ProgressPercentage,
		ProgressPct: intPtr(63), Assignee: "pi", Priority: apptypes.PriorityHigh,
	}}
	m.rows = []apptypes.Row{row}
	m.work = map[string]apptypes.AgentActivity{
		"1": {EntityType: "task", EntityID: "1", AgentID: "hermes", Kind: "working"},
	}
	m.liveAgents = map[string]bool{"hermes": true}
	m.animFrame = 0

	stripped := ansi.Strip(m.renderRow(row, 100, testBg, nil))
	idx := func(s string) int { return strings.Index(stripped, s) }
	pos := map[string]int{
		"progress":      idx("63%"),
		"agentPresence": idx("hermes"),
		"assignee":      idx("@pi"),
		"priority":      idx("● HIGH"),
		"icon":          idx(detailsIcon),
	}
	for name, i := range pos {
		if i < 0 {
			t.Fatalf("badge %s missing from row: %q", name, stripped)
		}
	}
	if !(pos["progress"] < pos["agentPresence"] && pos["agentPresence"] < pos["assignee"] &&
		pos["assignee"] < pos["priority"] && pos["priority"] < pos["icon"]) {
		t.Fatalf("right block out of order (want progress < agentPresence < assignee < priority < icon): %q", stripped)
	}
	// Adjacency: nothing but cell padding between the priority badge and the
	// icon column; priority is the cell immediately left of it.
	between := stripped[pos["priority"]+len("● HIGH") : pos["icon"]]
	if strings.Trim(between, " ") != "" {
		t.Fatalf("priority badge not immediately left of the icon column, %q sits between them in: %q", between, stripped)
	}
}

// TestUnassignKeysEmitTheReleaseRequests pins the two release keys: u asks for
// the selected task and U for the whole active list. The tree only ever asks —
// AppModel owns the store write, the same split space/d already use.
func TestUnassignKeysEmitTheReleaseRequests(t *testing.T) {
	tree := New()
	updated, _ := tree.Update(cmds.SetFocus(focusedZoneID)())
	updated, _ = updated.Update(cmds.RefreshTasksMsg{
		ListID: "list-1",
		Rows: []apptypes.Row{
			{Task: apptypes.Task{ID: "t1", Title: "one", Assignee: "claude"}},
			{Task: apptypes.Task{ID: "t2", Title: "two"}},
		},
	})

	after, cmd := updated.Update(tea.KeyPressMsg{Text: "u", Code: 'u'})
	if cmd == nil {
		t.Fatal("u produced no command")
	}
	msg, ok := cmd().(cmds.UnassignTaskMsg)
	if !ok {
		t.Fatalf("u produced %T, want UnassignTaskMsg", cmd())
	}
	if msg.TaskID != "t1" {
		t.Errorf("u released %q, want the selected task t1", msg.TaskID)
	}

	_, cmd = after.Update(tea.KeyPressMsg{Text: "U", Code: 'U'})
	if cmd == nil {
		t.Fatal("U produced no command")
	}
	rel, ok := cmd().(cmds.ReleaseListMsg)
	if !ok {
		t.Fatalf("U produced %T, want ReleaseListMsg", cmd())
	}
	if rel.ListID != "list-1" {
		t.Errorf("U released list %q, want the active list list-1", rel.ListID)
	}
}
