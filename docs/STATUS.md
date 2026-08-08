# Project status

## Alpha shipped

The repo/tool/command rename is finished and all roadmap phases are in
`main`, tagged `v0.1.0`:

- Repo/tool name: **chore-crusher**
- Command/binary name: **crush**
- Go module: `github.com/filipemolina/chore-crusher`
- Config dir: `~/.config/chore-crusher`
- Data dir / DB: `~/.local/share/chore-crusher/chore-crusher.db`
- Default theme: `gruvbox-dark`
- Current tag: `v0.1.0`

Phases 0–9 (scaffolding through polish/release) are complete. See
`docs/ROADMAP.md` for the split between the shipped alpha and the live
post-alpha backlog.

## Latest change

- **H13: Session-end claim release scoped to agent identity.** The MCP server enhancement plan (`§3.1`)
  promised that agent-claim spinners would be removed when the MCP session
  ends, but it was never wired (the 120s `WorkTTL` covered the gap). `Run` in
  `src/mcpserver/server.go` now calls `store.ReleaseAgentClaims()` after
  `server.Run` returns (client disconnected or context cancelled), clearing
  only the exiting agent's claims from the `AgentActivity` table so the TUI
  shows no stale spinners for that agent while other agents' claims remain.
  The new `ReleaseAgentClaims()` method in `src/store/activity.go` deletes
  all claims for the given agentID regardless of staleness (unlike
  `PruneStaleWork` which preserves fresh claims). Tests:
  `TestReleaseAgentClaimsLeavesOtherAgentsAlone` (store),
  `TestReleaseAgentClaimsRejectsEmptyAgent` (store),
  `TestReleaseAgentClaimsClearsOwnClaims` (store, retargeted),
  `TestAssignmentSurvivesReleaseAgentClaims` (assignment, retargeted),
  `TestMCPPendingClaimsClearedOnSessionEnd` (MCP integration, retargeted),
  `TestMCPAgentLivePresenceSurvivesOtherAgentSessionEnd` (MCP integration).

- Implemented the MCP server wrapper (`src/mcpserver`). `crush mcp` exposes
  every CLI operation as an MCP tool over stdin/stdout: lists, tasks, add,
  complete/reopen/toggle, rename, notes, progress, move, delete, and search.
  Tools return the same JSON shapes that the CLI emits with `--json`, and
  destructive operations still require `force=true`. Tests run against an
  in-memory MCP client/server pair in `src/mcpserver/server_test.go`.

- Fixed a flaky ordering bug in `src/store/tasks.go` (`ListTasks`). The
  previous SQL `ORDER BY position, created_at, id` produced a random order
  whenever two tasks shared the same `position` and the same one-second
  `created_at` bucket, because ULIDs contain random entropy in the same
  millisecond. `ListTasks` now returns rows in deterministic depth-first
  preorder (roots by position, then recursively each parent's children by
  position), so parents always precede their children and siblings stay in
  their stored order. The store test suite is stable again.

## Verification

- `go test -count=1 ./...` — pass
- `go test -race ./...` — pass
- `go vet ./...` — pass

## Caveats / deferred work

- No migration was written for existing `~/.config/complete` /
  `~/.local/share/complete` data. That data is left in place; a fresh install
  will use the new `chore-crusher` paths.

## Media

- `demo/demo.gif` and `demo/*.png` are regenerated for the UX redesign
  (Phase A: keymap + focus contract; Phase B: card chrome + sections +
  create UX). The `.tape` scripts use the current `tab`/`shift+tab` panel
  focus and `[`/`]` create-level bindings.
- Regenerate with:
  ```
  ./demo/seed.sh
  vhs demo/screenshots.tape
  vhs demo/demo.tape
  ```

## External plan file

A plan for improving the `pi` `sweep` extension (based on the repetitive audit
behavior during this rename) lives outside this repo at:

`~/.pi/agent/extensions/PLAN_SWEEP_IMPROVEMENTS.md`

It is not part of this project and should not be committed here.
