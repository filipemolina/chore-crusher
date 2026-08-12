package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/store"
)

// workClaimJSON is one live presence claim — the CLI equivalent of a single
// row of the retired MCP farol:///work resource (cli-first migration, parity
// gap #1). The shape is an exact mirror of the resource's array: id is the
// AgentActivity row's ULID, entity_type/entity_id name the claimed task or
// list, agent_id is who holds the claim, kind is "working" | "inspecting",
// and acquired_at is the unix second the claim was last renewed. Presence,
// not assignment: who OWNS a task is the task row's assignee field, which is
// a different axis (docs/DESIGN.md §3) and not part of this read.
type workClaimJSON struct {
	ID         string `json:"id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	AgentID    string `json:"agent_id"`
	Kind       string `json:"kind"`
	AcquiredAt int64  `json:"acquired_at"`
}

func newWorkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "work",
		Short: "live presence claims: which tasks/lists an agent is working on right now",
		Long: `The CLI equivalent of the retired MCP farol:///work resource: a read-only
mirror of the store's live presence claims (docs/DESIGN.md §3). It shows who
is at the keyboard right now — every agent claim acquired within the store's
120-second WorkTTL — not who owns a task (that is the assignee field on the
task row, a separate axis). This is the same set the TUI renders a spinner
for. The command is a read and claims no presence of its own.`,
		Args: cobra.NoArgs,
		RunE: runWork,
	}
	return cmd
}

// runWork is the CLI equivalent of the farol:///work resource: a single
// read of store.ListWork, the live claims the TUI shows spinners for. Like
// every other CLI read it claims no presence, and it returns one JSON value
// (a bare array, matching the resource and the other list-returning commands)
// or a human table. An empty claim set is a normal state, not an error: it
// prints nothing in human mode and [] in --json mode.
func runWork(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		work, err := s.ListWork()
		if err != nil {
			return err
		}
		claims := make([]workClaimJSON, 0, len(work))
		for _, w := range work {
			claims = append(claims, workClaimJSON{
				ID:         w.ID,
				EntityType: w.EntityType,
				EntityID:   w.EntityID,
				AgentID:    w.AgentID,
				Kind:       string(w.Kind),
				AcquiredAt: w.AcquiredAt,
			})
		}
		printResult(jsonMode, func() {
			renderWorkHuman(s, claims)
		}, claims)
		return nil
	})
}

// renderWorkHuman prints the live claims as a plain table (no ANSI escapes,
// per §9): one row per claim with the holding agent, the claimed entity, the
// entity's title resolved best-effort, the claim kind, and how long ago the
// claim was renewed. An empty claim set prints nothing, matching the §9
// no-output rule for empty reads. The resource carried only ids; resolving
// the title here is the ergonomic a static JSON blob could not offer, so a
// human reading `farol work` sees "which task" directly rather than having to
// re-read each entity by id.
func renderWorkHuman(s *store.Store, claims []workClaimJSON) {
	if len(claims) == 0 {
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tENTITY\tTITLE\tKIND\tAGE")
	for _, c := range claims {
		title := workEntityTitle(s, c.EntityType, c.EntityID)
		if title == "" {
			title = c.EntityID // entity gone or unreadable: keep the id
		}
		fmt.Fprintf(w, "%s\t%s:%s\t%s\t%s\t%s\n",
			c.AgentID, c.EntityType, c.EntityID, title, c.Kind, humanAge(c.AcquiredAt))
	}
	w.Flush()
}

// workEntityTitle resolves the human-readable name of a claimed entity
// (task title or list name) for the human table. Best-effort: any read
// failure — a claim left behind on a deleted task, a transient error — falls
// back to an empty string so the caller keeps the id rather than the whole
// row being dropped.
func workEntityTitle(s *store.Store, entityType, entityID string) string {
	switch entityType {
	case "task":
		if t, err := s.GetTask(entityID); err == nil {
			return t.Title
		}
	case "list":
		if l, err := s.GetList(entityID); err == nil {
			return l.Name
		}
	}
	return ""
}

// humanAge renders a claim's age the way the takeover message in §9 does
// ("2h14m ago"): seconds under a minute, then minutes+seconds, then
// hours+minutes. Claims are always within WorkTTL, so the value is small, but
// the format is stable if a clock ever reports a larger span.
func humanAge(acquiredAt int64) string {
	d := time.Now().Unix() - acquiredAt
	if d < 0 {
		d = 0
	}
	switch {
	case d < 60:
		return fmt.Sprintf("%ds ago", d)
	case d < 3600:
		return fmt.Sprintf("%dm%ds ago", d/60, d%60)
	default:
		return fmt.Sprintf("%dh%dm ago", d/3600, (d%3600)/60)
	}
}
