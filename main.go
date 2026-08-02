// Command crush is a terminal to-do list manager with a CLI surface an
// agent can drive. Phase 2 (docs/plans/phase-2-cli.md) replaced phase 0's
// hand-rolled flag handling with Cobra: the root command dispatches every
// subcommand in docs/DESIGN.md §9, and with no subcommand it will launch the
// TUI (phase 3) — until then, the root prints the phase-0 placeholder.
package main

import (
	"os"

	"github.com/filipemolina/chore-crusher/src/cli"
)

func main() {
	// cli.Execute owns the exit-code contract (0/1/2, docs/DESIGN.md §9);
	// main only forwards it.
	os.Exit(cli.Execute(os.Args[1:]))
}
