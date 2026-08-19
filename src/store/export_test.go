package store

import (
	"testing"
)

func TestExportImportRoundTrip(t *testing.T) {
	s := newTestStore(t)

	listID, err := s.CreateList("Groceries", "pi")
	if err != nil {
		t.Fatal(err)
	}
	parent, err := s.CreateTask(listID, "Produce", nil, "buy fresh")
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.CreateTask(listID, "Apples", &parent, "red ones")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPriority(child, "high"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddComment(child, "pi", "get a kilo"); err != nil {
		t.Fatal(err)
	}

	doc, err := s.Export(nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if doc.Version != ExportVersion {
		t.Fatalf("version = %d, want %d", doc.Version, ExportVersion)
	}
	if len(doc.Lists) != 1 || doc.Lists[0].Name != "Groceries" || doc.Lists[0].CreatedBy != "pi" {
		t.Fatalf("export lists = %+v", doc.Lists)
	}
	if len(doc.Lists[0].Tasks) != 2 {
		t.Fatalf("export tasks = %d, want 2", len(doc.Lists[0].Tasks))
	}

	// Import into a fresh store and verify structure + content survive.
	s2 := newTestStore(t)
	if err := s2.ImportList(doc.Lists[0]); err != nil {
		t.Fatalf("ImportList: %v", err)
	}
	lists, err := s2.ListLists()
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || lists[0].Name != "Groceries" || lists[0].CreatedBy != "pi" {
		t.Fatalf("imported lists = %+v", lists)
	}
	tasks, err := s2.ListTasks(lists[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("imported tasks = %d, want 2", len(tasks))
	}
	// Find the child by title and verify its parent is the imported parent.
	var gotChild, gotParent *Task
	for i := range tasks {
		if tasks[i].Title == "Apples" {
			gotChild = &tasks[i]
		}
		if tasks[i].Title == "Produce" {
			gotParent = &tasks[i]
		}
	}
	if gotChild == nil || gotParent == nil {
		t.Fatalf("missing tasks after import: %+v", tasks)
	}
	if gotChild.ParentID == nil || *gotChild.ParentID != gotParent.ID {
		t.Fatalf("child parent_id = %v, want %q", gotChild.ParentID, gotParent.ID)
	}
	if gotChild.Priority != PriorityHigh {
		t.Fatalf("child priority = %q, want high", gotChild.Priority)
	}
	comments, err := s2.ListComments(gotChild.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].Note != "get a kilo" {
		t.Fatalf("imported comments = %+v", comments)
	}
}

// TestExportIncludesArchivedLists: a whole-store export must not silently
// drop archived lists, even though ListLists (the query most callers use)
// excludes them by default.
func TestExportIncludesArchivedLists(t *testing.T) {
	s := newTestStore(t)
	active, err := s.CreateList("Active", "pi")
	if err != nil {
		t.Fatal(err)
	}
	archived, err := s.CreateList("Archived", "pi")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ArchiveList(archived); err != nil {
		t.Fatalf("ArchiveList: %v", err)
	}

	doc, err := s.Export(nil)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(doc.Lists) != 2 {
		t.Fatalf("export lists = %d, want 2 (active + archived)", len(doc.Lists))
	}
	var gotActive, gotArchived *ExportList
	for i := range doc.Lists {
		switch doc.Lists[i].ID {
		case active:
			gotActive = &doc.Lists[i]
		case archived:
			gotArchived = &doc.Lists[i]
		}
	}
	if gotActive == nil || gotArchived == nil {
		t.Fatalf("export missing a list: %+v", doc.Lists)
	}
	if gotActive.ArchivedAt != nil {
		t.Errorf("active list ArchivedAt = %v, want nil", gotActive.ArchivedAt)
	}
	if gotArchived.ArchivedAt == nil {
		t.Error("archived list ArchivedAt = nil, want a timestamp")
	}

	// Round-trip: importing must restore the archived state, not just the
	// list's other fields.
	s2 := newTestStore(t)
	if err := s2.ImportList(*gotArchived); err != nil {
		t.Fatalf("ImportList: %v", err)
	}
	// ImportList assigns a fresh id, so look the list up via the
	// include-archived path since ListLists hides it by default.
	imported, err := listListsIncludingArchived(s2.db)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported) != 1 || imported[0].ArchivedAt == nil {
		t.Fatalf("imported list = %+v, want one archived list", imported)
	}
}
