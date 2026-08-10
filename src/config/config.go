// Package config reads and writes the user's persistent preferences:
// ~/.config/chore-crusher/config.yaml (or $XDG_CONFIG_HOME if set).
//
// The config file is a small YAML document:
//
//	theme: crush-ember
//	poll_interval_ms: 1000
//
// More fields will land later (docs/ROADMAP.md). The struct and the write
// path are designed to absorb them without changing existing callers: Add a
// field, tag it, and LoadConfig/SaveConfig round-trip it automatically.
//
// The package also owns user-directory resolution for the data side — where
// the SQLite store lives (src/config/dirs.go, docs/DESIGN.md §8) — so
// main.go's TUI path and every CLI subcommand agree on the database location
// without each re-deriving XDG rules.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the persistent user preferences. Fields are exported for YAML
// marshalling; the zero value of each is the "not set" sentinel, and
// LoadConfig leaves a missing field at its zero rather than erroring.
type Config struct {
	// Theme is the registered theme name to activate on startup.
	// Empty means "use appstyles.DefaultTheme".
	Theme string `yaml:"theme,omitempty"`

	// PollIntervalMs is how often the TUI re-polls the store (docs/DESIGN.md
	// §7). Zero means "use the compiled default" — the poll tick reads
	// PollInterval(config) rather than a raw field, so a config file that
	// predates the field still polls.
	PollIntervalMs int `yaml:"poll_interval_ms,omitempty"`
}

// DefaultPollInterval is how often the TUI re-polls the store when the
// config file leaves PollIntervalMs at its zero value (docs/DESIGN.md §7).
// One second is the DESIGN's number: CLI and TUI changes become visible to
// the other view within that window.
const DefaultPollInterval = time.Second

// PollInterval returns how often the TUI should re-poll the store: the
// config's explicit value when set, DefaultPollInterval otherwise. Callers
// read this rather than the raw field so a config file that predates the
// field still polls.
func PollInterval(cfg Config) time.Duration {
	if cfg.PollIntervalMs > 0 {
		return time.Duration(cfg.PollIntervalMs) * time.Millisecond
	}
	return DefaultPollInterval
}

// configDir returns the directory the config file lives in:
// $XDG_CONFIG_HOME/chore-crusher if XDG_CONFIG_HOME is set, otherwise
// ~/.config/chore-crusher.
func configDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "chore-crusher"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".config", "chore-crusher"), nil
}

// configPath returns the full path to the config file.
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// LoadConfig reads the config file and returns the parsed Config. A missing
// file is not an error: it returns the zero Config and a nil error, so the
// caller can apply defaults (DefaultTheme) without special-casing "first run".
// A malformed file is an error worth reporting — the caller decides whether
// to surface it or fall back to defaults.
func LoadConfig() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// SaveConfig writes cfg to the config file, creating the directory if
// needed. It is the whole persistence story for now: one call after a
// theme is chosen, one file, one write.
func SaveConfig(cfg Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0o644)
}
