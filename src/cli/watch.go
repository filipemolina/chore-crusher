package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/store"
)

// watchPollInterval is the default poll interval: the TUI's poll_interval_ms
// default of 1000 (docs/DESIGN.md §7). A watcher and the TUI reading the same
// store on the same cadence see each other's writes on the same tick, so the
// change feed never lags the screen by a different beat.
const watchPollInterval = time.Second

// The watch event discriminators, pinned in docs/DESIGN.md §9 alongside the
// payload shapes. Each event carries exactly one payload field: task_created
// and task_updated carry the task row (`task`), task_deleted carries the ids
// (`task_id`, `list_id`), list_changed carries the list row (`list`), and
// list_deleted carries `list_id`.
const (
	watchEventTaskCreated = "task_created"
	watchEventTaskUpdated = "task_updated"
	watchEventTaskDeleted = "task_deleted"
	watchEventListChanged = "list_changed"
	watchEventListDeleted = "list_deleted"
)

// watchListJSON is the list_changed payload: every List column a write can
// change (docs/DESIGN.md §2), so a caller can see what moved without a
// follow-up `farol lists`. id and created_at are immutable and deliberately
// absent — a change event reports changeable state.
type watchListJSON struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	CreatedAt        int64  `json:"created_at"`
	Position         int    `json:"position"`
	CreatedBy        string `json:"created_by"`
	Collaborative    bool   `json:"collaborative"`
	CommentsDisabled bool   `json:"comments_disabled"`
}

func watchListJSONOf(l store.List) watchListJSON {
	return watchListJSON{
		ID:               l.ID,
		Name:             l.Name,
		CreatedAt:        l.CreatedAt,
		Position:         l.Position,
		CreatedBy:        l.CreatedBy,
		Collaborative:    l.Collaborative,
		CommentsDisabled: l.CommentsDisabled,
	}
}

// watchEvent is one change report: the event discriminator plus one payload
// field. The per-event shape is the contract — watch's --json stream is a
// deliberate exception to the one-value rule (docs/DESIGN.md §9), so each
// line's shape is pinned the way a one-shot command's single value is.
type watchEvent struct {
	Event  string         `json:"event"`
	Task   *taskRowJSON   `json:"task,omitempty"`
	TaskID string         `json:"task_id,omitempty"`
	ListID string         `json:"list_id,omitempty"`
	List   *watchListJSON `json:"list,omitempty"`
}

// taskChangeKind is how one current task is reported: created (never seen
// before), updated (seen, with a newer updated_at), or not at all.
type taskChangeKind int

const (
	changeNone    taskChangeKind = iota
	changeCreated                // appeared since the last poll (or inside a --since window)
	changeUpdated                // existed at the last poll, activity since
)

// taskChange pairs a changed task with how it is reported.
type taskChange struct {
	t    store.Task
	kind taskChangeKind
}

// watchState is the per-poll diff state of one `farol watch` run. It holds
// the previous read (task id -> last emitted updated_at, the last list row)
// so each poll can diff against what was already reported.
//
// The store's change detection is the same signal `farol diff --since` and
// list_tasks(since=...) read — a task's updated_at, bumped by creation,
// status/progress, rename, notes, re-parent and new comments (docs/DESIGN.md
// §9) — but applied per row against the previous poll instead of through a
// global watermark query. That is deliberate: the id-set diff is the only way
// to see deletions (a removed task is simply absent), and a per-task watermark
// does not miss a same-second change to a task the query's `updated_at >
// since` could no longer see. The one blind spot shared with the rest of the
// app is second-granularity: two writes to the same task within one unix
// second leave updated_at unchanged, so the second one is indistinguishable
// from the first.
type watchState struct {
	s         *store.Store
	jsonMode  bool
	scopeTask bool   // true: watch one task only; false: watch the whole list
	listID    string // the list the events are read from, either scope
	taskID    string // the watched task when scopeTask
	baseline  int64  // the --since value; -1 means "start live, no replay"
	known     map[string]int64
	listState *store.List
	listGone  bool
	started   bool
}

// newWatchState builds the state for one watch run. baseline is the --since
// value, or -1 for a live start (first poll records the current state and
// reports nothing).
func newWatchState(s *store.Store, jsonMode, scopeTask bool, listID, taskID string, baseline int64) *watchState {
	return &watchState{
		s:         s,
		jsonMode:  jsonMode,
		scopeTask: scopeTask,
		listID:    listID,
		taskID:    taskID,
		baseline:  baseline,
		known:     make(map[string]int64),
	}
}

