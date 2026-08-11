package config

import (
	"path/filepath"
	"testing"
)

// TestDBPathHonorsXDG pins the store location contract (docs/DESIGN.md §8):
// XDG_DATA_HOME wins when set, HOME/.local/share is the fallback.
func TestDBPathHonorsXDG(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_DATA_HOME", data)
	if got := DBPath(); got != filepath.Join(data, "farol", "farol.db") {
		t.Errorf("DBPath with XDG_DATA_HOME: got %q, want the XDG path", got)
	}

	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	if got := DBPath(); got != filepath.Join(home, ".local", "share", "farol", "farol.db") {
		t.Errorf("DBPath without XDG_DATA_HOME: got %q, want the ~/.local/share path", got)
	}
}
