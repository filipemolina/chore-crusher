# Roadmap

The ordered plan from an empty repository to a usable alpha, and **why that
order**. The alpha (phases 0–9) is shipped; the rest of this file is the
live backlog after it.

This file is the record. The per-phase and per-feature plans it was written
from were drafting artifacts and are no longer in the repository; what they
decided is summarised here, and the contracts they set are in
`docs/DESIGN.md`.

## Alpha shipped

**2026-08: renamed Chore Crusher → Farol** (repo, binary, MCP URIs, env var,
themes, config/data dirs). Past entries below keep the names the app had when
they were written; this note covers them. Same day: `farol-dark` and
`farol-day` stopped being copies of stack-stitcher's palettes and became the
brand pair built from the logo's navy/amber/cream (see `docs/DESIGN.md` §11).

Phases 0 through 9 are done and merged in `main`, tagged `v0.1.0`. Each phase
was a feature branch of small commits, `go build ./... && go vet ./... && go
test ./...` green at every commit, merged with `--no-ff` so the phase can be
reverted as a unit — the same convention stack-stitcher's roadmap sets out,
adopted unchanged here.

| # | Phase |
| --- | --- |
| 0 | Repo scaffolding: module, directory skeleton, CI, Makefile, release config |
| 1 | Storage layer: schema, migrations, `store` package, the full state machine, unit tests |
| 2 | CLI surface: every subcommand in `docs/DESIGN.md` §9, wired to `store` |
| 3 | TUI shell: `AppModel`, ported theme system, layout, poll-tick refresh, quit/help/theme-picker |
| 4 | Task tree: hierarchical rendering, vim/arrow nav, expand/collapse, `space` toggle with cascade |
| 5 | Add input: the level rules from `docs/DESIGN.md` §4, submit/clear |
| 6 | Lists panel: toggle, CRUD gated on visible+focused, switching lists |
| 7 | Details screen: notes textarea, progress-kind/percent editor |
| 8 | Search: local fuzzy filter (`/`) and the cross-list picker (`F`) |
| 9 | Polish and release: narrow-terminal handling, `farol mv`, VHS demo, tagged release |

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
- **The binary is called `farol`.** `farol <task-id>` marking a task
  complete without opening the TUI is the founding example this whole project
  was scoped around; the name has to make that command read naturally.
  (Renamed from `crush` in 2026-08, with the repo and the MCP URIs.)
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
- **Re-parenting outside the ±1-level add-flow (`farol mv`) is CLI-only
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

The founding use case after alpha: **agents use Farol as their todo
list, and humans create tasks for agents to work on**, with the TUI as the live
dashboard.

**Surface today:** the CLI is the single agent-facing front end. The MCP
server (`farol mcp`) was retired in the cli-first migration in favour of the
CLI; this section is kept as a record of what the MCP track shipped. The
identity an agent acts under comes from `FAROL_AGENT` when
set, and is otherwise generated per process; ownership is cooperative trust,
not auth. Full contract for the CLI: `docs/DESIGN.md` §9. The planned CLI
surface trades tool count against *call* count, and call count wins — which is
why batch reads (`farol show <id>...`), `--since`, and `--include` are part of
the parity work.

### Shipped

**Discovery, presence and better tool descriptions** — the foundation. Added
discovery so an agent could learn the API without trial and error (resources
and prompts), presence claims with the TUI spinner, and tightened every tool
description.

**Self-maintaining presence** — any status or progress write refreshes the
writing agent's claim, so the spinner tracks real work instead of an explicit
claim call, and a list shows a spinner when anything inside it is claimed.

**The comfort round** — the fixes that made the server usable in anger: the
`SQLITE_BUSY` failure under concurrent access, `show_task` returning children,
prefixed names in the `Instructions` blob, and `my_list` as a one-call session
opener.

**List ownership enforcement** — gave lists a `created_by` tag and taught
the server to refuse structural writes on lists it does not own, while leaving
status and progress open to everyone. This is the rule the whole multi-agent
story rests on.

