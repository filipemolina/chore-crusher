package cli

import "github.com/filipemolina/farol/src/store"

// liveAgents returns the set of agent tags with a live presence claim (a
// claim acquired within the store's WorkTTL). It mirrors the MCP server's
// liveAgents: one ListWork read per request, so every row in that request
// agrees on who is "at the keyboard right now" (docs/DESIGN.md §3).
func liveAgents(s *store.Store) (map[string]bool, error) {
	work, err := s.ListWork()
	if err != nil {
		return nil, err
	}
	live := make(map[string]bool, len(work))
	for _, w := range work {
		live[w.AgentID] = true
	}
	return live, nil
}

// assigneeLive reports whether the task's assignee currently holds a live
// presence claim. The TUI's stale tier is "assignee != ” && !assignee_live",
// so an unassigned task (or one whose assignee's session died) reads as
// abandoned — exactly what this field lets an agent distinguish too.
func assigneeLive(live map[string]bool, assignee string) bool {
	return assignee != "" && live[assignee]
}

// autoClaim renews the writing agent's live claim on entityID, or opens one
// if none exists. It is the CLI mirror of the MCP server's autoClaim and
// exists for the same reason: the moment farol mcp is removed, every agent
// task write must keep the TUI presence spinner alive, or presence goes dark.
//
// Best-effort, like MCP: any error is swallowed because the write already
// committed and presence tracking is not a write guarantee. TouchWork first
// (a heartbeat that extends a still-live claim without creating a new row or
// reviving an expired one); if there is no claim to touch, ClaimWork opens one
// — but a claim held by another agent returns ErrActivityConflict, which is
// silently dropped here: we do not steal their spinner, and the write itself
// is allowed because the write path does not gate on claims (unchanged
// behaviour from the MCP surface).
func autoClaim(s *store.Store, entityType, entityID, agentID string) {
	if err := s.TouchWork(entityType, entityID, agentID); err == nil {
		_, _ = s.ClaimWork(entityType, entityID, agentID, store.ActivityWorking)
	}
}

// autoClaimTask claims presence on a task under the default agent identity.
// The MCP server's auto-claim sites only ever claim the task (never its
// owning list), and claim failures are non-fatal, so this matches them.
func autoClaimTask(s *store.Store, taskID string) {
	autoClaim(s, "task", taskID, agentIdentity())
}
