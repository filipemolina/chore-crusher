package cli

import (
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/filipemolina/chore-crusher/src/apptypes"
	"github.com/filipemolina/chore-crusher/src/store"
)

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

// taskRowJSON is one row of `crush tasks` (and `crush show`'s
// children) in JSON mode: a flat preorder array with depth, so a caller
// walks the same shape whether or not it asked for --flat (docs/DESIGN.md
// §9).
type taskRowJSON struct {
	ID        string       `json:"id"`
	ParentID  *string      `json:"parent_id"`
	Title     string       `json:"title"`
	Status    string       `json:"status"`
	Progress  progressJSON `json:"progress"`
	Depth     int          `json:"depth"`
	ListOwner string       `json:"list_owner"`
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

	showCmd := &cobra.Command{
		Use:   "show <task-id>",
		Short: "show a task: title, notes, status, progress, children",
		Args:  cobra.ExactArgs(1),
		RunE:  runShow,
	}

	renameCmd := &cobra.Command{
		Use:   "rename <task-id> <title>",
		Short: "rename a task",
		Args:  cobra.ExactArgs(2),
		RunE:  runRename,
	}

	notesCmd := &cobra.Command{
		Use:   "notes <task-id> <text>",
		Short: "replace a task's notes (whole text, not append)",
		Args:  cobra.ExactArgs(2),
		RunE:  runNotes,
	}

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

	commentCmd := &cobra.Command{
		Use:   "comment <task-id> <note>",
		Short: "add a comment to a task; prints its id",
		Args:  cobra.ExactArgs(2),
		RunE:  runComment,
	}

	return []*cobra.Command{addCmd, showCmd, renameCmd, notesCmd,
		reopenCmd, toggleCmd, progressCmd, rmCmd, mvCmd, commentCmd}
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
// shared Flatten (and its Row type) never sees store.Task
// (docs/plans/phase-3-tui-shell.md step 3).
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
	if !validStatusFilter(status) {
		err := fmt.Errorf("invalid --status %q: want pending, in_progress, complete, or all", status)
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
			payload = append(payload, taskRowJSON{
				ID:        v.row.Task.ID,
				ParentID:  v.row.Task.ParentID,
				Title:     v.row.Task.Title,
				Status:    string(v.row.Task.Status),
				Progress:  v.prog,
				Depth:     v.row.Depth,
				ListOwner: l.CreatedBy,
			})
		}

		printResult(jsonMode, func() {
			if len(pendingViews) == 0 && len(completeViews) == 0 {
				return // an empty result prints nothing in human mode (§9)
			}
			if flat {
				renderFlat(pendingViews)
				renderFlat(completeViews)
				return
			}
			renderSection("Pending", pendingViews)
			renderSection("Complete", completeViews)
		}, payload)
		return nil
	})
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

func runAdd(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	parentPrefix, _ := cmd.Flags().GetString("parent")
	notes, _ := cmd.Flags().GetString("notes")
	return runStore(cmd, func(s *store.Store) error {
		var parent *string
		if parentPrefix != "" {
			resolved, err := s.ResolveID("task", parentPrefix)
			if err != nil {
				return err
			}
			parent = &resolved
		}
		id, err := s.CreateTask(args[0], args[1], parent, notes)
		if err != nil {
			return err
		}
		printResult(jsonMode, func() { fmt.Println(id) }, idJSON{id})
		return nil
	})
}

// commentJSON is one comment on a task, mirrored between the CLI's
// `crush show --json` and the MCP's show_task/comments payload (docs/DESIGN.md
// §9: MCP is a superset of CLI --json, additive fields only).
type commentJSON struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"created_at"`
}

// showJSON is `crush show`'s payload: the task's own fields plus its
// descendants as the same flat, depth-annotated rows `tasks` emits, so a
// caller that can read one can read the other.
type showJSON struct {
	ID          string        `json:"id"`
	ListID      string        `json:"list_id"`
	ListOwner   string        `json:"list_owner"`
	Title       string        `json:"title"`
	Notes       string        `json:"notes"`
	Status      string        `json:"status"`
	Progress    progressJSON  `json:"progress"`
	CreatedAt   int64         `json:"created_at"`
	UpdatedAt   int64         `json:"updated_at"`
	CompletedAt *int64        `json:"completed_at"`
	Children    []taskRowJSON `json:"children"`
	Comments    []commentJSON `json:"comments"`
}

func runShow(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		t, err := s.GetTask(id)
		if err != nil {
			return err
		}
		prog, err := progressOf(s, id)
		if err != nil {
			return err
		}
		all, err := s.ListTasks(t.ListID)
		if err != nil {
			return err
		}
		l, err := s.GetList(t.ListID)
		if err != nil {
			return err
		}
		childViews, err := viewsOf(s, apptypes.DescendantsOf(apptypes.FromStoreTasks(all), id))
		if err != nil {
			return err
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
			})
		}

		comments, err := s.ListComments(id)
		if err != nil {
			return err
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

		printResult(jsonMode, func() {
			fmt.Fprintf(os.Stdout, "Title: %s\n", t.Title)
			fmt.Fprintf(os.Stdout, "ID: %s\n", id)
			fmt.Fprintf(os.Stdout, "List: %s\n", t.ListID)
			fmt.Fprintf(os.Stdout, "Status: %s\n", t.Status)
			fmt.Fprintf(os.Stdout, "Progress: %s\n", progressHuman(prog))
			fmt.Fprintf(os.Stdout, "Notes:\n")
			for _, line := range strings.Split(t.Notes, "\n") {
				fmt.Fprintf(os.Stdout, "  %s\n", line)
			}
			if len(childViews) > 0 {
				fmt.Fprintf(os.Stdout, "Children (%d):\n", len(childViews))
				for _, v := range childViews {
					renderRow(v)
				}
			}
			if len(comments) > 0 {
				fmt.Fprintf(os.Stdout, "Comments (%d):\n", len(comments))
				for _, c := range comments {
					fmt.Fprintf(os.Stdout, "  - %s (%s): %s\n", c.Author, formatTime(c.CreatedAt), c.Note)
				}
			}
		}, showJSON{
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
			Children:    children,
			Comments:    commentJSONs,
		})
		return nil
	})
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
// os/user.Current fails — some minimal containers lack /etc/passwd
// (docs/plan/task-comments.md §1).
func osUser() string {
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
		printResult(jsonMode, func() { fmt.Println(cid) }, idJSON{cid})
		return nil
	})
}

func runRename(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		if err := s.RenameTask(id, args[1]); err != nil {
			return err
		}
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}

func runNotes(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
		}
		if err := s.SetNotes(id, args[1]); err != nil {
			return err
		}
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
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}

// runMv wires `crush mv` to store.Reparent. The --parent flag carries the
// new parent's id (prefix accepted); an empty --parent — the flag's default,
// so omitting it entirely — is how a caller asks to move a task to the list
// root, recorded in docs/DESIGN.md §9 (docs/plans/phase-9-polish-release.md
// step 1).
func runMv(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("task", args[0])
		if err != nil {
			return err
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
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}

// runRm mirrors runListsRm: no store call at all without --force — the flag
// is the CLI's confirmation (docs/DESIGN.md §9).
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
		if err := s.DeleteTask(id); err != nil {
			return err
		}
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}
