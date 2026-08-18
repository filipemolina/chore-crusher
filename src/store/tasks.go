package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/filipemolina/farol/src/mentions"
)

// CreateTask creates a task in listID, with the given title, notes, and
// optional parent, appends it after the current last sibling, and returns its
// id. The id is generated before the transaction opens so the caller can
// reference the new task without a re-query (docs/DESIGN.md §2).
//
// The parent, when given, must exist and belong to the same list — a task
// belongs to exactly one List, and a cross-list parent would orphan the task
// from every tree reader that scopes by list_id.
func (s *Store) CreateTask(listID, title string, parentID *string, notes string) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", fmt.Errorf("task title must not be empty")
	}

	id := NewID()
	now := time.Now().Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var one int
	if err := tx.QueryRow(`SELECT 1 FROM List WHERE id = ?`, listID).Scan(&one); err != nil {
		if isNoRows(err) {
			return "", fmt.Errorf("list %q not found", listID)
		}
		return "", err
	}

	if parentID != nil {
		var parentList string
		var parentStatus Status
		err := tx.QueryRow(`SELECT list_id, status FROM Task WHERE id = ?`, *parentID).Scan(&parentList, &parentStatus)
		if err != nil {
			if isNoRows(err) {
				return "", fmt.Errorf("parent task %q not found", *parentID)
			}
			return "", err
		}
		if parentList != listID {
			return "", fmt.Errorf("parent task %q belongs to a different list", *parentID)
		}
		// A complete parent cannot have a pending child (docs/DESIGN.md §3).
		// Reopen the parent so the new child can be added under a pending parent.
		if parentStatus == StatusComplete {
			if err := reopenTaskTx(tx, *parentID, now); err != nil {
				return "", err
			}
		}
	}

	var position int
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(position), -1) + 1 FROM Task WHERE list_id = ? AND parent_id IS ?`,
		listID, parentID,
	).Scan(&position); err != nil {
		return "", err
	}

	if _, err := tx.Exec(
		`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind,
		                  progress_pct, position, created_at, updated_at, completed_at)
		 VALUES (?, ?, ?, ?, ?, 'pending', 'none', NULL, ?, ?, ?, NULL)`,
		id, listID, parentID, title, notes, position, now, now,
	); err != nil {
		return "", err
	}

	// Auto-switch the parent to subtasks mode when it gains its first
	// subtask — "Auto percentage on parent tasks" (backlog #7). A parent
	// whose progress kind is unset ('none') derives its percentage from its
	// children through the shared SetProgress write path; an explicit kind is
	// never overridden, and a complete parent is never touched.
	if parentID != nil {
		if err := autoSwitchParentToSubtasks(tx, *parentID, now); err != nil {
			return "", err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// CreateTaskAfter creates a task and positions it immediately after a reference
// sibling. If afterID is empty, behaves like CreateTask (appends to end).
// The reference sibling and new task must share the same parent.
func (s *Store) CreateTaskAfter(listID, title string, parentID *string, notes, afterID string) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", fmt.Errorf("task title must not be empty")
	}

	id := NewID()
	now := time.Now().Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var one int
	if err := tx.QueryRow(`SELECT 1 FROM List WHERE id = ?`, listID).Scan(&one); err != nil {
		if isNoRows(err) {
			return "", fmt.Errorf("list %q not found", listID)
		}
		return "", err
	}

	if parentID != nil {
		var parentList string
		var parentStatus Status
		err := tx.QueryRow(`SELECT list_id, status FROM Task WHERE id = ?`, *parentID).Scan(&parentList, &parentStatus)
		if err != nil {
			if isNoRows(err) {
				return "", fmt.Errorf("parent task %q not found", *parentID)
			}
			return "", err
		}
		if parentList != listID {
			return "", fmt.Errorf("parent task %q belongs to a different list", *parentID)
		}
		// A complete parent cannot have a pending child (docs/DESIGN.md §3).
		// Reopen the parent so the new child can be added under a pending parent.
		if parentStatus == StatusComplete {
			if err := reopenTaskTx(tx, *parentID, now); err != nil {
				return "", err
			}
		}
	}

	// If no afterID given, append to end like CreateTask
	if afterID == "" {
		var position int
		if err := tx.QueryRow(
			`SELECT COALESCE(MAX(position), -1) + 1 FROM Task WHERE list_id = ? AND parent_id IS ?`,
			listID, parentID,
		).Scan(&position); err != nil {
			return "", err
		}

		if _, err := tx.Exec(
			`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind,
			                  progress_pct, position, created_at, updated_at, completed_at)
			 VALUES (?, ?, ?, ?, ?, 'pending', 'none', NULL, ?, ?, ?, NULL)`,
			id, listID, parentID, title, notes, position, now, now,
		); err != nil {
			return "", err
		}

		// Same auto-switch as CreateTask: the TUI's inline create routes
		// through CreateTaskAfter, so the CLI and the TUI must not diverge on
		// whether a new subtask starts its parent (docs/DESIGN.md §3).
		if parentID != nil {
			if err := autoSwitchParentToSubtasks(tx, *parentID, now); err != nil {
				return "", err
			}
		}

		if err := tx.Commit(); err != nil {
			return "", err
		}
		return id, nil
	}

	// Resolve afterID and verify it's a sibling
	var refParentID sql.NullString
	var refPosition int
	if err := tx.QueryRow(
		`SELECT parent_id, position FROM Task WHERE id = ? AND list_id = ?`,
		afterID, listID,
	).Scan(&refParentID, &refPosition); err != nil {
		if isNoRows(err) {
			return "", fmt.Errorf("reference task %q not found in list %q", afterID, listID)
		}
		return "", err
	}

	// Verify the reference task is a sibling (same parent)
	var refParentPtr *string
	if refParentID.Valid {
		refParentPtr = &refParentID.String
	}
	if (parentID == nil) != (refParentPtr == nil) || (parentID != nil && refParentPtr != nil && *parentID != *refParentPtr) {
		return "", fmt.Errorf("reference task %q has a different parent", afterID)
	}

	// Insert position is one after the reference
	newPosition := refPosition + 1

	// Shift all siblings at position >= newPosition up by 1
	if _, err := tx.Exec(
		`UPDATE Task SET position = position + 1
		 WHERE list_id = ? AND parent_id IS ? AND position >= ?`,
		listID, parentID, newPosition,
	); err != nil {
		return "", err
	}

	// Insert the new task at the new position
	if _, err := tx.Exec(
		`INSERT INTO Task (id, list_id, parent_id, title, notes, status, progress_kind,
		                  progress_pct, position, created_at, updated_at, completed_at)
		 VALUES (?, ?, ?, ?, ?, 'pending', 'none', NULL, ?, ?, ?, NULL)`,
		id, listID, parentID, title, notes, newPosition, now, now,
	); err != nil {
		return "", err
	}

	// Same auto-switch as CreateTask (docs/DESIGN.md §3, backlog #7).
	if parentID != nil {
		if err := autoSwitchParentToSubtasks(tx, *parentID, now); err != nil {
			return "", err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// GetTask returns the task with the given id.
func (s *Store) GetTask(id string) (Task, error) {
	return getTask(s.db, id)
}

// RenameTask sets a task's title.
func (s *Store) RenameTask(id, title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("task title must not be empty")
	}
	res, err := s.db.Exec(`UPDATE Task SET title = ?, updated_at = ? WHERE id = ?`, title, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	return requireAffected(res, "task", id)
}

// SetNotes replaces a task's notes wholesale (the CLI's notes command is
// "replace, not append"; clearing notes is setting the empty string).
// It validates that any @<ULID> mentions in the notes reference existing tasks.
func (s *Store) SetNotes(id, notes string) error {
	// Validate mentions before storing
	for _, m := range mentions.ParseMentions(notes) {
		if _, err := s.GetTask(m.ID); err != nil {
			return fmt.Errorf("mention @%s references non-existent task", m.ID)
		}
	}

	res, err := s.db.Exec(`UPDATE Task SET notes = ?, updated_at = ? WHERE id = ?`, notes, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	return requireAffected(res, "task", id)
}

// Reparent moves taskID under parentID within its current list. A nil
// parentID moves the task to the list root — the CLI represents this as an
// empty --parent flag (docs/DESIGN.md §9, documented there in the same
// commit). Unlike the TUI's add flow (docs/DESIGN.md §4) there is no
// ±1-level restriction: a CLI re-parent is a deliberate restructure, so any
// valid target parent is accepted.
//
// The proposed parent must belong to the same list and must not be the task
// itself or one of its own descendants — a parent task that is reached when
// walking upward from the proposed parent would make parent_id chains loop
// forever, breaking Flatten (src/apptypes) and every ancestor walk here
// (recomputeAncestors). Moving a non-complete task under a complete parent
// is also rejected: a "complete ancestor, pending descendant" state is one
// docs/DESIGN.md §3 forbids to exist, and completing the task is the only
// way to arrive there legitimately.
func (s *Store) Reparent(taskID string, parentID *string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	task, err := getTask(tx, taskID)
	if err != nil {
		return err
	}

	// Reparenting to the current parent is a no-op.
	if (parentID == nil && task.ParentID == nil) ||
		(parentID != nil && task.ParentID != nil && *parentID == *task.ParentID) {
		return nil
	}

	if parentID != nil {
		if *parentID == taskID {
			return fmt.Errorf("task %q cannot be its own parent", taskID)
		}
		parent, err := getTask(tx, *parentID)
		if err != nil {
			return err
		}
		if parent.ListID != task.ListID {
			return fmt.Errorf("parent task %q belongs to a different list", *parentID)
		}
		if err := ensureNotDescendant(tx, *parentID, taskID); err != nil {
			return err
		}
		// A complete parent cannot have a pending child (docs/DESIGN.md §3).
		// Reopen the parent instead of rejecting the move, then auto-switch
		// to subtasks mode so the parent derives progress from its children.
		if parent.Status == StatusComplete && task.Status != StatusComplete {
			now := time.Now().Unix()
			if err := reopenTaskTx(tx, *parentID, now); err != nil {
				return err
			}
			if err := autoSwitchParentToSubtasks(tx, *parentID, now); err != nil {
				return err
			}
		}
	}

	// Close the gap the task leaves in its old sibling run.
	if _, err := tx.Exec(
		`UPDATE Task SET position = position - 1
		 WHERE list_id = ? AND parent_id IS ? AND position > ?`,
		task.ListID, task.ParentID, task.Position,
	); err != nil {
		return err
	}

	// Append the task to the end of its new parent's children.
	var position int
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(position), -1) + 1 FROM Task WHERE list_id = ? AND parent_id IS ?`,
		task.ListID, parentID,
	).Scan(&position); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`UPDATE Task SET parent_id = ?, position = ?, updated_at = ? WHERE id = ?`,
		parentID, position, time.Now().Unix(), taskID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// MoveTask repositions taskID to be the immediate successor of afterID — its
