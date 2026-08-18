package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filipemolina/farol/src/config"
	"github.com/filipemolina/farol/src/store"
)

// runCLIStdin runs the CLI with the given bytes available on stdin — the
// same redirection runCLI applies to stdout and stderr, via a regular file
// so io.ReadAll sees a clean EOF and the test needs no goroutine.
func runCLIStdin(t *testing.T, dataDir, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("create stdin file: %v", err)
	}
	if _, err := f.WriteString(stdin); err != nil {
		t.Fatalf("write stdin file: %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek stdin file: %v", err)
	}
	oldIn := os.Stdin
	os.Stdin = f
	defer func() {
		os.Stdin = oldIn
		f.Close()
	}()
	return runCLI(t, dataDir, args...)
}

// listAttachments opens the store at dataDir and returns the task's
// attachments (the CLI's own `farol attachments` re-reads the same rows, but
// opening the store directly is the tighter assertion).
func listAttachments(t *testing.T, dataDir, taskID string) []store.Attachment {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", dataDir)
	s, err := store.Open(config.DBPath())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()
	atts, err := s.ListAttachments(taskID)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	return atts
}

// attachmentFiles returns the names materialized under
// $XDG_DATA_HOME/farol/attachments, so a test can assert no orphaned files
// were left behind by a failed attach.
func attachmentFiles(t *testing.T, dataDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dataDir, "farol", "attachments"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadDir attachments dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestAttachFromStdin pins `cat file.txt | farol attach <task-id> -`: the
// stdin bytes are materialized as a file under the data dir and that path is
// what the store records.
func TestAttachFromStdin(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	content := "attached via stdin\nsecond line\n"
	code, out, errOut := runCLIStdin(t, data, content, "attach", tid, "-")
	if code != 0 {
		t.Fatalf("attach -: exit %d, stderr %q", code, errOut)
	}
	aid := strings.TrimSpace(out)
	if aid == "" {
		t.Fatal("attach - printed no attachment id")
	}

	atts := listAttachments(t, data, tid)
	if len(atts) != 1 || atts[0].ID != aid {
		t.Fatalf("attachments = %+v, want one row with id %s", atts, aid)
	}
	if !filepath.HasPrefix(atts[0].Path, filepath.Join(data, "farol", "attachments")) {
		t.Errorf("attachment path %q, want it under the data-dir attachments folder", atts[0].Path)
	}
	got, err := os.ReadFile(atts[0].Path)
	if err != nil {
		t.Fatalf("read attached file: %v", err)
	}
	if string(got) != content {
		t.Errorf("attached file content = %q, want the exact stdin bytes %q", got, content)
	}
}

// TestAttachStdinJSON pins the §9 one-value contract on the new stdin path:
// --json emits exactly {"id": ...}.
func TestAttachStdinJSON(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	code, out, errOut := runCLIStdin(t, data, "json content", "attach", tid, "-", "--json")
	if code != 0 || errOut != "" {
		t.Fatalf("attach - --json: exit %d stderr %q, want exit 0 with empty stderr", code, errOut)
	}
	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("stdout %q is not one JSON value: %v", out, err)
	}
	if res.ID == "" {
		t.Errorf("attach --json = %q, want a non-empty id", out)
	}
}

// TestAttachPathOptionalReadsStdin pins the "path argument optional when
// stdin is provided" half of the contract: `farol attach <task-id>` with no
// source reads stdin, same as an explicit "-".
func TestAttachPathOptionalReadsStdin(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	code, _, errOut := runCLIStdin(t, data, "piped stdin", "attach", tid)
	if code != 0 {
		t.Fatalf("attach with no source: exit %d, stderr %q", code, errOut)
	}
	atts := listAttachments(t, data, tid)
	if len(atts) != 1 {
		t.Fatalf("attachments = %+v, want one", atts)
	}
	got, err := os.ReadFile(atts[0].Path)
	if err != nil || string(got) != "piped stdin" {
		t.Errorf("attached content = %q (err %v), want the piped bytes", got, err)
	}
}

// TestAttachFromURL pins `farol attach <task-id> https://...`: the URL is
// downloaded, stored under the attachment dir named after the URL's basename,
// and that path is recorded.
func TestAttachFromURL(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	body := "the pdf bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	var res struct {
		ID string `json:"id"`
	}
	mustJSONCLI(t, data, &res, "attach", tid, srv.URL+"/file.pdf", "--json")
	if res.ID == "" {
		t.Fatal("attach URL --json emitted an empty id")
	}

	atts := listAttachments(t, data, tid)
	if len(atts) != 1 || atts[0].ID != res.ID {
		t.Fatalf("attachments = %+v, want one row with id %s", atts, res.ID)
	}
	if !strings.HasSuffix(atts[0].Path, "file.pdf") {
		t.Errorf("attachment path = %q, want it to keep the URL's basename file.pdf", atts[0].Path)
	}
	got, err := os.ReadFile(atts[0].Path)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != body {
		t.Errorf("downloaded content = %q, want %q", got, body)
	}
}

