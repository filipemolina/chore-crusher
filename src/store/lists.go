package store

import (
	"fmt"
	"strings"
	"time"
)

// CreateList creates a list with the given name, appended after the current
// last list, and returns its id. An empty name is rejected here so both the
// CLI and the TUI get the same error.
//
// createdBy is the owning agent tag ("pi", "claudecode", ...); an empty
// string marks the list as owned by nobody (human-managed) — see
// docs/plan/list-ownership-enforcement.md §3.3. The store does not validate
// the tag format; that is the MCP layer's job.
func (s *Store) CreateList(name, createdBy string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("list name must not be empty")
	}

	id := NewID()
	now := time.Now().Unix()

	var position int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(position), -1) + 1 FROM List`).Scan(&position); err != nil {
		return "", err
	}
	if _, err := s.db.Exec(
		`INSERT INTO List (id, name, created_at, position, created_by) VALUES (?, ?, ?, ?, ?)`,
		id, name, now, position, createdBy,
	); err != nil {
		return "", err
	}
	return id, nil
}

// GetList returns the list with the given id, including its CreatedBy owner.
// An empty CreatedBy means the list is owned by nobody (human-managed); the MCP
// policy layer uses this to gate structural writes
// (docs/plan/list-ownership-enforcement.md).
func (s *Store) GetList(id string) (List, error) {
	var l List
	if err := s.db.QueryRow(
		`SELECT id, name, created_at, position, created_by, comments_disabled, collaborative FROM List WHERE id = ?`,
		id,
	).Scan(&l.ID, &l.Name, &l.CreatedAt, &l.Position, &l.CreatedBy, &l.CommentsDisabled, &l.Collaborative); err != nil {
		if isNoRows(err) {
			return List{}, fmt.Errorf("list %q not found", id)
		}
		return List{}, err
	}
	return l, nil
}

// ListLists returns every list, in creation order, each with its pending and
// complete task counts. One GROUP BY query — never an N+1 per list.
func (s *Store) ListLists() ([]ListSummary, error) {
	rows, err := s.db.Query(`
		SELECT l.id, l.name, l.created_at, l.position, l.created_by, l.comments_disabled, l.collaborative,
		       COUNT(t.id),
		       COALESCE(SUM(CASE WHEN t.status = 'complete' THEN 1 ELSE 0 END), 0)
		FROM List l
		LEFT JOIN Task t ON t.list_id = l.id
		GROUP BY l.id
		ORDER BY l.position, l.created_at, l.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ListSummary
	for rows.Next() {
		var ls ListSummary
		var total, done int
		if err := rows.Scan(&ls.ID, &ls.Name, &ls.CreatedAt, &ls.Position, &ls.CreatedBy, &ls.CommentsDisabled, &ls.Collaborative, &total, &done); err != nil {
			return nil, err
		}
		ls.CompleteCount = done
		ls.PendingCount = total - done
		out = append(out, ls)
	}
	return out, rows.Err()
}

// RenameList renames the list with the given id.
func (s *Store) RenameList(id, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("list name must not be empty")
	}
	// Adopt-on-tag (docs/plan/mcp-agent-todo-hardening.md §4.7): renaming an
	// untagged list into the "<tag>:" convention adopts the tag as owner in
	// the same write — the human handoff path (rename Groceries to
	// "pi: Groceries") takes effect immediately instead of at the next
	// store.Open. An existing owner is kept: a rename never transfers
	// ownership. The CASE leaves created_by untouched when it is non-empty,
	// and stays a no-op when the new name has no tag.
	owner := adoptOwnerTag(name)
	res, err := s.db.Exec(`UPDATE List SET name = ?,
		created_by = CASE WHEN created_by = '' THEN ? ELSE created_by END
		WHERE id = ?`, name, owner, id)
	if err != nil {
		return err
	}
	return requireAffected(res, "list", id)
}

// DeleteList deletes the list and, via the list_id foreign key's ON DELETE
// CASCADE, every task in it. There is no confirmation here — the CLI's
// --force flag and the TUI's confirm modal are the callers' jobs.
func (s *Store) DeleteList(id string) error {
	res, err := s.db.Exec(`DELETE FROM List WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireAffected(res, "list", id)
}

// SetCollaborative toggles the list-level collaborative flag: an explicit
// opt-in that lets any agent make structural edits on the list, not just its
// created_by owner. Modeled on SetCommentsDisabled.
func (s *Store) SetCollaborative(id string, collaborative bool) error {
	flag := 0
	if collaborative {
		flag = 1
	}
	res, err := s.db.Exec(
		`UPDATE List SET collaborative = ? WHERE id = ?`,
		flag, id,
	)
	if err != nil {
		return err
	}
	return requireAffected(res, "list", id)
}

// GetOrCreateAgentList returns the id of the list owned by identity, creating
// one named "<identity>: Inbox" when none exists yet. It is idempotent: a
// second call for the same identity returns the same list without creating a
// duplicate.
//
// The lookup is owner-first (WHERE created_by = ?): a list merely *named*
// "<identity>: ..." but created by the human in the CLI/TUI (created_by
// empty) is foreign to every agent and must not satisfy this call — silently
// adopting it would hand the agent a list the server then refuses to write
// (docs/plan/mcp-agent-todo-hardening.md §4.3). The my_list MCP tool is the
// caller-facing wrapper this store method supports.
func (s *Store) GetOrCreateAgentList(identity string) (string, error) {
	prefix := identity + ": "
	var id string
	if err := s.db.QueryRow(`SELECT id FROM "list" WHERE created_by = ? ORDER BY created_at LIMIT 1`, identity).Scan(&id); err != nil {
		if !isNoRows(err) {
			return "", err
		}
		return s.CreateList(prefix+"Inbox", identity)
	}
	return id, nil
}
