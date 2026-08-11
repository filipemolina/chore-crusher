package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filipemolina/farol/src/appstyles"
)

// runCLI executes the root command with args against a store rooted at
// dataDir (one temp dir per test gives one shared database across the calls
// of a sequence, like the plan's manual verification) and returns the exit
// code and captured stdout/stderr. XDG_DATA_HOME is pinned to dataDir so the
// commands resolve the store exactly where the test put it (docs/DESIGN.md
// §8); stdout/stderr are redirected through pipes so a test asserts on the
// exact bytes a real caller would see.
func runCLI(t *testing.T, dataDir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", dataDir)
	// The migration (config.MigrateLegacyDirs) runs before the store
	// opens in production paths, so every CLI invocation in tests must
	// point the CONFIG side at a scratch dir too — otherwise the suite
	// migrates (or warns about) the real ~/.config on this machine.
	// A test that already pinned XDG_CONFIG_HOME (e.g. the saved-theme
	// round trip) keeps its dir.
	if os.Getenv("XDG_CONFIG_HOME") == "" {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	}

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	code = Execute(args)
	os.Stdout, os.Stderr = oldOut, oldErr
	outW.Close()
	errW.Close()

	out, err := io.ReadAll(outR)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	er, err := io.ReadAll(errR)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return code, string(out), string(er)
}

// mustCLI runs the command, failing the test unless it exits 0, and returns
// stdout. Write commands that print an id are the common caller.
func mustCLI(t *testing.T, dataDir string, args ...string) string {
	t.Helper()
	code, out, errOut := runCLI(t, dataDir, args...)
	if code != 0 {
		t.Fatalf("%v: exit %d, stderr %q", args, code, errOut)
	}
	return out
}

// mustJSONCLI runs the command in --json mode, fails unless it exits 0, and
// returns the parsed payload.
func mustJSONCLI(t *testing.T, dataDir string, payload any, args ...string) {
	t.Helper()
	code, out, errOut := runCLI(t, dataDir, args...)
	if code != 0 {
		t.Fatalf("%v: exit %d, stderr %q", args, code, errOut)
	}
	if err := json.Unmarshal([]byte(out), payload); err != nil {
		t.Fatalf("%v: stdout %q is not the expected JSON: %v", args, out, err)
	}
}

// TestExecuteExitCodes pins the whole exit-code contract (docs/DESIGN.md
// §9): 0 success, 1 domain failure, 2 usage error. Cobra's own flag/arg
// errors and an unknown subcommand must fall through to 2, not be remapped.
func TestExecuteExitCodes(t *testing.T) {
	data := t.TempDir()

	if code, _, _ := runCLI(t, data, "--version"); code != 0 {
		t.Errorf("--version: exit %d, want 0", code)
	}
	if _, out, _ := runCLI(t, data, "--version"); !strings.HasPrefix(out, "farol ") {
		t.Errorf("--version: stdout %q, want a 'farol <version>' line", out)
	}
	if code, _, _ := runCLI(t, data, "--help"); code != 0 {
		t.Errorf("--help: exit %d, want 0", code)
	}
	// TUI requires a TTY; in a test environment it will fail to launch.
	// This is expected behavior — the CLI is the interface for automation.
	code, _, _ := runCLI(t, data)
	if code != 1 {
		t.Errorf("no subcommand: exit %d, want 1 (TUI requires TTY)", code)
	}

	// Usage errors: missing argument, unknown flag, unknown subcommand.
	if code, _, _ := runCLI(t, data, "tasks"); code != 2 {
		t.Errorf("tasks (no args): exit %d, want 2", code)
	}
	if code, _, _ := runCLI(t, data, "--bogus-flag"); code != 2 {
		t.Errorf("unknown flag: exit %d, want 2", code)
	}
	if code, _, _ := runCLI(t, data, "frobnicate"); code != 2 {
		t.Errorf("unknown subcommand: exit %d, want 2", code)
	}
	if code, _, _ := runCLI(t, data, "progress", "01ARZ"); code != 2 {
		t.Errorf("progress without --mode: exit %d, want 2 (missing required flag)", code)
	}

	// Domain failure: an id that does not resolve.
	if code, _, _ := runCLI(t, data, "show", "01ARZ"); code != 1 {
		t.Errorf("show of missing task: exit %d, want 1", code)
	}
	if code, _, _ := runCLI(t, data, "tasks", "01ARZ"); code != 1 {
		t.Errorf("tasks of missing list: exit %d, want 1", code)
	}
}

// TestSavedThemeAppliedAtStartup pins the read half of the theme round
// trip: the picker's Enter writes theme: to config.yaml (cmds.ApplyTheme),
// and the TUI boot path must apply it — without the read half a chosen
// theme dies with the process. The TUI launch itself fails for lack of a
// TTY (exit 1, as TestExecuteExitCodes pins), but only after the saved
// theme has been activated, so the assertion on the global Active theme
// holds on the failure path too.
func TestSavedThemeAppliedAtStartup(t *testing.T) {
	data := t.TempDir()
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	dir := filepath.Join(cfgHome, "farol")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("theme: catppuccin-mocha\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	appstyles.SetTheme(appstyles.DefaultTheme)
	defer appstyles.SetTheme(appstyles.DefaultTheme)

	if code, _, _ := runCLI(t, data); code != 1 {
		t.Fatalf("TUI launch: exit %d, want 1 (no TTY)", code)
	}
	if appstyles.Active.Name != "catppuccin-mocha" {
		t.Errorf("Active theme = %q, want the saved %q", appstyles.Active.Name, "catppuccin-mocha")
	}
}

// TestJSONErrorShape pins the §9 error contract: in --json mode a failure is
// exactly one JSON value on stdout — {"error": "..."} — with the message off
// stdout's only stream, so a caller parses one stream and reads the exit
// code to know which shape it got.
func TestJSONErrorShape(t *testing.T) {
	data := t.TempDir()
	code, out, errOut := runCLI(t, data, "show", "nope", "--json")
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if errOut != "" {
		t.Errorf("stderr must stay empty in --json mode, got %q", errOut)
	}
	var parsed struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout %q is not one JSON value: %v", out, err)
	}
	if !strings.Contains(parsed.Error, "task") {
		t.Errorf("error %q should name the failing lookup", parsed.Error)
	}
}

// TestHumanErrorGoesToStderr pins the human-mode failure shape: one
// "farol: ..." line on stderr, nothing on stdout, exit 1.
func TestHumanErrorGoesToStderr(t *testing.T) {
	data := t.TempDir()
	code, out, errOut := runCLI(t, data, "show", "nope")
	if code != 1 || out != "" {
		t.Errorf("exit %d stdout %q, want exit 1 with empty stdout", code, out)
	}
	if !strings.HasPrefix(errOut, "farol: ") {
		t.Errorf("stderr %q, want a 'farol: ' prefix", errOut)
	}
}

// TestVersionMatchesPhaseZero keeps the phase-0 output shape ("farol
// <version>") now that Cobra owns the flag.
func TestVersionMatchesPhaseZero(t *testing.T) {
	data := t.TempDir()
	_, out, _ := runCLI(t, data, "--version")
	if !strings.HasPrefix(out, "farol ") || strings.Contains(out, "version") {
		t.Errorf("--version output %q should be 'farol <version>' (phase-0 shape)", out)
	}
}
