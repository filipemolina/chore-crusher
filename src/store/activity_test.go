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

func TestTouchWorkRefreshesLiveClaim(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	if _, err := s.ClaimWork("task", tid, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}

	// Age the claim within the TTL window; a touch must push acquired_at forward.
	aged := time.Now().Add(-30 * time.Second).Unix()
	if _, err := s.db.Exec(
		`UPDATE AgentActivity SET acquired_at = ? WHERE entity_type = 'task' AND entity_id = ?`,
		aged, tid,
	); err != nil {
		t.Fatalf("age claim: %v", err)
	}

	if err := s.TouchWork("task", tid, "a1"); err != nil {
		t.Fatalf("TouchWork: %v", err)
	}

	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 1 {
		t.Fatalf("expected 1 active claim, got %d", len(work))
	}
	if work[0].AcquiredAt <= aged {
		t.Fatalf("TouchWork should refresh acquired_at (was %d, now %d)", aged, work[0].AcquiredAt)
	}
}

func TestTouchWorkNoOpWithoutClaim(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	if err := s.TouchWork("task", tid, "a1"); err != nil {
		t.Fatalf("TouchWork on unclaimed entity: %v", err)
	}

	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 0 {
		t.Fatalf("TouchWork must not create a claim, got %v", work)
	}
}

func TestTouchWorkNoOpForOtherAgent(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	if _, err := s.ClaimWork("task", tid, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}
	aged := time.Now().Add(-30 * time.Second).Unix()
	if _, err := s.db.Exec(
		`UPDATE AgentActivity SET acquired_at = ? WHERE entity_type = 'task' AND entity_id = ?`,
		aged, tid,
	); err != nil {
		t.Fatalf("age claim: %v", err)
	}

	if err := s.TouchWork("task", tid, "a2"); err != nil {
		t.Fatalf("TouchWork: %v", err)
	}

	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(work))
	}
	if work[0].AcquiredAt != aged {
		t.Fatalf("a2's touch must not touch a1's claim (acquired_at %d, want %d)", work[0].AcquiredAt, aged)
	}
}

func TestTouchWorkDoesNotReviveStale(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	if _, err := s.ClaimWork("task", tid, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}
	stale := time.Now().Add(-WorkTTL - time.Minute).Unix()
	if _, err := s.db.Exec(
		`UPDATE AgentActivity SET acquired_at = ? WHERE entity_type = 'task' AND entity_id = ?`,
		stale, tid,
	); err != nil {
		t.Fatalf("age claim: %v", err)
	}

	if err := s.TouchWork("task", tid, "a1"); err != nil {
		t.Fatalf("TouchWork: %v", err)
	}

	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 0 {
		t.Fatalf("TouchWork must not revive a stale claim, got %v", work)
	}
}

func TestTouchWorkValidatesEntityType(t *testing.T) {
	s := newActivityStore(t)
	if err := s.TouchWork("invalid", "x", "a1"); err == nil {
		t.Fatal("expected error for invalid entity type")
	}
}

func TestClaimedTaskListIDs(t *testing.T) {
	s := newActivityStore(t)
	lid1 := mustList(t, s, "one")
	lid2 := mustList(t, s, "two")
	tid1 := mustTask(t, s, lid1, "t1", nil)
	tid2 := mustTask(t, s, lid2, "t2", nil)

	// No claims yet — empty set.
	set, err := s.ClaimedTaskListIDs()
	if err != nil {
		t.Fatalf("ClaimedTaskListIDs: %v", err)
	}
	if len(set) != 0 {
		t.Fatalf("expected empty set, got %v", set)
	}

	// Claim a task in list one.
	if _, err := s.ClaimWork("task", tid1, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}
	set, err = s.ClaimedTaskListIDs()
	if err != nil {
		t.Fatalf("ClaimedTaskListIDs: %v", err)
	}
	if len(set) != 1 || !set[lid1] {
		t.Fatalf("expected only list one, got %v", set)
	}

	// A list-level claim on list one must not add it again (task claims only),
	// and a task claim in list two must add list two.
	if _, err := s.ClaimWork("list", lid1, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork list: %v", err)
	}
	if _, err := s.ClaimWork("task", tid2, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork task two: %v", err)
	}
	set, err = s.ClaimedTaskListIDs()
	if err != nil {
		t.Fatalf("ClaimedTaskListIDs: %v", err)
	}
	if len(set) != 2 || !set[lid1] || !set[lid2] {
		t.Fatalf("expected lists one and two, got %v", set)
	}
}

func TestClaimedTaskListIDsCountsNestedTasks(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "one")
	parent := mustTask(t, s, lid, "parent", nil)
	child := mustTask(t, s, lid, "child", &parent)

	if _, err := s.ClaimWork("task", child, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork child: %v", err)
	}

	set, err := s.ClaimedTaskListIDs()
	if err != nil {
		t.Fatalf("ClaimedTaskListIDs: %v", err)
	}
	if len(set) != 1 || !set[lid] {
		t.Fatalf("a nested task claim must count toward its list, got %v", set)
	}
}

