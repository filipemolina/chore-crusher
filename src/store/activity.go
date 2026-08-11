package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ActivityKind is what kind of work an agent claimed an entity for.
type ActivityKind string

const (
	ActivityWorking    ActivityKind = "working"
	ActivityInspecting ActivityKind = "inspecting"
)

// AgentActivity is one live claim that "an agent is on this entity." It
// mirrors one row of the AgentActivity table. EntityID is a logical FK:
// Task.id when EntityType is "task", List.id when "list".
type AgentActivity struct {
	ID         string
	EntityType string // "task" | "list"
	EntityID   string
	AgentID    string
	Kind       ActivityKind
	AcquiredAt int64
}

// WorkTTL is how long a claim stays "live" after the last ClaimWork. The TUI
// only shows claims acquired within this window; the app prunes older
// rows opportunistically. A dead agent's spinner vanishes after this.
const WorkTTL = 120 * time.Second

// ErrActivityConflict is returned when an agent tries to claim an entity that
// is already held by a different agent.
var ErrActivityConflict = errors.New("entity is claimed by another agent")

// ClaimWork records that agentID is working on entityID (a task or list).
// It is idempotent for the same agent (renews acquired_at = now, a
// heartbeat) and returns ErrActivityConflict if a different agent holds it.
// The returned string is the activity row's ULID.
func (s *Store) ClaimWork(entityType, entityID, agentID string, kind ActivityKind) (string, error) {
	switch entityType {
	case "task", "list":
	default:
		return "", fmt.Errorf("claim work: unknown entity type %q (must be \"task\" or \"list\")", entityType)
	}
	if agentID == "" {
		return "", fmt.Errorf("claim work: agent_id must not be empty")
	}
	if kind == "" {
		kind = ActivityWorking
	}
	if kind != ActivityWorking && kind != ActivityInspecting {
		return "", fmt.Errorf("claim work: unknown kind %q (must be \"working\" or \"inspecting\")", kind)
	}
	if entityID == "" {
		return "", fmt.Errorf("claim work: entity_id must not be empty")
	}

	// Validate the entity actually exists.
	if err := s.validateEntityExists(entityType, entityID); err != nil {
		return "", err
	}

	// Opportunistically prune stale rows so the table never grows without bound.
	if _, err := s.PruneStaleWork(time.Now().Unix()); err != nil {
		return "", fmt.Errorf("claim work: prune stale: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	// Check whether the entity is already claimed.
	var existingID, existingAgent string
	err = tx.QueryRow(
		`SELECT id, agent_id FROM AgentActivity WHERE entity_type = ? AND entity_id = ?`,
		entityType, entityID,
	).Scan(&existingID, &existingAgent)

	if err == nil {
		// Entity is already claimed.
		if existingAgent != agentID {
			return "", ErrActivityConflict
		}
		// Same agent — refresh the heartbeat.
		if _, err := tx.Exec(
			`UPDATE AgentActivity SET acquired_at = ?, kind = ? WHERE id = ?`,
			time.Now().Unix(), kind, existingID,
		); err != nil {
			return "", err
		}
		return existingID, tx.Commit()
	}
	if !isNoRows(err) {
		return "", err
	}

	// No existing claim — insert a new one.
	id := NewID()
	if _, err := tx.Exec(
		`INSERT INTO AgentActivity (id, entity_type, entity_id, agent_id, kind, acquired_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, entityType, entityID, agentID, kind, time.Now().Unix(),
	); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

// ReleaseWork removes the claim on entityID if agentID holds it. A no-op
// (nil error) when nobody claims it, so an agent can release without
// remembering whether it actually claimed.
func (s *Store) ReleaseWork(entityType, entityID, agentID string) error {
	switch entityType {
	case "task", "list":
	default:
		return fmt.Errorf("release work: unknown entity type %q (must be \"task\" or \"list\")", entityType)
	}

	if _, err := s.PruneStaleWork(time.Now().Unix()); err != nil {
		return fmt.Errorf("release work: prune stale: %w", err)
	}

	// Only delete the row if agentID matches (or there is no claim at all).
	res, err := s.db.Exec(
		`DELETE FROM AgentActivity WHERE entity_type = ? AND entity_id = ? AND agent_id = ?`,
		entityType, entityID, agentID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	// n == 0 is fine: no matching claim to release (the agent may not have
	// claimed it, or the claim already expired).
	_ = n
	return nil
}

// TouchWork extends a live claim held by the same agent — a write-heartbeat:
// the agent just wrote to this entity. It never creates a claim, never
// touches another agent's, and never revives an expired row (the WHERE
// acquired_at >= cutoff guard): a stale claim only comes back via ClaimWork
// (a re-claim). The TUI renders a claim only while acquired_at is within
// WorkTTL, so extending it keeps the spinner alive through continuous work.
func (s *Store) TouchWork(entityType, entityID, agentID string) error {
	switch entityType {
	case "task", "list":
	default:
		return fmt.Errorf("touch work: unknown entity type %q (must be \"task\" or \"list\")", entityType)
	}
	cutoff := time.Now().Add(-WorkTTL).Unix()
	_, err := s.db.Exec(
		`UPDATE AgentActivity SET acquired_at = ?
		 WHERE entity_type = ? AND entity_id = ? AND agent_id = ? AND acquired_at >= ?`,
		time.Now().Unix(), entityType, entityID, agentID, cutoff,
	)
	return err
}

// ClaimedTaskListIDs returns the set of list ids that have at least one live
// task claim (any task in the list — nested or root, every Task row carries
// its ListID). The lists panel uses it to show a spinner on a list row when
// an agent is working inside it. List-level claims are excluded; the
// caller renders those from ListWork.
func (s *Store) ClaimedTaskListIDs() (map[string]bool, error) {
	cutoff := time.Now().Add(-WorkTTL).Unix()
	rows, err := s.db.Query(
		`SELECT DISTINCT t.list_id
		 FROM AgentActivity a JOIN Task t ON t.id = a.entity_id
		 WHERE a.entity_type = 'task' AND a.acquired_at >= ?`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// ListWork returns claims whose acquired_at is within WorkTTL of now —
// the set the TUI renders a spinner for. Stale rows are excluded by the
// WHERE, not deleted, so a briefly-quiet agent re-appears without a write.
func (s *Store) ListWork() ([]AgentActivity, error) {
	cutoff := time.Now().Add(-WorkTTL).Unix()
	rows, err := s.db.Query(
		`SELECT id, entity_type, entity_id, agent_id, kind, acquired_at
		 FROM AgentActivity WHERE acquired_at >= ?`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentActivity
	for rows.Next() {
		var a AgentActivity
		var kind string
		if err := rows.Scan(&a.ID, &a.EntityType, &a.EntityID, &a.AgentID, &kind, &a.AcquiredAt); err != nil {
			return nil, err
		}
		a.Kind = ActivityKind(kind)
		out = append(out, a)
	}
	return out, rows.Err()
}

// PruneStaleWork deletes claims older than WorkTTL. The CLI/TUI call it
// inside ClaimWork and ReleaseWork (one cheap DELETE) so the table never grows
// without bound; the TUI never deletes.
func (s *Store) PruneStaleWork(now int64) (int, error) {
	cutoff := now - int64(WorkTTL.Seconds())
	res, err := s.db.Exec(
		`DELETE FROM AgentActivity WHERE acquired_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// ReleaseAgentClaims removes rows from the AgentActivity table for the given agentID.
// It is called when an agent session ends so that
// the exiting agent's own spinners do not linger in the TUI beyond the process's lifetime.
// Unlike PruneStaleWork, this does not filter by WorkTTL — it clears all of the agent's
// claims regardless of staleness because the process that made them is going away.
//
// Decision 1: scoped by agent_id. Two concurrent sessions running under the same
// FAROL_AGENT tag still clear each other's claims on exit.
func (s *Store) ReleaseAgentClaims(agentID string) (int, error) {
	if agentID == "" {
		return 0, fmt.Errorf("release agent claims: agent_id must not be empty")
	}
	res, err := s.db.Exec(`DELETE FROM AgentActivity WHERE agent_id = ?`, agentID)
	if err != nil {
		return 0, fmt.Errorf("release agent claims: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// validateEntityExists checks that entityID refers to an existing row in the
// target table. It uses the same "SELECT 1 WHERE EXISTS" pattern the rest of
// the store uses (e.g. CreateTask checks list existence).
func (s *Store) validateEntityExists(entityType, entityID string) error {
	var tableName string
	switch entityType {
	case "task":
		tableName = "Task"
	case "list":
		tableName = "List"
	default:
		return fmt.Errorf("validate entity: unknown entity type %q", entityType)
	}
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM `+tableName+` WHERE id = ?`, entityID).Scan(&one)
	if err != nil {
		if isNoRows(err) {
			return fmt.Errorf("%s %q not found", entityType, entityID)
		}
		return err
	}
	return nil
}

// activityColumns is shared by every query that reads a full AgentActivity row.
const activityColumns = `id, entity_type, entity_id, agent_id, kind, acquired_at`

// scanActivity converts one result row into an AgentActivity.
func scanActivity(r rowScanner) (AgentActivity, error) {
	var a AgentActivity
	var kind string
	err := r.Scan(&a.ID, &a.EntityType, &a.EntityID, &a.AgentID, &kind, &a.AcquiredAt)
	if err != nil {
		return AgentActivity{}, err
	}
	a.Kind = ActivityKind(kind)
	return a, nil
}

// unused-but-kept-for-consistency: scanActivity is used by ListWork inline
// but also serves as the canonical scanner for any future read path.
var _ = scanActivity

// unused-but-kept-for-consistency: isNoRows is already defined in store.go;
// we use it above but do not redeclare it.
var _ = isNoRows(sql.ErrNoRows)
