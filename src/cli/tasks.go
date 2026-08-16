package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/store"
)

// showBatchCap is the maximum number of task ids a single `farol show` may
// request, matching the MCP show_task cap (50).
const showBatchCap = 50

// nextEmptyJSON is the --json shape when a list has no eligible task: an
// explicit {ok:false} rather than an error, so an agent can branch on it
// without treating an empty board as a failure (docs/DESIGN.md §9).
type nextEmptyJSON struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason"`
}

// idJSON is the success payload of the two add commands — the one value an
// agent captures from either (docs/DESIGN.md §9).
type idJSON struct {
	ID string `json:"id"`
}

// progressJSON is the derived-progress shape both front ends share for a
// task (docs/DESIGN.md §3): kind is the stored progress_kind, percent is
// meaningful only for percentage and subtasks modes that have something to
// display, and display_as_simple reports the zero-children subtasks
// fallback.
type progressJSON struct {
	Kind            string `json:"kind"`
	Percent         *int   `json:"percent"`
	DisplayAsSimple bool   `json:"display_as_simple"`
}

// progressOf runs the store's derived-progress read and maps it to the JSON
// shape — percent is null whenever the kind has nothing to display.
func progressOf(s *store.Store, id string) (progressJSON, error) {
	kind, pct, simple, err := s.DerivedProgress(id)
	if err != nil {
		return progressJSON{}, err
	}
	var p *int
	if (kind == store.ProgressPercentage || kind == store.ProgressSubtasks) && !simple {
		p = &pct
	}
	return progressJSON{Kind: string(kind), Percent: p, DisplayAsSimple: simple}, nil
}

// listTasksResult is the `farol tasks --json` envelope: the tasks array plus
// the ids of the rows the --include body budget dropped (so the caller can
// re-fetch them with `farol show`) and whether the budget was exceeded at all
// — true exactly when elided is non-empty. Mirrors the MCP list_tasks result.
type listTasksResult struct {
	Tasks          []taskRowJSON `json:"tasks"`
	Elided         []string      `json:"elided"`
	BudgetExceeded bool          `json:"budget_exceeded"`
}

// children) in JSON mode: a flat preorder array with depth, so a caller
// walks the same shape whether or not it asked for --flat (docs/DESIGN.md
// §9). It is a superset of the MCP list_tasks row: the additive fields
// (has_notes, notes_len, assigned_at, assignee_live, context_only, the
// inlined notes/comments) are MCP-compatible so the CLI can fully replace
// the server. omitempty keeps the payload legible when the fields are empty.
type taskRowJSON struct {
	ID           string        `json:"id"`
	ParentID     *string       `json:"parent_id"`
	Title        string        `json:"title"`
	Status       string        `json:"status"`
	Progress     progressJSON  `json:"progress"`
	Depth        int           `json:"depth"`
	ListOwner    string        `json:"list_owner"`
	Assignee     string        `json:"assignee"`
	AssignedAt   *int64        `json:"assigned_at,omitempty"`
	AssigneeLive bool          `json:"assignee_live"`
	Priority     string        `json:"priority"`
	HasNotes     bool          `json:"has_notes"`
	NotesLen     int           `json:"notes_len"`
	Notes        string        `json:"notes,omitempty"`
	ContextOnly  bool          `json:"context_only,omitempty"`
	Comments     []commentJSON `json:"comments,omitempty"`
}

// taskView is one flattened row with its derived progress computed once, so
// the human tree and the JSON payload render the same numbers without each
// re-querying DerivedProgress.
type taskView struct {
	row  apptypes.Row
	prog progressJSON
}

func newTasksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tasks <list-id>",
		Short: "list a list's tasks as a tree",
		Args:  cobra.ExactArgs(1),
		RunE:  runTasks,
	}
	cmd.Flags().String("status", "all",
		"filter by root task status: pending, in_progress, complete, or all")
	cmd.Flags().Bool("flat", false,
		"print id, status, and title per line instead of the indented tree")
	cmd.Flags().Int64("since", 0,
		"unix seconds; return only tasks whose activity changed strictly after this (widens default status to all)")
	cmd.Flags().StringSlice("include", nil,
		"inline extra per-row fields: 'notes' and/or 'comments'; a byte budget caps the response and over-budget rows are named in 'elided'")
	return cmd
}

