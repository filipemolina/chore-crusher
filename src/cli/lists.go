package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/store"
)

// listJSON is the --json shape of one list row (docs/DESIGN.md §9): the
// counts ListLists computes, plus the id and name a caller acts on. CreatedBy
// mirrors List.created_by — the owning agent tag — so an agent reading the
// list surface can tell at a glance which lists are its own (the CLI
// equivalent of the MCP my_list tool's mine/foreign split).
type listJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Pending   int    `json:"pending"`
	Complete  int    `json:"complete"`
	CreatedBy string `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
}

func listsJSON(ls []store.ListSummary) []listJSON {
	out := make([]listJSON, 0, len(ls))
	for _, l := range ls {
		out = append(out, listJSON{
			ID:        l.ID,
			Name:      l.Name,
			Pending:   l.PendingCount,
			Complete:  l.CompleteCount,
			CreatedBy: l.CreatedBy,
			CreatedAt: l.CreatedAt,
		})
	}
	return out
}

func newListsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lists",
		Short: "list all lists",
		Args:  cobra.NoArgs,
		RunE:  runLists,
	}
	// --mine / --foreign replicate the MCP my_list tool's split: --mine shows
	// only lists this agent owns (created_by == FAROL_AGENT); --foreign shows
	// only lists owned by another agent (so an agent can find work to pick up).
	// Both default to false, which prints every list (human and agent alike).
	cmd.Flags().Bool("mine", false, "show only lists owned by this agent (FAROL_AGENT)")
	cmd.Flags().Bool("foreign", false, "show only lists owned by another agent")
	cmd.MarkFlagsMutuallyExclusive("mine", "foreign")

	cmd.AddCommand(
		func() *cobra.Command {
			addCmd := &cobra.Command{
				Use:   "add <name>",
				Short: "create a list; prints its id",
				Args:  cobra.ExactArgs(1),
				RunE:  runListsAdd,
			}
			// --owner provisions the list for an agent up front (the ownership
			// rule refuses structural writes on an untagged list); empty keeps the
			// human-managed behaviour where only the human restructures it.
			addCmd.Flags().String("owner", "", "owning agent tag (e.g. pi); empty keeps the list human-managed")
			return addCmd
		}(),
		&cobra.Command{
			Use:   "rename <list-id> <name>",
			Short: "rename a list",
			Args:  cobra.ExactArgs(2),
			RunE:  runListsRename,
		},
		&cobra.Command{
			Use:   "rm <list-id>",
			Short: "delete a list and its tasks",
			Args:  cobra.ExactArgs(1),
			RunE:  runListsRm,
		},
	)

	rmCmd := cmd.Commands()[len(cmd.Commands())-1]
	rmCmd.Flags().Bool("force", false, "delete without confirmation")
	return cmd
}

func runLists(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	mine, _ := cmd.Flags().GetBool("mine")
	foreign, _ := cmd.Flags().GetBool("foreign")
	return runStore(cmd, func(s *store.Store) error {
		ls, err := s.ListLists()
		if err != nil {
			return err
		}
		me := agentIdentity()
		if mine || foreign {
			kept := ls[:0]
			for _, l := range ls {
				ownedByMe := l.CreatedBy == me
				switch {
				case mine && ownedByMe:
					kept = append(kept, l)
				case foreign && !ownedByMe:
					kept = append(kept, l)
				}
			}
			ls = kept
		}
		printResult(jsonMode, func() {
			if len(ls) == 0 {
				return // an empty result prints nothing in human mode (§9)
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID	NAME	PENDING	COMPLETE")
			for _, l := range ls {
				fmt.Fprintf(w, "%s	%s	%d	%d\n", l.ID, l.Name, l.PendingCount, l.CompleteCount)
			}
			w.Flush()
		}, listsJSON(ls))
		return nil
	})
}

func runListsAdd(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	owner, _ := cmd.Flags().GetString("owner")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.CreateList(args[0], owner)
		if err != nil {
			return err
		}
		// Human mode prints only the id — an agent capturing
		// `id=$(farol lists add …)` strips nothing (§9).
		printResult(jsonMode, func() { fmt.Println(id) }, idJSON{id})
		return nil
	})
}

func runListsRename(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("list", args[0])
		if err != nil {
			return err
		}
		if err := s.RenameList(id, args[1]); err != nil {
			return err
		}
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}

// runListsRm refuses to touch the store without --force: the CLI has no
// confirm modal and no human to ask, so the flag is the confirmation
// (docs/DESIGN.md §9). The check runs before the store opens at all — a
// missing --force must not even create the database file.
func runListsRm(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	force, _ := cmd.Flags().GetBool("force")
	if !force {
		err := fmt.Errorf("refusing to delete list %q without --force", args[0])
		printError(jsonMode, err)
		return domainError(err)
	}
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.ResolveID("list", args[0])
		if err != nil {
			return err
		}
		if err := s.DeleteList(id); err != nil {
			return err
		}
		printResult(jsonMode, func() {}, okPayload{true})
		return nil
	})
}
