package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
)

// legacyThemeRE matches the four pre-rename palette names as they appear in
// a saved config.yaml (theme: crush-ember, etc.). The migration rewrites
// them to a theme that still exists in the registry, so a saved theme keeps
// working after the rename instead of silently falling back to the default.
// crush-ember and crush-slate both fold onto farol-dusk - farol-ember and
// farol-slate were retired (docs/DESIGN.md §11) and farol-dusk is the
// closest surviving relative: a second dark amber variant.
var legacyThemeRE = regexp.MustCompile(`crush-(dark|ember|slate|day)`)

// legacyThemeReplacement maps one crush-* match to its farol-* equivalent.
func legacyThemeReplacement(match []byte) []byte {
	switch string(match) {
	case "crush-ember", "crush-slate":
		return []byte("farol-dusk")
	default:
		return []byte("farol-" + string(match[len("crush-"):]))
	}
}

// MigrateLegacyDirs is a one-shot, first-launch migration that runs after
// the chore-crusher → farol rename. It moves a pre-rename
// $XDG_CONFIG_HOME/chore-crusher (or ~/.config/chore-crusher) config dir
// and $XDG_DATA_HOME/chore-crusher (or ~/.local/share/chore-crusher) data
// dir to their farol names, renames chore-crusher.db (plus its WAL/SHM
// siblings) to farol.db inside the moved data dir, and rewrites pre-rename
// theme values in the moved config.yaml.
//
// Call it before the store opens and before the config file is read — the
// CLI and the TUI share this one call (src/cli/root.go, the MCP server).
// It is idempotent: once the legacy dirs are gone, later launches are
// no-ops. It NEVER overwrites: if both the legacy and the farol dirs exist
// it leaves both alone, logs a warning, and keeps going.
func MigrateLegacyDirs() error {
	legCfg, err := legacyConfigDir()
	if err != nil {
		// No HOME to resolve ~/.config from; configDir() will fail the
		// same way and the caller surfaces it. Nothing to migrate.
		log.Printf("farol: skipping config migration (%v)", err)
	} else {
		curCfg, err := configDir()
		if err != nil {
			return err
		}
		if err := migrateDirPair(legCfg, curCfg, rewriteConfigTheme); err != nil {
			return err
		}
	}
	return migrateDirPair(legacyDataDir(), DataDir(), renameLegacyDB)
}

// migrateDirPair moves a legacy chore-crusher directory to its current
// farol name. Rules:
//
//   - legacy exists, current absent → os.Rename the whole directory, then
//     run afterMove on the moved dir (DB rename, theme rewrite);
//   - both exist → leave both untouched and log a warning (never
//     overwrite; the user may have created the new dirs deliberately);
//   - legacy absent → no-op.
//
// A non-NotExist stat or rename error is returned — the caller treats it
// as fatal so the app never silently starts against an empty store while
// the user's data still sits under the old name.
func migrateDirPair(legacy, current string, afterMove func(dir string) error) error {
	legacyExists, err := pathExists(legacy)
	if err != nil {
		return err
	}
	currentExists, err := pathExists(current)
	if err != nil {
		return err
	}

	switch {
	case legacyExists && currentExists:
		log.Printf("farol: both %s and %s exist; leaving both untouched (no data moved)", legacy, current)
		return nil
	case legacyExists:
		if err := os.Rename(legacy, current); err != nil {
			return fmt.Errorf("migrate %s -> %s: %w", legacy, current, err)
		}
		if afterMove != nil {
			if err := afterMove(current); err != nil {
				return fmt.Errorf("migrate %s -> %s: %w", legacy, current, err)
			}
		}
		log.Printf("farol: moved %s -> %s", legacy, current)
		return nil
	default:
		return nil
	}
}

// pathExists reports whether path exists, distinguishing "not found" from
// real stat errors.
func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat %s: %w", path, err)
}

// renameLegacyDB renames chore-crusher.db and its WAL/SHM siblings to
// farol.db* inside a freshly moved data dir. The WAL and SHM files carry
// live pages for an uncheckpointed store, so they must move with the DB
// name or SQLite would start the renamed file with a half-applied WAL.
func renameLegacyDB(dir string) error {
	for _, ext := range []string{".db", ".db-wal", ".db-shm"} {
		old := filepath.Join(dir, "chore-crusher"+ext)
		exists, err := pathExists(old)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if err := os.Rename(old, filepath.Join(dir, "farol"+ext)); err != nil {
			return fmt.Errorf("rename %s: %w", old, err)
		}
	}
	return nil
}

// rewriteConfigTheme rewrites pre-rename theme values in the config.yaml
// inside a freshly moved config dir. A file with no crush-* theme (or no
// config file at all) is left exactly as it is.
func rewriteConfigTheme(dir string) error {
	path := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	updated := legacyThemeRE.ReplaceAllFunc(data, legacyThemeReplacement)
	if string(updated) == string(data) {
		return nil
	}
	return os.WriteFile(path, updated, 0o644)
}
