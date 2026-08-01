package store

import (
	"database/sql"
	"fmt"
	"math"
	"time"
)

// Complete marks the task complete and cascades the same to every descendant
// at every depth, all in one transaction — a "complete parent, pending child"
// state must never exist (docs/DESIGN.md §3). It then walks the task's
// ancestors: any subtasks-mode ancestor whose direct children are all complete
// is promoted too, and that promotion propagates upward.
func (s *Store) Complete(taskID string) error {
	now := time.Now().Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := getTask(tx, taskID); err != nil {
		return err
	}
	if err := setComplete(tx, taskID, now); err != nil {
		return err
	}
	if err := completeDescendants(tx, taskID, now); err != nil {
		return err
	}
	if err := recomputeAncestors(tx, taskID, now); err != nil {
		return err
	}
	return tx.Commit()
}

// Reopen returns only that task to pending — it does not cascade to children
// (a reopen is intentionally lossy; see docs/DESIGN.md §3) and it does not
// touch the parent: re-derivation only ever promotes toward complete, never
// demotes, so a reopened task can only reduce an ancestor's derived ratio.
func (s *Store) Reopen(taskID string) error {
	res, err := s.db.Exec(
		`UPDATE Task SET status = 'pending', progress_kind = 'none', progress_pct = NULL,
		                completed_at = NULL, updated_at = ? WHERE id = ?`,
		time.Now().Unix(), taskID,
	)
	if err != nil {
		return err
	}
	return requireAffected(res, "task", taskID)
}

// Toggle flips a task between complete and pending, "whichever applies":
// complete → reopen (that task only, no cascade), anything else → complete
// (cascading to every descendant). One implementation here so the CLI's
// toggle and the TUI's space key (docs/DESIGN.md §9, §5) never each decide
// which direction a toggle goes — that decision is a store transition, not
// front-end logic.
func (s *Store) Toggle(taskID string) error {
	t, err := getTask(s.db, taskID)
	if err != nil {
		return err
	}
	if t.Status == StatusComplete {
		return s.Reopen(taskID)
	}
	return s.Complete(taskID)
}

// SetProgress sets a task's progress kind (and, for percentage, its percent),
// starting the task as a side effect: setting any progress implies it has
// started, so a pending task becomes in_progress (docs/DESIGN.md §3).
//
// Validation happens here, the one place all writers pass through:
// percent must be 0..100 and non-nil when kind is percentage, and must be nil
// for every other kind — passing a percent alongside simple/subtasks/none is
// a caller bug, rejected rather than silently ignored. Setting progress on an
// already-complete task is a domain error, not a silent reopen.
func (s *Store) SetProgress(taskID string, kind ProgressKind, percent *int) error {
	switch kind {
	case ProgressPercentage:
		if percent == nil {
			return fmt.Errorf("task %q: progress_kind percentage requires a percent", taskID)
		}
		if *percent < 0 || *percent > 100 {
			return fmt.Errorf("task %q: percent %d out of range 0..100", taskID, *percent)
		}
	case ProgressNone, ProgressSimple, ProgressSubtasks:
		if percent != nil {
			return fmt.Errorf("task %q: percent is only valid with progress_kind percentage", taskID)
		}
	default:
		return fmt.Errorf("task %q: unknown progress kind %q", taskID, kind)
	}

	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	t, err := getTask(tx, taskID)
	if err != nil {
		return err
	}
	if t.Status == StatusComplete {
		return fmt.Errorf("task %q is complete; reopen it before setting progress", taskID)
	}

	status := t.Status
	if status == StatusPending {
		status = StatusInProgress
	}
	if _, err := tx.Exec(
		`UPDATE Task SET status = ?, progress_kind = ?, progress_pct = ?, updated_at = ? WHERE id = ?`,
		status, kind, percent, now, taskID,
	); err != nil {
		return err
	}
	// A kind change can make a subtasks-mode ancestor's derived condition
	// hold; the walk only ever promotes on the verified fact that every
	// direct child is complete, so it is safe to run unconditionally.
	if err := recomputeAncestors(tx, taskID, now); err != nil {
		return err
	}
	return tx.Commit()
}

