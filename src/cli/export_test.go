package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filipemolina/farol/src/store"
)

// TestExportSingleListToFile pins the file-writing path: `farol export
// <list-id> --out <file>` writes one list with its tasks to the file, and
// the file parses to the versioned ExportDocument shape (§9).
func TestExportSingleListToFile(t *testing.T) {
	data := t.TempDir()
	// A list owned by the test agent can carry tasks (the ownership gate,
	// parity 1.8, refuses structural writes on an untagged list).
	t.Setenv("FAROL_AGENT", "pi")
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Work", "--owner", "pi"))
	strings.TrimSpace(mustCLI(t, data, "add", lid, "Write plan", "--notes", "draft it"))
	// Parent first, then a child under it — `farol add` prints the new id
	// (§9), so it is captured inline like every other CLI test.
	parent := strings.TrimSpace(mustCLI(t, data, "add", lid, "Parent"))
	strings.TrimSpace(mustCLI(t, data, "add", lid, "Sub", "--parent", parent))

	out := filepath.Join(data, "out.json")
	mustCLI(t, data, "export", lid, "--out", out)

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	var doc store.ExportDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse export file: %v", err)
	}
	if doc.Version != store.ExportVersion {
		t.Fatalf("version = %d, want %d", doc.Version, store.ExportVersion)
	}
	if len(doc.Lists) != 1 || doc.Lists[0].Name != "Work" {
		t.Fatalf("exported lists = %+v, want exactly [Work]", doc.Lists)
	}
	if len(doc.Lists[0].Tasks) != 3 {
		t.Fatalf("exported tasks = %d, want 3", len(doc.Lists[0].Tasks))
	}
}

// TestExportWholeStoreJSON pins the --json path: `farol export --json` with
// no list id emits exactly one JSON value on stdout carrying the version
// field, with two lists when two exist.
func TestExportWholeStoreJSON(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	strings.TrimSpace(mustCLI(t, data, "lists", "add", "ListA", "--owner", "pi"))
	strings.TrimSpace(mustCLI(t, data, "lists", "add", "ListB", "--owner", "pi"))

	var doc store.ExportDocument
	mustJSONCLI(t, data, &doc, "export", "--json")
	if doc.Version != store.ExportVersion {
		t.Fatalf("version = %d, want %d", doc.Version, store.ExportVersion)
	}
	if len(doc.Lists) != 2 {
		t.Fatalf("exported lists = %d, want 2", len(doc.Lists))
	}
}
