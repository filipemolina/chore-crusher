package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/filipemolina/farol/src/store"
)

// watchEventJSON is the parsed envelope of one emitted event, with the
// payload fields kept raw so each test unmarshals only what it asserts.
type watchEventJSON struct {
	Event  string          `json:"event"`
	Task   json.RawMessage `json:"task"`
	TaskID string          `json:"task_id"`
	ListID string          `json:"list_id"`
	List   json.RawMessage `json:"list"`
}

// watchTaskJSON is the subset of the taskRowJSON payload a watch test asserts
// on (the full shape is pinned by tasks_test.go).
type watchTaskJSON struct {
	ID        string  `json:"id"`
	ParentID  *string `json:"parent_id"`
	Title     string  `json:"title"`
	Status    string  `json:"status"`
	Depth     int     `json:"depth"`
	ListOwner string  `json:"list_owner"`
	Priority  string  `json:"priority"`
}

type watchListPayloadJSON struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CreatedBy     string `json:"created_by"`
	Collaborative bool   `json:"collaborative"`
}

// watchTestStore opens a fresh store in a temp dir for one watch test.
func watchTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "farol.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// captureStdout runs fn with os.Stdout swapped for a pipe and returns what it
// wrote — the same technique runCLI uses, so a poll's events are asserted on
// the exact bytes a real caller would see.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = old
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(out)
}

// watchPoll runs one poll pass and returns the events it emitted, one JSON
// value per line, in order. It fails the test on any poll error.
func watchPoll(t *testing.T, st *watchState) []watchEventJSON {
	t.Helper()
	var pollErr error
	out := captureStdout(t, func() { pollErr = st.poll() })
	if pollErr != nil {
		t.Fatalf("poll: %v", pollErr)
	}
	var events []watchEventJSON
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev watchEventJSON
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("event line %q is not one JSON value: %v", line, err)
		}
		events = append(events, ev)
	}
	return events
}

func wantEventCount(t *testing.T, ev []watchEventJSON, name string, n int) {
	t.Helper()
	if len(ev) != n {
		t.Fatalf("%s: %d events, want %d: %+v", name, len(ev), n, ev)
	}
}

func watchTaskPayload(t *testing.T, raw json.RawMessage) watchTaskJSON {
	t.Helper()
	var p watchTaskJSON
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("task payload %s is not taskRowJSON: %v", raw, err)
	}
	return p
}