// new parent is afterID's parent, and it lands one position after afterID —
// or, when afterID is empty, the first child of taskID's current parent.
// One primitive covers three TUI gestures (docs/DESIGN.md §5): move-up and
// move-down swap a task with its previous/next same-status sibling, and
// outdent places a task right after its own parent. Unlike Reparent there is
// no ±1-level rule — a deliberate move may cross any number of levels in one
// step — but the same validity rules apply: same list, no descendant target,
// and a non-complete task never lands under a complete parent (§3).
func (s *Store) MoveTask(taskID, afterID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	task, err := getTask(tx, taskID)
	if err != nil {
		return err
	}

	// The target run is afterID's parent run (afterID may be any task in the
	// list: outdent passes the task's own parent). An empty afterID targets
	// the front of the task's own parent run.
	var targetParent *string
	if afterID != "" {
		after, err := getTask(tx, afterID)
		if err != nil {
			return err
		}
		if after.ListID != task.ListID {
			return fmt.Errorf("task %q belongs to a different list than %q", afterID, taskID)
		}
		if afterID == taskID {
			return fmt.Errorf("task %q cannot be moved after itself", taskID)
		}
		if err := ensureNotDescendant(tx, afterID, taskID); err != nil {
			return err
		}
		targetParent = after.ParentID
	} else {
		targetParent = task.ParentID
	}

	// A non-complete task cannot move under a complete parent: the
	// "complete ancestor, pending descendant" state is one §3 forbids.
	if targetParent != nil {
		parent, err := getTask(tx, *targetParent)
		if err != nil {
			return err
		}
		if parent.Status == StatusComplete && task.Status != StatusComplete {
			return fmt.Errorf("cannot move non-complete task %q under complete task %q; complete it first", taskID, *targetParent)
		}
	}

	// Close the gap the task leaves in its old sibling run.
	if _, err := tx.Exec(
		`UPDATE Task SET position = position - 1
		 WHERE list_id = ? AND parent_id IS ? AND position > ?`,
		task.ListID, task.ParentID, task.Position,
	); err != nil {
		return err
	}

	// The insertion point: one after afterID (whose position may have just
	// shifted down if it sat after the task in the same run), or the front
	// of the run.
	var targetPos int
	if afterID != "" {
		after, err := getTask(tx, afterID)
		if err != nil {
			return err
		}
		targetPos = after.Position + 1
	} else {
		targetPos = 0
	}

	// Make room in the target run, excluding the task itself when it moves
	// within the same run (its row still holds its stale old position).
	if _, err := tx.Exec(
		`UPDATE Task SET position = position + 1
		 WHERE list_id = ? AND parent_id IS ? AND position >= ? AND id != ?`,
		task.ListID, targetParent, targetPos, taskID,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`UPDATE Task SET parent_id = ?, position = ?, updated_at = ? WHERE id = ?`,
		targetParent, targetPos, time.Now().Unix(), taskID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteTask deletes the task and, via the parent_id foreign key's ON DELETE
// CASCADE, every descendant at every depth. Sibling subtrees are untouched.
func (s *Store) DeleteTask(id string) error {
	// Collect every id in the subtree FIRST: the parent_id foreign key's
	// CASCADE removes the descendants from Task, but AgentActivity has no FK
	// to Task, so we must delete those claim rows ourselves (a live claim on a
	// deleted task would leave an orphaned spinner).
	var ids []string
	rows, err := s.db.Query(`WITH RECURSIVE subtree AS (
		SELECT id FROM Task WHERE id = ?
		UNION ALL
		SELECT t.id FROM Task t JOIN subtree s ON t.parent_id = s.id
	) SELECT id FROM subtree`, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, tid)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("task %q not found", id)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, tid := range ids {
		// AgentActivity has no FK to Task, so claim rows for the deleted
		// subtree must be removed explicitly. TaskComment cascades via
		// its task_id FK, so it needs no explicit delete here.
		if _, err := tx.Exec(`DELETE FROM AgentActivity WHERE entity_type = 'task' AND entity_id = ?`, tid); err != nil {
			return err
		}
	}
	res, err := tx.Exec(`DELETE FROM Task WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return requireAffected(res, "task", id)
}

// ListTasks returns every task in listID as flat rows, each carrying its own
// ParentID — building the tree shape is the caller's job (src/apptypes in a
// later phase), so the CLI's flat mode and the TUI's tree renderer read the
// exact same query. Rows are returned in depth-first preorder: roots ordered
// by position, then creation time, and every parent's children ordered the
// same way before the next root or sibling subtree. This keeps parents before
// their children and siblings in their stored position order.
func (s *Store) ListTasks(listID string) ([]Task, error) {
	rows, err := s.db.Query(`SELECT `+taskColumns+` FROM Task WHERE list_id = ?`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	children := make(map[string][]*Task)
	var roots []*Task
	for i := range out {
		t := &out[i]
		if t.ParentID == nil {
			roots = append(roots, t)
			continue
		}
		children[*t.ParentID] = append(children[*t.ParentID], t)
	}

	taskLess := func(a, b *Task) bool {
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt < b.CreatedAt
		}
		return a.ID < b.ID
	}
	sort.Slice(roots, func(i, j int) bool { return taskLess(roots[i], roots[j]) })
	for _, cs := range children {
		sort.Slice(cs, func(i, j int) bool { return taskLess(cs[i], cs[j]) })
	}

	ordered := make([]Task, 0, len(out))
	var walk func(*Task)
	walk = func(t *Task) {
		ordered = append(ordered, *t)
		for _, c := range children[t.ID] {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}

	return ordered, nil
}

// TasksChangedSince returns the tasks in listID whose updated_at is strictly
// greater than since (unix seconds), ordered by updated_at ascending. "Changed"
// covers creation, status/progress, rename, notes, re-parent, and a new comment
// (AddComment bumps updated_at for exactly this reason). Deletions are not
// represented — a task removed after `since` is simply absent from the
// result.
func (s *Store) TasksChangedSince(listID string, since int64) ([]Task, error) {
	rows, err := s.db.Query(
		`SELECT `+taskColumns+` FROM Task WHERE list_id = ? AND updated_at > ? ORDER BY updated_at ASC`,
		listID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TasksAddedOrChangedSince returns the tasks in listID whose created_at or
// updated_at is strictly greater than since (unix seconds). Deletions are not
// represented — a task removed after `since` is simply absent from the
// result. Results are ordered by the later of created_at and updated_at
// ascending, then by task ID to break ties.
func (s *Store) TasksAddedOrChangedSince(listID string, since int64) ([]Task, error) {
	rows, err := s.db.Query(
		`SELECT `+taskColumns+` FROM Task WHERE list_id = ? AND (created_at > ? OR updated_at > ?) ORDER BY 
		 CASE WHEN created_at > updated_at THEN created_at ELSE updated_at END, id`,
		listID, since, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// taskColumns is shared by every query that reads a full Task row.
const taskColumns = `id, list_id, parent_id, title, notes, status, progress_kind, progress_pct,
	position, created_at, updated_at, completed_at, assignee, assigned_at, priority`

// getTask reads one Task row, converting sql.Null* columns to pointers.
func getTask(q querier, id string) (Task, error) {
	t, err := scanTask(q.QueryRow(`SELECT `+taskColumns+` FROM Task WHERE id = ?`, id))
	if err != nil && isNoRows(err) {
		return Task{}, fmt.Errorf("task %q not found", id)
	}
	return t, err
}

// scanTask converts one result row into a Task. The Scan argument order must
// match taskColumns exactly; a column added to the constant in one place and
// not the other surfaces as a runtime scan error on every read.
func scanTask(r rowScanner) (Task, error) {
	var t Task
	var parent sql.NullString
	var pct sql.NullInt64
	var done sql.NullInt64
	var assigned sql.NullInt64
	err := r.Scan(
		&t.ID, &t.ListID, &parent, &t.Title, &t.Notes,
		&t.Status, &t.ProgressKind, &pct,
		&t.Position, &t.CreatedAt, &t.UpdatedAt, &done,
		&t.Assignee, &assigned, &t.Priority,
	)
	if err != nil {
		return Task{}, err
	}
	if parent.Valid {
		t.ParentID = &parent.String
	}
	if pct.Valid {
		v := int(pct.Int64)
		t.ProgressPct = &v
	}
	if done.Valid {
		v := done.Int64
		t.CompletedAt = &v
	}
	if assigned.Valid {
		v := assigned.Int64
		t.AssignedAt = &v
	}
	return t, nil
}

// ensureNotDescendant reports an error when ascending parent links from
// proposedParent reach rootTask — i.e. proposedParent is already a descendant
// of the task being moved — which is the cycle signal: reparenting under it
// would make parent_id chains loop forever.
func ensureNotDescendant(tx *sql.Tx, proposedParent, rootTask string) error {
	cur := proposedParent
	for {
		par, err := getParentID(tx, cur)
		if err != nil {
			return err
		}
		if par == nil {
			return nil
		}
		if *par == rootTask {
			return fmt.Errorf("task %q is a descendant of proposed parent %q; cannot move without creating a cycle", rootTask, proposedParent)
		}
		cur = *par
	}
}

// getParentID returns the parent of id, or nil for a root-level task.
func getParentID(q querier, id string) (*string, error) {
	var parent sql.NullString
	if err := q.QueryRow(`SELECT parent_id FROM Task WHERE id = ?`, id).Scan(&parent); err != nil {
		return nil, err
	}
	if !parent.Valid {
		return nil, nil
	}
	return &parent.String, nil
}
