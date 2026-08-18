package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/apptypes"
	"github.com/filipemolina/farol/src/mentions"
	"github.com/filipemolina/farol/src/store"
)

// inboxListJSON is one list block in `farol inbox --json` (docs/DESIGN.md
// §9): the list's identity and counts plus the top pending tasks an agent
// would act on. The CLI is the single agent-facing front end (the MCP server
// was retired in the cli-first migration), and this shape is the inbox's
// canonical JSON. CreatedBy carries the owning agent tag; Collaborative
// carries the list's opt-in any-agent-structural-edit flag. The store is
// unenforced, so the CLI reports both rather than acting on them.
type inboxListJSON struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Pending       int           `json:"pending"`
	Complete      int           `json:"complete"`
	CreatedBy     string        `json:"created_by,omitempty"`
	Collaborative bool          `json:"collaborative,omitempty"`
	Tasks         []taskRowJSON `json:"tasks"`
}

// inboxJSON is the whole `farol inbox --json` envelope — the CLI equivalent
// of the MCP farol:///inbox resource. mine is the single list this agent
// owns (created_by == FAROL_AGENT); foreign_lists is every other list, so an
// agent sees its own board and the work available to pick up in one read.
type inboxJSON struct {
	Mine         inboxListJSON   `json:"mine"`
	ForeignLists []inboxListJSON `json:"foreign_lists"`
}

// inboxCap is the maximum number of pending tasks the inbox reports per list
// (mirrors the MCP resource's cap), so a long list never floods the opener.
const inboxCap = 20

func newInboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "start-of-session context: your list plus every foreign list, each with its top pending tasks",
		Args:  cobra.NoArgs,
		RunE:  runInbox,
	}
	cmd.Flags().StringSlice("include", nil,
		"inline extra per-row fields: 'notes' (the MCP resource always inlines notes, matching that here)")
	return cmd
}

// runInbox is the CLI equivalent of the MCP farol:///inbox resource: a
// single one-read session opener. It lists every list, splits them into the
// reader's own list (created_by == FAROL_AGENT) and every foreign list, and
// for each one emits its top pending tasks (per-task status filter, capped at
// inboxCap) with notes inlined — exactly what the resource does, so the CLI
// fully replaces the server for the session-opener use case.
//
// The order of operations per list mirrors the resource: one presence read
// for the whole request (not per list), the per-task pending filter
// (sectionRows in the server), the top-20 cap, then notes inlined. Like
// every other CLI read, this is a read and claims no presence.
func runInbox(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	include, _ := cmd.Flags().GetStringSlice("include")
	includeNotes := false
	for _, v := range include {
		switch v {
		case "notes":
			includeNotes = true
		case "comments":
			// The MCP resource inlines notes only; comments are not part of
			// the opener. Reject the value rather than silently ignoring it,
			// matching parseInclude's hard-error discipline (§9).
			err := fmt.Errorf("unknown --include %q: the inbox inlines notes only", v)
			printError(jsonMode, err)
			return domainError(err)
		default:
			err := fmt.Errorf("unknown --include %q: supported value is notes", v)
			printError(jsonMode, err)
			return domainError(err)
		}
	}
	return runStore(cmd, func(s *store.Store) error {
		lists, err := s.ListLists()
		if err != nil {
			return err
		}
		// One presence read for the whole inbox, passed down to every row's
		// assignee_live field — the server does this to avoid one ListWork
		// query per list (docs/DESIGN.md §3).
		live, err := liveAgents(s)
		if err != nil {
			return err
		}
		me := agentIdentity()
		var mine inboxListJSON
		foreign := make([]inboxListJSON, 0, len(lists))
		for _, l := range lists {
			tasks, err := s.ListTasks(l.ID)
			if err != nil {
				return err
			}
			// Per-task pending filter: a task is kept when its own status is
			// pending (open = pending + in_progress; the opener shows the
			// immediately-actable work). Ancestors of a pending task that are
			// themselves complete are NOT emitted as skeletons here — the
			// inbox resource inlines full task bodies, not tree context, so a
			// pending child of a complete parent still arrives, and its parent
			// chain is recoverable via parent_id.
			rows := pendingInboxRows(s, tasks, l.CreatedBy, live)
			if len(rows) > inboxCap {
				rows = rows[:inboxCap]
			}
			if includeNotes {
				inlineNotesInto(rows, tasks)
			}
			block := inboxListJSON{
				ID:            l.ID,
				Name:          l.Name,
				Pending:       l.PendingCount,
				Complete:      l.CompleteCount,
				CreatedBy:     l.CreatedBy,
				Collaborative: l.Collaborative,
				Tasks:         rows,
			}
			if l.CreatedBy == me {
				// Mirror the resource: the reader's own tag is implicit, so it
				// is dropped from the mine block (not in the foreign ones).
				block.CreatedBy = ""
				mine = block
			} else {
				foreign = append(foreign, block)
			}
		}
		printResult(jsonMode, func() {
			renderInboxHuman(mine, foreign)
		}, inboxJSON{Mine: mine, ForeignLists: foreign})
		return nil
	})
}

