package cli

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/appstyles"
	"github.com/filipemolina/farol/src/config"
)

// The two keys the config file holds (docs/DESIGN.md §8). Constants, not
// literals, so get/set/list share one spelling and the "unknown key" error
// can name them without drift.
const (
	configKeyTheme          = "theme"
	configKeyPollIntervalMs = "poll_interval_ms"
)

// configEntryJSON is one config key/value row, shared by `farol config get`,
// `set`, and `list`: get returns the row for one key, list returns every
// row in the file's canonical order, and set echoes the row that landed.
// The value keeps its native YAML type — a string for theme, a number for
// poll_interval_ms — so a caller that read the file's types reads the same
// types here.
type configEntryJSON struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

// configSetResultJSON is `config set`'s success payload: it echoes the key
// and the value that landed (the same row shape get/list use), so a caller
// never needs a follow-up get to confirm what was written — the assign and
// priority commands set that precedent (docs/DESIGN.md §9).
type configSetResultJSON struct {
	OK    bool   `json:"ok"`
	Key   string `json:"key"`
	Value any    `json:"value"`
}

// newConfigCmd is `farol config`: view and edit the user's persistent
// preferences in ~/.config/farol/config.yaml (docs/DESIGN.md §8). It is a
// config-file command, not a store read — like `farol skill` it has no
// runStore, because the file is the whole data. A bare `farol config` prints
// the group's help.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "view and edit the config file (~/.config/farol/config.yaml)",
		Long: `Read and write the user's persistent preferences (docs/DESIGN.md §8):
theme and poll_interval_ms, stored in ~/.config/farol/config.yaml (or
$XDG_CONFIG_HOME/farol/config.yaml). Both fields are optional in the file —
a missing field falls back to the compiled default, and get/list report the
effective value, not the raw file contents.`,
	}
	cmd.AddCommand(newConfigGetCmd())
	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigListCmd())
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "print one config value (theme, poll_interval_ms)",
		Args:  cobra.ExactArgs(1),
		RunE:  runConfigGet,
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "set one config value (theme, poll_interval_ms)",
		Args:  cobra.ExactArgs(2),
		RunE:  runConfigSet,
	}
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "print every config value (theme, poll_interval_ms)",
		Args:  cobra.NoArgs,
		RunE:  runConfigList,
	}
}

// runConfigGet prints one key's effective value. Human mode prints the value
// alone — the one piece of information a script capturing `farol config get
// theme` wants, like the id a write command prints (docs/DESIGN.md §9).
func runConfigGet(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	// The one-shot migration moves a pre-rename chore-crusher config.yaml to
	// the farol path before it is read, the way runStore runs it before the
	// store opens — a legacy config must not read as a first-run default.
	if err := config.MigrateLegacyDirs(); err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	entry, err := configEntry(cfg, args[0])
	if err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	printResult(jsonMode, func() {
		fmt.Fprintln(os.Stdout, entry.Value)
	}, entry)
	return nil
}

// runConfigSet applies one key/value and saves the whole struct back —
// LoadConfig → SaveConfig round-trips every field, so setting one key never
// drops the other (docs/DESIGN.md §8).
func runConfigSet(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	if err := config.MigrateLegacyDirs(); err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	entry, err := applyConfigSet(&cfg, args[0], args[1])
	if err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	if err := config.SaveConfig(cfg); err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	printResult(jsonMode, func() {}, configSetResultJSON{OK: true, Key: entry.Key, Value: entry.Value})
	return nil
}

// runConfigList prints every effective value in the file's canonical order
// (theme, then poll_interval_ms — the order §8's YAML block uses) as a plain
// KEY/VALUE table, or the same rows as a JSON array.
func runConfigList(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	if err := config.MigrateLegacyDirs(); err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	entries := configEntries(cfg)
	printResult(jsonMode, func() {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "KEY\tVALUE")
		for _, e := range entries {
			fmt.Fprintf(w, "%s\t%v\n", e.Key, e.Value)
		}
		w.Flush()
	}, entries)
	return nil
}

// configEntries returns the effective config as the ordered row slice `list`
// prints and `get` picks from. Each value is the stored one when set and the
// compiled default when the field is absent, so a config file that predates
// a field still reads with it filled in (docs/DESIGN.md §8).
func configEntries(cfg config.Config) []configEntryJSON {
	return []configEntryJSON{
		{Key: configKeyTheme, Value: configTheme(cfg)},
		{Key: configKeyPollIntervalMs, Value: configPollIntervalMs(cfg)},
	}
}

// configTheme returns the effective theme name: the stored one, or the
// compiled default when the file leaves it unset (docs/DESIGN.md §11).
func configTheme(cfg config.Config) string {
	if cfg.Theme != "" {
		return cfg.Theme
	}
	return appstyles.DefaultTheme
}

// configPollIntervalMs returns the effective poll interval in milliseconds:
// the stored one, or the compiled default (docs/DESIGN.md §7).
func configPollIntervalMs(cfg config.Config) int {
	if cfg.PollIntervalMs > 0 {
		return cfg.PollIntervalMs
	}
	return int(config.DefaultPollInterval.Milliseconds())
}

// configEntry returns the effective row for one key, or a domain error
// naming the supported keys when the caller asked for something else.
func configEntry(cfg config.Config, key string) (configEntryJSON, error) {
	for _, e := range configEntries(cfg) {
		if e.Key == key {
			return e, nil
		}
	}
	return configEntryJSON{}, fmt.Errorf("unknown config key %q: supported keys are %s, %s",
		key, configKeyTheme, configKeyPollIntervalMs)
}

// applyConfigSet applies one key/value to cfg and returns the row that
// landed (the shape set echoes). The two keys' types and their validation
// live here — in the one place — so get/set/list agree on what a valid
// value is.
func applyConfigSet(cfg *config.Config, key, value string) (configEntryJSON, error) {
	switch key {
	case configKeyTheme:
		// An empty theme is refused rather than silently dropped: SaveConfig's
		// omitempty would make "set theme to nothing" a no-op write. Unknown
		// names are allowed — the app itself tolerates them (SetTheme falls
		// back to DefaultTheme, docs/DESIGN.md §11).
		if value == "" {
			return configEntryJSON{}, fmt.Errorf("theme must not be empty")
		}
		cfg.Theme = value
		return configEntryJSON{Key: configKeyTheme, Value: value}, nil
	case configKeyPollIntervalMs:
		ms, err := strconv.Atoi(value)
		if err != nil {
			return configEntryJSON{}, fmt.Errorf("poll_interval_ms must be an integer, got %q", value)
		}
		// Zero is the struct's "unset" sentinel (config.LoadConfig), so a
		// positive value is the only unambiguous thing to write.
		if ms <= 0 {
			return configEntryJSON{}, fmt.Errorf("poll_interval_ms must be a positive integer, got %d", ms)
		}
		cfg.PollIntervalMs = ms
		return configEntryJSON{Key: configKeyPollIntervalMs, Value: ms}, nil
	default:
		return configEntryJSON{}, fmt.Errorf("unknown config key %q: supported keys are %s, %s",
			key, configKeyTheme, configKeyPollIntervalMs)
	}
}
