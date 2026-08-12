package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/store"
)

// claimResultJSON is the success payload for `farol claim` (docs/DESIGN.md
// §9, cli-first migration Phase 2). It echoes the row's ULID so a caller can
// tell which claim it owns, and the entity it now holds — presence, not
// assignment, so no assignee field appears. A conflict (another agent holds
// the entity) is a domain error (exit 1), matching the refuse-with-no-steal
// rule from §3: we never take another agent's spinner.
type claimResultJSON struct {
	OK         bool   `json:"ok"`
	ID         string `json:"id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Kind       string `json:"kind"`
}

func newClaimCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claim <task-id|list-id> [--kind working|inspecting]",
		Short: "claim presence on a task or list (lights the TUI spinner)",
		Long: `Claim presence on an entity under the current agent identity
(FAROL_AGENT, or the per-process default). This is the explicit form of the
auto-claim every write already performs (cli-first migration Phase 2): it
renews the claiming agent's heartbeat when it already holds the entity, and
returns ErrActivityConflict — a domain error, not a stolen claim — when
another agent holds it. Presence is orthogonal to assignment (docs/DESIGN.md
§3): claiming a task does not move it to in_progress, and it does not take
ownership from another agent.`,
		Args: cobra.ExactArgs(1),
		RunE: runClaim,
	}
	cmd.Flags().String("kind", "working",
		"what the claim is for: working (default) or inspecting")
	return cmd
}

func runClaim(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	kindStr, _ := cmd.Flags().GetString("kind")
	return runStore(cmd, func(s *store.Store) error {
		entityType, id, err := resolveEntity(s, args[0])
		if err != nil {
			return err
		}
		kind, err := parseActivityKind(kindStr)
		if err != nil {
			return err
		}
		rowID, err := s.ClaimWork(entityType, id, agentIdentity(), kind)
		if err != nil {
			return err
		}
		printResult(jsonMode, func() {
			fmt.Fprintf(os.Stdout, "claimed %s %s as %s\n", entityType, id, kind)
		}, claimResultJSON{
			OK:         true,
			ID:         rowID,
			EntityType: entityType,
			EntityID:   id,
			Kind:       string(kind),
		})
		return nil
	})
}

// releaseResultJSON is `farol release`'s success payload: it reports whether a
// live claim was actually cleared (cleared=false is a normal state — the agent
// may not have held it, or the claim already expired, per ReleaseWork's no-op
// contract). `farol release --all` reports the count of cleared claims.
type releaseResultJSON struct {
	OK      bool `json:"ok"`
	Cleared int  `json:"cleared"`
}

func newReleaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release <task-id|list-id> [--all]",
		Short: "release presence on a task or list (clears the TUI spinner)",
		Long: `Release the current agent's presence claim on an entity. It is a no-op
when the agent does not hold the claim (or the claim already expired), so an
agent can release without remembering whether it claimed. Pass --all to clear
every live claim this agent holds across all entities (the per-process
session-end cleanup the retired MCP server performed), reusing
store.ReleaseAgentClaims.`,
		Args: func(cmd *cobra.Command, args []string) error {
			// --all clears every claim this agent holds, so the <id>
			// argument is optional then; otherwise exactly one id is required.
			all, _ := cmd.Flags().GetBool("all")
			if all {
				return nil
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: runRelease,
	}
	cmd.Flags().Bool("all", false, "release every claim this agent holds (ignore the <id> argument)")
	return cmd
}

func runRelease(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	all, _ := cmd.Flags().GetBool("all")
	return runStore(cmd, func(s *store.Store) error {
		if all {
			n, err := s.ReleaseAgentClaims(agentIdentity())
			if err != nil {
				return err
			}
			printResult(jsonMode, func() {
				if n == 0 {
					return // nothing to clear is a normal state (§9)
				}
				fmt.Fprintf(os.Stdout, "released %d claim(s)\n", n)
			}, releaseResultJSON{OK: true, Cleared: n})
			return nil
		}
		entityType, id, err := resolveEntity(s, args[0])
		if err != nil {
			return err
		}
		if err := s.ReleaseWork(entityType, id, agentIdentity()); err != nil {
			return err
		}
		printResult(jsonMode, func() {}, releaseResultJSON{OK: true, Cleared: 1})
		return nil
	})
}

// resolveEntity resolves an id prefix against both tables, picking the one it
// matches, mirroring how `farol work` resolves entity titles: a task id
// resolves against the task table, a list id against the list table. An id
// that matches neither is a domain error (exit 1).
func resolveEntity(s *store.Store, prefix string) (string, string, error) {
	if id, err := s.ResolveID("task", prefix); err == nil {
		return "task", id, nil
	}
	if id, err := s.ResolveID("list", prefix); err == nil {
		return "list", id, nil
	}
	return "", "", fmt.Errorf("no task or list matches id prefix %q", prefix)
}

// parseActivityKind validates the --kind flag into a store ActivityKind,
// rejecting anything other than the two the store accepts.
func parseActivityKind(s string) (store.ActivityKind, error) {
	switch store.ActivityKind(s) {
	case store.ActivityWorking, store.ActivityInspecting:
		return store.ActivityKind(s), nil
	}
	return "", fmt.Errorf("unknown kind %q (must be %q or %q)",
		s, store.ActivityWorking, store.ActivityInspecting)
}