// poll performs one change-detection pass and emits one event per change.
// Events are built before any state is committed, so a transient read failure
// mid-pass leaves the diff state untouched and the next tick re-diffs cleanly
// — an event is never emitted twice and a change is never dropped because a
// sibling's payload failed to build.
func (st *watchState) poll() error {
	// The list row. A GetList failure in the store's not-found shape is the
	// list's deletion — the id was resolved against a live row moments before
	// the first poll, and a read on a local WAL database has no other
	// realistic failure worth treating as terminal (this is the same
	// strings.Contains convention the store's own tests use).
	list, listNotFound := st.readList()

	tasks, err := st.s.ListTasks(st.listID)
	if err != nil {
		if listNotFound {
			return nil // the cascade already deleted every task; nothing to read
		}
		return err // transient; the caller logs and retries next tick
	}

	// Current id set, plus each task's depth from the shared flatten, so task
	// events carry the same depth column `farol tasks --json` emits.
	current := make(map[string]store.Task, len(tasks))
	for _, t := range tasks {
		current[t.ID] = t
	}
	depth := make(map[string]int, len(tasks))
	for _, r := range apptypes.Flatten(apptypes.FromStoreTasks(tasks)) {
		depth[r.Task.ID] = r.Depth
	}

	if !st.started && st.baseline < 0 {
		// Live start: everything present now is background, not change. Record
		// it and report nothing, so the first reported event is one that
		// actually happened after the watch began. One exception: the entity
		// was resolved against a live row moments ago, so if it is already
		// gone at this first read, that deletion happened during the watch —
		// report it rather than silently watching nothing.
		_, taskPresent := current[st.taskID]
		switch {
		case st.scopeTask && (listNotFound || !taskPresent):
			st.emit(watchEvent{Event: watchEventTaskDeleted, TaskID: st.taskID, ListID: st.listID})
			if listNotFound {
				st.listGone = true
			}
		case !st.scopeTask && listNotFound:
			st.listGone = true
			st.emit(watchEvent{Event: watchEventListDeleted, ListID: st.listID})
		default:
			st.listState = &list
		}
		st.recordBaseline(tasks)
		st.started = true
		return nil
	}

	deleted, changed := st.computeDiff(current, tasks)

	events, err := st.buildEvents(listNotFound, deleted, changed, list, depth)
	if err != nil {
		return err
	}

	// Commit the diff — only now that every event built successfully.
	if !st.started {
		st.recordBaseline(tasks) // a --since replay starts from the current state
	}
	for _, id := range deleted {
		delete(st.known, id)
	}
	for _, c := range changed {
		st.known[c.t.ID] = c.t.UpdatedAt
	}
	if listNotFound {
		st.listGone = true
	} else {
		st.listState = &list
	}
	st.started = true

	st.emitAll(events)
	return nil
}

// readList reads the watched list's row, reporting whether the failure was
// the list's deletion (the not-found shape) rather than a transient error.
func (st *watchState) readList() (list store.List, notFound bool) {
	list, err := st.s.GetList(st.listID)
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return store.List{}, false
		}
		return store.List{}, true
	}
	return list, false
}

// recordBaseline records every current task's updated_at as last-seen without
// emitting. Scope-limited: a task watch records only the watched task.
func (st *watchState) recordBaseline(tasks []store.Task) {
	for _, t := range tasks {
		if st.scopeTask && t.ID != st.taskID {
			continue
		}
		st.known[t.ID] = t.UpdatedAt
	}
}

// computeDiff returns the deletions (known ids no longer present) and the
// changed tasks for this poll. The first poll with an explicit --since is a
// replay: tasks whose activity is strictly after the timestamp are reported —
// created if they were created inside the window, updated if they predate it —
// and the diff is taken against the baseline afterwards.
func (st *watchState) computeDiff(current map[string]store.Task, tasks []store.Task) (deleted []string, changed []taskChange) {
	if !st.started {
		// --since replay window.
		for _, t := range tasks {
			if st.scopeTask && t.ID != st.taskID {
				continue
			}
			if t.UpdatedAt > st.baseline {
				kind := changeUpdated
				if t.CreatedAt > st.baseline {
					kind = changeCreated
				}
				changed = append(changed, taskChange{t, kind})
			}
		}
		return nil, changed
	}

	for id := range st.known {
		if _, ok := current[id]; !ok {
			deleted = append(deleted, id)
		}
	}
	for _, t := range tasks {
		if st.scopeTask && t.ID != st.taskID {
			continue
		}
		last, seen := st.known[t.ID]
		if seen && t.UpdatedAt <= last {
			continue
		}
		kind := changeCreated
		if seen {
			kind = changeUpdated
		}
		changed = append(changed, taskChange{t, kind})
	}
	return deleted, changed
}

