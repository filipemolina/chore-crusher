package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runConfigCLI executes the config command tree in isolation and returns the
// exit code and captured stdout/stderr, mirroring runCLI's contract. The
// config group is registered into root.go by the orchestrator separately
// from this change, so these tests drive the tree directly rather than
// through NewRootCommand — they must pass before that registration lands.
// The tree declares the --json flag the root command would supply as a
// persistent flag (docs/DESIGN.md §9), and the exit-code mapping mirrors
// Execute() exactly: 0 success, 1 domain failure, 2 usage error.
func runConfigCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	if os.Getenv("XDG_CONFIG_HOME") == "" {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	}

	cmd := newConfigCmd()
	// The tree's root is already named "config", so a leading "config" in
	// the args (the way a caller types it) is dropped before SetArgs.
	if len(args) > 0 && args[0] == "config" {
		args = args[1:]
	}
	cmd.SetArgs(args)
	cmd.PersistentFlags().Bool("json", false,
		"emit exactly one JSON value on stdout (docs/DESIGN.md §9)")

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
	runErr := cmd.Execute()
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
	code = 0
	if runErr != nil {
		var de *domainErr
		if errors.As(runErr, &de) {
			code = 1
		} else {
			code = 2
		}
	}
	return code, string(out), string(er)
}

// mustConfigCLI runs the config command, failing the test unless it exits 0.
func mustConfigCLI(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errOut := runConfigCLI(t, args...)
	if code != 0 {
		t.Fatalf("%v: exit %d, stderr %q", args, code, errOut)
	}
	return out
}

// mustConfigJSONCLI runs the config command in --json mode, fails unless it
// exits 0, and returns the parsed payload.
func mustConfigJSONCLI(t *testing.T, payload any, args ...string) {
	t.Helper()
	code, out, errOut := runConfigCLI(t, args...)
	if code != 0 {
		t.Fatalf("%v: exit %d, stderr %q", args, code, errOut)
	}
	if err := json.Unmarshal([]byte(out), payload); err != nil {
		t.Fatalf("%v: stdout %q is not the expected JSON: %v", args, out, err)
	}
}