// taskCommands are the task mutators — root-level subcommands per
// docs/DESIGN.md §9, grouped in this file.
func taskCommands() []*cobra.Command {
	addCmd := &cobra.Command{
		Use:   "add <list-id> <title>",
		Short: "add a task; prints its id",
		Args:  cobra.ExactArgs(2),
		RunE:  runAdd,
	}
	addCmd.Flags().String("parent", "", "parent task id (prefix accepted)")
	addCmd.Flags().String("notes", "", "notes for the new task")
	addCmd.Flags().Bool("force", false, "allow adding to a list owned by another agent or by nobody")

	showCmd := &cobra.Command{
		Use:   "show <task-id> [<task-id> ...]",
		Short: "show one or more tasks (up to 50): title, notes, status, progress, children",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("requires at least one task id")
			}
			if len(args) > showBatchCap {
				return fmt.Errorf("show accepts at most %d task ids, got %d", showBatchCap, len(args))
			}
			return nil
		},
		RunE: runShow,
	}

	renameCmd := &cobra.Command{
		Use:   "rename <task-id> <title>",
		Short: "rename a task",
		Args:  cobra.ExactArgs(2),
		RunE:  runRename,
	}
	renameCmd.Flags().Bool("force", false, "allow renaming a task on a list owned by another agent or by nobody")

	notesCmd := &cobra.Command{
		Use:   "notes <task-id> <text>",
		Short: "replace a task's notes (whole text, not append)",
		Args:  cobra.ExactArgs(2),
		RunE:  runNotes,
	}
	notesCmd.Flags().Bool("force", false, "allow changing notes on a list owned by another agent or by nobody")

	reopenCmd := &cobra.Command{
		Use:   "reopen <task-id>",
		Short: "mark pending (does not cascade)",
		Args:  cobra.ExactArgs(1),
		RunE:  runReopen,
	}

	toggleCmd := &cobra.Command{
		Use:   "toggle <task-id>",
		Short: "complete <-> reopen, whichever applies",
		Args:  cobra.ExactArgs(1),
		RunE:  runToggle,
	}

	progressCmd := &cobra.Command{
		Use:   "progress <task-id>",
		Short: "set progress: simple, percentage, or subtasks",
		Args:  cobra.ExactArgs(1),
		RunE:  runProgress,
	}
	progressCmd.Flags().String("mode", "", "simple, percentage, or subtasks")
	progressCmd.MarkFlagRequired("mode")
	progressCmd.Flags().Int("percent", 0, "percent 0-100 (required for --mode percentage)")

	rmCmd := &cobra.Command{
		Use:   "rm <task-id>",
		Short: "delete a task and its descendants",
		Args:  cobra.ExactArgs(1),
		RunE:  runRm,
	}
	rmCmd.Flags().Bool("force", false, "delete without confirmation")

	mvCmd := &cobra.Command{
		Use:   "mv <task-id>",
		Short: "re-parent a task under another task, or to the list root",
		Args:  cobra.ExactArgs(1),
		RunE:  runMv,
	}
	mvCmd.Flags().String("parent", "",
		"new parent task id (prefix accepted); an empty value moves the task to the list root")
	mvCmd.Flags().Bool("force", false, "allow re-parenting a task on a list owned by another agent or by nobody")

	commentCmd := &cobra.Command{
		Use:   "comment <task-id> <note>",
		Short: "add a comment to a task; prints its id",
		Args:  cobra.ExactArgs(2),
		RunE:  runComment,
	}
	commentRmCmd := &cobra.Command{
		Use:   "rm <comment-id>",
		Short: "delete a comment",
		Args:  cobra.ExactArgs(1),
		RunE:  runCommentRm,
	}
	commentRmCmd.Flags().Bool("force", false, "delete without confirmation")
	commentCmd.AddCommand(commentRmCmd)

	assignCmd := &cobra.Command{
		Use:   "assign <task-id>",
		Short: "assign the task to the current agent; --force takes it from another",
		Args:  cobra.ExactArgs(1),
		RunE:  runAssign,
	}
	assignCmd.Flags().Bool("force", false, "take the task from its current holder")

	unassignCmd := &cobra.Command{
		Use:   "unassign <task-id>",
		Short: "release the current agent's assignment, or every task with --list",
		Args: func(cmd *cobra.Command, args []string) error {
			listID, _ := cmd.Flags().GetString("list")
			if listID != "" {
				if len(args) != 0 {
					return fmt.Errorf("--list takes no task-id argument")
				}
				return nil
			}
			if len(args) != 1 {
				return fmt.Errorf("requires a task id or --list <list-id>")
			}
			return nil
		},
		RunE: runUnassign,
	}
	unassignCmd.Flags().String("list", "", "release the assignment on every task in this list (prefix accepted)")

	diffCmd := &cobra.Command{
		Use:   "diff <list-id> [--since <unix-seconds>]",
		Short: "get tasks added or changed since a timestamp",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("requires a list id")
			}
			if len(args) > 2 {
				return fmt.Errorf("too many arguments")
			}
			return nil
		},
		RunE: runDiff,
	}
	diffCmd.Flags().Int64("since", 0, "unix seconds; return only tasks whose activity changed strictly after this")

	priorityCmd := &cobra.Command{
		Use:   "priority <task-id>",
		Short: "set a task's priority: none, low, medium, or high",
		Args:  cobra.ExactArgs(1),
		RunE:  runPriority,
	}
	priorityCmd.Flags().String("level", "", "none, low, medium, or high (required)")
	priorityCmd.Flags().Bool("force", false, "allow re-prioritising a task on a list owned by another agent or by nobody")

	nextCmd := &cobra.Command{
		Use:   "next <list-id>",
		Short: "grab and show the top eligible task (highest priority, then tree order)",
		Args:  cobra.ExactArgs(1),
		RunE:  runNext,
	}

	return []*cobra.Command{addCmd, showCmd, renameCmd, notesCmd,
		reopenCmd, toggleCmd, progressCmd, rmCmd, mvCmd, commentCmd,
		assignCmd, unassignCmd, priorityCmd, nextCmd, diffCmd}
}