func TestClaimedTaskListIDsExcludesStale(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "one")
	tid := mustTask(t, s, lid, "t1", nil)

	if _, err := s.ClaimWork("task", tid, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}
	stale := time.Now().Add(-WorkTTL - time.Minute).Unix()
	if _, err := s.db.Exec(
		`UPDATE AgentActivity SET acquired_at = ? WHERE entity_type = 'task' AND entity_id = ?`,
		stale, tid,
	); err != nil {
		t.Fatalf("age claim: %v", err)
	}

	set, err := s.ClaimedTaskListIDs()
	if err != nil {
		t.Fatalf("ClaimedTaskListIDs: %v", err)
	}
	if len(set) != 0 {
		t.Fatalf("stale task claim must be excluded, got %v", set)
	}
}

// TestReleaseAgentClaimsLeavesOtherAgentsAlone verifies that releasing claims
// for one agent leaves other agents' claims intact, and returns the count of
// claims released for the target agent.
func TestReleaseAgentClaimsLeavesOtherAgentsAlone(t *testing.T) {
	s := newActivityStore(t)
	lid1 := mustList(t, s, "list1")
	lid2 := mustList(t, s, "list2")
	tid1 := mustTask(t, s, lid1, "task1", nil)
	tid2 := mustTask(t, s, lid2, "task2", nil)

	// Agent a1 claims a task and a list
	if _, err := s.ClaimWork("task", tid1, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}
	if _, err := s.ClaimWork("list", lid1, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}
	// Agent a2 claims a task and a list
	if _, err := s.ClaimWork("task", tid2, "a2", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}
	if _, err := s.ClaimWork("list", lid2, "a2", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}

	// Release a1's claims
	n, err := s.ReleaseAgentClaims("a1")
	if err != nil {
		t.Fatalf("ReleaseAgentClaims: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 deleted rows for agent a1, got %d", n)
	}

	// a2's claims should still exist
	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 2 {
		t.Fatalf("expected 2 active claims (agent a2's), got %d", len(work))
	}
	for _, w := range work {
		if w.AgentID != "a2" {
			t.Fatalf("expected agent a2's claims, got %v", work)
		}
	}

	// Calling on an agent with no claims is a no-op, not an error.
	if n2, err := s.ReleaseAgentClaims("a1"); err != nil || n2 != 0 {
		t.Fatalf("empty ReleaseAgentClaims = (%d, %v), want (0, nil)", n2, err)
	}
}

// TestReleaseAgentClaimsRejectsEmptyAgent verifies that an empty agentID
// returns an error and deletes nothing.
func TestReleaseAgentClaimsRejectsEmptyAgent(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	// Create a claim so we can verify nothing gets deleted
	if _, err := s.ClaimWork("task", tid, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}

	n, err := s.ReleaseAgentClaims("")
	if err == nil {
		t.Fatalf("ReleaseAgentClaims with empty agentID expected error, got (%d, nil)", n)
	}
	if n != 0 {
		t.Fatalf("ReleaseAgentClaims with empty agentID should delete 0 rows, got %d", n)
	}

	// Original claim should still exist
	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 1 || work[0].AgentID != "a1" {
		t.Fatalf("expected original claim to still exist, got %v", work)
	}
}

// TestReleaseAgentClaimsClearsOwnClaims verifies the session-end cleanup
// (hardening plan H13): when the MCP process shuts down, the agent's own claims—regardless
// of staleness—are deleted so the TUI does not show lingering spinners
// for that agent, while other agents' claims remain.
// Retargeted: a released agent's claims go regardless of staleness
// (unlike PruneStaleWork).
func TestReleaseAgentClaimsClearsOwnClaims(t *testing.T) {
	s := newActivityStore(t)
	lid := mustList(t, s, "list")
	tid := mustTask(t, s, lid, "task", nil)

	// Two claims: one fresh, one stale, from the same agent.
	if _, err := s.ClaimWork("task", tid, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}
	if _, err := s.ClaimWork("list", lid, "a1", ActivityWorking); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}
	// Force one row stale so PruneStaleWork would normally skip it.
	stale := time.Now().Add(-WorkTTL - time.Minute).Unix()
	if _, err := s.db.Exec(
		`UPDATE AgentActivity SET acquired_at = ? WHERE entity_type = 'task' AND entity_id = ?`,
		stale, tid,
	); err != nil {
		t.Fatalf("age claim: %v", err)
	}

	n, err := s.ReleaseAgentClaims("a1")
	if err != nil {
		t.Fatalf("ReleaseAgentClaims: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 deleted rows, got %d", n)
	}

	work, err := s.ListWork()
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(work) != 0 {
		t.Fatalf("expected 0 active claims after release, got %d", len(work))
	}

	// Calling on an agent with no claims is a no-op, not an error.
	if n2, err := s.ReleaseAgentClaims("a1"); err != nil || n2 != 0 {
		t.Fatalf("empty ReleaseAgentClaims = (%d, %v), want (0, nil)", n2, err)
	}
}
