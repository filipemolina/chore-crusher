# Roadmap

The ordered plan from an empty repository to a usable alpha, and **why that
order**. `docs/plans/` holds the how for each phase; this file is the
sequence, so picking up at phase 4 does not mean re-deciding what phases 1–3
already settled. This file is **live** — update it as phases land, the same
way stack-stitcher's roadmap is kept current, and move a finished phase's
design detail into `docs/DESIGN.md` rather than leaving it only here.

There is no "alpha shipped" line yet to split this file the way
stack-stitcher's is split into "where we are" (done) and "the order after"
(live) — every phase below is still ahead of us. Re-split this file into
those two halves once phase 6 (the first point at which the app is usable
end-to-end by both a human and a script) lands.

## Decisions already taken

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
- **The binary is called `complete`.** `complete <task-id>` marking a task
  complete without opening the TUI is the founding example this whole project
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
- **Re-parenting outside the ±1-level add-flow (`complete mv`) is CLI-only
  and scheduled last (phase 9).** It's useful, it's cheap given the schema,
  and it is not blocking anything before it.

## Phases

Each phase is a feature branch of small commits, `go build ./... && go vet
./... && go test ./...` green at every commit, merged with `--no-ff` so the
phase can be reverted as a unit — the same convention stack-stitcher's
roadmap sets out, adopted unchanged here.

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
| 9 | Polish and release: narrow-terminal handling, `complete mv`, VHS demo, tagged release | [`docs/plans/phase-9-polish-release.md`](plans/phase-9-polish-release.md) |

**Why this order, briefly** (each plan file repeats its own reasoning in
more depth): storage before anything else because §3's state machine is the
part most likely to be gotten subtly wrong, and it's cheapest to get right
while it's plain Go functions under unit tests with no UI to also debug. CLI
before TUI, not after, because a CLI-complete, TUI-less binary is already the
tool an agent needs — shipping that early means the founding use case works
before the second, harder-to-get-right front end exists, and it forces the
`store` API to be good enough for a caller with no screen before a UI's
convenience methods get a chance to leak into it. TUI shell before the task
tree because layout, theming, and the poll loop are infrastructure every
later phase's screen sits inside. Details and search are deliberately last
among the UI phases: both are additive to an already-usable list-and-tree
app, where the tree, the add-flow, and the lists panel are not.

## Post-alpha, parked on purpose

Not scheduled, and not forgotten — each needs a decision this plan
deliberately isn't making yet:

- **Due dates and a `StatusOverdue` that means something.** The theme
  registry already reserves the color (`docs/DESIGN.md` §11); nothing reads
  it until this lands.
- **Sync or export** (git-friendly export a la Backlog.md; a CalDAV bridge).
  Needs a decision about which, if either, before any code — see
  `docs/DESIGN.md` §1 on why this isn't "just add a sync target."
- **An MCP server wrapper**, so an agent can call `complete`'s operations as
  tool calls instead of shelling out to the CLI. The CLI contract in
  `docs/DESIGN.md` §9 is deliberately uniform enough that this would be a
  thin adapter over it whenever it's wanted, not a redesign.
- **Priorities and assignees.** Two more optional columns on `Task`, cheap in
  the schema, deferred because neither was part of the founding use case and
  adding fields nobody asked for yet is exactly the kind of scope creep
  `docs/DESIGN.md` §1 rules out for the first alpha.