// runNext grabs the top eligible task in a list and shows it — the CLI
// equivalent of the MCP next_task tool. Eligibility (not complete, unassigned,
// no cross-agent subtree reservation) and the priority-then-preorder ordering
// come from store.NextAssignable, which atomically assigns the pick to this
// agent and returns the row. An exhausted list is a normal state, not an
// error: it prints nothing in human mode and {ok:false,reason:...} in --json.
// The grab is a write, so it claims presence under FAROL_AGENT (best-effort —
// a conflicting live claim is ignored, matching the prior autoClaim), keeping
// the TUI spinner live on the CLI front end.
func runNext(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		listID, err := s.ResolveID("list", args[0])
		if err != nil {
			return err
		}
		t, err := s.NextAssignable(listID, agentIdentity())
		if err != nil {
			if errors.Is(err, store.ErrNoAssignable) {
				printResult(jsonMode, func() {}, nextEmptyJSON{OK: false, Reason: "no eligible task in this list"})
				return nil
			}
			return err
		}
		// Best-effort presence claim: a conflicting claim from another agent
		// is not a failure here (the assignment already succeeded).
		if _, cerr := s.ClaimWork("task", t.ID, agentIdentity(), store.ActivityWorking); cerr != nil && !errors.Is(cerr, store.ErrActivityConflict) {
			return cerr
		}
		v, err := buildShowJSON(s, t.ID)
		if err != nil {
			return err
		}
		printResult(jsonMode, func() {
			fmt.Fprintf(os.Stdout, "Title: %s\n", v.t.Title)
			fmt.Fprintf(os.Stdout, "ID: %s\n", t.ID)
			fmt.Fprintf(os.Stdout, "List: %s\n", t.ListID)
			fmt.Fprintf(os.Stdout, "Status: %s\n", v.t.Status)
			fmt.Fprintf(os.Stdout, "Progress: %s\n", progressHuman(v.payload.Progress))
			fmt.Fprintf(os.Stdout, "Notes:\n")
			for _, line := range strings.Split(v.t.Notes, "\n") {
				fmt.Fprintf(os.Stdout, "  %s\n", line)
			}
			if len(v.childViews) > 0 {
				fmt.Fprintf(os.Stdout, "Children (%d):\n", len(v.childViews))
				for _, cv := range v.childViews {
					renderRow(cv)
				}
			}
			if len(v.comments) > 0 {
				fmt.Fprintf(os.Stdout, "Comments (%d):\n", len(v.comments))
				for _, c := range v.comments {
					fmt.Fprintf(os.Stdout, "  - %s (%s): %s\n", c.Author, formatTime(c.CreatedAt), c.Note)
				}
			}
			if len(v.attachments) > 0 {
				fmt.Fprintf(os.Stdout, "Attachments (%d):\n", len(v.attachments))
				for _, a := range v.attachments {
					fmt.Fprintf(os.Stdout, "  - %s: %s\n", a.ID, a.Path)
				}
			}
		}, v.payload)
		return nil
	})
}

func runDiff(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	sinceFlag, _ := cmd.Flags().GetInt64("since")
	return runStore(cmd, func(s *store.Store) error {
		// Determine the timestamp: if a second positional argument is provided, use it; otherwise, use the flag.
		var since int64
		if len(args) == 2 {
			t, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid timestamp %q: %w", args[1], err)
			}
			since = t
		} else {
			since = sinceFlag
		}
		// Resolve the list ID
		listID, err := s.ResolveID("list", args[0])
		if err != nil {
			return err
		}
		// Get the tasks that have been added or changed since the timestamp
		tasks, err := s.TasksAddedOrChangedSince(listID, since)
		if err != nil {
			return err
		}
		// Convert to apptypes.Rows and flatten for tree rendering
		rows := apptypes.FromStoreTasks(tasks)
		flattened := apptypes.Flatten(rows)
		views, err := viewsOf(s, flattened)
		if err != nil {
			return err
		}
		// Build the payload for JSON mode: an array of showJSON. It is
		// initialized empty (not nil) so "no changes" marshals to [] rather
		// than null, matching every other read command (docs/DESIGN.md §9).
		payload := []showJSON{}
		for _, t := range tasks {
			v, err := buildShowJSON(s, t.ID)
			if err != nil {
				return err
			}
			payload = append(payload, v.payload)
		}
		// Human mode: print the tree (without section headers)
		printResult(jsonMode, func() {
			if len(views) == 0 {
				return // print nothing in human mode
			}
			for _, v := range views {
				renderRow(v)
			}
		}, payload)
		return nil
	})
}

func validStatusFilter(s string) bool {
	switch s {
	case "pending", "in_progress", "complete", "all":
		return true
	}
	return false
}

// sectionRows flattens a list's tasks and splits the rows by each tree's
// root status (docs/DESIGN.md §6: a root's own status decides its section,
// so a pending root's complete children stay in Pending), applying the
// --status filter at the root — a root's whole subtree is included or not as
// a unit. The store rows are converted to apptypes at the boundary, so the
// shared Flatten (and its Row type) never sees store.Task.
func sectionRows(tasks []store.Task, status string) (pending, complete []apptypes.Row) {
	converted := apptypes.FromStoreTasks(tasks)
	rows := apptypes.Flatten(converted)
	byID := make(map[string]apptypes.Task, len(converted))
	for _, t := range converted {
		byID[t.ID] = t
	}
	for _, r := range rows {
		root := r.Task
		for root.ParentID != nil {
			root = byID[*root.ParentID]
		}
		switch {
		case root.Status == apptypes.StatusComplete:
			if status == "all" || status == "complete" {
				complete = append(complete, r)
			}
		case status == "all" ||
			(status == "pending" && root.Status == apptypes.StatusPending) ||
			(status == "in_progress" && root.Status == apptypes.StatusInProgress):
			pending = append(pending, r)
		}
	}
	return pending, complete
}

