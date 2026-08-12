package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/store"
)

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "import lists and tasks from a farol export file",
		Args:  cobra.ExactArgs(1),
		RunE:  runImport,
	}
	cmd.Flags().String("list", "", "import only the list with this id from the document")
	return cmd
}

func runImport(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	onlyList, _ := cmd.Flags().GetString("list")
	return runStore(cmd, func(s *store.Store) error {
		raw, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		var doc store.ExportDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("parse export file: %w", err)
		}
		if doc.Version != store.ExportVersion {
			return fmt.Errorf("unsupported export version %d (want %d)", doc.Version, store.ExportVersion)
		}
		n := 0
		for _, el := range doc.Lists {
			if onlyList != "" && el.ID != onlyList {
				continue
			}
			if err := s.ImportList(el); err != nil {
				return err
			}
			n++
		}
		if n == 0 && onlyList != "" {
			return fmt.Errorf("list %q not found in export file", onlyList)
		}
		printResult(jsonMode, func() {
			fmt.Printf("imported %d list(s)\n", n)
		}, okPayload{OK: true})
		return nil
	})
}