// buildEvents assembles every event this poll will emit, in a fixed order:
// the list's own change or deletion first, then task deletions, then task
// creations and updates in the list's preorder. Any failure — a derived-
// progress read, the one presence read — propagates so the caller retries
// with the state untouched.
func (st *watchState) buildEvents(listDeleted bool, deleted []string, changed []taskChange, list store.List, depth map[string]int) ([]watchEvent, error) {
	events := make([]watchEvent, 0, len(deleted)+len(changed)+1)

	switch {
	case listDeleted:
		// A deleted list is reported once and never diffed again — the row is
		// gone, so a list_changed against a zero-value row would be garbage.
		if !st.listGone && !st.scopeTask {
			events = append(events, watchEvent{Event: watchEventListDeleted, ListID: st.listID})
		}
	case st.started && !st.scopeTask && st.listState != nil && listRowChanged(*st.listState, list):
		lj := watchListJSONOf(list)
		events = append(events, watchEvent{Event: watchEventListChanged, List: &lj})
	}

	if len(deleted) > 0 || len(changed) > 0 {
		// One presence read per poll, shared by every event's assignee_live
		// field (docs/DESIGN.md §3), matching `farol tasks`.
		live, err := liveAgents(st.s)
		if err != nil {
			return nil, err
		}
		for _, id := range deleted {
			events = append(events, watchEvent{
				Event:  watchEventTaskDeleted,
				TaskID: id,
				ListID: st.listID,
			})
		}
		for _, c := range changed {
			row, err := st.taskRow(c.t, depth[c.t.ID], live, list.CreatedBy)
			if err != nil {
				return nil, err
			}
			name := watchEventTaskUpdated
			if c.kind == changeCreated {
				name = watchEventTaskCreated
			}
			events = append(events, watchEvent{Event: name, Task: &row})
		}
	}
	return events, nil
}

// taskRow builds the taskRowJSON payload for one changed task — the same row
// shape `farol tasks --json` emits, so a caller that can read one can read
// the other.
func (st *watchState) taskRow(t store.Task, depth int, live map[string]bool, listOwner string) (taskRowJSON, error) {
	prog, err := progressOf(st.s, t.ID)
	if err != nil {
		return taskRowJSON{}, err
	}
	return taskRowJSON{
		ID:           t.ID,
		ParentID:     t.ParentID,
		Title:        t.Title,
		Status:       string(t.Status),
		Progress:     prog,
		Depth:        depth,
		ListOwner:    listOwner,
		Assignee:     t.Assignee,
		AssignedAt:   t.AssignedAt,
		AssigneeLive: assigneeLive(live, t.Assignee),
		Priority:     string(t.Priority),
		HasNotes:     t.Notes != "",
		NotesLen:     len(t.Notes),
	}, nil
}

// listRowChanged reports whether two reads of one list row differ in any
// field a write can change. id and created_at are immutable, so they are
// deliberately not compared.
func listRowChanged(prev, cur store.List) bool {
	return prev.Name != cur.Name ||
		prev.Position != cur.Position ||
		prev.CreatedBy != cur.CreatedBy ||
		prev.Collaborative != cur.Collaborative ||
		prev.CommentsDisabled != cur.CommentsDisabled
}

// emitAll writes a poll's events to stdout in order: one JSON value per line
// in --json mode, one plain-text line per change in human mode.
func (st *watchState) emitAll(events []watchEvent) {
	for _, ev := range events {
		st.emit(ev)
	}
}

func (st *watchState) emit(ev watchEvent) {
	if !st.jsonMode {
		st.emitHuman(ev)
		return
	}
	b, err := json.Marshal(ev)
	if err != nil {
		// Every field is marshalable by construction; if one ever isn't, keep
		// the stream alive rather than dying mid-watch on an unencodable line.
		fmt.Fprintf(os.Stderr, "farol: watch: encode event: %v\n", err)
		return
	}
	fmt.Println(string(b))
}

