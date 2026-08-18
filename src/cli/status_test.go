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

// runStatusCLI executes the status command directly, bypassing root
// registration: `farol status` is unregistered until the orchestrator adds it
// to root.go, so the tests drive newStatusCmd() themselves. It pins the same
// XDG env runCLI does and captures stdout/stderr the same way, so the
// assertions read exactly what a real caller would see. The --json flag is
// root.go's persistent flag, declared here on the bare command because it has
// no root to inherit from.
func runStatusCLI(t *testing.T, dataDir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", dataDir)
	if os.Getenv("XDG_CONFIG_HOME") == "" {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	}

	cmd := newStatusCmd()
	cmd.Flags().Bool("json", false,
		"emit exactly one JSON value on stdout, success or failure (docs/DESIGN.md §9)")
	cmd.SetArgs(args)

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
	err = cmd.Execute()
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
	// Map the error the way Execute does (docs/DESIGN.md §9): a domain
	// failure is 1, anything else 2.
	if err != nil {
		var de *domainErr
		if errors.As(err, &de) {
			return 1, string(out), string(er)
		}
		return 2, string(out), string(er)
	}
	return 0, string(out), string(er)
}

// TestStatusJSONShape pins `farol status --json` against a store with one
// list and one task in each status: the counts must be exact, the store file
// must be non-empty, the last migration must be the schema's, and the config
// path must be the resolved farol/config.yaml under the pinned
// XDG_CONFIG_HOME.
func TestStatusJSONShape(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")

	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	inprog := strings.TrimSpace(mustCLI(t, data, "add", lid, "in-progress task"))
	mustCLI(t, data, "progress", inprog, "--mode", "percentage", "--percent", "50")
	done := strings.TrimSpace(mustCLI(t, data, "add", lid, "done task"))
	mustCLI(t, data, "complete", done)
	mustCLI(t, data, "add", lid, "pending task")

	code, out, errOut := runStatusCLI(t, data, "--json")
	if code != 0 {
		t.Fatalf("status --json: exit %d, stderr %q", code, errOut)
	}
	var st statusJSON
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("stdout %q is not the status JSON: %v", out, err)
	}

	if st.Lists != 1 {
		t.Errorf("lists = %d, want 1", st.Lists)
	}
	if st.Tasks != 3 {
		t.Errorf("tasks = %d, want 3", st.Tasks)
	}
	if st.Pending != 1 || st.InProgress != 1 || st.Complete != 1 {
		t.Errorf("status counts = %d pending / %d in_progress / %d complete, want 1/1/1",
			st.Pending, st.InProgress, st.Complete)
	}
	if st.StoreSize <= 0 {
		t.Errorf("store_size_bytes = %d, want > 0", st.StoreSize)
	}
	if st.LastMigration < 1 {
		t.Errorf("last_migration = %d, want at least 1 (0001_init.sql)", st.LastMigration)
	}
	if !strings.HasSuffix(st.ConfigPath, filepath.Join("farol", "config.yaml")) {
		t.Errorf("config_path = %q, want a farol/config.yaml path", st.ConfigPath)
	}
}

// TestStatusEmptyStoreIsNormal pins that an empty store is a normal state,
// not an error: exit 0, a labeled human readout with zero counts, and a
// --json object of all-zero counts.
func TestStatusEmptyStoreIsNormal(t *testing.T) {
	data := t.TempDir()

	code, out, errOut := runStatusCLI(t, data)
	if code != 0 {
		t.Fatalf("status on an empty store: exit %d, stderr %q", code, errOut)
	}
	// Unlike the list-returning reads, status never prints nothing: zero
	// counts are still a report.
	if !strings.Contains(out, "Lists:") || !strings.Contains(out, "Tasks:") {
		t.Errorf("human status = %q, want the labeled summary lines", out)
	}

	code, out, errOut = runStatusCLI(t, data, "--json")
	if code != 0 {
		t.Fatalf("status --json on an empty store: exit %d, stderr %q", code, errOut)
	}
	var st statusJSON
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("stdout %q is not the status JSON: %v", out, err)
	}
	if st.Lists != 0 || st.Tasks != 0 || st.Pending != 0 || st.InProgress != 0 || st.Complete != 0 {
		t.Errorf("empty-store status = %+v, want all-zero counts", st)
	}
}

// TestStatusJSONIsOneValue pins the §9 contract mechanically: --json writes
// exactly one JSON value to stdout — a single object — nothing around it.
func TestStatusJSONIsOneValue(t *testing.T) {
	data := t.TempDir()
	code, out, errOut := runStatusCLI(t, data, "--json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("json status stdout = %q, want a single object starting with '{'", out)
	}
	var v any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Errorf("stdout %q is not one JSON value: %v", out, err)
	}
}

// TestStatusHumanReadout pins the human-mode labels so the readout is
// greppable and stable.
func TestStatusHumanReadout(t *testing.T) {
	data := t.TempDir()
	code, out, errOut := runStatusCLI(t, data)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	for _, label := range []string{"Lists:", "Tasks:", "Pending:", "In progress:", "Complete:", "Store size:", "Last migration:", "Config:"} {
		if !strings.Contains(out, label) {
			t.Errorf("human status = %q, want a %q line", out, label)
		}
	}
}

// TestHumanBytes pins the byte-count formatting for the human readout.
func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{1023, "1023B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024*1024 + 512*1024, "1.5 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