func viewsOf(s *store.Store, rows []apptypes.Row) ([]taskView, error) {
	views := make([]taskView, 0, len(rows))
	for _, r := range rows {
		p, err := progressOf(s, r.Task.ID)
		if err != nil {
			return nil, err
		}
		views = append(views, taskView{row: r, prog: p})
	}
	return views, nil
}

func runTasks(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	status, _ := cmd.Flags().GetString("status")
	flat, _ := cmd.Flags().GetBool("flat")
	since, _ := cmd.Flags().GetInt64("since")
	include, _ := cmd.Flags().GetStringSlice("include")
	if !validStatusFilter(status) {
		err := fmt.Errorf("invalid --status %q: want pending, in_progress, complete, or all", status)
		printError(jsonMode, err)
		return domainError(err)
	}
	includeNotes, includeComments, err := parseInclude(include)
	if err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("list", args[0])
		if err != nil {
			return err
		}
		l, err := s.GetList(id)
		if err != nil {
			return err
		}
		tasks, err := s.ListTasks(id)
		if err != nil {
			return err
		}
		// One presence read per request, shared by every row's
		// assignee_live field (docs/DESIGN.md §3).
		live, err := liveAgents(s)
		if err != nil {
			return err
		}
		pending, complete := sectionRows(tasks, status)
		pendingViews, err := viewsOf(s, pending)
		if err != nil {
			return err
		}
		completeViews, err := viewsOf(s, complete)
		if err != nil {
			return err
		}

		payload := make([]taskRowJSON, 0, len(pendingViews)+len(completeViews))
		for _, v := range append(pendingViews, completeViews...) {
			t := v.row.Task
			payload = append(payload, taskRowJSON{
				ID:           t.ID,
				ParentID:     t.ParentID,
				Title:        t.Title,
				Status:       string(t.Status),
				Progress:     v.prog,
				Depth:        v.row.Depth,
				ListOwner:    l.CreatedBy,
				Assignee:     t.Assignee,
				AssignedAt:   t.AssignedAt,
				AssigneeLive: assigneeLive(live, t.Assignee),
				Priority:     string(t.Priority),
				HasNotes:     t.Notes != "",
				NotesLen:     len(t.Notes),
			})
		}

		// The folded list_changes: keep only rows whose activity changed
		// strictly after `since`. An explicit --since widens the default
		// status to all, matching the MCP list_tasks contract (a change
		// feed must not be blind to completions).
		if since > 0 {
			changed, err := s.TasksChangedSince(id, since)
			if err != nil {
				return err
			}
			changedSet := make(map[string]bool, len(changed))
			for _, t := range changed {
				changedSet[t.ID] = true
			}
			kept := payload[:0]
			for _, r := range payload {
				if changedSet[r.ID] {
					kept = append(kept, r)
				}
			}
			payload = kept
		}

		var elided []string
		var budgetExceeded bool
		if includeNotes || includeComments {
			elided, budgetExceeded, err = inlineBodyBudget(s, id, payload, tasks, includeNotes, includeComments)
			if err != nil {
				return err
			}
		}

		humanViews := append(pendingViews, completeViews...)
		printResult(jsonMode, func() {
			if len(humanViews) == 0 {
				return // an empty result prints nothing in human mode (§9)
			}
			if flat {
				renderFlat(pendingViews)
				renderFlat(completeViews)
				return
			}
			renderSection("Pending", pendingViews)
			renderSection("Complete", completeViews)
		}, listTasksResult{Tasks: payload, Elided: elided, BudgetExceeded: budgetExceeded})
		return nil
	})
}

// parseInclude validates the --include values and maps them to the two
// boolean flags inlineBodyBudget understands. Unknown values are a hard error
// (§9), not a silent no-op.
func parseInclude(values []string) (notes, comments bool, err error) {
	for _, v := range values {
		switch v {
		case "notes":
			notes = true
		case "comments":
			comments = true
		default:
			return false, false, fmt.Errorf("unknown --include %q: supported values are notes, comments", v)
		}
	}
	return notes, comments, nil
}

// renderSection prints a §6 section header and its rows — the header shows
// only when the section has rows, and the count is the row count, so the
// two numbers a reader sees always agree.
func renderSection(name string, views []taskView) {
	if len(views) == 0 {
		return
	}
	fmt.Fprintf(os.Stdout, "%s (%d)\n", name, len(views))
	for _, v := range views {
		renderRow(v)
	}
}