// pendingInboxRows flattens a list's tasks and keeps the rows whose own
// status is pending or in_progress (the per-task filter the MCP inbox
// resource applies — distinct from `farol tasks`'s root-based Pending
// section). Parent chains survive via parent_id on each row, so the tree is
// still reconstructable without emitting ancestor skeletons.
func pendingInboxRows(s *store.Store, tasks []store.Task, listOwner string, live map[string]bool) []taskRowJSON {
	converted := apptypes.FromStoreTasks(tasks)
	rows := apptypes.Flatten(converted)
	out := make([]taskRowJSON, 0, len(rows))
	for _, r := range rows {
		t := r.Task
		if t.Status != apptypes.StatusPending && t.Status != apptypes.StatusInProgress {
			continue
		}
		p, err := progressOf(s, t.ID)
		if err != nil {
			// A derived-progress read can only fail on a missing task; the
			// task came from this same list, so skip rather than sink.
			continue
		}
		row := taskRowJSON{
			ID:           t.ID,
			ParentID:     t.ParentID,
			Title:        t.Title,
			Status:       string(t.Status),
			Progress:     p,
			Depth:        r.Depth,
			ListOwner:    listOwner,
			HasNotes:     t.Notes != "",
			NotesLen:     len(t.Notes),
			Assignee:     t.Assignee,
			AssignedAt:   t.AssignedAt,
			AssigneeLive: assigneeLive(live, t.Assignee),
			Priority:     string(t.Priority),
		}
		out = append(out, row)
	}
	return out
}

// inlineNotesInto copies each task's full notes body into the row whose id
// matches, mirroring the MCP inbox's inlineNotes. Skeleton rows do not exist
// in the inbox (no ancestors are emitted separately), so every row is
// eligible — a row with no notes simply gets an empty body.
func inlineNotesInto(rows []taskRowJSON, tasks []store.Task) {
	byID := make(map[string]string, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t.Notes
	}
	for i := range rows {
		if body, ok := byID[rows[i].ID]; ok {
			rows[i].Notes = body
		}
	}
}

// renderInboxHuman prints the human-readable opener: the agent's own list
// first, then every foreign list, each as a header with its pending/complete
// counts and its pending rows in the shared tree layout (docs/DESIGN.md §12).
// An empty inbox prints nothing in human mode, matching the §9 no-output
// rule for empty reads.
func renderInboxHuman(mine inboxListJSON, foreign []inboxListJSON) {
	var b strings.Builder
	// Skip the "Your list" header when the agent owns no list (mine is the
	// zero value), so the opener never leads with an empty block.
	if mine.ID != "" {
		renderInboxBlock(&b, "Your list", mine)
	}
	for _, f := range foreign {
		renderInboxBlock(&b, "Foreign list", f)
	}
	if b.Len() == 0 {
		return
	}
	fmt.Print(b.String())
}

// renderInboxBlock writes one list's section to w: a header line carrying the
// list name and pending/complete counts (the header is shown even when there
// are no pending tasks, so the reader sees the list exists and how full it
// is), then its pending rows via the shared renderRow.
func renderInboxBlock(w *strings.Builder, label string, block inboxListJSON) {
	fmt.Fprintf(w, "%s: %s (%d pending, %d complete)\n", label, block.Name, block.Pending, block.Complete)
	if len(block.Tasks) == 0 {
		fmt.Fprintf(w, "  (no pending tasks)\n")
		return
	}
	views := rowsToViews(block.Tasks)
	for i, v := range views {
		var titleMentions []mentions.MentionMetadata
		if i < len(block.Tasks) {
			titleMentions = block.Tasks[i].TitleMentions
		}
		renderRowTo(w, v, titleMentions)
	}
}

// rowsToViews adapts inbox taskRowJSON rows to taskView for the shared
// renderRowTo, so the inbox renders with the exact same row chrome as
// `farol tasks` / `farol show` rather than a second layout.
func rowsToViews(rows []taskRowJSON) []taskView {
	views := make([]taskView, 0, len(rows))
	for _, r := range rows {
		hasChildren := false
		for _, c := range rows {
			if c.ParentID != nil && *c.ParentID == r.ID {
				hasChildren = true
				break
			}
		}
		views = append(views, taskView{
			row: apptypes.Row{
				Task: apptypes.Task{
					ID:       r.ID,
					ParentID: r.ParentID,
					Title:    r.Title,
					Status:   apptypes.Status(r.Status),
					Assignee: r.Assignee,
					Priority: apptypes.Priority(r.Priority),
				},
				Depth:       r.Depth,
				HasChildren: hasChildren,
			},
			prog: r.Progress,
		})
	}
	return views
}
