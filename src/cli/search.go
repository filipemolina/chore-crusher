package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"

	"github.com/filipemolina/chore-crusher/src/store"
)

// searchResultJSON is one `crush search` hit: the task plus the name of
// the list it lives in, so a cross-list result carries its context without a
// second lookup.
type searchResultJSON struct {
	ID       string       `json:"id"`
	ListID   string       `json:"list_id"`
	ListName string       `json:"list_name"`
	Title    string       `json:"title"`
	Status   string       `json:"status"`
	Progress progressJSON `json:"progress"`
}

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "fuzzy search across titles and notes",
		Args:  cobra.ExactArgs(1),
		RunE:  runSearch,
	}
	cmd.Flags().String("list", "", "restrict the search to one list (id prefix)")
	return cmd
}

func runSearch(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	listPrefix, _ := cmd.Flags().GetString("list")
	return runStore(cmd, func(s *store.Store) error {
		var listID *string
		if listPrefix != "" {
			resolved, err := s.ResolveID("list", listPrefix)
			if err != nil {
				return err
			}
			listID = &resolved
		}
		candidates, err := s.SearchTasks(args[0], listID)
		if err != nil {
			return err
		}
		results, err := rankSearch(s, args[0], candidates)
		if err != nil {
			return err
		}
		printResult(jsonMode, func() {
			if len(results) == 0 {
				return // an empty result prints nothing in human mode (§9)
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tLIST\tTITLE")
			for _, r := range results {
				fmt.Fprintf(w, "%s\t%s\t%s\n", r.ID, r.ListName, r.Title)
			}
			w.Flush()
		}, results)
		return nil
	})
}

// rankSearch orders the LIKE-candidates store.SearchTasks returns
// (docs/plans/phase-2-cli.md step 5): title matches first, ranked by fuzzy
// score, then candidates that matched only on notes — which cannot
// fuzzy-match a title they never hit — in store order, so a notes hit is
// still a hit, just a weaker one. This is the minimal single-pass version;
// phase 8 refines the ranking for the TUI's picker rather than rebuilding
// it.
func rankSearch(s *store.Store, query string, candidates []store.Task) ([]searchResultJSON, error) {
	lists, err := s.ListLists()
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(lists))
	for _, l := range lists {
		names[l.ID] = l.Name
	}

	titles := make([]string, len(candidates))
	for i, c := range candidates {
		titles[i] = c.Title
	}
	matched := make([]bool, len(candidates))
	out := make([]searchResultJSON, 0, len(candidates))
	for _, m := range fuzzy.Find(query, titles) {
		matched[m.Index] = true
		r, err := searchResultOf(s, candidates[m.Index], names)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	for i, c := range candidates {
		if !matched[i] {
			r, err := searchResultOf(s, c, names)
			if err != nil {
				return nil, err
			}
			out = append(out, r)
		}
	}
	return out, nil
}

func searchResultOf(s *store.Store, t store.Task, names map[string]string) (searchResultJSON, error) {
	p, err := progressOf(s, t.ID)
	if err != nil {
		return searchResultJSON{}, err
	}
	return searchResultJSON{
		ID:       t.ID,
		ListID:   t.ListID,
		ListName: names[t.ListID],
		Title:    t.Title,
		Status:   string(t.Status),
		Progress: p,
	}, nil
}
