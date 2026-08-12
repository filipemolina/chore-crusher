// Package cli holds the Cobra command definitions — one file per subcommand
// group — each a thin adapter from flags to a src/store call and a
// --json-aware printer. See docs/DESIGN.md §9 for the full contract. This
// package never imports src/model, and src/model never imports this one:
// they are siblings over src/store, not layered on each other.
package cli

import (
	"errors"
	"fmt"
	"regexp"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/config"
	"github.com/filipemolina/farol/src/constants"
	"github.com/filipemolina/farol/src/model"
	"github.com/filipemolina/farol/src/store"
)

// domainErr marks a RunE failure as a domain failure — docs/DESIGN.md §9's
// exit code 1 — as opposed to a usage error (bad flag, wrong argument count,
// unknown subcommand), which exits 2. Cobra does not distinguish the two
// classes in the errors it returns, and the CLI's contract is written in
// exit codes, so the classes have to be distinguishable at Execute.
type domainErr struct{ err error }

func (e *domainErr) Error() string { return e.err.Error() }
func (e *domainErr) Unwrap() error { return e.err }

// domainError wraps err for the exit-code mapping. printError has already
// printed err's message by the time this is called; the wrap only carries
// the code.
func domainError(err error) error { return &domainErr{err} }

// Execute runs the root command with args (main.go passes os.Args[1:]) and
// returns the process exit code: 0 success, 1 domain failure (id not found,
// invalid state transition, validation failure), 2 usage error. Usage errors
// — Cobra's flag parsing, argument-count validation, an unknown subcommand —
// are left to Cobra to report and fall through to 2 unmodified, per
// docs/DESIGN.md §9; only domain failures are wrapped and mapped to 1.
func Execute(args []string) int {
	root := NewRootCommand()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		var de *domainErr
		if errors.As(err, &de) {
			return 1
		}
		return 2
	}
	return 0
}

// taskIDPattern matches a ULID, or an unambiguous prefix of one: uppercase
// letters and digits (the Crockford alphabet), 1–26 characters. A single
// argument that looks like a task id is treated as the mark-complete
// shorthand; anything else falls through to the unknown-command path so
// typos like `farol frobnicate` still produce Cobra's usage error.
var taskIDPattern = regexp.MustCompile(`^[0-9A-Z]{1,26}$`)

func looksLikeTaskID(s string) bool { return taskIDPattern.MatchString(s) }

// NewRootCommand builds the farol command tree. Cobra owns argument
// parsing and --help; --version comes from the Version field (fed by
// constants.Version), which also gives the -v shorthand phase 0 had.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "farol",
		Short: "a terminal to-do list manager, and a CLI an agent can drive",
		Long: `farol is a keyboard-driven terminal to-do list, paired with a
full command-line interface for the same operations — the TUI and the CLI
are two views of one store, and either one's changes are visible to the
other within a second (docs/DESIGN.md §7).

With no subcommand it launches the TUI. With one ULID-looking argument it
marks the task with that id complete (cascading to descendants).`,
		Args:    cobra.ArbitraryArgs,
		Version: constants.Version(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && looksLikeTaskID(args[0]) {
				return runComplete(cmd, args)
			}
			if len(args) >= 1 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.Name())
			}
			// One-shot migration: move a pre-rename chore-crusher config/data
			// pair to the farol paths before either is opened (idempotent).
			if err := config.MigrateLegacyDirs(); err != nil {
				return domainError(err)
			}
			s, err := store.Open(config.DBPath())
			if err != nil {
				return domainError(err)
			}
			defer s.Close()

			cfg, err := config.LoadConfig()
			if err != nil {
				return domainError(err)
			}
			// A saved theme preference wins over the compiled default: the
			// picker's Enter writes theme: to config.yaml (cmds.ApplyTheme),
			// and this is the read half of that round trip — without it a
			// chosen theme dies with the process. An empty or unknown name
			// leaves DefaultTheme in effect (SetTheme only assigns registered
			// names).
			appstyles.SetTheme(cfg.Theme)
			m := model.GetInitialModel(s, cfg)
			p := tea.NewProgram(m)
			if _, err := p.Run(); err != nil {
				return domainError(err)
			}
			return nil
		},
	}
	// Phase 0 printed "farol <version>"; keep that exact shape now that
	// Cobra owns the flag.
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	root.PersistentFlags().Bool("json", false,
		"emit exactly one JSON value on stdout, success or failure (docs/DESIGN.md §9)")

	root.AddCommand(
		newListsCmd(),
		newTasksCmd(),
		newSearchCmd(),
		newInboxCmd(),
		newWorkCmd(),
		newClaimCmd(),
		newReleaseCmd(),
		newSkillCmd(),
		newExportCmd(),
		newImportCmd(),
	)
	root.AddCommand(taskCommands()...)
	root.AddCommand(attachmentCommands()...)
	return root
}

// runStore opens the default store and hands it to fn, containing the
// per-subcommand error path in one place: an open failure or a non-nil error
// from fn is reported via printError and returned wrapped so Execute maps it
// to exit code 1; a nil error from fn means fn already printed its result
// via printResult. No RunE writes its own error handling (docs/DESIGN.md §9).
func runStore(cmd *cobra.Command, fn func(s *store.Store) error) error {
	jsonMode, _ := cmd.Flags().GetBool("json")
	// One-shot migration before the store opens (idempotent; see
	// config.MigrateLegacyDirs). Failure is fatal: starting against an
	// empty store while the user's data sits under the old name would
	// look like data loss.
	if err := config.MigrateLegacyDirs(); err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	s, err := store.Open(config.DBPath())
	if err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	defer s.Close()
	if err := fn(s); err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	return nil
}

// okPayload is the success payload for write commands that have no id or
// value of their own to report — the JSON contract requires exactly one
// value on stdout even when there is nothing interesting to say
// (docs/DESIGN.md §9).
type okPayload struct {
	OK bool `json:"ok"`
}

// errSilence is the two lines every RunE starts with: a domain error has
// already been printed by printError, so Cobra must not re-print it or dump
// usage on top of it. Flag and argument errors happen before RunE runs, so
// they keep their normal Cobra reporting and exit 2 (docs/DESIGN.md §9).
func errSilence(cmd *cobra.Command) {
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
}
