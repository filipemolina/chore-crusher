// Command complete is a terminal to-do list manager with a CLI surface an
// agent can drive. This is a phase-0 scaffolding placeholder: it answers
// --version and otherwise says so, nothing more. See
// docs/plans/phase-0-scaffolding.md and docs/plans/phase-2-cli.md, which
// replaces this file's flag handling with Cobra.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/filipemolina/chore-completer/src/constants"
)

// errVersionRequested is how parseFlags reports --version. Not a failure —
// see stack-stitcher's main.go, which this mirrors.
var errVersionRequested = errors.New("version requested")

func main() {
	err := parseFlags(os.Args[1:])
	if errors.Is(err, errVersionRequested) {
		fmt.Println("complete", constants.Version())
		os.Exit(0)
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "complete:", err)
		os.Exit(1)
	}

	fmt.Println("complete: not yet implemented — see docs/ROADMAP.md")
}

// parseFlags is a placeholder for phase 0. Phase 2
// (docs/plans/phase-2-cli.md) replaces this with Cobra's root command and
// subcommand dispatch; until then this only recognizes --version so the
// scaffolding has something real to build and test against.
func parseFlags(args []string) error {
	flags := flag.NewFlagSet("complete", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), usage)
	}

	var showVersion bool
	flags.BoolVar(&showVersion, "version", false, "print the version and exit")
	flags.BoolVar(&showVersion, "v", false, "shorthand for --version")

	if err := flags.Parse(args); err != nil {
		return err
	}

	if showVersion {
		return errVersionRequested
	}

	return nil
}

const usage = `Chore Completer - a terminal to-do list manager, and a CLI an agent can drive.

Usage:
  complete [flags]

Flags:
  -v, --version     print the version and exit
  -h, --help        show this help

This is a scaffolding build (docs/plans/phase-0-scaffolding.md). Phase 2 adds
the full subcommand surface documented in docs/DESIGN.md §9.
`
