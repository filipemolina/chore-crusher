package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withConfigHome points XDG_CONFIG_HOME at dir for the test's lifetime,
// so LoadConfig/SaveConfig read from there instead of the real home.
func withConfigHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", dir)
}

func TestLoadConfigMissingFile(t *testing.T) {
	withConfigHome(t, t.TempDir())

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("missing config file should not error: %v", err)
	}
	if cfg.Theme != "" {
		t.Errorf("missing config should yield empty theme, got %q", cfg.Theme)
	}
}

func TestRoundTrip(t *testing.T) {
	home := t.TempDir()
	withConfigHome(t, home)

	original := Config{Theme: "farol-slate", PollIntervalMs: 500}
	if err := SaveConfig(original); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.Theme != original.Theme {
		t.Errorf("round-tripped theme = %q, want %q", loaded.Theme, original.Theme)
	}
	if loaded.PollIntervalMs != original.PollIntervalMs {
		t.Errorf("round-tripped poll_interval_ms = %d, want %d", loaded.PollIntervalMs, original.PollIntervalMs)
	}

	// The file should live at $XDG_CONFIG_HOME/chore-crusher/config.yaml.
	path := filepath.Join(home, "chore-crusher", "config.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not found at %s: %v", path, err)
	}
}

func TestLoadConfigMalformed(t *testing.T) {
	home := t.TempDir()
	withConfigHome(t, home)

	dir := filepath.Join(home, "chore-crusher")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("[invalid"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := LoadConfig()
	if err == nil {
		t.Error("malformed config should return an error")
	}
}

func TestSaveConfigCreatesDirectory(t *testing.T) {
	home := t.TempDir()
	withConfigHome(t, home)

	dir := filepath.Join(home, "chore-crusher")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected %s to not exist yet", dir)
	}

	if err := SaveConfig(Config{Theme: "farol-dark"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("SaveConfig should have created %s: %v", dir, err)
	}
}

func TestSaveConfigOverwrites(t *testing.T) {
	home := t.TempDir()
	withConfigHome(t, home)

	if err := SaveConfig(Config{Theme: "farol-slate"}); err != nil {
		t.Fatalf("first SaveConfig: %v", err)
	}
	if err := SaveConfig(Config{Theme: "farol-ember"}); err != nil {
		t.Fatalf("second SaveConfig: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.Theme != "farol-ember" {
		t.Errorf("overwritten theme = %q, want %q", loaded.Theme, "farol-ember")
	}
}
