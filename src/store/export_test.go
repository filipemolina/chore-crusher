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
