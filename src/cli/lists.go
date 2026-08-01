package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/filipemolina/chore-completer/src/store"
)

// listJSON is the --json shape of one list row (docs/DESIGN.md §9): the
// counts ListLists computes, plus the id and name a caller acts on.
type listJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Pending   int    `json:"pending"`
	Complete  int    `json:"complete"`
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

	cmd.AddCommand(
		&cobra.Command{
			Use:   "add <name>",
			Short: "create a list; prints its id",
			Args:  cobra.ExactArgs(1),
			RunE:  runListsAdd,
		},
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
	return runStore(cmd, func(s *store.Store) error {
		ls, err := s.ListLists()
		if err != nil {
			return err
		}
		printResult(jsonMode, func() {
			if len(ls) == 0 {
				return // an empty result prints nothing in human mode (§9)
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tPENDING\tCOMPLETE")
			for _, l := range ls {
				fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", l.ID, l.Name, l.PendingCount, l.CompleteCount)
			}
			w.Flush()
		}, listsJSON(ls))
		return nil
	})
}

func runListsAdd(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		id, err := s.CreateList(args[0])
		if err != nil {
			return err
		}
		// Human mode prints only the id — an agent capturing
		// `id=$(complete lists add …)` strips nothing (§9).
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
