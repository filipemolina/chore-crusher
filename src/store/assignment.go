package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrAssigned is returned when a task is assigned to another agent. The
// wrapping error names the holder; the MCP layer turns that into the
// force=true hint.
var ErrAssigned = errors.New("task is assigned to another agent")

// ErrNoAssignable reports that NextAssignable found no eligible task. It is
// a normal outcome, not a failure — the MCP layer maps it to the
// {ok: false, reason: ...} shape (plan §4), never to a tool error.
var ErrNoAssignable = errors.New("no eligible task in this list")

// ErrSubtreeAssigned reports that the blocker is an ancestor or descendant,
// not the task itself. It wraps ErrAssigned, so a caller that only cares
// "somebody else holds this" still matches with errors.Is(err, ErrAssigned).
//
// The distinction matters to the MCP layer and nowhere else: force takes a
// task from its holder, but it does NOT override the subtree invariant
// (decision 4), so the "pass force=true to take it" hint is correct for
// ErrAssigned and WRONG for this. Tell the caller to release the named
// blocker instead (plan §4).
var ErrSubtreeAssigned = fmt.Errorf("%w (via its subtree)", ErrAssigned)

// ErrInvalidPriority is returned when SetPriority receives a value outside
// the four locked ones (plan decision 6): none, low, medium, high.
var ErrInvalidPriority = errors.New("invalid priority")

// SetPriority sets taskID's priority. Valid values are exactly the four
// locked by decision 6 — anything else is rejected with an error wrapping
// ErrInvalidPriority rather than stored, so the column can only ever hold
// a value priorityRank and NextAssignable understand.
func (s *Store) SetPriority(taskID string, p Priority) error {
	switch p {
	case PriorityNone, PriorityLow, PriorityMedium, PriorityHigh:
	default:
		return fmt.Errorf("%w: %q (want none, low, medium, or high)", ErrInvalidPriority, p)
	}
	res, err := s.db.Exec(
		`UPDATE Task SET priority = ?, updated_at = ? WHERE id = ?`,
		p, time.Now().Unix(), taskID,
	)
	if err != nil {
		return err
	}
	return requireAffected(res, "task", taskID)
}

// priorityRank orders priorities for NextAssignable: high > medium > low >
// none. The column is TEXT and its values collate the wrong way
// ("high" < "low" < "medium" < "none" alphabetically), so the rank is
// spelled out, not derived from the strings (plan §6.5, trap 1). Anything
// unrecognised sorts last, like none.
func priorityRank(p Priority) int {
	switch p {
	case PriorityHigh:
		return 0
	case PriorityMedium:
		return 1
	case PriorityLow:
		return 2
	default:
		return 3
	}
}