// DerivedProgress is the read-side counterpart to the state machine: for a
// subtasks-mode task it computes round(100 * complete_direct_children /
// total_direct_children) over direct children only, and reports
// displayAsSimple = true when the task has no children at all, so the caller
// shows no percentage instead of a misleading "0%". For percentage it returns
// the stored percent. Callers use this instead of reading ProgressPct
// directly, since that column is meaningless for subtasks mode.
func (s *Store) DerivedProgress(taskID string) (kind ProgressKind, percent int, displayAsSimple bool, err error) {
	t, err := getTask(s.db, taskID)
	if err != nil {
		return "", 0, false, err
	}
	switch t.ProgressKind {
	case ProgressSubtasks:
		complete, total, err := directChildCounts(s.db, taskID)
		if err != nil {
			return "", 0, false, err
		}
		if total == 0 {
			return ProgressSubtasks, 0, true, nil
		}
		pct := int(math.Round(100 * float64(complete) / float64(total)))
		return ProgressSubtasks, pct, false, nil
	case ProgressPercentage:
		if t.ProgressPct == nil {
			return "", 0, false, fmt.Errorf("task %q: progress_kind is percentage but progress_pct is NULL", taskID)
		}
		return ProgressPercentage, *t.ProgressPct, false, nil
	default:
		return t.ProgressKind, 0, true, nil
	}
}

// recomputeAncestors walks upward from taskID's parent. Every subtasks-mode
// ancestor whose direct children are all complete is promoted to complete —
// a verified fact the store does not wait for a human to confirm — and the
// promotion can itself complete the ancestor above it, so the walk continues
// until the first ancestor whose ratio is below 1.0 (a parent two levels up
// cannot be more complete than the one directly above it). It only ever walks
// upward; it never re-descends onto children, so a promoted subtasks parent
// cannot trigger an infinite recursion back onto the walk's origin.
func recomputeAncestors(tx *sql.Tx, taskID string, now int64) error {
	current := taskID
	for {
		parent, err := getParentID(tx, current)
		if err != nil {
			return err
		}
		if parent == nil {
			return nil
		}

		anc, err := getTask(tx, *parent)
		if err != nil {
			return err
		}
		if anc.ProgressKind != ProgressSubtasks {
			// This ancestor cannot auto-promote. Nothing above it can either
			// unless it is already complete (a subtasks grandparent at 1.0
			// needs every direct child complete) — if it is not, stop.
			if anc.Status != StatusComplete {
				return nil
			}
			current = *parent
			continue
		}

		complete, total, err := directChildCounts(tx, *parent)
		if err != nil {
			return err
		}
		// 0/0 (no children) is the displayAsSimple fallback, not 100%.
		if total == 0 || complete != total {
			return nil
		}
		if err := setComplete(tx, *parent, now); err != nil {
			return err
		}
		current = *parent
	}
}

// setComplete writes the complete state onto one task.
func setComplete(q querier, id string, now int64) error {
	_, err := q.Exec(
		`UPDATE Task SET status = 'complete', progress_kind = 'none', progress_pct = NULL,
		                completed_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id,
	)
	return err
}

// completeDescendants marks every descendant of id (at every depth) complete.
func completeDescendants(q querier, id string, now int64) error {
	_, err := q.Exec(`
		WITH RECURSIVE descendants(id) AS (
			SELECT id FROM Task WHERE parent_id = ?
			UNION ALL
			SELECT t.id FROM Task t JOIN descendants d ON t.parent_id = d.id
		)
		UPDATE Task SET status = 'complete', progress_kind = 'none', progress_pct = NULL,
		                completed_at = ?, updated_at = ?
		WHERE id IN (SELECT id FROM descendants)`,
		id, now, now,
	)
	return err
}

// directChildCounts counts id's direct children, and how many are complete.
func directChildCounts(q querier, id string) (complete, total int, err error) {
	err = q.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status = 'complete' THEN 1 ELSE 0 END), 0)
		 FROM Task WHERE parent_id = ?`,
		id,
	).Scan(&total, &complete)
	return complete, total, err
}