**Batch reads and batch writes** — so an agent touching fifty tasks makes one
call rather than fifty. The write half (`update_tasks`) was later absorbed
into `set_status`.

**Change detection** — a way to ask what changed since a timestamp instead of
re-reading a whole list. Later folded into `list_tasks(since=…)`.

**Hardening the agent-facing text** — closed the P0 bugs the four plans above
left behind, most of them lies in agent-facing text: a hardcoded identity in
the `Instructions` blob, presence defaulting to the wrong tag, `my_list`
returning a list the agent could not write.

**Tool consolidation** — cut the surface from 24 tools to 14 by
merging tools that differed only in which field they wrote, and moved the
working loop out of the always-on blob into a prompt.

**Assignment and priorities** — the largest plan in the track, thirteen
steps. It added durable task assignment (`assign_task` / `next_task`, so two
agents never research the same thing), a four-value priority that `next_task`
sorts by, self-contained read payloads with a byte budget that never cuts a
note mid-text, and the TUI surfaces for both. It also cut the surface to its
current 12 tools and 2 resources. Its §13 end-to-end verification has been run
against live sessions, and found two real defects that its own test suite had
missed — both since fixed.

**Presence on all writes** — closed without ever being run. It asked
for `autoClaim` on comment, add, rename, notes and move so that any write, not
just a status change, lights the TUI spinner. The consolidation merged
rename/notes/move into `edit_task` and `add_comment` into `comment`, and those
merged handlers already carried the call; grabs were taught to claim
separately. Verified behaviourally against a running server, including the
deliberate carve-out that `delete_task` claims nothing.

**Session-end claim release, scoped** — a verification finding. Session-end
cleanup deleted *every* agent's presence claims, not just the exiting one's, so
one agent disconnecting made every other live agent read as abandoned.

**Session-scoped agent identity** — removed the setup burden. The identity
used to default to the constant `"agent"`, so two unconfigured clients acted as
one and overwrote each other with no refusal and no audit trail. Identity is now
unique per process, and a session releases its claims, its assignments and its
empty auto-created Inbox when it ends.

### Ahead

**Working-loop instructions** — better always-on guidance so an agent
keeps the store current as it works without being asked: update status as you
go, scale percentages to task size, read notes and comments before and after,
re-check the list between tasks. No new tools. This is the highest-value item
left in the track, because everything else here is machinery an agent only
benefits from if its instructions tell it to use them.

**Scratch-list cleanup** — deleting the agent's auto-created
`<identity>: Inbox` when it is finished with, never a human-named list. Now
partly done: session end already removes that Inbox when it is *empty*. What
remains is the harder case — an Inbox holding completed work — which needs a
product decision about what "done with it" means.

### Deferred follow-ups

- Optional: show list owner in the TUI; comments gated by owner.
- Do **not** add an `adopt_list` tool unless the hardening CLI path proves
  insufficient — keep the command surface lean.

---

## Other live backlog

Not scheduled as a single queue — pick one up after writing down the product
decision it depends on.

### Product / data model

- ~~**Priorities and assignees.**~~ **Shipped** — both landed with the
  assignment-and-priorities work as `Task.priority` and `Task.assignee`,
  along with the tools, CLI commands and TUI badges that read them. The
  "Decisions already taken" note above deferring them was scoped to the
  *first alpha* and stands as a record of that scope.

- ~~**Export and import.**~~ **Shipped** — `farol export [list-id] [--out <file>]`
  and `farol import <file> [--list <id>]` (mirror with `e` / `i` in the Lists
  panel) move lists and tasks between stores as versioned JSON. Export reuses
  `ListTasks`' depth-first preorder so parents precede children; import
  regenerates ULIDs and rewrites parent links through an old→new id map
  within a single transaction. Additive: existing data is never overwritten.
  See `docs/DESIGN.md` §6 (keybindings) and §9 (CLI).

### TUI and bug-fix plans

These came out of a triage of the Chore Crusher list itself (2026-08-05) and
are independent of the MCP track. All four have since landed; the paragraphs
below are what each one found, because the findings outlive the fixes.

