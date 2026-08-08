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
list, and humans create tasks for agents to work on**, with the TUI as the live
dashboard. Plans live under `docs/plan/` (and the comfort plan at the repo
root).

**Surface today:** `crush mcp` — **12 tools, 2 resources, 2 prompts**. The
identity an agent acts under comes from `CRUSH_AGENT` when set, and is
otherwise generated per process; ownership is cooperative trust, not auth. Full
contract: `docs/DESIGN.md` §9. The tool count is a deliberate ceiling: it trades
tool count against *call* count, and call count wins.

### Shipped

**`mcp-server-enhancement.md`** — the foundation. Added discovery so an agent
could learn the API without trial and error (resources and prompts), presence
claims with the TUI spinner, and tightened every tool description.

**`agent-presence-heartbeat.md`** — made presence self-maintaining. Any status
or progress write refreshes the writing agent's claim, so the spinner tracks
real work instead of an explicit claim call, and a list shows a spinner when
anything inside it is claimed.

**`MCP_COMFORT_PLAN.md`** — the round of fixes that made the server usable in
anger: the `SQLITE_BUSY` failure under concurrent access, `show_task` returning
children, prefixed names in the `Instructions` blob, and `my_list` as a
one-call session opener.

**`list-ownership-enforcement.md`** — gave lists a `created_by` tag and taught
the server to refuse structural writes on lists it does not own, while leaving
status and progress open to everyone. This is the rule the whole multi-agent
story rests on.

**`mcp-agent-fewer-roundtrips.md`** and **`mcp-batch-writes.md`** — batch reads
and batch writes, so an agent touching fifty tasks makes one call rather than
fifty. The write half (`update_tasks`) was later absorbed into `set_status`.

**`mcp-list-changes-since.md`** — a way to ask what changed since a timestamp
instead of re-reading a whole list. Later folded into `list_tasks(since=…)`.

**`mcp-agent-todo-hardening.md`** — closed the P0 bugs the four plans above
left behind, most of them lies in agent-facing text: a hardcoded identity in
the `Instructions` blob, presence defaulting to the wrong tag, `my_list`
returning a list the agent could not write.

**`mcp-tool-consolidation.md`** — cut the surface from 24 tools to 14 by
merging tools that differed only in which field they wrote, and moved the
working loop out of the always-on blob into a prompt.

**`mcp-assignment-and-priorities.md`** — the largest plan in the track, thirteen
steps. It added durable task assignment (`assign_task` / `next_task`, so two
agents never research the same thing), a four-value priority that `next_task`
sorts by, self-contained read payloads with a byte budget that never cuts a
note mid-text, and the TUI surfaces for both. It also cut the surface to its
current 12 tools and 2 resources. Its §13 end-to-end verification has been run
against live sessions, and found two real defects that its own test suite had
missed — both since fixed.

**`session-end-claim-release-scoping.md`** — a verification finding. Session-end
cleanup deleted *every* agent's presence claims, not just the exiting one's, so
one agent disconnecting made every other live agent read as abandoned.

**`session-scoped-agent-identity.md`** — removed the setup burden. The identity
used to default to the constant `"agent"`, so two unconfigured clients acted as
one and overwrote each other with no refusal and no audit trail. Identity is now
unique per process, and a session releases its claims, its assignments and its
empty auto-created Inbox when it ends.

### Ahead

**`agent-working-loop-instructions.md`** — better always-on guidance so an agent
keeps the store current as it works without being asked: update status as you
go, scale percentages to task size, read notes and comments before and after,
re-check the list between tasks. No new tools. This is the highest-value item
left in the track, because everything else here is machinery an agent only
benefits from if its instructions tell it to use them.

**`agent-scratch-list-cleanup.md`** — deleting the agent's auto-created
`<identity>: Inbox` when it is finished with, never a human-named list. Now
partly done: session end already removes that Inbox when it is *empty*. What
remains is the harder case — an Inbox holding completed work — which needs a
product decision about what "done with it" means.

**`mcp-presence-on-all-writes.md`** — **believed already satisfied.** It asked
for `autoClaim` on comment, add, rename, notes and move; those all now claim,
partly because `edit_task` absorbed rename/notes/move and partly because grabs
were taught to claim. Worth a read-through to confirm and close rather than
implement.

**`rename-lists-to-projects.md`** — the "Lists" → "Projects" rename across DB,
store, CLI, MCP tool names, TUI and docs. Breaking, touches everything, and
should run **last**.

### Deferred MCP follow-ups

- Optional: show list owner in the TUI; comments gated by `requireWritable`.
- Do **not** add an `adopt_list` MCP tool unless the hardening CLI path proves
  insufficient — keep the tool-count ceiling.

---

## Other live backlog

Not scheduled as a single queue — pick one up after writing down the product
decision it depends on.

### Product / data model

- **Due dates and a `StatusOverdue` that means something.** The theme
  registry already reserves the color (`docs/DESIGN.md` §11); nothing reads
  it until this lands.
- ~~**Priorities and assignees.**~~ **Shipped** — both landed with
  `docs/plan/mcp-assignment-and-priorities.md` as `Task.priority` and
  `Task.assignee`, along with the tools, CLI commands and TUI badges that
  read them. The "Decisions already taken" note above deferring them was
  scoped to the *first alpha* and stands as a record of that scope.
- **Sync or export** (git-friendly export a la Backlog.md; a CalDAV bridge).
  Needs a decision about which, if either, before any code — see
  `docs/DESIGN.md` §1.

### TUI and bug-fix plans

These came out of a triage of the Chore Crusher list itself (2026-08-05) and
are independent of the MCP track.

**`chore-crusher-bug-fixes.md`** — *in progress.* Six filed bugs, each small
and free of product decisions: filter-escape behaviour, the Lists-panel
filter, paste handling, task indenting, creating a task above one that has
children, and deleting an empty list.

**`ui-improvements.md`** — *partly landed.* The catppuccin-mocha default is
in; the remainder is a loading animation, a responsive Lists panel, a
document-icon column, scrollable panels, and the list name in the Tasks
header.

**`tui-interaction-polish.md`** — *not started.* Five small independent
interaction fixes: Enter/Esc panel-close behaviour, selecting a new list
closing the panel, a copy-id shortcut, and collapse semantics (which still
needs confirmation).

**`task-comments.md`** — *in progress,* commits 1–3 done. Comments on tasks
through schema, store, CLI and MCP; what remains is commit 4, the TUI icon
column.

**`agent-scratch-list-cleanup.md`** and **`agent-working-loop-instructions.md`**
are tracked in the MCP track above.

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