// emitHuman prints one plain-text line per change (no ANSI escapes, per §9):
// the event, the id, and the title where the payload carries one.
func (st *watchState) emitHuman(ev watchEvent) {
	switch ev.Event {
	case watchEventTaskCreated:
		fmt.Printf("task created %s  %s\n", ev.Task.ID, ev.Task.Title)
	case watchEventTaskUpdated:
		fmt.Printf("task updated %s  %s  (%s)\n", ev.Task.ID, ev.Task.Title, ev.Task.Status)
	case watchEventTaskDeleted:
		fmt.Printf("task deleted %s\n", ev.TaskID)
	case watchEventListChanged:
		fmt.Printf("list changed %s  %s\n", ev.List.ID, ev.List.Name)
	case watchEventListDeleted:
		fmt.Printf("list deleted %s\n", ev.ListID)
	}
}

func newWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch <task-id|list-id>",
		Short: "watch a task or list and print one line per change until Ctrl+C",
		Long: `Long-poll the store and report every change to the watched entity as it
happens — the real-time change feed for agents and scripts that want updates
without running the TUI (docs/DESIGN.md §7's poll, at the command line).

A list target reports every task created, updated or deleted in the list,
plus changes to the list itself (rename, owner adoption, collaborative or
comments_disabled toggles, reorder) and the list's deletion. A task target
reports only that task's own changes — including new comments, which bump its
updated_at — and its deletion.

--json emits one JSON value per change, one per line. This is a deliberate
exception to the one-value rule in docs/DESIGN.md §9: a watch's output is a
stream, not a single payload, so the per-event shape is the contract. Human
mode prints one plain-text line per change. Ctrl+C (or SIGTERM) ends the
watch and exits 0.

With --since <unix-seconds>, the first poll replays every task whose activity
changed strictly after that timestamp (the same window farol diff --since
returns, as events), then continues live. Without it the watch starts from
the current state and reports only changes that happen after it begins.

Change detection is the store's updated_at signal (docs/DESIGN.md §9), read
per poll and diffed against the previous poll — so deletions are visible, at
the same second-granularity the rest of the app's change detection has.`,
		Args: cobra.ExactArgs(1),
		RunE: runWatch,
	}
	cmd.Flags().Int64("since", 0,
		"unix seconds; on the first poll emit every task whose activity is strictly after this, then continue live")
	cmd.Flags().Duration("interval", watchPollInterval,
		"poll interval (default 1s, the TUI's poll cadence)")
	return cmd
}

// runWatch is `farol watch`: resolve the target, then poll the store every
// interval until SIGINT/SIGTERM. A transient poll failure is logged to stderr
// and retried — a long-running watcher must survive one bad tick — while a
// startup failure (unresolvable id, store open failure) uses the normal §9
// error shape and exits 1.
func runWatch(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	sinceFlag, _ := cmd.Flags().GetInt64("since")
	interval, _ := cmd.Flags().GetDuration("interval")
	if interval <= 0 {
		interval = watchPollInterval
	}
	return runStore(cmd, func(s *store.Store) error {
		// Task ids resolve against the task table first, then the list table —
		// the same order claim/release use (docs/DESIGN.md §9).
		entityType, id, err := resolveEntity(s, args[0])
		if err != nil {
			return err
		}
		scopeTask := entityType == "task"
		listID := id
		var taskID string
		if scopeTask {
			t, err := s.GetTask(id)
			if err != nil {
				return err
			}
			taskID = id
			listID = t.ListID
		}

		// A bare start watches from now (no replay); an explicit --since —
		// even 0, which replays everything — sets the replay window.
		baseline := int64(-1)
		if cmd.Flags().Changed("since") {
			baseline = sinceFlag
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		st := newWatchState(s, jsonMode, scopeTask, listID, taskID, baseline)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := st.poll(); err != nil {
				fmt.Fprintf(os.Stderr, "farol: watch: %v\n", err)
			}
			select {
			case <-ctx.Done():
				// Ctrl+C / SIGTERM is the normal end of a watch, not an error.
				return nil
			case <-ticker.C:
			}
		}
	})
}