**The bug-fix triage** — *done.* Six bugs, each small and free of
product decisions. Three had a confirmed cause on inspection: `esc` never left
task-filter mode, because the esc ladder claimed the key only once a filter was
*applied* and not while one was being typed; the Lists panel could not be
filtered at all, because its filter was switched off deliberately to avoid
colliding with the global `/` — resolved by making `/` contextual, so it
filters whichever panel has focus; and paste was silently dropped everywhere,
because Bubble Tea v2 delivers a bracketed paste as a `tea.PasteMsg` that no
component forwarded to its text input. Two needed reproduction first and turned
out not to be the bugs they were reported as. "Tasks sometimes don't indent"
is a stale-rows race: every structural gesture computes its target from the
tree's rows, and a second keypress inside the window before the refresh lands
acts on a layout the user is no longer looking at — fixed by deferring the
gesture and replaying it against the fresh rows. "Creating a task above a task
with children" is a disagreement between the ghost create row's render anchor
and the real insertion anchor — fixed by rendering after the anchor's last
visible descendant, where the committed task was already going. The sixth,
"can't delete an empty list", was the delete handler targeting the *active*
list instead of the *highlighted* one; the two agree until a picker jump or a
previous delete pulls them apart. A seventh item was documentation only: the
database is already per-OS-user because its path derives from `$XDG_DATA_HOME`,
and `docs/DESIGN.md` §8 now says so, so nobody re-files it as a feature.

One gap it deliberately left open: `lastError` was written in several places and
rendered in none, so errors were invisible in the TUI. **Closed** — `statusView`
now renders `lastError` as a themed strip between the body and the footer (see
`src/model/View.go` and `docs/STATUS.md`), so a failed export/import surfaces
visibly and a clean refresh clears the message.

**TUI interaction polish** — *four of five landed.* Enter on a list
selects it and closes the panel, `esc` closes the panel (after first clearing
an active filter, so the ladder keeps its precedence), and a newly created list
becomes the active one — the Lists panel is now a transient picker rather than
a surface you dismiss by hand. Copy-id landed in the Details panel rather than
the tree: `ctrl+y` copies the task id and `y` a comment id, both through Bubble
Tea's own OSC 52 `tea.SetClipboard`, so it needs no `xclip`/`pbcopy` and works
over SSH. The fifth item, "collapsing a task should collapse all children", was
never reproducible as filed — rendering already hid every descendant of a
collapsed parent — and collapse now cascades for real. What the report might
have meant is still unanswered: whether re-expanding a parent should also clear
its descendants' own collapse flags, or whether the ask was for a bulk
collapse-all that does not exist.

**Task comments** — *shipped, all five commits.* Comments are append-only
and authored: a human's comment is attributed to the OS username, an agent's to
its MCP identity, since the app has no login to hang a user concept off. They
are sorted oldest-first, and a list can turn them off with a
`comments_disabled` flag — the one gate, deliberately not the ownership gate,
because anyone may comment on anyone's task. The open question was
whether writing a comment justified breaking the MCP tool ceiling; it did, and
reading them cost nothing because they ride along in `show_task`. The TUI shows
a `💬` in the same fixed trailing icon column as the notes glyph, and renders
each comment as its own card in the Details panel with a compose control on its
own key, distinct from the panel's save.

**UI improvements** — *shipped, all six commits.* gruvbox-dark is the
fresh-install default (a saved theme still wins); the Lists panel opens itself
at 120 columns or wider, and thereafter `L` is authoritative for the session so
a resize never reverses a user's toggle; the first frame renders before the
database is read, animating until the first refresh lands; the notes glyph
moved into a fixed-width trailing column so status labels start at the same
column on every row; the task tree scrolls by keeping the selection visible,
built on a single line plan shared by layout and rendering rather than a
viewport wrapped around an opaque string; and the active list's name sits bold
and right-aligned on the Tasks title line.

**Scratch-list cleanup** and **working-loop instructions** are tracked in the
MCP track above.

### UI (post-alpha)

