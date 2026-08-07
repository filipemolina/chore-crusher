package store

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrAssigned is returned when a task is assigned to another agent. The
// wrapping error names the holder; the MCP layer turns that into the
// force=true hint (docs/plan/mcp-assignment-and-priorities.md §4).
var ErrAssigned = errors.New("task is assigned to another agent")

// ErrNoAssignable reports that NextAssignable found no eligible task. It is
// a normal outcome, not a failure — the MCP layer maps it to the
// {ok: false, reason: ...} shape (plan §4), never to a tool error.
var ErrNoAssignable = errors.New("no eligible task in this list")

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

	now := time.Now().Unix()
	var res interface{ RowsAffected() (int64, error) }
	var err error
	if force {
		res, err = s.db.Exec(
			`UPDATE Task SET assignee = '', assigned_at = NULL, updated_at = ? WHERE id = ?`,
			now, taskID,
		)
	} else {
		res, err = s.db.Exec(
			`UPDATE Task SET assignee = '', assigned_at = NULL, updated_at = ? WHERE id = ? AND assignee IN ('', ?)`,
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
		return holderError(s.db, taskID)
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

	now := time.Now().Unix()
	for _, t := range rows {
		if t.Status == StatusComplete || t.Assignee != "" {
			continue
		}
		if err := subtreeReserved(s.db, t.ID, agentID); err != nil {
			if errors.Is(err, ErrAssigned) {
				continue
			}
			return Task{}, err
		}
		res, err := s.db.Exec(
			`UPDATE Task SET assignee = ?, assigned_at = ?, updated_at = ? WHERE id = ? AND assignee = ''`,
			agentID, now, now, t.ID,
		)
		if err != nil {
			return Task{}, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return Task{}, err
		}
		if n == 0 {
			continue // lost the race to another process; try the next candidate
		}
		return getTask(s.db, t.ID)
	}
	return Task{}, ErrNoAssignable
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
			return fmt.Errorf("%w: ancestor task %q is held by %q", ErrAssigned, *parent, holder)
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
		return fmt.Errorf("%w: descendant task %q is held by %q", ErrAssigned, id, holder)
	}
	return nil
}

// holderError reads the current holder of taskID and returns the conflict
// error naming them. Call only after a guarded update affected zero rows —
// the task exists and is held by someone else.
func holderError(q querier, taskID string) error {
	var holder string
	if err := q.QueryRow(`SELECT assignee FROM Task WHERE id = ?`, taskID).Scan(&holder); err != nil {
		return err
	}
	return fmt.Errorf("%w: task %q is held by %q", ErrAssigned, taskID, holder)
}
