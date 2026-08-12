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

- **Export and import.** `farol export [list-id] [--out <file>]` writes the store
  (or one list) to a versioned JSON document; `farol import <file> [--list <id>]`
  recreates lists additively with fresh ULIDs. The TUI mirrors this with `e` /
  `i` keys in the Lists panel: `e` opens an export modal (path + whole-store /
  this-list toggle), `i` opens an import modal (path input). Both route through
  `store.Export` / `store.ImportList`, the same mutators the CLI uses, so the
  two surfaces never diverge on a write path. `IMPORT`/`EXPORT` keys are bound to
  `L` context only (free in the existing binding set). See `docs/DESIGN.md` §6
  (keybindings) and §9 (CLI). Tests: `go test ./src/cli/... ./src/store/... ./src/components/importexportmodal/...`.

- **lastError is now rendered in the TUI.** A failed export/import (bad path,
  unparseable file, version mismatch) was previously invisible: `lastError` was
  written on every error but never surfaced. `statusView` now renders it as a
  single themed strip (Danger color) between the body and the footer, truncated
  to width. A clean `RefreshListsMsg` / `RefreshTasksMsg` clears the message so a
  later success removes the stale error. Tests:
  `TestFailedImportRendersErrorInStatusLine` and
  `TestSuccessfulRoundTripClearsLastError` in `src/model/lists_panel_trio_test.go`.

- **The brand themes are the default.** `farol-dark` and `farol-day` are no
  longer copies of stack-stitcher's palettes: both derive from the logo
  family's navy/amber/cream (deep navy `#0E1B30` surfaces, lamp amber
  `#F0B263` accent, cream `#F5EDE4` text; the light variant inverts them
  with a darkened lamp amber `#A06A0E` accent). `DefaultTheme` moved from
  `farol-ember` to `farol-dark`, so a fresh install opens in the brand
  colors; farol-ember stays selectable through `T` and as a saved
  `theme:` value. Demo media is pinned to farol-dark (see `demo/seed.sh`)
  to match the fresh-install default. Tests:
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

- **CLI now enforces list ownership on structural writes** (closing the
  parity-1 policy gap between the CLI and the retired MCP server). `farol
  add`, `rename`, `notes`, `mv`, `priority`, and `rm` each resolve the owning
  list and refuse the write when its `created_by` is neither the current
  agent (`FAROL_AGENT`) nor the list's `collaborative` flag — with the exact
  refusal message the server used (`list <id> is owned by <owner> — you may
  read it and update task status/progress only`). Every structural command
  gained a `--force` flag that overrides the refusal, mirroring the server's
  refuse-with-override rule. Status/progress writes, assignment,
  comment add/delete, and all reads stay ungated, exactly as the server had
  them: any agent may update status/progress and assign/unassign
  cooperatively. `docs/DESIGN.md` §9's ownership section now records the CLI
  as the enforcement site (previously it described the gate as never
  enforced on the CLI/TUI); `src/cli/ownership.go` holds the shared guard,
  and the CLI tests were repointed to create agent-owned lists (or pass
  `--force` where a list must stay unowned). Tests: `go test ./src/cli/...`
  green, including the rewritten `TestCLICanRenameForeignOwnedList` which now
  pins the refusal-without-`--force` and the override-with-`--force`.

- **`farol work` closes parity gap #1** (the retired `farol:///work`
  resource). `farol work` is a read-only, non-interactive command that mirrors
  the resource exactly in `--json` mode: one JSON value on stdout, a bare
  array of `{id, entity_type, entity_id, agent_id, kind, acquired_at}` rows
  — the same field names and order the resource emitted, so a host that read
  `farol:///work` reads the same live-presence rows here (presence, not
  assignment: `assignee` is a separate axis and deliberately not part of this
  read). Human mode prints a plain `tabwriter` table (`AGENT ENTITY TITLE KIND
  AGE`) with the claimed entity's title resolved best-effort and the claim age
  in the `2h14m ago` form the §9 takeover message uses — the ergonomic a
  static JSON blob could not offer — and prints nothing when there are no
  live claims (empty is a normal state, not an error). It claims no presence
  of its own. `docs/DESIGN.md` §9 gains the subcommand and pins both shapes;
  `src/cli/work.go` is the implementation, `src/cli/work_test.go` pins the
  mirror (`TestWorkJSONMirrorsResource`), the one-value rule
  (`TestWorkJSONIsOneValue`), the empty state (`TestWorkEmptyPrintsNothing`),
  and the human table including the presence-not-assignment boundary
  (`TestWorkHumanTable`). Tests: `go test ./src/cli/...` green.

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

## MCP server retired

The `src/mcpserver` package and the `farol mcp` command described in the
"Latest change" entries above were deleted once the CLI reached full parity
(cli-first migration: `farol inbox` parity 1.7 was the last milestone). The
release-agent-claims and presence machinery those entries describe now lives
only in `src/store` and the CLI (`src/cli/presence.go`); the `modelcontextprotocol/go-sdk`
dependency was dropped. The historical entries above are kept as a record of
what shipped.
