package cli

import (
	"fmt"

	"github.com/filipemolina/farol/src/store"
)

// ownershipError returns the refusal for a structural write (add, rename,
// notes, move, delete, priority) by the current agent to the list with the
// given id, or nil when the agent may write. It mirrors the retired MCP
// server's ownership guard (docs/DESIGN.md §9): a list is writable by the
// agent only when its created_by equals the agent's identity (the same
// FAROL_AGENT tag §9 defines) or its collaborative flag is set. An empty
// created_by is owned by nobody, so it is foreign to every identity — an
// agent may only read it and update task status/progress, never restructure
// it. The message is the server's exact refusal shape so an agent gets the
// same signal it used to.
func ownershipError(s *store.Store, listID string) error {
	l, err := s.GetList(listID)
	if err != nil {
		return err
	}
	me := agentIdentity()
	if l.CreatedBy == me || l.Collaborative {
		return nil
	}
	owner := l.CreatedBy
	if owner == "" {
		owner = "nobody (human-managed)"
	}
	return fmt.Errorf("list %s is owned by %s — you may read it and update task status/progress only", listID, owner)
}

// taskOwnershipError resolves the list a task belongs to and returns the
// structural-write refusal for it (docs/DESIGN.md §9). A missing task is an
// ordinary resolution error and is surfaced untouched.
func taskOwnershipError(s *store.Store, taskID string) error {
	t, err := s.GetTask(taskID)
	if err != nil {
		return err
	}
	return ownershipError(s, t.ListID)
}