// renderRow prints one tree row per docs/DESIGN.md §12's fixed layout:
// {2 spaces × depth}{expand-glyph-or-blank}{space}{checkbox}{space}{title}
// {progress suffix if any}. The CLI's tree is always fully expanded, so a
// non-leaf row draws the expanded glyph ▾.
func renderRow(v taskView) {
	t := v.row.Task
	fmt.Fprint(os.Stdout, strings.Repeat("  ", v.row.Depth))
	if v.row.HasChildren {
		fmt.Fprint(os.Stdout, "▾")
	} else {
		fmt.Fprint(os.Stdout, " ")
	}
	fmt.Fprint(os.Stdout, " ")
	switch t.Status {
	case apptypes.StatusComplete:
		fmt.Fprint(os.Stdout, "[x]")
	case apptypes.StatusInProgress:
		fmt.Fprint(os.Stdout, "[~]")
	default:
		fmt.Fprint(os.Stdout, "[ ]")
	}
	fmt.Fprint(os.Stdout, " ", t.Title)
	if !v.prog.DisplayAsSimple {
		fmt.Fprintf(os.Stdout, " (%d%%)", *v.prog.Percent)
	}
	fmt.Fprintln(os.Stdout)
}

// renderFlat prints id, status, and title per line — the script-greppable
// view; the tree structure is recoverable via --json's depth column.
func renderFlat(views []taskView) {
	for _, v := range views {
		fmt.Fprintf(os.Stdout, "%s\t%s\t%s\n", v.row.Task.ID, v.row.Task.Status, v.row.Task.Title)
	}
}

// runAdd adds a task to a list. The list it targets is a structural write
// (docs/DESIGN.md §9), so a list owned by another agent (or an untagged,
// human-managed list) is refused unless --force — the same ownership guard
// the retired MCP server applied to add_task.
func runAdd(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	force, _ := cmd.Flags().GetBool("force")
	parentPrefix, _ := cmd.Flags().GetString("parent")
	notes, _ := cmd.Flags().GetString("notes")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("list", args[0])
		if err != nil {
			return err
		}
		if !force {
			if err := ownershipError(s, id); err != nil {
				return err
			}
		}
		var parent *string
		if parentPrefix != "" {
			resolved, err := s.ResolveID("task", parentPrefix)
			if err != nil {
				return err
			}
			parent = &resolved
		}
		taskID, err := s.CreateTask(id, args[1], parent, notes)
		if err != nil {
			return err
		}
		autoClaimTask(s, taskID)
		printResult(jsonMode, func() { fmt.Println(taskID) }, idJSON{taskID})
		return nil
	})
}