// TestAttachURLFailure pins that a failed download is a domain error (exit
// 1) in both modes, creates no attachment row, and leaves no orphaned file.
func TestAttachURLFailure(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	// Human mode: one "farol: ..." line on stderr, nothing on stdout.
	code, out, errOut := runCLI(t, data, "attach", tid, srv.URL+"/missing.pdf")
	if code != 1 || out != "" {
		t.Fatalf("attach 404: exit %d stdout %q, want exit 1 with empty stdout", code, out)
	}
	if !strings.Contains(errOut, "HTTP 404") {
		t.Errorf("stderr %q, want it to mention HTTP 404", errOut)
	}

	// JSON mode: exactly one JSON error value on stdout, empty stderr.
	code, out, errOut = runCLI(t, data, "attach", tid, srv.URL+"/missing.pdf", "--json")
	if code != 1 || errOut != "" {
		t.Fatalf("attach 404 --json: exit %d stderr %q, want exit 1 with empty stderr", code, errOut)
	}
	var errPayload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &errPayload); err != nil {
		t.Fatalf("stdout %q is not one JSON value: %v", out, err)
	}
	if !strings.Contains(errPayload.Error, "HTTP 404") {
		t.Errorf("error %q, want it to mention HTTP 404", errPayload.Error)
	}

	if atts := listAttachments(t, data, tid); len(atts) != 0 {
		t.Errorf("attachments after failed downloads = %+v, want none", atts)
	}
	if files := attachmentFiles(t, data); len(files) != 0 {
		t.Errorf("orphaned files after failed downloads: %v", files)
	}
}

// TestAttachRejectsUnsupportedScheme pins that only http/https are treated as
// URLs; anything else with a scheme is refused rather than silently stored as
// a path (or fetched through a scheme nothing supports).
func TestAttachRejectsUnsupportedScheme(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	code, _, errOut := runCLI(t, data, "attach", tid, "ftp://example.com/file.pdf")
	if code != 1 || !strings.Contains(errOut, "unsupported URL scheme") {
		t.Errorf("ftp source: exit %d stderr %q, want exit 1 naming the scheme", code, errOut)
	}
	if atts := listAttachments(t, data, tid); len(atts) != 0 {
		t.Errorf("attachments after refused scheme = %+v, want none", atts)
	}
}

// TestAttachLocalPathStoredAsIs is the regression guard for the original
// contract: a plain local path is stored exactly as given, nothing copied.
func TestAttachLocalPathStoredAsIs(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	src := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(src, []byte("local file"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	mustCLI(t, data, "attach", tid, src)

	atts := listAttachments(t, data, tid)
	if len(atts) != 1 || atts[0].Path != src {
		t.Fatalf("attachments = %+v, want path %s stored as-is", atts, src)
	}
}

// TestAttachArgErrors pins the argument contract: exactly one or two
// positionals (task-id alone means stdin), and a failed insert cleans up the
// file it materialized instead of orphaning it.
func TestAttachArgErrors(t *testing.T) {
	data := t.TempDir()

	// No arguments is a usage error (exit 2), Cobra's own.
	if code, _, _ := runCLI(t, data, "attach"); code != 2 {
		t.Errorf("attach with no args: exit %d, want 2", code)
	}

	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "l", "--owner", "pi"))
	tid := strings.TrimSpace(mustCLI(t, data, "add", lid, "task"))

	// Three arguments is a usage error too.
	if code, _, _ := runCLI(t, data, "attach", tid, "a", "b"); code != 2 {
		t.Errorf("attach with 3 args: exit %d, want 2", code)
	}

	// Attaching to a task that does not resolve is a domain error (exit 1) —
	// and the stdin bytes were materialized first, so the file must not be
	// left behind.
	code, _, errOut := runCLIStdin(t, data, "x", "attach", "01ARZ", "-")
	if code != 1 || !strings.Contains(errOut, "not found") {
		t.Errorf("attach to missing task: exit %d stderr %q, want exit 1 mentioning not found", code, errOut)
	}
	if files := attachmentFiles(t, data); len(files) != 0 {
		t.Errorf("orphaned files after failed attach: %v", files)
	}
}
