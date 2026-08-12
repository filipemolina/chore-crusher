package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/store"
)

// exportJSON is the --json success payload of `farol export`: the whole
// document, exactly one JSON value on stdout (§9).
type exportJSON struct {
	Version int                `json:"version"`
	Lists   []store.ExportList `json:"lists"`
}

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export [list-id]",
		Short: "export the whole store, or one list, to JSON",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runExport,
	}
	cmd.Flags().String("out", "", "write the JSON to this file instead of stdout")
	return cmd
}

func runExport(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	out, _ := cmd.Flags().GetString("out")
	return runStore(cmd, func(s *store.Store) error {
		var listID *string
		if len(args) == 1 {
			id, err := s.ResolveID("list", args[0])
			if err != nil {
				return err
			}
			listID = &id
		}
		doc, err := s.Export(listID)
		if err != nil {
			return err
		}
		// File output: write the document, print a one-line human summary.
		if out != "" {
			b, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(out, b, 0o644); err != nil {
				return err
			}
			printResult(jsonMode, func() {
				fmt.Printf("exported %d list(s) to %s\n", len(doc.Lists), out)
			}, exportJSON{Version: doc.Version, Lists: doc.Lists})
			return nil
		}
		// stdout: human mode prints the document pretty-printed; --json prints
		// exactly one JSON value.
		printResult(jsonMode, func() {
			b, _ := json.MarshalIndent(doc, "", "  ")
			fmt.Println(string(b))
		}, exportJSON{Version: doc.Version, Lists: doc.Lists})
		return nil
	})
}
