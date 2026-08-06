# Roadmap

The ordered plan from an empty repository to a usable alpha, and **why that
order**. The alpha (phases 0–9) is shipped; the rest of this file is the
live backlog after it.

`docs/plans/` holds the original how for each shipped phase.
`docs/plan/` holds post-alpha feature plans (MCP, UI, etc.).

## Alpha shipped

Phases 0 through 9 are done and merged in `main`, tagged `v0.1.0`. Each phase
was a feature branch of small commits, `go build ./... && go vet ./... && go
test ./...` green at every commit, merged with `--no-ff` so the phase can be
reverted as a unit — the same convention stack-stitcher's roadmap sets out,
adopted unchanged here.

| # | Phase | Plan |
| --- | --- | --- |
| 0 | Repo scaffolding: module, directory skeleton, CI, Makefile, release config | [`docs/plans/phase-0-scaffolding.md`](plans/phase-0-scaffolding.md) |
| 1 | Storage layer: schema, migrations, `store` package, the full state machine, unit tests | [`docs/plans/phase-1-storage.md`](plans/phase-1-storage.md) |
| 2 | CLI surface: every subcommand in `docs/DESIGN.md` §9, wired to `store` | [`docs/plans/phase-2-cli.md`](plans/phase-2-cli.md) |
| 3 | TUI shell: `AppModel`, ported theme system, layout, poll-tick refresh, quit/help/theme-picker | [`docs/plans/phase-3-tui-shell.md`](plans/phase-3-tui-shell.md) |
| 4 | Task tree: hierarchical rendering, vim/arrow nav, expand/collapse, `space` toggle with cascade | [`docs/plans/phase-4-task-tree.md`](plans/phase-4-task-tree.md) |
| 5 | Add input: the level rules from `docs/DESIGN.md` §4, submit/clear | [`docs/plans/phase-5-add-input.md`](plans/phase-5-add-input.md) |
| 6 | Lists panel: toggle, CRUD gated on visible+focused, switching lists | [`docs/plans/phase-6-lists-panel.md`](plans/phase-6-lists-panel.md) |
| 7 | Details screen: notes textarea, progress-kind/percent editor | [`docs/plans/phase-7-details-screen.md`](plans/phase-7-details-screen.md) |
| 8 | Search: local fuzzy filter (`/`) and the cross-list picker (`F`) | [`docs/plans/phase-8-search.md`](plans/phase-8-search.md) |
| 9 | Polish and release: narrow-terminal handling, `crush mv`, VHS demo, tagged release | [`docs/plans/phase-9-polish-release.md`](plans/phase-9-polish-release.md) |

### Decisions already taken

Settled while planning this project; do not re-open without a good reason
written down next to the reversal, the same courtesy stack-stitcher's
equivalent list asks for:

- **SQLite (`modernc.org/sqlite`, pure Go), not JSON-on-disk.** A human TUI
  and an agent's CLI invocations both mutate the same store, potentially
  concurrently; SQLite's WAL mode gives correct concurrent access for free,
  where a hand-rolled JSON-file lock would be new code carrying its own bugs
  for a problem a database already solved. See `docs/DESIGN.md` §8.
- **Polling, not a file watcher or a daemon.** Same non-IPC stance
  stack-stitcher takes with Docker; see `docs/DESIGN.md` §7. Default interval
  1000ms, configurable.
- **Cobra for the CLI**, not stdlib `flag`. Stack-stitcher's flat flag set
  never needed subcommands; this app's CLI *is* a first-class product surface
  with a dozen-plus verbs, and Cobra's `--help` generation matters for an
  agent introspecting the interface. This is a deliberate, documented
  departure from stack-stitcher's minimal-deps stance, not an oversight.
- **The binary is called `crush`.** `crush <task-id>` marking a task
  crush without opening the TUI is the founding example this whole project
  was scoped around; the name has to make that command read naturally.
- **Themes are ported from stack-stitcher, hex-for-hex**, with only the
  status-color field names changed to fit this app's domain. See
  `docs/DESIGN.md` §11. Do not redesign the palette or the tier-derivation
  math; import it.
