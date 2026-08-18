package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/config"
	"github.com/filipemolina/farol/src/store"
)

// statusJSON is `farol status`'s payload: the store's health in one object —
// table counts, the task status breakdown, the store file's size in bytes,
// the highest applied migration, and the resolved config path. It is a read,
// so (unlike the write commands) it carries no ok field.
type statusJSON struct {
	Lists         int    `json:"lists"`
	Tasks         int    `json:"tasks"`
	Pending       int    `json:"pending"`
	InProgress    int    `json:"in_progress"`
	Complete      int    `json:"complete"`
	StoreSize     int64  `json:"store_size_bytes"`
	LastMigration int    `json:"last_migration"`
	ConfigPath    string `json:"config_path"`
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "store health and summary: counts, size, migrations, config path",
		Long: `A read-only snapshot of the store's health: how many lists and tasks it
holds, the task status breakdown (pending / in_progress / complete), the
store file's size in bytes, the highest migration version applied, and the
resolved config file path. The first thing to run when a store looks wrong;
like every read, it claims no presence. An empty store is a normal state,
not an error: zero counts, exit 0.`,
		Args: cobra.NoArgs,
		RunE: runStatus,
	}
	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	return runStore(cmd, func(s *store.Store) error {
		sum, err := s.StatusSummary()
		if err != nil {
			return err
		}
		// The store file exists by the time runStore hands us the handle —
		// store.Open creates it — so a Stat failure here is a real error,
		// not a "file not there yet" case.
		info, err := os.Stat(config.DBPath())
		if err != nil {
			return err
		}
		cfgPath, err := config.ConfigPath()
		if err != nil {
			return err
		}
		payload := statusJSON{
			Lists:         sum.Lists,
			Tasks:         sum.Tasks,
			Pending:       sum.Pending,
			InProgress:    sum.InProgress,
			Complete:      sum.Complete,
			StoreSize:     info.Size(),
			LastMigration: sum.LastMigration,
			ConfigPath:    cfgPath,
		}
		printResult(jsonMode, func() { renderStatusHuman(payload) }, payload)
		return nil
	})
}

// renderStatusHuman prints the health summary as a labeled key/value readout
// (plain text, no ANSI escapes, per §9): one line per field, aligned by
// tabwriter. Unlike the list-returning reads it never prints nothing — an
// empty store still has counts to report, all zero.
func renderStatusHuman(p statusJSON) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Lists:\t%d\n", p.Lists)
	fmt.Fprintf(w, "Tasks:\t%d\n", p.Tasks)
	fmt.Fprintf(w, "Pending:\t%d\n", p.Pending)
	fmt.Fprintf(w, "In progress:\t%d\n", p.InProgress)
	fmt.Fprintf(w, "Complete:\t%d\n", p.Complete)
	fmt.Fprintf(w, "Store size:\t%s\n", humanBytes(p.StoreSize))
	fmt.Fprintf(w, "Last migration:\t%d\n", p.LastMigration)
	fmt.Fprintf(w, "Config:\t%s\n", p.ConfigPath)
	w.Flush()
}

// humanBytes renders a byte count the way a human reads a store size: plain
// bytes under 1 KiB, then KiB/MiB/GiB with one decimal place. Only the human
// status readout uses it; JSON mode carries the raw byte count.
func humanBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1024*1024*1024))
	}
}
