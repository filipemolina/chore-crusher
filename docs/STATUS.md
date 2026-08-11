# Project status

## Alpha shipped

**2026-08: renamed Chore Crusher → Farol** (repo, binary, MCP URIs, env var,
themes, config/data dirs). The entries below keep the names the app had when
they were written; this note covers them.

The repo/tool/command rename is finished and all roadmap phases are in
`main`, tagged `v0.1.0`:

- Repo/tool name: **farol**
- Command/binary name: **farol**
- Go module: `github.com/filipemolina/farol`
- Config dir: `~/.config/farol`
- Data dir / DB: `~/.local/share/farol/farol.db`
- Default theme: `farol-dark` (brand pair; see `docs/DESIGN.md` §11)
- Current tag: `v0.1.0`

Phases 0–9 (scaffolding through polish/release) are complete. See
`docs/ROADMAP.md` for the split between the shipped alpha and the live
post-alpha backlog.

## Latest change

- **The brand themes are the default.** `farol-dark` and `farol-day` are no
  longer copies of stack-stitcher's palettes: both derive from the logo
  family's navy/amber/cream (deep navy `#0E1B30` surfaces, lamp amber
  `#F0B263` accent, cream `#F5EDE4` text; the light variant inverts them
  with a darkened lamp amber `#A06A0E` accent). `DefaultTheme` moved from
  `farol-ember` to `farol-dark`, so a fresh install opens in the brand
  colors; farol-ember stays selectable through `T` and as a saved
  `theme:` value. Demo media stays pinned to farol-ember (a deliberate
  recording choice, see `demo/seed.sh`). Tests:
  `TestDefaultThemeIsFarolDark` (appstyles). See `docs/DESIGN.md` §11.

- **Light-theme readability: no glyph ever draws in the terminal's default
  color.** On `crush-day`, pending and in-progress task titles rendered
  white-on-white: the row renderer styled complete titles but left the
  pending/in-progress ones unstyled, so they inherited the terminal default,
  and the same class of leak lived in every Bubbles text input (default
  style carries no foreground on focused text, a hardcoded white on the
  blurred one, and a white `> ` prompt) plus the lists panel's filter bar.
  The fix is an invariant, not a spot-patch: every glyph now draws from an
  `appstyles.Active` tier — `renderRow` styles every title state and the
  `▾`/`▸` marker with the row's own tier — and every input is sealed every
  render by the new shared `chrome.SealInput` / `chrome.SealListFilter`
  (detailspanel's inline seal refactored onto the same helper). New
  `appstyles.HasDefaultForeground` asserts the invariant: full-frame tests
  under crush-day and the default theme (`src/model/foreground_test.go`),
  per-row tests (`src/components/tasktree/foreground_test.go`), and an SGR
  state-machine table (`src/appstyles/Foreground_test.go`).
  `docs/DESIGN.md` §12 gains the foreground-tier rule beside the
  background-sealing rule.

- **Default theme is now crush-ember, and a saved theme survives restart.**
  `DefaultTheme` moved from `gruvbox-dark` to the app's own warm
  `crush-ember` palette (gruvbox-dark stays selectable through `T`). It
  also completes the theme persistence round trip: the picker's Enter
  always wrote `theme:` to `~/.config/chore-crusher/config.yaml`
  (`cmds.ApplyTheme`), but the TUI boot path never read it back, so a
  chosen theme died with the process. The root command's TUI path now
  applies `cfg.Theme` via `appstyles.SetTheme` before the first frame
  (an empty or unknown name falls back to the default). Tests:
  `TestDefaultThemeIsCrushEmber` (appstyles),
  `TestSavedThemeAppliedAtStartup` (cli). Demo media regenerated under
  the new default.

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

- The 2026-08 farol rename ships a one-shot migration
  (`config.MigrateLegacyDirs`) that moves `~/.config/chore-crusher` and
  `~/.local/share/chore-crusher` to the farol paths on first launch. Data
  from the earlier `~/.config/complete` era (pre-chore-crusher) predates
  that and is left in place.

## Media

- `demo/demo.gif` and `demo/*.png` are regenerated for the UX redesign
  (Phase A: keymap + focus contract; Phase B: card chrome + sections +
  create UX). The `.tape` scripts use the current `[`/`]` create-level
  bindings; tab/shift+tab panel focus works everywhere except while the
  inline create input is live (focus is locked to the input then).
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