// TestWatchLiveStartReportsOnlyChanges pins the live-start semantics: the
// first poll records the current state and reports nothing — a pre-existing
// task is background, not change — and every task created after that emits a
// task_created event carrying the taskRowJSON shape (depth, parent_id and
// list_owner included).
func TestWatchLiveStartReportsOnlyChanges(t *testing.T) {
	s := watchTestStore(t)
	lid, err := s.CreateList("Groceries", "pi")
	if err != nil {
		t.Fatal(err)
	}
	pre, err := s.CreateTask(lid, "pre-existing", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	st := newWatchState(s, true, false, lid, "", -1)
	if ev := watchPoll(t, st); len(ev) != 0 {
		t.Fatalf("baseline poll emitted %d events, want 0: %+v", len(ev), ev)
	}

	// A fresh root task.
	rootID, err := s.CreateTask(lid, "buy milk", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	// A subtask, to pin depth and parent_id on the event payload.
	childID, err := s.CreateTask(lid, "at the corner shop", &rootID, "")
	if err != nil {
		t.Fatal(err)
	}

	ev := watchPoll(t, st)
	wantEventCount(t, ev, "after two creates", 2)
	if ev[0].Event != watchEventTaskCreated || ev[1].Event != watchEventTaskCreated {
		t.Fatalf("events = %+v, want two task_created", ev)
	}
	root := watchTaskPayload(t, ev[0].Task)
	if root.ID != rootID || root.Title != "buy milk" || root.Depth != 0 || root.ParentID != nil {
		t.Errorf("root payload = %+v, want id %s, depth 0, no parent", root, rootID)
	}
	child := watchTaskPayload(t, ev[1].Task)
	if child.ID != childID || child.Depth != 1 || child.ParentID == nil || *child.ParentID != rootID {
		t.Errorf("child payload = %+v, want id %s, depth 1, parent %s", child, childID, rootID)
	}
	// The root gained its first subtask, which auto-starts it to in_progress
	// in subtasks mode (docs/DESIGN.md §3) — the created event carries the
	// post-switch state. The child is created plain pending.
	if root.Status != "in_progress" {
		t.Errorf("root payload %+v: want status in_progress (auto-started by its first subtask)", root)
	}
	if child.Status != "pending" {
		t.Errorf("child payload %+v: want status pending", child)
	}
	for _, p := range []watchTaskJSON{root, child} {
		if p.ListOwner != "pi" || p.Priority != "none" {
			t.Errorf("payload %+v: want list_owner pi, priority none", p)
		}
	}
	if pre == "" {
		t.Fatal("unreachable")
	}
}

// TestWatchReportsTaskUpdated pins the updated_at diff: a known task whose
// activity moved past what was last reported emits task_updated with the new
// row. The last-seen watermark is set one second in the past directly rather
// than sleeping across a second boundary — the diff rule under test is
// updated_at > last, and second-granularity timestamps otherwise make the
// test clock-dependent.
func TestWatchReportsTaskUpdated(t *testing.T) {
	s := watchTestStore(t)
	lid, _ := s.CreateList("Groceries", "pi")
	tid, _ := s.CreateTask(lid, "buy milk", nil, "")

	st := newWatchState(s, true, false, lid, "", -1)
	watchPoll(t, st) // baseline

	// Simulate one second having passed since the baseline read.
	st.known[tid] = st.known[tid] - 1
	if err := s.RenameTask(tid, "buy almond milk"); err != nil {
		t.Fatal(err)
	}

	ev := watchPoll(t, st)
	wantEventCount(t, ev, "after rename", 1)
	if ev[0].Event != watchEventTaskUpdated {
		t.Fatalf("event = %q, want task_updated: %+v", ev[0].Event, ev[0])
	}
	if p := watchTaskPayload(t, ev[0].Task); p.ID != tid || p.Title != "buy almond milk" {
		t.Errorf("payload = %+v, want id %s title %q", p, tid, "buy almond milk")
	}

	// An unchanged second poll reports nothing.
	if ev := watchPoll(t, st); len(ev) != 0 {
		t.Fatalf("unchanged poll emitted %d events, want 0: %+v", len(ev), ev)
	}
}

// TestWatchReportsTaskDeleted pins deletion detection via the id-set diff: a
// task removed after the baseline emits task_deleted carrying the ids.
func TestWatchReportsTaskDeleted(t *testing.T) {
	s := watchTestStore(t)
	lid, _ := s.CreateList("Groceries", "pi")
	gone, _ := s.CreateTask(lid, "gone soon", nil, "")
	kept, _ := s.CreateTask(lid, "stays", nil, "")

	st := newWatchState(s, true, false, lid, "", -1)
	watchPoll(t, st) // baseline

	if err := s.DeleteTask(gone); err != nil {
		t.Fatal(err)
	}
	ev := watchPoll(t, st)
	wantEventCount(t, ev, "after delete", 1)
	if ev[0].Event != watchEventTaskDeleted {
		t.Fatalf("event = %q, want task_deleted", ev[0].Event)
	}
	if ev[0].TaskID != gone || ev[0].ListID != lid {
		t.Errorf("task_deleted = %s/%s, want %s/%s", ev[0].TaskID, ev[0].ListID, gone, lid)
	}
	if kept == "" {
		t.Fatal("unreachable")
	}
}

// TestWatchReportsListChanged pins list-row diffing: a rename and a
// collaborative toggle each emit one list_changed carrying the new row.
// List rows have no updated_at column (docs/DESIGN.md §2), so list changes
// are detected by comparing the row against the previous poll — no clock
// dependency, unlike task updates.
func TestWatchReportsListChanged(t *testing.T) {
	s := watchTestStore(t)
	lid, _ := s.CreateList("Groceries", "pi")

	st := newWatchState(s, true, false, lid, "", -1)
	watchPoll(t, st) // baseline

	if err := s.RenameList(lid, "pi: Shopping"); err != nil {
		t.Fatal(err)
	}
	ev := watchPoll(t, st)
	wantEventCount(t, ev, "after rename", 1)
	if ev[0].Event != watchEventListChanged {
		t.Fatalf("event = %q, want list_changed", ev[0].Event)
	}
	var lj watchListPayloadJSON
	if err := json.Unmarshal(ev[0].List, &lj); err != nil {
		t.Fatalf("list payload %s: %v", ev[0].List, err)
	}
	if lj.ID != lid || lj.Name != "pi: Shopping" || lj.CreatedBy != "pi" {
		t.Errorf("list payload = %+v, want id %s name %q owner pi", lj, lid, "pi: Shopping")
	}

	if err := s.SetCollaborative(lid, true); err != nil {
		t.Fatal(err)
	}
	ev = watchPoll(t, st)
	wantEventCount(t, ev, "after collaborative toggle", 1)
	if ev[0].Event != watchEventListChanged {
		t.Fatalf("event = %q, want list_changed", ev[0].Event)
	}
	if err := json.Unmarshal(ev[0].List, &lj); err != nil {
		t.Fatal(err)
	}
	if !lj.Collaborative {
		t.Errorf("list payload %+v: want collaborative true", lj)
	}

	// An unchanged poll reports nothing.
	if ev := watchPoll(t, st); len(ev) != 0 {
		t.Fatalf("unchanged poll emitted %d events, want 0: %+v", len(ev), ev)
	}
}

// TestWatchReportsListDeleted pins list deletion: the list_deleted event
// comes first, then task_deleted for every task the watch knew (the delete
// cascades, so each is genuinely gone).
func TestWatchReportsListDeleted(t *testing.T) {
	s := watchTestStore(t)
	lid, _ := s.CreateList("Groceries", "pi")
	tid, _ := s.CreateTask(lid, "casualty", nil, "")

	st := newWatchState(s, true, false, lid, "", -1)
	watchPoll(t, st) // baseline

	if err := s.DeleteList(lid); err != nil {
		t.Fatal(err)
	}
	ev := watchPoll(t, st)
	wantEventCount(t, ev, "after list delete", 2)
	if ev[0].Event != watchEventListDeleted || ev[0].ListID != lid {
		t.Errorf("events[0] = %+v, want list_deleted %s", ev[0], lid)
	}
	if ev[1].Event != watchEventTaskDeleted || ev[1].TaskID != tid {
		t.Errorf("events[1] = %+v, want task_deleted %s", ev[1], tid)
	}

	// A deleted list is reported once; later polls stay silent.
	if ev := watchPoll(t, st); len(ev) != 0 {
		t.Fatalf("post-deletion poll emitted %d events, want 0: %+v", len(ev), ev)
	}
}

// TestWatchTaskScopeIgnoresSiblings pins the task-scope contract: sibling
// tasks are invisible — created, renamed or deleted — while the watched
// task's own changes and deletion emit.
func TestWatchTaskScopeIgnoresSiblings(t *testing.T) {
	s := watchTestStore(t)
	lid, _ := s.CreateList("Groceries", "pi")
	watched, _ := s.CreateTask(lid, "the one", nil, "")
	sibling, _ := s.CreateTask(lid, "a sibling", nil, "")

	st := newWatchState(s, true, true, lid, watched, -1)
	if ev := watchPoll(t, st); len(ev) != 0 {
		t.Fatalf("baseline poll emitted %d events, want 0: %+v", len(ev), ev)
	}

	// A sibling created after the baseline must not emit.
	if _, err := s.CreateTask(lid, "another sibling", nil, ""); err != nil {
		t.Fatal(err)
	}
	if ev := watchPoll(t, st); len(ev) != 0 {
		t.Fatalf("sibling create emitted %d events, want 0: %+v", len(ev), ev)
	}

	// A sibling deleted must not emit either.
	if err := s.DeleteTask(sibling); err != nil {
		t.Fatal(err)
	}
	if ev := watchPoll(t, st); len(ev) != 0 {
		t.Fatalf("sibling delete emitted %d events, want 0: %+v", len(ev), ev)
	}

	// The watched task's own update emits (watermark one second back, as in
	// TestWatchReportsTaskUpdated).
	st.known[watched] = st.known[watched] - 1
	if err := s.RenameTask(watched, "the renamed one"); err != nil {
		t.Fatal(err)
	}
	ev := watchPoll(t, st)
	wantEventCount(t, ev, "after watched rename", 1)
	if ev[0].Event != watchEventTaskUpdated {
		t.Fatalf("event = %q, want task_updated", ev[0].Event)
	}
	if p := watchTaskPayload(t, ev[0].Task); p.ID != watched || p.Title != "the renamed one" {
		t.Errorf("payload = %+v, want id %s", p, watched)
	}

	// And its deletion emits.
	if err := s.DeleteTask(watched); err != nil {
		t.Fatal(err)
	}
	ev = watchPoll(t, st)
	wantEventCount(t, ev, "after watched delete", 1)
	if ev[0].Event != watchEventTaskDeleted || ev[0].TaskID != watched {
		t.Errorf("event = %+v, want task_deleted %s", ev[0], watched)
	}
}

// TestWatchSinceReplay pins the --since replay: the first poll emits every
// current task whose activity is strictly after the timestamp as task_created,
// then the watch continues live from the current state.
func TestWatchSinceReplay(t *testing.T) {
	s := watchTestStore(t)
	lid, _ := s.CreateList("Groceries", "pi")
	a, _ := s.CreateTask(lid, "one", nil, "")
	b, _ := s.CreateTask(lid, "two", nil, "")

	// since 0: both pre-existing tasks replay as created.
	st := newWatchState(s, true, false, lid, "", 0)
	ev := watchPoll(t, st)
	wantEventCount(t, ev, "replay since 0", 2)
	for _, e := range ev {
		if e.Event != watchEventTaskCreated {
			t.Fatalf("replay event = %q, want task_created: %+v", e.Event, e)
		}
	}

	// Live continues: a new task after the replay emits created; the replayed
	// ones are not re-reported.
	if _, err := s.CreateTask(lid, "three", nil, ""); err != nil {
		t.Fatal(err)
	}
	ev = watchPoll(t, st)
	wantEventCount(t, ev, "after replay", 1)
	if ev[0].Event != watchEventTaskCreated {
		t.Fatalf("event = %q, want task_created", ev[0].Event)
	}
	if p := watchTaskPayload(t, ev[0].Task); p.Title != "three" {
		t.Errorf("payload = %+v, want title three", p)
	}
	if a == "" || b == "" {
		t.Fatal("unreachable")
	}
}

// TestWatchSinceReplayUpdatedClassification pins the created-vs-updated rule
// inside the replay window: a task created before the timestamp but changed
// after it reports as task_updated, not task_created. This is the one watch
// test that crosses a second boundary on purpose — the classification
// compares created_at against the window, which needs the update to land in a
// later second than the creation.
func TestWatchSinceReplayUpdatedClassification(t *testing.T) {
	s := watchTestStore(t)
	lid, _ := s.CreateList("Groceries", "pi")
	tid, _ := s.CreateTask(lid, "buy milk", nil, "")
	orig, err := s.GetTask(tid)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := s.RenameTask(tid, "buy almond milk"); err != nil {
		t.Fatal(err)
	}

	// Replay from the task's creation time: created before the window, so
	// the in-window rename must report as updated.
	st := newWatchState(s, true, false, lid, "", orig.CreatedAt)
	ev := watchPoll(t, st)
	wantEventCount(t, ev, "replay with in-window update", 1)
	if ev[0].Event != watchEventTaskUpdated {
		t.Fatalf("event = %q, want task_updated (created_at %d <= baseline)", ev[0].Event, orig.CreatedAt)
	}
	if p := watchTaskPayload(t, ev[0].Task); p.Title != "buy almond milk" {
		t.Errorf("payload = %+v, want the renamed title", p)
	}

	// Replay is one-shot: the same task is not re-reported.
	if ev := watchPoll(t, st); len(ev) != 0 {
		t.Fatalf("second poll emitted %d events, want 0: %+v", len(ev), ev)
	}
}

// TestWatchHumanMode pins the plain-text lines: one per change, no ANSI, and
// a human mode that reads the same diff the JSON stream does.
func TestWatchHumanMode(t *testing.T) {
	s := watchTestStore(t)
	lid, _ := s.CreateList("Groceries", "")
	tid, _ := s.CreateTask(lid, "buy milk", nil, "")

	st := newWatchState(s, false, false, lid, "", -1)
	captureStdout(t, func() { _ = st.poll() }) // baseline

	// The rename lands in the same second as the creation, so step the
	// watermark one second back for it to be visible (as in the JSON tests).
	st.known[tid] = st.known[tid] - 1
	if err := s.RenameTask(tid, "buy almond milk"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameList(lid, "Shopping"); err != nil {
		t.Fatal(err)
	}
	var pollErr error
	out := captureStdout(t, func() { pollErr = st.poll() })
	if pollErr != nil {
		t.Fatalf("poll: %v", pollErr)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("human mode printed %d lines, want 2: %q", len(lines), out)
	}
	// Events emit in the fixed batch order: the list's own change first,
	// then task events (docs/DESIGN.md §9).
	if want := "list changed " + lid + "  Shopping"; lines[0] != want {
		t.Errorf("line 0 = %q, want %q", lines[0], want)
	}
	if want := "task updated " + tid + "  buy almond milk  (pending)"; lines[1] != want {
		t.Errorf("line 1 = %q, want %q", lines[1], want)
	}
}

// TestNewWatchCmdArgsAndFlags pins the command shape: exactly one id
// argument, the --since and --interval flags, and the 1s default interval
// matching the TUI's poll cadence (docs/DESIGN.md §7).
func TestNewWatchCmdArgsAndFlags(t *testing.T) {
	cmd := newWatchCmd()
	if err := cmd.Args(cmd, nil); err == nil {
		t.Error("no argument must be a usage error")
	}
	if err := cmd.Args(cmd, []string{"01ARZ", "01ARZ"}); err == nil {
		t.Error("two arguments must be a usage error")
	}
	if err := cmd.Args(cmd, []string{"01ARZ"}); err != nil {
		t.Errorf("one argument must be accepted: %v", err)
	}
	if cmd.Flags().Lookup("interval") == nil {
		t.Error("--interval flag missing")
	}
	if cmd.Flags().Lookup("since") == nil {
		t.Error("--since flag missing")
	}
	if cmd.Flags().Lookup("interval").DefValue != watchPollInterval.String() {
		t.Errorf("--interval default = %q, want %s", cmd.Flags().Lookup("interval").DefValue, watchPollInterval)
	}
}
