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
