package cmds

import (
	"path/filepath"
	"testing"

	"github.com/filipemolina/farol/src/store"
)

// openTestStore opens a throwaway store in a per-test temp dir.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRefreshDetailsLoadsTask(t *testing.T) {
	s := openTestStore(t)
	listID, err := s.CreateList("Chores", "")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	taskID, err := s.CreateTask(listID, "Water plants", nil, "twice a week")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	msg, ok := RefreshDetails(s, taskID)().(RefreshDetailsMsg)
	if !ok {
		t.Fatalf("RefreshDetails produced %T, want RefreshDetailsMsg", msg)
	}
	if msg.Err != nil {
		t.Fatalf("unexpected error: %v", msg.Err)
	}
	if msg.TaskID != taskID {
		t.Errorf("TaskID = %q, want %q", msg.TaskID, taskID)
	}
	if msg.Task.Title != "Water plants" {
		t.Errorf("Task.Title = %q, want %q", msg.Task.Title, "Water plants")
	}
	if msg.Task.Notes != "twice a week" {
		t.Errorf("Task.Notes = %q, want %q", msg.Task.Notes, "twice a week")
	}
}

func TestRefreshDetailsReportsMissingTask(t *testing.T) {
	s := openTestStore(t)

	msg, ok := RefreshDetails(s, "does-not-exist")().(RefreshDetailsMsg)
	if !ok {
		t.Fatalf("RefreshDetails produced %T, want RefreshDetailsMsg", msg)
	}
	if msg.Err == nil {
		t.Fatal("expected an error for a missing task, got nil")
	}
	if msg.TaskID != "does-not-exist" {
		t.Errorf("TaskID = %q, want the requested id echoed back", msg.TaskID)
	}
}
