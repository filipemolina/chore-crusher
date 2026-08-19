package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// legacyFixture lays down a pre-rename chore-crusher config + data pair
// under the given XDG roots, the way the app stored them before the rename.
func legacyFixture(t *testing.T, cfgRoot, dataRoot string) (legCfgDir, legDataDir string) {
	t.Helper()
	legCfgDir = filepath.Join(cfgRoot, "chore-crusher")
	if err := os.MkdirAll(legCfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legCfgDir, "config.yaml"), []byte("theme: crush-ember\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	legDataDir = filepath.Join(dataRoot, "chore-crusher")
	if err := os.MkdirAll(legDataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A live store can carry WAL/SHM siblings; all three must move with
	// the DB name or SQLite would start the renamed file with a
	// half-applied WAL.
	for _, name := range []string{"chore-crusher.db", "chore-crusher.db-wal", "chore-crusher.db-shm"} {
		if err := os.WriteFile(filepath.Join(legDataDir, name), []byte("fake "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return legCfgDir, legDataDir
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s exists after migration, want it gone (err=%v)", path, err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("%s missing after migration: %v", path, err)
	}
}

// TestMigrateLegacyDirsMovesEverything is the happy path: legacy exists,
// farol absent → both dirs move, the DB (+ WAL/SHM) is renamed, and the
// saved theme is rewritten to the farol name.
func TestMigrateLegacyDirsMovesEverything(t *testing.T) {
	cfgRoot := t.TempDir()
	dataRoot := t.TempDir()
	legCfg, legData := legacyFixture(t, cfgRoot, dataRoot)
	t.Setenv("XDG_CONFIG_HOME", cfgRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)

	if err := MigrateLegacyDirs(); err != nil {
		t.Fatalf("MigrateLegacyDirs: %v", err)
	}

	// Old dirs gone, new dirs present.
	mustNotExist(t, legCfg)
	mustNotExist(t, legData)
	newCfg := filepath.Join(cfgRoot, "farol")
	newData := filepath.Join(dataRoot, "farol")
	mustExist(t, newCfg)
	mustExist(t, newData)

	// DB renamed (with WAL/SHM), old names gone.
	mustExist(t, filepath.Join(newData, "farol.db"))
	mustExist(t, filepath.Join(newData, "farol.db-wal"))
	mustExist(t, filepath.Join(newData, "farol.db-shm"))
	mustNotExist(t, filepath.Join(newData, "chore-crusher.db"))

	// Theme rewritten.
	cfg, err := os.ReadFile(filepath.Join(newCfg, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "theme: farol-dusk") {
		t.Errorf("config.yaml = %q, want theme rewritten to farol-dusk", cfg)
	}
	if strings.Contains(string(cfg), "crush-") {
		t.Errorf("config.yaml = %q, still contains a pre-rename theme name", cfg)
	}

	// Idempotent: a second launch is a no-op and changes nothing.
	if err := MigrateLegacyDirs(); err != nil {
		t.Fatalf("second MigrateLegacyDirs: %v", err)
	}
	mustExist(t, filepath.Join(newData, "farol.db"))
}

// TestMigrateLegacyDirsNoopWhenAbsent: no legacy dirs, nothing happens,
// and no farol dirs are created by the migration itself.
func TestMigrateLegacyDirsNoopWhenAbsent(t *testing.T) {
	cfgRoot := t.TempDir()
	dataRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)

	if err := MigrateLegacyDirs(); err != nil {
		t.Fatalf("MigrateLegacyDirs: %v", err)
	}
	mustNotExist(t, filepath.Join(cfgRoot, "farol"))
	mustNotExist(t, filepath.Join(dataRoot, "farol"))
}

// TestMigrateLegacyDirsBothExistWarnsNothingDestroyed: when both the
// legacy and the farol dirs exist, the migration must leave both untouched
// — no rename, no theme rewrite, no DB move.
func TestMigrateLegacyDirsBothExistWarnsNothingDestroyed(t *testing.T) {
	cfgRoot := t.TempDir()
	dataRoot := t.TempDir()
	legCfg, legData := legacyFixture(t, cfgRoot, dataRoot)
	t.Setenv("XDG_CONFIG_HOME", cfgRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)

	// A farol dir that already exists — e.g. a partial manual migration —
	// with its own distinct contents.
	newCfg := filepath.Join(cfgRoot, "farol")
	newData := filepath.Join(dataRoot, "farol")
	if err := os.MkdirAll(newCfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newCfg, "config.yaml"), []byte("theme: farol-day\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newData, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newData, "farol.db"), []byte("new store"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyDirs(); err != nil {
		t.Fatalf("MigrateLegacyDirs: %v", err)
	}

	// Both sides still exist with their original contents.
	mustExist(t, legCfg)
	mustExist(t, legData)
	mustExist(t, filepath.Join(legData, "chore-crusher.db"))
	mustExist(t, newCfg)
	mustExist(t, newData)
	mustExist(t, filepath.Join(newData, "farol.db"))

	cfg, err := os.ReadFile(filepath.Join(newCfg, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(cfg) != "theme: farol-day\n" {
		t.Errorf("existing farol config.yaml = %q, want untouched", cfg)
	}
}

// TestMigrateLegacyDirsThemeAlreadyFarol: a config that already uses the
// new names must survive the move byte-for-byte.
func TestMigrateLegacyDirsThemeAlreadyFarol(t *testing.T) {
	cfgRoot := t.TempDir()
	dataRoot := t.TempDir()
	legCfg, _ := legacyFixture(t, cfgRoot, dataRoot)
	// Overwrite the fixture's crush-ember config with one that already
	// names the farol theme.
	already := []byte("theme: farol-dusk\npoll_interval_ms: 2000\n")
	if err := os.WriteFile(filepath.Join(legCfg, "config.yaml"), already, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)

	if err := MigrateLegacyDirs(); err != nil {
		t.Fatalf("MigrateLegacyDirs: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(cfgRoot, "farol", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(already) {
		t.Errorf("config.yaml = %q, want byte-identical %q", got, already)
	}
}

// TestMigrateLegacyDirsPartialMove: config migrated, data side absent →
// the config side still migrates on its own (the pairs are independent).
func TestMigrateLegacyDirsPartialMove(t *testing.T) {
	cfgRoot := t.TempDir()
	dataRoot := t.TempDir()
	legCfg, _ := legacyFixture(t, cfgRoot, dataRoot)
	// Remove the legacy data side so only config exists.
	if err := os.RemoveAll(filepath.Join(dataRoot, "chore-crusher")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfgRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)

	if err := MigrateLegacyDirs(); err != nil {
		t.Fatalf("MigrateLegacyDirs: %v", err)
	}

	mustNotExist(t, legCfg)
	mustExist(t, filepath.Join(cfgRoot, "farol", "config.yaml"))
	mustNotExist(t, filepath.Join(dataRoot, "farol"))
}