- **No due dates, priorities, or assignees in the schema for the first
  alpha.** Every one of these is a plausible fast-follow and none is what
  the founding use case (an agent's task list, watched live) needs first.
  Adding one later is a migration, not a redesign, because `store`'s tables
  already isolate "the fields a task has" from "the code that reads them" —
  see `docs/DESIGN.md` §1.
- **Re-parenting outside the ±1-level add-flow (`crush mv`) is CLI-only
  and scheduled last (phase 9).** It's useful, it's cheap given the schema,
  and it is not blocking anything before it.

### Why that order, briefly

- Storage first because the status/progress state machine in
  `docs/DESIGN.md` §3 is the part most likely to be gotten subtly wrong,
  and it's cheapest to get right as plain Go functions under unit tests with
  no UI to also debug.
- CLI before TUI because a CLI-complete, TUI-less binary is already the tool
  an agent needs — shipping that early means the founding use case works
  before the second, harder-to-get-right front end exists, and it forces the
  `store` API to be good enough for a caller with no screen before a UI's
  convenience methods get a chance to leak into it.
- TUI shell before task tree because layout, theming, and the poll loop are
  infrastructure every later screen sits inside.
- Details and search are deliberately last among the UI phases: both are
  additive to an already-usable list-and-tree app, while the tree, the
  add-flow, and the lists panel are not.

---

## MCP server track (agent todo store)

The founding use case after alpha: **agents use Chore Crusher as their todo
list, and humans create tasks for agents to work on**, with the TUI as the
live dashboard. Plans live under `docs/plan/` (and the comfort plan at the
repo root). Do not add MCP tools past the ~20 ceiling without revisiting
comfort S4.

### Shipped

| Plan | What landed | Status |
| --- | --- | --- |
| [`docs/plan/mcp-server-enhancement.md`](plan/mcp-server-enhancement.md) | Discovery (resources + prompts), agent presence (`claim_work` / spinner), tightened tool descriptions | ✅ Complete (A–L) |
| [`docs/plan/agent-presence-heartbeat.md`](plan/agent-presence-heartbeat.md) | Write-heartbeat on status/progress; lists-panel spinner for any claimed task inside a list; `CRUSH_AGENT` identity | ✅ Complete (A–F) |
| [`MCP_COMFORT_PLAN.md`](../MCP_COMFORT_PLAN.md) | `SQLITE_BUSY` fix, `show_task` children, prefixed `Instructions` names, always-on todo rule, `my_list` | ✅ Complete (S1–S6) |
| [`docs/plan/list-ownership-enforcement.md`](plan/list-ownership-enforcement.md) | `created_by` on lists; MCP refuses structural writes on foreign/untagged lists; status/progress stay open | ✅ Complete (A–F) |

**Surface today:** `crush mcp` — **21 tools**, **6 resources**, **2 prompts**;
identity from `CRUSH_AGENT` (default `agent`); cooperative-trust ownership
(not auth). Full contract: `docs/DESIGN.md` §9.

### Next (do this before new MCP features)

| Plan | Why it is next | Status |
| --- | --- | --- |
| [`docs/plan/mcp-agent-todo-hardening.md`](plan/mcp-agent-todo-hardening.md) | Closes P0 bugs left by the four plans above: hardcoded `"pi"` in `Instructions`, `claim_work` defaulting `agent_id` to `"agent"` (breaks the write-heartbeat when `CRUSH_AGENT≠agent`), `my_list` returning an untagged name-prefixed list, CLI `crush show` empty children, missing `created_by` on list resources, no clean human→agent handoff (`--owner` / rename-adopt), `list_owner` on task read shapes | ✅ Complete (A–I + §10.5) |

Recommended order inside that plan: **A → B → C** (presence + identity
lies) before **D–I** (CLI parity, handoff, docs/tests).

### Chore Crusher list triage (2026-08-05)

Six new plan docs, written to close out every pending item in the
"Chore Crusher" list (`crush show` on the parent Bugs/UI/Features/MCP
Servers tasks). Recommended order — bugs first (small, no product
decisions), then UI, then features, rename last (breaking, touches
everything above it):

| Plan | Covers | Status |
| --- | --- | --- |
| [`docs/plan/chore-crusher-bug-fixes.md`](plan/chore-crusher-bug-fixes.md) | 6 filed bugs (filter-esc, Lists-panel filter, paste, task indent, create-above-with-children, delete-empty-list) + per-user-DB doc note | 🔲 Planned |
| [`docs/plan/ui-improvements.md`](plan/ui-improvements.md) | Loading animation, responsive Lists panel, document-icon column, scrollable panels, default theme (retargeted catppuccin mocha), + new Commit 6: list name in Tasks panel header | 🔲 Planned (retargeted + extended 2026-08-05) |
| [`docs/plans/details-panel-title-editing.md`](plans/details-panel-title-editing.md) | Editable task title + `e` edit shortcut — depends on `docs/plans/details-as-sidepanel.md` landing first | 🔲 Planned |
| [`docs/plan/tui-interaction-polish.md`](plan/tui-interaction-polish.md) | Enter/Esc panel-close behavior, select-new-list-closes-panel, copy-id-to-clipboard shortcut, collapse-semantics (needs confirmation) | 🔲 Planned |
| [`docs/plan/task-comments.md`](plan/task-comments.md) | Comments on tasks — schema, store, CLI, MCP (raises tool ceiling to 21, needs sign-off), TUI icon + cards | 🔲 Planned |
| [`docs/plan/agent-scratch-list-cleanup.md`](plan/agent-scratch-list-cleanup.md) | Agent's auto-created `<identity>: Inbox` list deleted at session end once fully complete — never a human-named list | 🔲 Planned |
| [`docs/plan/rename-lists-to-projects.md`](plan/rename-lists-to-projects.md) | Full "Lists" → "Projects" rename: DB, store, CLI, MCP tool names, TUI, docs. Run **last**, after everything above | 🔲 Planned |

### Deferred MCP follow-ups (after hardening)

- ~~Session-end claim release (enhancement promised it; TTL covers it today).~~ **Done** — `Run` now calls `s.ReleaseAllClaims()` after `server.Run` returns, closing the gap between the enhancement plan §3.1 promise and the implementation (H13). All claims clear on session disconnect so the TUI shows no stale spinners.
- Optional: show list owner in the TUI; comments gated by `requireWritable`.
- Do **not** add an `adopt_list` MCP tool unless the hardening CLI path
  proves insufficient — keep the tool-count ceiling.

---

## Other live backlog

Not scheduled as a single queue — pick one up after writing down the product
decision it depends on.

### Product / data model

- **Due dates and a `StatusOverdue` that means something.** The theme
  registry already reserves the color (`docs/DESIGN.md` §11); nothing reads
  it until this lands.
- **Priorities and assignees.** Two more optional columns on `Task`, cheap in
  the schema, deferred because neither was part of the founding use case.
- **Sync or export** (git-friendly export a la Backlog.md; a CalDAV bridge).
  Needs a decision about which, if either, before any code — see
  `docs/DESIGN.md` §1.

### UI (post-alpha)

| Plan | Notes | Status |
| --- | --- | --- |
| [`docs/plan/ui-improvements.md`](plan/ui-improvements.md) | catppuccin-mocha default, loading animation, responsive lists panel, document-icon column, scrollable panels, list name in Tasks header | See "Chore Crusher list triage" above |
| [`docs/plans/details-as-sidepanel.md`](plans/details-as-sidepanel.md) | Details as a side panel instead of a full screen | Proposal — [`docs/plans/details-panel-title-editing.md`](plans/details-panel-title-editing.md) depends on this landing first |
| [`docs/plan/task-row-redesign-and-inline-creation.md`](plan/task-row-redesign-and-inline-creation.md) | Task row / inline creation redesign | See plan status |
| [`docs/plan/task-row-cards-and-status.md`](plan/task-row-cards-and-status.md) | Card chrome + status presentation | See plan status |
| [`docs/plans/ux-redesign.md`](plans/ux-redesign.md) | Broader UX redesign notes | Historical / reference |

### Sister / context

- [`docs/plans/stack-stitcher-sister-tui.md`](plans/stack-stitcher-sister-tui.md) —
  why this project mirrors stack-stitcher's discipline.