// commentJSON is one comment on a task, mirrored between the CLI's
// `farol show --json` and the MCP's show_task/comments payload (docs/DESIGN.md
// §9: MCP is a superset of CLI --json, additive fields only).
type commentJSON struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"created_at"`
}

// attachmentJSON is one attachment on a task, included in `farol show --json`
// and the MCP show_task payload.
type attachmentJSON struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	CreatedAt int64  `json:"created_at"`
}

// showJSON is `farol show`'s payload: the task's own fields plus its
// descendants as the same flat, depth-annotated rows `tasks` emits, so a
// caller that can read one can read the other.
type showJSON struct {
	ID          string           `json:"id"`
	ListID      string           `json:"list_id"`
	ListOwner   string           `json:"list_owner"`
	Title       string           `json:"title"`
	Notes       string           `json:"notes"`
	Status      string           `json:"status"`
	Progress    progressJSON     `json:"progress"`
	CreatedAt   int64            `json:"created_at"`
	UpdatedAt   int64            `json:"updated_at"`
	CompletedAt *int64           `json:"completed_at"`
	Assignee    string           `json:"assignee"`
	AssignedAt  *int64           `json:"assigned_at"`
	Priority    string           `json:"priority"`
	Children    []taskRowJSON    `json:"children"`
	Comments    []commentJSON    `json:"comments"`
	Attachments []attachmentJSON `json:"attachments"`
}

// showView is everything runShow (and farol next) need to render one task:
// the full JSON payload plus the human-mode data the printResult closure
// reads. Extracted so farol next can grab-then-show with the identical shape.
type showView struct {
	payload     showJSON
	t           store.Task
	childViews  []taskView
	comments    []store.Comment
	attachments []store.Attachment
}

// buildShowJSON reads one task and its subtree/comments/attachments and
// returns a showView. ids may be a prefix; an unresolvable id returns the
// ResolveID error untouched.
func buildShowJSON(s *store.Store, id string) (showView, error) {
	t, err := s.GetTask(id)
	if err != nil {
		return showView{}, err
	}
	prog, err := progressOf(s, id)
	if err != nil {
		return showView{}, err
	}
	all, err := s.ListTasks(t.ListID)
	if err != nil {
		return showView{}, err
	}
	l, err := s.GetList(t.ListID)
	if err != nil {
		return showView{}, err
	}
	childViews, err := viewsOf(s, apptypes.DescendantsOf(apptypes.FromStoreTasks(all), id))
	if err != nil {
		return showView{}, err
	}

	children := make([]taskRowJSON, 0, len(childViews))
	for _, v := range childViews {
		children = append(children, taskRowJSON{
			ID:        v.row.Task.ID,
			ParentID:  v.row.Task.ParentID,
			Title:     v.row.Task.Title,
			Status:    string(v.row.Task.Status),
			Progress:  v.prog,
			Depth:     v.row.Depth,
			ListOwner: l.CreatedBy,
			Assignee:  v.row.Task.Assignee,
			Priority:  string(v.row.Task.Priority),
		})
	}

	comments, err := s.ListComments(id)
	if err != nil {
		return showView{}, err
	}
	commentJSONs := make([]commentJSON, 0, len(comments))
	for _, c := range comments {
		commentJSONs = append(commentJSONs, commentJSON{
			ID:        c.ID,
			Author:    c.Author,
			Note:      c.Note,
			CreatedAt: c.CreatedAt,
		})
	}

	attachments, err := s.ListAttachments(id)
	if err != nil {
		return showView{}, err
	}
	attachmentJSONs := make([]attachmentJSON, 0, len(attachments))
	for _, a := range attachments {
		attachmentJSONs = append(attachmentJSONs, attachmentJSON{
			ID:        a.ID,
			Path:      a.Path,
			CreatedAt: a.CreatedAt,
		})
	}

	v := showView{
		t:           t,
		childViews:  childViews,
		comments:    comments,
		attachments: attachments,
		payload: showJSON{
			ID:          t.ID,
			ListID:      t.ListID,
			ListOwner:   l.CreatedBy,
			Title:       t.Title,
			Notes:       t.Notes,
			Status:      string(t.Status),
			Progress:    prog,
			CreatedAt:   t.CreatedAt,
			UpdatedAt:   t.UpdatedAt,
			CompletedAt: t.CompletedAt,
			Assignee:    t.Assignee,
			AssignedAt:  t.AssignedAt,
			Priority:    string(t.Priority),
			Children:    children,
			Comments:    commentJSONs,
			Attachments: attachmentJSONs,
		},
	}
	return v, nil
}

// showErrorRow is a per-id error in the `farol show <id>...` batch JSON:
// an unresolvable id reports {id, error} rather than failing the whole
// call, matching the MCP show_task contract (one bad id does not sink the
// rest).
type showErrorRow struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

func runShow(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		// Build the result for every requested id. Unresolvable ids become
		// showErrorRow entries in --json; in human mode they print an error
		// line and are otherwise skipped.
		human := strings.Builder{}
		payload := make([]any, 0, len(args))
		anyErr := false
		for _, raw := range args {
			id, err := s.ResolveID("task", raw)
			if err != nil {
				anyErr = true
				// A single requested id that cannot be resolved is the
				// classic §9 failure: runStore prints one JSON error on
				// stdout and exits 1. Only multi-id requests embed the
				// error per row and succeed (matching MCP show_task's
				// mixed-array 200).
				if len(args) == 1 {
					return err
				}
				if jsonMode {
					payload = append(payload, showErrorRow{ID: raw, Error: err.Error()})
				} else {
					fmt.Fprintf(os.Stderr, "farol: %s\n", err)
				}
				continue
			}
			v, err := buildShowJSON(s, id)
			if err != nil {
				anyErr = true
				if len(args) == 1 {
					return err
				}
				if jsonMode {
					payload = append(payload, showErrorRow{ID: raw, Error: err.Error()})
				} else {
					fmt.Fprintf(os.Stderr, "farol: %s\n", err)
				}
				continue
			}
			if !jsonMode {
				renderShowHuman(&human, v)
			}
			payload = append(payload, v.payload)
		}
		if jsonMode {
			// Exactly one JSON value on stdout (§9): an array of showJSON
			// and showErrorRow. A single id still yields a one-element
			// array, matching the MCP batch contract. Per-id failures live
			// in the array (showErrorRow), so the call itself succeeds —
			// like MCP show_task, which returns the mixed array as a 200.
			b, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		}
		fmt.Print(human.String())
		if anyErr {
			// Human mode sends errors to stderr (stdout stays empty, §9)
			// and exits 1.
			return domainError(fmt.Errorf("one or more task ids could not be resolved"))
		}
		return nil
	})
}

// renderShowHuman writes one task's human-readable block to w.
func renderShowHuman(w *strings.Builder, v showView) {
	t := v.t
	fmt.Fprintf(w, "Title: %s\n", t.Title)
	fmt.Fprintf(w, "ID: %s\n", t.ID)
	fmt.Fprintf(w, "List: %s\n", t.ListID)
	fmt.Fprintf(w, "Status: %s\n", t.Status)
	fmt.Fprintf(w, "Progress: %s\n", progressHuman(v.payload.Progress))
	fmt.Fprintf(w, "Notes:\n")
	for _, line := range strings.Split(t.Notes, "\n") {
		fmt.Fprintf(w, "  %s\n", line)
	}
	if len(v.childViews) > 0 {
		fmt.Fprintf(w, "Children (%d):\n", len(v.childViews))
		for _, cv := range v.childViews {
			renderRowTo(w, cv)
		}
	}
	if len(v.comments) > 0 {
		fmt.Fprintf(w, "Comments (%d):\n", len(v.comments))
		for _, c := range v.comments {
			fmt.Fprintf(w, "  - %s (%s): %s\n", c.Author, formatTime(c.CreatedAt), c.Note)
		}
	}
	if len(v.attachments) > 0 {
		fmt.Fprintf(w, "Attachments (%d):\n", len(v.attachments))
		for _, a := range v.attachments {
			fmt.Fprintf(w, "  - %s: %s\n", a.ID, a.Path)
		}
	}
	fmt.Fprintln(w)
}

// renderRowTo writes one tree row to w (the batch show path prints to a
// buffer rather than stdout directly).
func renderRowTo(w *strings.Builder, v taskView) {
	t := v.row.Task
	fmt.Fprint(w, strings.Repeat("  ", v.row.Depth))
	if v.row.HasChildren {
		fmt.Fprint(w, "▾")
	} else {
		fmt.Fprint(w, " ")
	}
	fmt.Fprint(w, " ")
	switch t.Status {
	case apptypes.StatusComplete:
		fmt.Fprint(w, "[x]")
	case apptypes.StatusInProgress:
		fmt.Fprint(w, "[~]")
	default:
		fmt.Fprint(w, "[ ]")
	}
	fmt.Fprint(w, " ", t.Title)
	if !v.prog.DisplayAsSimple {
		fmt.Fprintf(w, " (%d%%)", *v.prog.Percent)
	}
	fmt.Fprintln(w)
}

// progressHuman renders the show line: a subtasks task with no children
// displays and behaves as simple (docs/DESIGN.md §3), which the suffix
// spells out rather than printing a misleading 0%.
func progressHuman(p progressJSON) string {
	switch p.Kind {
	case "percentage":
		return fmt.Sprintf("percentage (%d%%)", *p.Percent)
	case "subtasks":
		if p.DisplayAsSimple {
			return "subtasks (simple)"
		}
		return fmt.Sprintf("subtasks (%d%%)", *p.Percent)
	default:
		return p.Kind
	}
}

// formatTime renders a Unix-second timestamp for human output.
func formatTime(unix int64) string {
	return time.Unix(unix, 0).Format("2006-01-02 15:04:05")
}

// osUser returns the current OS username for human-authored writes
// (comments, task creation). Falls back to $USER/$LOGNAME when
// os/user.Current fails — some minimal containers lack /etc/passwd.
func osUser() string {
	// FAROL_AGENT env var takes precedence: when an agent drives the CLI,
	// comments should be attributed to the agent, not the OS user.
	if agent := os.Getenv("FAROL_AGENT"); agent != "" {
		return agent
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return os.Getenv("LOGNAME")
}

func runComment(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		cid, err := s.AddComment(id, osUser(), args[1])
		if err != nil {
			return err
		}
		autoClaimTask(s, id)
		printResult(jsonMode, func() { fmt.Println(cid) }, idJSON{cid})
		return nil
	})
}

// runCommentRm mirrors runRm: the store is unenforced (docs/DESIGN.md §9),
// so the CLI may delete any comment, and --force is the only confirmation
// since there is no modal to route through here.
func runCommentRm(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	force, _ := cmd.Flags().GetBool("force")
	if !force {
		err := fmt.Errorf("refusing to delete comment %q without --force", args[0])
		printError(jsonMode, err)
		return domainError(err)
	}
	return runStore(cmd, func(s *store.Store) error {
		if err := s.DeleteComment(args[0]); err != nil {
			return err
		}
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}

// runRename renames a task. Re-titleing is a structural edit (docs/DESIGN.md
// §9), so it is refused on a list the current agent does not own unless
// --force — the same re-ranking/rename guard the retired MCP server applied
// to edit_task.
func runRename(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	force, _ := cmd.Flags().GetBool("force")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		if !force {
			if err := taskOwnershipError(s, id); err != nil {
				return err
			}
		}
		if err := s.RenameTask(id, args[1]); err != nil {
			return err
		}
		autoClaimTask(s, id)
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}

// runNotes replaces a task's notes. The retired MCP server treated
// set-notes as a structural edit, so it is refused on a foreign-owned list
// unless --force (docs/DESIGN.md §9).
func runNotes(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	force, _ := cmd.Flags().GetBool("force")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		if !force {
			if err := taskOwnershipError(s, id); err != nil {
				return err
			}
		}
		if err := s.SetNotes(id, args[1]); err != nil {
			return err
		}
		autoClaimTask(s, id)
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}

func runComplete(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		if err := s.Complete(id); err != nil {
			return err
		}
		autoClaimTask(s, id)
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}

func runReopen(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		if err := s.Reopen(id); err != nil {
			return err
		}
		autoClaimTask(s, id)
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}

func runToggle(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		if err := s.Toggle(id); err != nil {
			return err
		}
		autoClaimTask(s, id)
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}

// runProgress validates nothing beyond what Cobra already did for flag
// syntax: the mode/percent combination rules are store.SetProgress's domain
// (docs/DESIGN.md §3 — the one place every writer's validation lives), so a
// bad combination surfaces as a domain error with store's message, exit 1,
// not as a second hand-rolled check in the CLI.
func runProgress(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		mode, _ := cmd.Flags().GetString("mode")
		var percent *int
		if cmd.Flags().Changed("percent") {
			p, _ := cmd.Flags().GetInt("percent")
			percent = &p
		}
		if err := s.SetProgress(id, store.ProgressKind(mode), percent); err != nil {
			return err
		}
		autoClaimTask(s, id)
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}

// runMv wires `farol mv` to store.Reparent. The --parent flag carries the
// new parent's id (prefix accepted); an empty --parent — the flag's default,
// so omitting it entirely — is how a caller asks to move a task to the list
// root, recorded in docs/DESIGN.md §9. Re-parenting is a structural edit, so
// it is refused on a list the current agent does not own unless --force. The
// guard runs on the task's own list (the list it would move within); a
// cross-list parent would still be rejected by store.Reparent's own check,
// but ownership is reported first, matching the server's edit_task ordering.
func runMv(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	force, _ := cmd.Flags().GetBool("force")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		if !force {
			if err := taskOwnershipError(s, id); err != nil {
				return err
			}
		}
		parentPrefix, _ := cmd.Flags().GetString("parent")
		var parent *string
		if parentPrefix != "" {
			resolved, err := s.ResolveID("task", parentPrefix)
			if err != nil {
				return err
			}
			parent = &resolved
		}
		if err := s.Reparent(id, parent); err != nil {
			return err
		}
		autoClaimTask(s, id)
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}

// agentIdentity is the agent tag this CLI invocation assigns under:
// FAROL_AGENT, defaulting to "agent" — the same env var and default the MCP
// server reads (docs/DESIGN.md §9), so a shell that exports FAROL_AGENT
// owns tasks under the same tag over either surface.
func agentIdentity() string {
	if id := os.Getenv("FAROL_AGENT"); id != "" {
		return id
	}
	return "agent"
}

// assignResultJSON is the success shape of assign and single-task unassign:
// it echoes the assignee the task now carries ("" after an unassign), so a
// caller never needs a follow-up show to confirm what landed (docs/DESIGN.md
// §9).
type assignResultJSON struct {
	OK       bool   `json:"ok"`
	Assignee string `json:"assignee"`
}

// releasedJSON is `unassign --list`'s success shape: the count of tasks
// whose assignment was cleared — 0 when the list held none, which is not an
// error (docs/DESIGN.md §9).
type releasedJSON struct {
	OK       bool `json:"ok"`
	Released int  `json:"released"`
}

// priorityResultJSON is `priority`'s success shape: it echoes the level
// that landed (docs/DESIGN.md §9).
type priorityResultJSON struct {
	OK       bool   `json:"ok"`
	Priority string `json:"priority"`
}

func runAssign(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	force, _ := cmd.Flags().GetBool("force")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		agent := agentIdentity()
		if err := s.AssignTask(id, agent, force); err != nil {
			return err
		}
		autoClaimTask(s, id)
		printResult(jsonMode, func() {}, assignResultJSON{OK: true, Assignee: agent})
		return nil
	})
}

func runUnassign(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	listPrefix, _ := cmd.Flags().GetString("list")
	return runStore(cmd, func(s *store.Store) error {
		if listPrefix != "" {
			listID, err := s.ResolveID("list", listPrefix)
			if err != nil {
				return err
			}
			n, err := s.UnassignList(listID)
			if err != nil {
				return err
			}
			printResult(jsonMode, func() {}, releasedJSON{OK: true, Released: n})
			return nil
		}
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		if err := s.UnassignTask(id, agentIdentity(), false); err != nil {
			return err
		}
		autoClaimTask(s, id)
		printResult(jsonMode, func() {}, assignResultJSON{OK: true, Assignee: ""})
		return nil
	})
}

// runPriority rejects an empty --level with the §9 error shape rather than
// defaulting it to none: store.SetPriority refuses the zero value, so an
// omitted flag must surface as a failure, not a silent re-prioritisation
// The level's value itself is store.SetPriority's validation, like
// runProgress's mode. Re-prioritising is a structural steer about what gets
// picked up next, so it is refused on a list the current agent does not own
// unless --force (docs/DESIGN.md §9).
func runPriority(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	force, _ := cmd.Flags().GetBool("force")
	level, _ := cmd.Flags().GetString("level")
	if level == "" {
		err := fmt.Errorf("--level is required: none, low, medium, or high")
		printError(jsonMode, err)
		return domainError(err)
	}
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		if !force {
			if err := taskOwnershipError(s, id); err != nil {
				return err
			}
		}
		if err := s.SetPriority(id, store.Priority(level)); err != nil {
			return err
		}
		autoClaimTask(s, id)
		printResult(jsonMode, func() {}, priorityResultJSON{OK: true, Priority: level})
		return nil
	})
}

// runRm mirrors runListsRm: no store call at all without --force — the flag
// is the CLI's confirmation (docs/DESIGN.md §9). Deleting a task is a
// structural write, so it is also refused on a list the current agent does
// not own unless --force — a single --force covers both the confirmation and
// the ownership override, matching the server's delete_task.
func runRm(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	force, _ := cmd.Flags().GetBool("force")
	if !force {
		err := fmt.Errorf("refusing to delete task %q without --force", args[0])
		printError(jsonMode, err)
		return domainError(err)
	}
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		if !force {
			if err := taskOwnershipError(s, id); err != nil {
				return err
			}
		}
		// Claim presence before the delete: ClaimWork validates that the
		// entity still exists, so we light the spinner while the row is
		// present (matches MCP, which claims during the write). DeleteTask
		// then removes any AgentActivity rows for the task by design — an
		// orphaned spinner on a vanished task is worse than none — so this
		// claim is best-effort and leaves nothing behind. That is the
		// correct, TUI-safe behaviour; it is intentionally not asserted as a
		// surviving claim.
		autoClaimTask(s, id)
		if err := s.DeleteTask(id); err != nil {
			return err
		}
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}