The Tasks surface was redesigned in three overlapping passes; later passes
supersede earlier ones where they conflict, and `docs/DESIGN.md` §§6 and 12 are
authoritative over all of them.

**The UX redesign** — *shipped (phases A–C).* The interaction-and-density
pass, not another theme port: Lists rows became Stack Stitcher-style cards,
task rows got denser with a `▌` bar and status colors from the theme, a rule
separates the Pending and Complete sections, and navigation follows visual
order instead of store order — which had been a real bug, since the two differ
once a task completes. It also fixed create placement when a completed task is
selected. Its standing product constraints: inline create stays (no
bottom-docked add field), `tab`/`shift+tab` switch panels (except while the
inline create input is live — focus is locked to the text input then), and
`[`/`]` — not tab — change level while creating.

**Task-row redesign and inline creation** — *shipped.* The cutover from a
bottom-pinned add input to inline creation in the tree, which also moved the
startup focus onto the tree and deleted the `addinput` component entirely. Its
column layout and drop order (progress sheds before status, both whole) are
still current; its "the input row *is* the empty state" and
"esc on empty input never leaves" decisions are not.

**Task-row cards and status labels** — *shipped, one decision reverted.* Full-width
content-height row cards, the `▌` bar colored by status (accent when selected),
and the status label in colored caps at the line's end. Its empty-list decision
did ship and was then taken back out: `esc` used to close the input and replace
it with a "No tasks yet, press n" card, which gave one condition two
appearances and only explained how to add a task after the user dismissed the
thing that would have. `esc` now *parks* the input instead — it blurs, the row
stays, and the guidance sits beside it.

**Details as a side panel** — *shipped.* Replaced the details modal with a
framed Details surface, mutually exclusive with Lists so the body is never
three panels: `enter` opens it, `esc` closes it clean or prompts when dirty,
`ctrl+s` saves and closes. The exclusivity, and the rule that focus is computed
rather than listed in a static slice, were the decisions that made it
implementable — the previous draft contradicted itself on both. Details has
since been re-rendered as a centered modal over a scrimmed page, which changes
where it is drawn, not the state machine underneath.

**Title editing in the Details panel** — *mostly landed.* The title is a
single-line `textinput` in the Details panel, tab cycles through it, `ctrl+s`
saves it via the existing `RenameTask`, and dirty detection covers it — which
closes the "no way to edit a task from the TUI" bug. What did not land is the
dedicated `e` shortcut that would open Details with the title already focused.

**Default theme and theme persistence** — *shipped.* `DefaultTheme` moved
from gruvbox-dark to crush-ember (the app's own warm amber palette;
gruvbox-dark stays selectable). It also completed the persistence round
trip: the picker's Enter always wrote `theme:` to config.yaml, but the
boot path never read it back, so a chosen theme died with the process.
The TUI path now applies the saved theme before the first frame, with an
unknown name falling back to the default.

**Foreground tiers and input sealing** — *shipped.* The bug class that
`crush-day` exposed: text rendered with no foreground SGR inherits the
terminal's default color, which is light on nearly every terminal — pending
task titles vanished white-on-white, and every unsealed Bubbles input leaked
the same way. The fix is stated as an invariant in `docs/DESIGN.md` §12
(every glyph draws from an `appstyles.Active` tier; Bubbles inputs are
sealed every render via `chrome.SealInput` / `chrome.SealListFilter`), with
`appstyles.HasDefaultForeground` as the mechanical assertion, applied to
full frames under both the light and the default theme. It pairs with the
background-sealing rule: a sealed background does not save a glyph that has
no color of its own.

### Sister / context

**The Stack Stitcher chrome alignment** — *historical.* The chrome alignment with
this project's sister TUI: the two-surface titled-panel layout, the shared
frame with its `Padding(1, 2)`, the bar-column active-row indicator. Worth
keeping for one lesson it records: the panel padding had been silently reverted
to `Padding(0)` during an unrelated rework, contradicting a design contract
that never changed, and it took a regenerated screenshot to notice — the tests
asserted line indices that the regression happened to satisfy. Its "add input"
sections are superseded by inline creation.