// AssignTask durably assigns taskID to agentID. Assignment has no TTL —
// only an explicit unassign or completion clears it (docs/DESIGN.md §3).
//
// The write is an atomic conditional UPDATE, never read-then-write: the
// store file is shared across processes (TUI, CLI, every MCP session), so
// the condition on the current assignee is what makes a concurrent grab
// safe. Zero affected rows means another agent got there first; the error
// wraps ErrAssigned and names the holder. force drops the guard.
//
// Assignment reserves the subtree: the call is refused when any ancestor or
// descendant is assigned to a different agent, with or without force —
// force overrides one row's holder, not the subtree invariant.
func (s *Store) AssignTask(taskID, agentID string, force bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := getTask(tx, taskID); err != nil {
		return err
	}
	if err := subtreeReserved(tx, taskID, agentID); err != nil {
		return err
	}

	now := time.Now().Unix()
	query := `UPDATE Task SET assignee = ?, assigned_at = ?, updated_at = ? WHERE id = ?`
	if !force {
		// IN ('', agentID) lets a holder re-assign to itself (a no-op
		// refresh) while still refusing a different holder.
		query += ` AND assignee IN ('', ?)`
		res, err := tx.Exec(query, agentID, now, now, taskID, agentID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return holderError(tx, taskID)
		}
	} else {
		res, err := tx.Exec(query, agentID, now, now, taskID)
		if err != nil {
			return err
		}
		if err := requireAffected(res, "task", taskID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UnassignTask clears taskID's assignment. Releasing a task nobody holds is
// a silent no-op — an agent releasing after another already did must not be
// an error. Without force, releasing a task held by a different agent is
// refused with an error wrapping ErrAssigned.
func (s *Store) UnassignTask(taskID, agentID string, force bool) error {
	if _, err := getTask(s.db, taskID); err != nil {
		return err
	}

	// assignee != '' keeps a release that frees nothing from bumping
	// updated_at, so a no-op release does not surface the task in
	// list_tasks(since=...) as changed. UnassignList guards the same way.
	// The cost is that zero rows now means either "already free" or "held by
	// someone else", which holderError distinguishes by reading the row.
	now := time.Now().Unix()
	var res sql.Result
	var err error
	if force {
		res, err = s.db.Exec(
			`UPDATE Task SET assignee = '', assigned_at = NULL, updated_at = ?
			 WHERE id = ? AND assignee != ''`,
			now, taskID,
		)
	} else {
		res, err = s.db.Exec(
			`UPDATE Task SET assignee = '', assigned_at = NULL, updated_at = ?
			 WHERE id = ? AND assignee = ?`,
			now, taskID, agentID,
		)
	}
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Releasing a task nobody holds is a silent no-op; only a live
		// holder that is not this agent is a conflict.
		holder, err := currentHolder(s.db, taskID)
		if err != nil {
			return err
		}
		if holder == "" {
			return nil
		}
		return fmt.Errorf("%w: task %q is held by %q", ErrAssigned, taskID, holder)
	}
	return nil
}

// UnassignList releases every assignment in listID and returns how many
// tasks were freed. This is the human's "release the whole list" escape
// hatch — the reason stale assignments need no sweeper or TTL (plan
// decision 2).
func (s *Store) UnassignList(listID string) (int, error) {
	res, err := s.db.Exec(
		`UPDATE Task SET assignee = '', assigned_at = NULL, updated_at = ? WHERE list_id = ? AND assignee != ''`,
		time.Now().Unix(), listID,
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// NextAssignable atomically picks and assigns the top eligible task in
// listID for agentID, then returns the freshly-read row. Eligible means:
// not complete, unassigned, and no ancestor or descendant assigned to a
// different agent. Ordering is priority descending (high > medium > low >
// none), then ListTasks' depth-first preorder — a rank, not a collation,
// spliced onto a tree order ListTasks builds in Go, so it cannot be one SQL
// ORDER BY (plan §6.5).
//
// The pick is in Go and the write is the same guarded UPDATE as AssignTask,
// so losing a race between pick and write just advances to the next
// candidate; only an exhausted candidate list returns ErrNoAssignable —
// "nothing to grab" is not an error.
func (s *Store) NextAssignable(listID, agentID string) (Task, error) {
	rows, err := s.ListTasks(listID)
	if err != nil {
		return Task{}, err
	}
	// Stable sort keeps ListTasks' preorder as the tie-break within each
	// priority rank — that is what "then preorder position" means here.
	sort.SliceStable(rows, func(i, j int) bool {
		return priorityRank(rows[i].Priority) < priorityRank(rows[j].Priority)
	})

	for _, t := range rows {
		if t.Status == StatusComplete || t.Assignee != "" {
			continue
		}
		got, taken, err := s.tryGrab(t.ID, agentID)
		if err != nil {
			return Task{}, err
		}
		if taken {
			return got, nil
		}
	}
	return Task{}, ErrNoAssignable
}

// tryGrab attempts one candidate: subtree check and guarded UPDATE in a
// single transaction, so the check cannot go stale between the two the way
// it would across separate statements (AssignTask holds the same invariant
// the same way). taken is false — with a nil error — when the subtree is
// reserved or another process won the row; the caller moves to the next
// candidate rather than failing (plan §6.5, trap 3).
func (s *Store) tryGrab(taskID, agentID string) (Task, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Task{}, false, err
	}
	defer tx.Rollback()

	if err := subtreeReserved(tx, taskID, agentID); err != nil {
		if errors.Is(err, ErrAssigned) {
			return Task{}, false, nil
		}
		return Task{}, false, err
	}

	now := time.Now().Unix()
	res, err := tx.Exec(
		`UPDATE Task SET assignee = ?, assigned_at = ?, updated_at = ? WHERE id = ? AND assignee = ''`,
		agentID, now, now, taskID,
	)
	if err != nil {
		return Task{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Task{}, false, err
	}
	if n == 0 {
		return Task{}, false, nil // lost the race; try the next candidate
	}

	got, err := getTask(tx, taskID)
	if err != nil {
		return Task{}, false, err
	}
	return got, true, tx.Commit()
}

// subtreeReserved reports an error wrapping ErrAssigned when any ancestor or
// descendant of taskID is assigned to an agent other than agentID — the
// subtree-reservation invariant (plan decision 4). The guarded UPDATE only
// protects the row itself, so this must run before the write, not after.
func subtreeReserved(q querier, taskID, agentID string) error {
	// Ancestors: walk parent_id links (see getParentID) checking each.
	cur := taskID
	for {
		parent, err := getParentID(q, cur)
		if err != nil {
			return err
		}
		if parent == nil {
			break
		}
		var holder string
		if err := q.QueryRow(`SELECT assignee FROM Task WHERE id = ?`, *parent).Scan(&holder); err != nil {
			return err
		}
		if holder != "" && holder != agentID {
			return fmt.Errorf("%w: ancestor task %q is held by %q", ErrSubtreeAssigned, *parent, holder)
		}
		cur = *parent
	}

	// Descendants: the same recursive CTE completeDescendants uses.
	var id, holder string
	err := q.QueryRow(`
		WITH RECURSIVE descendants(id) AS (
			SELECT id FROM Task WHERE parent_id = ?
			UNION ALL
			SELECT t.id FROM Task t JOIN descendants d ON t.parent_id = d.id
		)
		SELECT t.id, t.assignee FROM Task t
		WHERE t.id IN (SELECT id FROM descendants)
		  AND t.assignee != '' AND t.assignee != ?
		LIMIT 1`,
		taskID, agentID,
	).Scan(&id, &holder)
	if err != nil && !isNoRows(err) {
		return err
	}
	if err == nil {
		return fmt.Errorf("%w: descendant task %q is held by %q", ErrSubtreeAssigned, id, holder)
	}
	return nil
}

// currentHolder reads taskID's assignee, "" when nobody holds it.
func currentHolder(q querier, taskID string) (string, error) {
	var holder string
	err := q.QueryRow(`SELECT assignee FROM Task WHERE id = ?`, taskID).Scan(&holder)
	return holder, err
}

// holderError reads the current holder of taskID and returns the conflict
// error naming them. Call only after a guarded update affected zero rows —
// the task exists and is held by someone else.
func holderError(q querier, taskID string) error {
	holder, err := currentHolder(q, taskID)
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: task %q is held by %q", ErrAssigned, taskID, holder)
}

// UnassignAgent releases every task agentID holds and returns how many were
// freed. It is the assignment half of session-end cleanup: an MCP session's
// identity is unique per process, so a tag that will never come back must
// not hold work forever. The `assignee = ?` condition is what keeps it from
// touching another session's grabs, exactly as ReleaseAgentClaims scopes
// presence.
//
// updated_at moves on the rows it frees, so the release surfaces in
// list_tasks(since=...) the way SetPriority does. Status, progress and
// completion are untouched — releasing is not finishing.
//
// An empty agentID is refused rather than matching every unassigned row: the
// column stores ” for "nobody", so an unguarded empty tag would be a silent
// table-wide no-op at best and a confusing count at worst.
func (s *Store) UnassignAgent(agentID string) (int, error) {
	if agentID == "" {
		return 0, fmt.Errorf("unassign agent: agent_id must not be empty")
	}
	res, err := s.db.Exec(
		`UPDATE Task SET assignee = '', assigned_at = NULL, updated_at = ? WHERE assignee = ?`,
		time.Now().Unix(), agentID,
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
