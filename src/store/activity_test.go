package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newActivityStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "activity_test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestClaimWorkIdempotentForSameAgent(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	id1, err := s.ClaimWork("task", tid, "a1", ActivityWorking)
	if err != nil {
		t.Fatalf("first ClaimWork: %v", err)
	}

	id2, err := s.ClaimWork("task", tid, "a1", ActivityWorking)
	if err != nil {
		t.Fatalf("second ClaimWork (same agent): %v", err)
	}
	if id1 != id2 {
		t.Fatalf("same-agent claim should return same id, got %q then %q", id1, id2)
	}

	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 1 {
		t.Fatalf("expected 1 active claim, got %d", len(work))
	}
	if work[0].AgentID != "a1" {
		t.Fatalf("expected agent a1, got %q", work[0].AgentID)
	}
}

func TestClaimWorkConflictsAcrossAgents(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	if _, err := s.ClaimWork("task", tid, "a1", ActivityWorking); err != nil {
		t.Fatalf("first ClaimWork: %v", err)
	}

	_, err := s.ClaimWork("task", tid, "a2", ActivityWorking)
	if !errors.Is(err, ErrActivityConflict) {
		t.Fatalf("expected ErrActivityConflict, got %v", err)
	}

	// Original claim still holds.
	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 1 || work[0].AgentID != "a1" {
		t.Fatalf("expected agent a1 to still hold, got %v", work)
	}
}

func TestReleaseWorkRemovesClaim(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	if _, err := s.ClaimWork("task", tid, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}

	if err := s.ReleaseWork("task", tid, "a1"); err != nil {
		t.Fatalf("ReleaseWork: %v", err)
	}

	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 0 {
		t.Fatalf("expected 0 active claims after release, got %d", len(work))
	}
}

func TestReleaseWorkIsNoOpWhenUnclaimed(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	// Releasing an entity that was never claimed must not error.
	if err := s.ReleaseWork("task", tid, "a1"); err != nil {
		t.Fatalf("ReleaseWork on unclaimed entity: %v", err)
	}
}

func TestListWorkExcludesStale(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	if _, err := s.ClaimWork("task", tid, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}

	// Simulate age by pushing acquired_at well into the past.
	staleTime := time.Now().Add(-WorkTTL - time.Minute).Unix()
	if _, err := s.db.Exec(
		`UPDATE AgentActivity SET acquired_at = ? WHERE entity_type = 'task' AND entity_id = ?`,
		staleTime, tid,
	); err != nil {
		t.Fatalf("stale-update: %v", err)
	}

	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 0 {
		t.Fatalf("expected 0 active claims for stale row, got %d", len(work))
	}

	// PruneStaleWork should delete the stale row.
	n, err := s.PruneStaleWork(time.Now().Unix())
	if err != nil {
		t.Fatalf("PruneStaleWork: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected PruneStaleWork to delete 1 row, got %d", n)
	}
}

func TestPruneStaleWorkDeletesOldRows(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	if _, err := s.ClaimWork("task", tid, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}

	// Make the row stale.
	staleTime := time.Now().Add(-WorkTTL - time.Minute).Unix()
	if _, err := s.db.Exec(
		`UPDATE AgentActivity SET acquired_at = ? WHERE entity_type = 'task' AND entity_id = ?`,
		staleTime, tid,
	); err != nil {
		t.Fatalf("stale-update: %v", err)
	}

	n, err := s.PruneStaleWork(time.Now().Unix())
	if err != nil {
		t.Fatalf("PruneStaleWork: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 deleted row, got %d", n)
	}

	// Confirm the row is actually gone from the table.
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM AgentActivity`).Scan(&count); err != nil {
		t.Fatalf("COUNT: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows after prune, got %d", count)
	}
}

func TestClaimWorkValidatesEntityType(t *testing.T) {
	s := newActivityStore(t)
	_, err := s.ClaimWork("invalid", "x", "a1", ActivityWorking)
	if err == nil {
		t.Fatal("expected error for invalid entity type")
	}
}

func TestClaimWorkValidatesNotFound(t *testing.T) {
	s := newActivityStore(t)
	_, err := s.ClaimWork("task", "nonexistent", "a1", ActivityWorking)
	if err == nil {
		t.Fatal("expected error for nonexistent entity")
	}
}

func TestClaimWorkValidatesEmptyAgent(t *testing.T) {
	s := newActivityStore(t)
	_, err := s.ClaimWork("task", "x", "", ActivityWorking)
	if err == nil {
		t.Fatal("expected error for empty agent_id")
	}
}

func TestClaimWorkValidatesKind(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	_, err := s.ClaimWork("task", tid, "a1", ActivityKind("invalid"))
	if err == nil {
		t.Fatal("expected error for invalid kind")
	}
}

func TestClaimWorkKindDefault(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	// Empty kind should default to "working".
	if _, err := s.ClaimWork("task", tid, "a1", ""); err != nil {
		t.Fatalf("ClaimWork with empty kind: %v", err)
	}
	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 1 || work[0].Kind != ActivityWorking {
		t.Fatalf("expected kind=working, got %v", work)
	}
}

func TestClaimWorkListEntity(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")

	id, err := s.ClaimWork("list", lid, "a1", ActivityWorking)
	if err != nil {
		t.Fatalf("ClaimWork on list: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty activity id")
	}

	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 1 || work[0].EntityType != "list" || work[0].EntityID != lid {
		t.Fatalf("unexpected work: %v", work)
	}
}

func TestReleaseWorkWrongAgentNoOp(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	if _, err := s.ClaimWork("task", tid, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}

	// Releasing with a different agent id should be a no-op (not an error,
	// not a removal of a1's claim).
	if err := s.ReleaseWork("task", tid, "a2"); err != nil {
		t.Fatalf("ReleaseWork with wrong agent: %v", err)
	}

	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 1 || work[0].AgentID != "a1" {
		t.Fatalf("a1's claim should still exist, got %v", work)
	}
}

func TestPruneStaleWorkKeepsFresh(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	if _, err := s.ClaimWork("task", tid, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}

	n, err := s.PruneStaleWork(time.Now().Unix())
	if err != nil {
		t.Fatalf("PruneStaleWork: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 deletions for fresh claim, got %d", n)
	}
}
