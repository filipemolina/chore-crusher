package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestImportWholeDocumentCreatesLists pins the additive whole-document
// import: an export file written by `farol export` recreates its lists in a
// fresh, empty store.
func TestImportWholeDocumentCreatesLists(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	// Build an export file first: a list the test agent owns can carry a
	// task (the ownership gate, parity 1.8).
	lid := strings.TrimSpace(mustCLI(t, data, "lists", "add", "Source", "--owner", "pi"))
	strings.TrimSpace(mustCLI(t, data, "add", lid, "Task A"))
	src := filepath.Join(data, "doc.json")
	mustCLI(t, data, "export", lid, "--out", src)

	// Import into a SECOND, empty data dir.
	data2 := t.TempDir()
	mustCLI(t, data2, "import", src)
	var lists []listJSON
	mustJSONCLI(t, data2, &lists, "lists", "--json")
	if len(lists) != 1 || lists[0].Name != "Source" {
		t.Fatalf("imported lists = %+v, want exactly [Source]", lists)
	}
}

// TestImportSingleListSelectsFromDocument pins the --list selector: importing
// a whole-store file with --list <id> creates only the matching list and
// ignores the rest.
func TestImportSingleListSelectsFromDocument(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FAROL_AGENT", "pi")
	// Document with two lists.
	a := strings.TrimSpace(mustCLI(t, data, "lists", "add", "ListA", "--owner", "pi"))
	b := strings.TrimSpace(mustCLI(t, data, "lists", "add", "ListB", "--owner", "pi"))
	strings.TrimSpace(mustCLI(t, data, "add", a, "In A"))
	strings.TrimSpace(mustCLI(t, data, "add", b, "In B"))
	src := filepath.Join(data, "doc.json")
	mustCLI(t, data, "export", "--out", src)

	data2 := t.TempDir()
	mustCLI(t, data2, "import", src, "--list", a)
	var lists []listJSON
	mustJSONCLI(t, data2, &lists, "lists", "--json")
	if len(lists) != 1 || lists[0].Name != "ListA" {
		t.Fatalf("single-list import = %+v, want exactly ListA", lists)
	}
}
