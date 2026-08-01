// Package config reads and writes ~/.config/complete/config.yaml (or
// $XDG_CONFIG_HOME), following stack-stitcher's src/config exactly: a
// missing file or field falls back to the compiled default, a malformed
// file is reported. See docs/DESIGN.md §8.
//
// It also owns user-directory resolution for the data side — where the
// SQLite store lives (docs/DESIGN.md §8) — so main.go's TUI path and every
// CLI subcommand agree on the database location without each re-deriving
// XDG rules.
package config

import (
	"os"
	"path/filepath"
)

// DataDir returns the directory that owns this app's data files:
// $XDG_DATA_HOME/complete, or ~/.local/share/complete when XDG_DATA_HOME is
// unset (docs/DESIGN.md §8). store.Open creates it on first use; a HOME-less
// environment degrades to a relative "complete" directory in the current
// working directory rather than failing.
func DataDir() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			base = "."
		} else {
			base = filepath.Join(home, ".local", "share")
		}
	}
	return filepath.Join(base, "complete")
}

// DBPath returns the path of the SQLite store file — DataDir()/complete.db.
func DBPath() string {
	return filepath.Join(DataDir(), "complete.db")
}