// writeConfig writes a config.yaml body under cfgHome (which the test must
// have pinned with t.Setenv("XDG_CONFIG_HOME", cfgHome) first) and returns
// the file's path.
func writeConfig(t *testing.T, cfgHome, body string) string {
	t.Helper()
	dir := filepath.Join(cfgHome, "farol")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// TestConfigGetHuman pins the human output: a stored value prints the value
// alone, one line, nothing else.
func TestConfigGetHuman(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	writeConfig(t, cfgHome, "theme: farol-dusk\n")

	if out := mustConfigCLI(t, "config", "get", "theme"); out != "farol-dusk\n" {
		t.Errorf("config get theme = %q, want %q", out, "farol-dusk\n")
	}
	if out := mustConfigCLI(t, "config", "get", "poll_interval_ms"); out != "1000\n" {
		t.Errorf("config get poll_interval_ms = %q, want %q (default)", out, "1000\n")
	}
}

// TestConfigGetEffectiveDefaults pins the §8 fallback: with no config file
// at all, get reports the compiled defaults (theme = farol-dusk,
// poll_interval_ms = 1000) rather than empty or an error — a missing file is
// a normal first-run state.
func TestConfigGetEffectiveDefaults(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if out := mustConfigCLI(t, "config", "get", "theme"); out != "farol-dusk\n" {
		t.Errorf("get theme (no file) = %q, want %q", out, "farol-dusk\n")
	}

	var theme struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	mustConfigJSONCLI(t, &theme, "config", "get", "theme", "--json")
	if theme.Key != "theme" || theme.Value != "farol-dusk" {
		t.Errorf("get theme --json = %+v, want {key:theme value:farol-dusk}", theme)
	}

	// poll_interval_ms is a number in the file, so it is a number here too —
	// not a quoted string.
	var ms struct {
		Key   string `json:"key"`
		Value int    `json:"value"`
	}
	mustConfigJSONCLI(t, &ms, "config", "get", "poll_interval_ms", "--json")
	if ms.Key != "poll_interval_ms" || ms.Value != 1000 {
		t.Errorf("get poll_interval_ms --json = %+v, want {key:poll_interval_ms value:1000}", ms)
	}
}

// TestConfigGetJSONExactShape pins the get payload byte-for-byte: one JSON
// value, the key echoed next to its value.
func TestConfigGetJSONExactShape(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	writeConfig(t, cfgHome, "poll_interval_ms: 500\n")

	code, out, errOut := runConfigCLI(t, "config", "get", "poll_interval_ms", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if want := `{"key":"poll_interval_ms","value":500}`; strings.TrimSpace(out) != want {
		t.Errorf("config get --json = %s, want %s", strings.TrimSpace(out), want)
	}
}

// TestConfigSetRoundTrip drives set then get end to end: what was written is
// what comes back, and a successful set prints nothing in human mode (the
// write-commands-print-nothing rule, §9).
func TestConfigSetRoundTrip(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if out := mustConfigCLI(t, "config", "set", "theme", "farol-dusk"); out != "" {
		t.Errorf("set theme printed %q, want nothing in human mode", out)
	}
	if out := mustConfigCLI(t, "config", "get", "theme"); out != "farol-dusk\n" {
		t.Errorf("get theme after set = %q, want %q", out, "farol-dusk\n")
	}

	if out := mustConfigCLI(t, "config", "set", "poll_interval_ms", "500"); out != "" {
		t.Errorf("set poll_interval_ms printed %q, want nothing in human mode", out)
	}
	if out := mustConfigCLI(t, "config", "get", "poll_interval_ms"); out != "500\n" {
		t.Errorf("get poll_interval_ms after set = %q, want %q", out, "500\n")
	}
}

// TestConfigSetPreservesOtherField pins the LoadConfig → SaveConfig whole-
// struct round trip: setting one key must not drop the other from the file
// (docs/DESIGN.md §8).
func TestConfigSetPreservesOtherField(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	writeConfig(t, cfgHome, "theme: farol-dusk\n")

	mustConfigCLI(t, "config", "set", "poll_interval_ms", "250")

	data, err := os.ReadFile(writeConfigPath(t, cfgHome))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "farol-dusk") {
		t.Errorf("saved config %q dropped the theme on a poll_interval_ms set", s)
	}
	if !strings.Contains(s, "250") {
		t.Errorf("saved config %q is missing the new poll_interval_ms", s)
	}
}

func writeConfigPath(t *testing.T, cfgHome string) string {
	t.Helper()
	return filepath.Join(cfgHome, "farol", "config.yaml")
}

// TestConfigSetJSONShape pins the set payload: {ok, key, value}, echoing the
// row that landed so a caller never needs a follow-up get (§9).
func TestConfigSetJSONShape(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	var res struct {
		OK    bool   `json:"ok"`
		Key   string `json:"key"`
		Value int    `json:"value"`
	}
	mustConfigJSONCLI(t, &res, "config", "set", "poll_interval_ms", "750", "--json")
	if !res.OK || res.Key != "poll_interval_ms" || res.Value != 750 {
		t.Errorf("set poll_interval_ms --json = %+v, want {ok:true key:poll_interval_ms value:750}", res)
	}

	// The value is a number in JSON, not a quoted string.
	code, out, errOut := runConfigCLI(t, "config", "set", "theme", "farol-dusk", "--json")
	if code != 0 {
		t.Fatalf("set theme --json: exit %d, stderr %q", code, errOut)
	}
	if want := `{"ok":true,"key":"theme","value":"farol-dusk"}`; strings.TrimSpace(out) != want {
		t.Errorf("set theme --json = %s, want %s", strings.TrimSpace(out), want)
	}
}

// TestConfigListJSON pins the list payload: an array of the same
// key/value rows get returns, in the file's canonical order (theme, then
// poll_interval_ms), carrying the effective values.
func TestConfigListJSON(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	writeConfig(t, cfgHome, "theme: farol-dusk\npoll_interval_ms: 250\n")

	var entries []struct {
		Key   string `json:"key"`
		Value any    `json:"value"`
	}
	mustConfigJSONCLI(t, &entries, "config", "list", "--json")
	if len(entries) != 2 {
		t.Fatalf("config list --json = %+v, want 2 entries", entries)
	}
	if entries[0].Key != "theme" || entries[0].Value != "farol-dusk" {
		t.Errorf("entry 0 = %+v, want theme/farol-dusk", entries[0])
	}
	// JSON numbers unmarshal into any as float64.
	if entries[1].Key != "poll_interval_ms" || entries[1].Value != float64(250) {
		t.Errorf("entry 1 = %+v, want poll_interval_ms/250", entries[1])
	}
}

// TestConfigListJSONExactShape pins the whole payload byte-for-byte — field
// order and number-vs-string typing included — so a future caller has one
// string to match against.
func TestConfigListJSONExactShape(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	writeConfig(t, cfgHome, "theme: farol-dusk\npoll_interval_ms: 250\n")

	code, out, errOut := runConfigCLI(t, "config", "list", "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	want := `[{"key":"theme","value":"farol-dusk"},{"key":"poll_interval_ms","value":250}]`
	if strings.TrimSpace(out) != want {
		t.Errorf("config list --json = %s, want %s", strings.TrimSpace(out), want)
	}
}

// TestConfigListHuman pins the human rendering: a KEY/VALUE header and one
// row per key, plain text (no ANSI, §9).
func TestConfigListHuman(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	writeConfig(t, cfgHome, "theme: farol-dusk\npoll_interval_ms: 250\n")

	out := mustConfigCLI(t, "config", "list")
	if !strings.Contains(out, "KEY") || !strings.Contains(out, "VALUE") {
		t.Errorf("config list header = %q, want KEY VALUE", out)
	}
	if !strings.Contains(out, "theme") || !strings.Contains(out, "farol-dusk") {
		t.Errorf("config list = %q, want a theme/farol-dusk row", out)
	}
	if !strings.Contains(out, "poll_interval_ms") || !strings.Contains(out, "250") {
		t.Errorf("config list = %q, want a poll_interval_ms/250 row", out)
	}
}

// TestConfigUnknownKey pins the unknown-key failure as a domain error (exit
// 1) that names the supported keys, for both get and set.
func TestConfigUnknownKey(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if code, _, errOut := runConfigCLI(t, "config", "get", "bogus"); code != 1 {
		t.Errorf("get bogus: exit %d, want 1", code)
	} else if !strings.Contains(errOut, "theme") || !strings.Contains(errOut, "poll_interval_ms") {
		t.Errorf("get bogus stderr %q, want the supported keys named", errOut)
	}

	if code, _, _ := runConfigCLI(t, "config", "set", "bogus", "x"); code != 1 {
		t.Errorf("set bogus: exit %d, want 1", code)
	}
}

// TestConfigSetInvalidValues pins the per-key validation: poll_interval_ms
// must be a positive integer, theme must not be empty. Each is a domain
// error (exit 1), never a silent write.
func TestConfigSetInvalidValues(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	for _, args := range [][]string{
		{"config", "set", "poll_interval_ms", "abc"},
		{"config", "set", "poll_interval_ms", "0"},
		// A bare -5 would be parsed as a flag (usage error, exit 2); the
		// -- separator makes it a positional argument so the domain
		// validation is what rejects it.
		{"config", "set", "poll_interval_ms", "--", "-5"},
		{"config", "set", "theme", ""},
	} {
		if code, _, _ := runConfigCLI(t, args...); code != 1 {
			t.Errorf("%v: exit %d, want 1", args, code)
		}
	}

	// None of the rejected writes may have landed.
	if out := mustConfigCLI(t, "config", "get", "poll_interval_ms"); out != "1000\n" {
		t.Errorf("get poll_interval_ms after rejected sets = %q, want %q (default)", out, "1000\n")
	}
}

// TestConfigJSONErrorShape pins the §9 error contract for the config group:
// in --json mode a failure is exactly one JSON value on stdout —
// {"error": "..."} — with stderr empty, exit 1.
func TestConfigJSONErrorShape(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	code, out, errOut := runConfigCLI(t, "config", "get", "bogus", "--json")
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
	if !strings.Contains(parsed.Error, "theme") {
		t.Errorf("error %q should name the supported keys", parsed.Error)
	}
}

// TestConfigUsageErrorExitCode pins that cobra's own argument-count failures
// stay at exit 2, not remapped — the config group must not break the §9
// exit-code contract when it is registered under the real root.
func TestConfigUsageErrorExitCode(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	if code, _, _ := runConfigCLI(t, "config", "get"); code != 2 {
		t.Errorf("get with no args: exit %d, want 2", code)
	}
	if code, _, _ := runConfigCLI(t, "config", "set", "theme"); code != 2 {
		t.Errorf("set with one arg: exit %d, want 2", code)
	}
	if code, _, _ := runConfigCLI(t, "config", "list", "extra"); code != 2 {
		t.Errorf("list with an extra arg: exit %d, want 2", code)
	}
}
