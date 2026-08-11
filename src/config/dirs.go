// Package config reads and writes ~/.config/farol/config.yaml (or
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

// dataBase returns the XDG data root the app's data dir hangs off:
// $XDG_DATA_HOME when set, ~/.local/share otherwise, degrading to "." when
// HOME is unresolvable (docs/DESIGN.md §8).
func dataBase() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			base = "."
		} else {
			base = filepath.Join(home, ".local", "share")
		}
	}
	return base
}

// DataDir returns the directory that owns this app's data files:
// $XDG_DATA_HOME/farol, or ~/.local/share/farol when XDG_DATA_HOME is
// unset (docs/DESIGN.md §8). store.Open creates it on first use; a HOME-less
// environment degrades to a relative "farol" directory in the current
// working directory rather than failing.
func DataDir() string {
	return filepath.Join(dataBase(), "farol")
}

// legacyDataDir returns where the pre-rename data lived
// ($XDG_DATA_HOME/chore-crusher). MigrateLegacyDirs moves it to DataDir()
// on first launch after the rename.
func legacyDataDir() string {
	return filepath.Join(dataBase(), "chore-crusher")
}

// DBPath returns the path of the SQLite store file — DataDir()/farol.db.
func DBPath() string {
	return filepath.Join(DataDir(), "farol.db")
}
