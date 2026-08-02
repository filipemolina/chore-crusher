# Design

The guiding decisions of Chore Completer, written down so a contributor —
human or agent — has a north star instead of a one-line feature description to
extrapolate from. Where a rule below looks oddly specific, it is specific on
purpose: it was written to close off a plausible wrong implementation, not to
describe the obvious one.

This document is this project's counterpart to
[stack-stitcher's `docs/DESIGN.md`](https://github.com/filipemolina/stack-stitcher/blob/main/docs/DESIGN.md).
Read that file too, once, before writing code here — not because the two apps
share a domain (they don't), but because they share an architecture, and its
reasoning is written there in more depth than is repeated here. This file
states the rule; that one, in several places, shows the failure mode that
made the rule necessary.

## 1. What this app is, and isn't

Chore Completer is a to-do list manager with two front ends over one store: a
terminal UI for a human, and a CLI for scripts and coding agents. Neither is
secondary. The TUI does not shell out to the CLI, and the CLI is not a
read-only reporting layer bolted onto a TUI-owned database — both talk to the
same `store` package (§8), and a write from either is visible to the other
within one poll tick (§7).

It is **not**:

- A sync client. No CalDAV, no Todoist, no Nextcloud Tasks. One local SQLite
  file is the entire backend. If sync matters later, it is a sync of that
  file (or an export from it), not a second source of truth — see
  `docs/ROADMAP.md`'s post-alpha list for where that idea is parked and why.
- A project-management tool. There is no assignee, no due date, no priority
  field, no Gantt view in the plan below. Every one of those is a reasonable
  future addition and none is in scope now — see §10 on why the schema is
  shaped to add them without a migration disaster, without them being added
  today.
- A Taskwarrior replacement. Taskwarrior's dependency graph and reporting DSL
  solve a different, harder problem for a different, more demanding user.
  This app's tree is a strict parent/child hierarchy, not an arbitrary DAG —
  see §3.

## 2. Data model

Two entities, `List` and `Task`. A `Task` belongs to exactly one `List` and
has at most one parent `Task`.

```
List
  id            text primary key   -- ULID, generated at insert
  name          text not null
  created_at    integer not null   -- unix seconds
  position      integer not null   -- manual ordering among lists

Task
  id             text primary key   -- ULID
  list_id        text not null references List(id)
  parent_id      text references Task(id)   -- null = root-level task
  title          text not null
  notes          text not null default ''
  status         text not null   -- 'pending' | 'in_progress' | 'complete'
  progress_kind  text not null default 'none'
                 -- 'none' | 'simple' | 'subtasks' | 'percentage'
  progress_pct   integer          -- 0-100, set only when progress_kind='percentage'
  position       integer not null   -- manual ordering among siblings
  created_at     integer not null
  updated_at     integer not null
  completed_at   integer          -- null unless status='complete'
```

Why ULIDs and not autoincrement integers: task and list ids are handed to the
CLI as arguments (`complete complete <task-id>`) and printed by `add`. A ULID
is a stable, copy-pasteable, sortable-by-creation-time string that never
collides across a `list add` and a concurrent `task add` from two processes —
an autoincrement id needs the database to hand it out, which is fine, but a
ULID lets `store.NewTaskID()` be generated before the transaction opens,
which matters for §7's transaction-shape rule. Ids are **not** meant to be
typed from memory; the CLI accepts an unambiguous *prefix* of an id
(§9, `resolveID`) so a human or an agent copying an 8-character prefix from
`complete tasks` output doesn't have to paste the full 26 characters.

Nesting depth is **not** capped in the schema. `parent_id` is self-referential
and a task can have a task at any depth as its parent. What *is* constrained
is how deep a single **add** operation can reach relative to the currently
selected task (§4) — that is a UI/workflow rule, not a data-model limit nudge
your way to depth 4 across several add operations, and the schema does not
notice or care.

## 3. The status and progress state machine

This is the section most likely to be implemented from intuition rather than
from what's written here. Intuition gets some of it right and a few of the
edges wrong, so read the whole thing before writing `store` code that touches
`status` or `progress_kind`.

**States.** A task's `status` is one of `pending`, `in_progress`, `complete`.
`progress_kind` only has meaning while `status = in_progress`; it is `none`
for `pending` and `complete` tasks (§3 keeps this an invariant the store
enforces, not a convention callers remember).

**The three flavors of `in_progress`:**

- `simple` — no number. The task is being worked on; that's the whole claim.
- `percentage` — a user- or agent-set integer 0–100 (`progress_pct`). An
  honest estimate, not a fact the store can verify.
- `subtasks` — `progress_pct` is **not stored**; it is computed on read as
  `round(100 * complete_children / total_children)` over the task's *direct*
  children only (not the whole subtree — a child's own `subtasks`-derived
  percentage already folds its grandchildren in, so re-descending would
  double-count depth). This is a fact the store can verify, which is why it
  behaves differently from `percentage` below.

**Switching a task to `subtasks` mode with zero children is not an error.**
It's a legitimate state: someone is about to add subtasks and set the mode
first. With no children to derive from, the task **displays and behaves as
`simple`** — no percentage shown, no auto-completion — until it has at least
one child. The stored `progress_kind` stays `subtasks` regardless; only the
*display and completion behavior* falls back. The moment a first child
exists, derivation resumes. Do not special-case "zero children" by silently
rewriting `progress_kind` to `simple` — that would lose the user's stated
intent the next time they check the details screen.

**Auto-completion is asymmetric between the two derived-vs-declared kinds,
and this is deliberate:**

- `subtasks` reaching 100% (every direct child `complete`) **promotes the
  parent to `complete` automatically.** This is a verified fact — if every
  child is done, the parent claiming otherwise would be a lie the store can
  see through — so the store does not wait for a human or a script to say so.
  This check re-runs on every child completion and must walk upward: completing
  a leaf can complete its parent, which can complete *its* parent, and so on.
  Implement this as a single `recomputeAncestors(taskID)` walk after any
  status write, not as a special case bolted onto `Complete()` alone —
  `Reopen()` and `SetProgress()` can also change whether a parent's derived
  condition holds.
- `percentage` reaching 100 **does not** auto-complete. It's a claim, not a
  verified fact, and the store has no way to distinguish "I meant it" from
  "I typed 100 out of habit." Completing is a separate, explicit action
  (`space` in the TUI, `complete complete <id>` on the CLI) even at 100%.
  If this surprises a future contributor enough to want to change it, that's
  a product decision to raise, not a bug to silently fix — it was chosen
  specifically to keep the one auto-promotion path (verified subtask
  completion) the only one, rather than accumulating several slightly
  different auto-complete triggers that a reader has to hold in their head.

**Completing cascades down; reopening does not.** Marking a task `complete`
(`space`, or `complete complete <id>`) sets every descendant, at every depth,
to `complete` too — a `complete` task with a `pending` grandchild is a state
this app does not allow to exist, because the two-list split (§6) would then
have to explain why a "done" tree still has visibly undone rows in it.
Reopening a task (`complete reopen <id>`, or `space` again on an already
complete task) does **not** cascade to children — it returns *only that task*
to `pending`. This is intentionally lossy: the task's prior `progress_kind`
and `progress_pct` are not restored, because tracking "what it was before
completion, one level deep" is a second piece of state for a rare path
(un-completing) that would otherwise need its own tests and its own edge
cases (what if it was `subtasks`-derived and a child changed while it sat
complete?). If this bites someone in practice, revisit it — but start from
`pending`, not from resurrected history.

**The store owns every transition.** None of the above should be duplicated
in both `store` and `cli` (or `store` and `components`). `store.Complete`,
`store.Reopen`, `store.SetProgress` are the only three functions that write
`status`/`progress_kind`/`progress_pct`, and every caller — CLI subcommand or
TUI keypress handler — goes through them. This mirrors stack-stitcher's rule
that the compose file has exactly one write path per kind of change; here the
equivalent invariant is enforced by Go visibility (these three are the only
exported mutators) rather than by file-write discipline, since there's a
database instead of a document to protect.

## 4. Adding a task: the level rules

The bottom-of-panel input adds a task relative to whatever is selected in the
tree. Call the selected task's depth `L` (root-level tasks are `L = 0`).
Before the input is submitted, `tab` and `shift+tab` change **where** the new
task lands, and the input's leading glyph and indentation reflect the current
choice:

| Keystrokes so far | New task's parent | New task's depth | Glyph |
| --- | --- | --- | --- |
| none (default) | selected task's parent (i.e., a sibling of the selection) | `L` | `-` |
| one `tab` | the selected task itself | `L + 1` | `+` |
| one `shift+tab` (only if `L > 0`) | the selected task's parent's parent | `L - 1` | `^` |

Further presses in the same direction **do not go further** — the range is
clamped to exactly one level either side of the selection, always. This is
the literal rule ("one level deeper, one level above") and it is a clamp, not
a wrap: pressing `tab` three times behaves the same as pressing it once.
`shift+tab` is a no-op (not an error, not a beep) when `L = 0`, because there
is no level above root.

Where the new task is **inserted** among its new siblings: immediately after
the reference task that determined its parent (the selection itself for the
default and `tab` cases; the selection's parent for the `shift+tab` case).
This keeps rapid entry predictable — hitting `enter` repeatedly at the
default level appends a flat run of siblings in the order typed, which is the
behavior anyone coming from Workflowy or a plain bullet list already expects.

**State resets, every time.** After a successful add, selection moves to the
newly created task, and the tab/shift-tab state resets to default (`-`,
sibling-of-whatever-is-now-selected) — it does not stay at `+` just because
the last add was a child. Selecting a *different* task (arrow keys, `j`/`k`,
mouse if that ever lands) while the input has unsent text also resets the
level to default for the newly selected task; the level indicator describes
a relationship to the *current* selection, and an old relationship to a task
that's no longer selected would be meaningless. `esc` while the input has
text clears the text and resets the level, and does nothing else — the input
itself never hides, unlike a modal, so `esc` here has exactly one job.

## 5. Navigation and focus

Three focusable zones, not two like stack-stitcher, because this app has a
sidebar that can be entirely absent from the cycle:

- **Lists panel** — present in the cycle only while visible (`L` toggles
  visibility; see §6). Hidden by default on every launch — this app's whole
  premise is "spend your time in one list," and a sidebar that's there by
  default fights that.
- **Task tree** — the main panel's Pending/Complete sections, one flat
  keyboard-navigable cursor across both (see §6 for why the split is visual
  section headers, not two independently-focusable lists).
- **Add input** — fixed to the bottom of the main panel, always visible,
  always reachable, never a modal.

`tab`/`shift+tab` cycle **only through the zones currently visible** — the
lists panel is skipped entirely from the cycle while hidden, the same way
stack-stitcher's nav bar is permanently absent from its own cycle
(`constants.FocusableComponents`) rather than being a focusable-but-inert
stop. Do not implement "hidden but still tabbable to an invisible panel";
that produces a focus ring with a silent dead stop in it.

**Vim and arrow bindings both work, always, on the task tree:** `↑`/`k` up,
`↓`/`j` down (moving the cursor across every *visible* row — a collapsed
node's children are not visible rows and are skipped), `←`/`h` collapses the
selected node if it has children and is expanded, else moves selection to its
parent; `→`/`l` expands the selected node if it has children and is
collapsed, else moves selection to its first child. This is the same
convention as `nnn`, `lf`, and most terminal file managers with a tree pane —
picked because it is already muscle memory for the audience, not invented
for this app.

**`space` toggles complete/pending** on the selected task, from wherever the
tree has focus — it does not open anything and does not move the cursor.
**`enter`** on a selected tree row opens the details screen — so it can't
also mean "toggle complete"; the two are deliberately different keys because
"open a thing" and "flip a checkbox" are different enough actions that
collapsing them into one key is what makes an app feel like a demo rather
than a tool. Note the asymmetry with stack-stitcher, which binds `Select` to
*both* `space` and `enter` — that works there because both mean "start"; here
they must mean two different things, so they are two different bindings from
the start rather than one alias split apart later.

Inside the details screen (phase 7): **`ctrl+s`** saves notes and progress
changes; **`tab`** cycles between the notes editor and the progress selector;
**`←`/`→`** (or `h`/`l`) cycle through the three progress modes
(`simple`/`subtasks`/`percentage`); `esc` closes with a discard-changes prompt
if anything is unsaved.

**`/?`** enters the task tree's local fuzzy filter (phase 8): a live input
narrows the current list's rows in place to each match plus its ancestor
chain, so a matched leaf never loses its parent rows. `enter` applies the
query and leaves the filtered view active; `esc` clears it.

**`F`** opens the cross-list search picker (phase 8): a text input searches
every list live, ranking title matches before notes-only hits, and showing
each result as `<list> › <task>`. `enter` on a result jumps to that task —
switching the active list when the match lives elsewhere — and `esc` cancels.

**Task renaming** in the TUI is not implemented yet — the details screen shows
the title read-only. A rename gesture (if added to the TUI before phase 9) should
be recorded here in §5 alongside the other task-tree keybindings.

`esc` follows the same "ladder of claims" stack-stitcher documents: a modal
(details screen, theme picker, confirm) closes itself first; the add input
with text in it claims `esc` next (§4); an applied filter (§8) claims it
after that; what's left is "back to the task tree from wherever else," same
shape as stack-stitcher's "back to the list from the details panel." Keep
this ladder in one function, tested against every claim in order, the way
stack-stitcher's `AppModel.escKept`/`keyboardOwned` pair is — a keystroke
handled by checking claims in the wrong order silently breaks whichever claim
got skipped.

## 6. The main panel: Pending and Complete

The right panel is two sections, headed `Pending` and `Complete`, not two
independently scrollable/focusable lists. One cursor moves through both,
top to bottom, `Pending` first.

**Which section a task-tree renders under is decided by the *root* task's own
`status` alone**, not by scanning descendants: a root-level task with
`status != complete` renders (with its whole visible subtree) under
`Pending`; a root-level task with `status = complete` renders under
`Complete`. Because completing cascades to every descendant (§3), a tree
under `Complete` is, by invariant, 100% complete rows all the way down — the
section header is a true claim, not an approximation. A tree under `Pending`
can and will contain a mix: a `pending` parent can have `complete` children
sitting inline (checked, perhaps struck through) underneath it, still nested
in place. **Do not move a completed subtask out to the `Complete` section
while its parent is still pending** — that would separate a task from the
tree it belongs to for a reader trying to see the shape of remaining work,
which defeats the reason the tree exists at all.

A list with no tasks yet shows an empty state in `Pending` (something like
"nothing here — type below to add one") and omits the `Complete` header
entirely rather than showing it empty; an empty `Complete` section with
nothing under it is noise no reader needs.

## 7. Live refresh: how the TUI sees the CLI's writes

There is no daemon, no socket, no file watcher — the same abstinence
stack-stitcher's design document argues for network services applies here to
IPC. The TUI polls.

`tea.Tick` fires every `poll_interval_ms` (config default: **1000**, a tenth
of stack-stitcher's 5000 because a local SQLite read costs microseconds where
a `docker compose ps` shells out; there is no reason to make a human wait five
seconds to see their own agent's last completion) and dispatches a
`cmds.PollMsg`. `AppModel` responds by re-running exactly two queries — list
summaries (for the lists panel and its counts) and the active list's task
tree — and diffs the result against what's currently rendered. On no change,
nothing re-renders; Bubble Tea already no-ops a `View()` call that produces
identical output to the terminal driver, but the diff also matters for one
thing that isn't free: **cursor position**. A poll that finds the previously
selected task still present keeps the cursor on it (matched by id, not by
row index — a CLI insert or delete during the interval can move every row
index without moving the task the user was looking at). A poll that finds
the selected task gone (deleted from elsewhere) moves the cursor to the
nearest surviving row, the same "what do you do when the ground moves"
question stack-stitcher answers by re-selecting by name after a config
reload.

**The TUI never holds a write transaction, and no poll tick is allowed to
block on one.** It opens a read connection at startup and keeps it; every
poll is a `SELECT`, full stop. All writes — from the TUI's own keypress
handlers as much as from an external CLI invocation — go through the same
`store` functions the CLI uses (§8), each wrapping one short transaction that
opens, writes, commits, and returns, so a rapid-fire agent loop calling
`complete complete` in a shell `for` loop is never waiting behind the TUI, and
the TUI is never waiting behind it either. SQLite's WAL mode (§8) is what
makes concurrent readers and a writer not block each other; do not disable it.

## 8. Storage and concurrency

**modernc.org/sqlite** — pure Go, no CGO. This preserves stack-stitcher's
build story unchanged: `CGO_ENABLED=0`, cross-compiled linux/darwin ×
amd64/arm64 by the same GoReleaser shape (`docs/plans/phase-0-scaffolding.md`
carries the exact config forward). A CGO-based SQLite driver would be the
more common choice by download count, but it would make this the one thing
in the whole toolchain that needs a C compiler to cross-compile, for no
capability this app uses that the pure-Go driver lacks.

**One file:** `$XDG_DATA_HOME/complete/complete.db` (falling back to
`~/.local/share/complete/complete.db`), opened in WAL journal mode. WAL is
what lets the TUI's long-lived read connection and a CLI process's short
write transaction coexist without either blocking the other — the default
rollback-journal mode takes an exclusive lock for the duration of a write,
which would stall the TUI's next poll for the length of the write, small as
that is. This is a one-line `PRAGMA journal_mode=WAL` at connection open, not
a design a contributor needs to build — but it is a design a contributor
could accidentally undo by opening the connection differently elsewhere, so:
**there is exactly one function that opens the database, `store.Open`,** and
every caller — `main.go`'s TUI path and every CLI subcommand — calls it. Do
not open a second `sql.DB` anywhere; a second connection that forgets the WAL
pragma is a subtle, load-bearing regression, not a stylistic one.

**Migrations** are numbered `.sql` files embedded via `embed.FS`
(`store/migrations/0001_init.sql`, `0002_*.sql`, …), applied in order inside
`store.Open`, tracked by a `schema_migrations(version integer primary key)`
table. Every invocation of the binary — TUI or any CLI subcommand — runs this
before touching data, idempotently (a migration that's already applied is a
no-op, not an error). This is the same "one resolution, passed down" instinct
stack-stitcher applies to compose-file discovery: one function decides the
schema is current, called from one place, rather than each caller assuming
someone else already did it.

**Config** (`~/.config/complete/config.yaml`, or `$XDG_CONFIG_HOME`) holds
exactly two fields at launch, in the same struct-designed-to-grow shape as
stack-stitcher's `config.Config`:

```yaml
theme: complete-dark
poll_interval_ms: 1000
```

Both optional; a missing file or a missing field falls back to the compiled
default, and a malformed file is reported rather than silently ignored for
everything *except* first-run (mirrors stack-stitcher's `LoadConfig`
contract exactly — see that package's doc comment for the reasoning already
written up once).

## 9. The CLI contract

Built on [Cobra](https://github.com/spf13/cobra). Every subcommand that reads
data accepts `--json`; every subcommand, reading or writing, reports failure
the same way regardless of `--json`. This uniformity is the whole point of
having a contract document at all — an agent that has read one subcommand's
`--help` should be able to predict the shape of every other one's output and
errors without reading the rest.

**Output shape, human mode (default):** a write command that succeeds prints
nothing but the one piece of information a script might want to capture
(`complete lists add` prints the new list's id and nothing else; `complete
add` prints the new task's id and nothing else). A read command prints a
formatted table or tree to stdout. Any failure prints one line to stderr,
prefixed `complete: `, and the process exits non-zero.

**Output shape, `--json` mode:** stdout is **always exactly one JSON value**,
whether the command succeeded or failed — `{"error": "list not found:
01ARZ…"}` on failure, the command's normal payload on success. Nothing else
is written to stdout in this mode; diagnostic/progress text, if any, goes to
stderr instead. This means a caller that passes `--json` never has to check
two streams to know what happened — parse stdout, then check the exit code to
know whether what you parsed was the payload or the error. Do not print
human-readable text to stdout *alongside* the JSON "for convenience"; that
breaks every caller's parser the first time it happens.

**Exit codes:** `0` success. `1` the operation failed for a domain reason
(id not found, invalid state transition, validation failure). `2` a usage
error — Cobra's own default for a malformed flag or missing argument, left
as-is rather than remapped, since that's the exit code every other Cobra-based
tool already uses for the same thing.

**ID prefixes.** Every `<list-id>`/`<task-id>` argument accepts an
unambiguous prefix of the full ULID, resolved by `store.ResolveID` against
the relevant table. An ambiguous prefix (matches more than one row) is a
domain error (exit `1`), not a silent pick of the first match — silently
guessing which task an agent meant is exactly the kind of behavior this
project exists to not have.

**Destructive commands need `--force`.** `complete lists rm` and `complete rm`
(task) refuse to run without `--force`. The TUI's equivalent actions go
through a confirm modal (the same pattern as stack-stitcher's
`ConfirmModal`); the CLI has no modal to route through and no human to ask,
so the flag *is* the confirmation. This is the one place the CLI is
deliberately less convenient than the TUI, on purpose: an agent's typo in a
task id should not have the blast radius of an unrecoverable delete with no
prompt at all.

**Full subcommand list**, grouped by the thing they act on. `<id>` accepts a
prefix (see above) throughout.

```
complete                                          launch the TUI
complete lists                                    list all lists
complete lists add <name>                         create a list; prints its id
complete lists rename <list-id> <name>            rename a list
complete lists rm <list-id> --force               delete a list and its tasks

complete tasks <list-id> [--status pending|in_progress|complete|all] [--flat]
                                                   list tasks (tree by default)
complete add <list-id> <title> [--parent <task-id>] [--notes <text>]
                                                   add a task; prints its id
complete show <task-id>                           title, notes, status, progress, children
complete rename <task-id> <title>                 rename a task
complete notes <task-id> <text>                   replace a task's notes (whole text, not append)
complete complete <task-id>                       mark complete (cascades to descendants)
complete reopen <task-id>                         mark pending (does not cascade)
complete toggle <task-id>                         complete <-> reopen, whichever applies
complete progress <task-id> --mode simple
complete progress <task-id> --mode percentage --percent <0-100>
complete progress <task-id> --mode subtasks
complete rm <task-id> --force                     delete a task and its descendants
complete search <query> [--list <list-id>]        fuzzy search across titles (+ notes)

complete --version
```

**Output shapes, pinned.** The subcommand list above fixes *which* commands
and flags exist; this fixes *what each prints*. The shapes below were
settled in phase 2 (docs/plans/phase-2-cli.md) and are part of the contract,
so an agent that has read one command's `--help` predicts the shape of the
rest; the tests in `src/cli` (lists_test.go, tasks_test.go, search_test.go)
pin them. Two contested calls, and the alternatives rejected: **`tasks` and
`show` JSON rows are a flat preorder array with `depth` and `parent_id`, not
a nested tree** — a caller walks one shape whether or not it passed `--flat`,
and reassembles the tree from the two fields; a nested tree would mean two
shapes to walk and a `--flat` mode that diverges from the default. **Writes
that have no id of their own print `{"ok": true}` in JSON mode, not
nothing** — §9 requires exactly one JSON value on stdout even when there is
nothing interesting to say, so the smallest change (print nothing) breaks
the one-value rule.

- **Writes, human mode:** `lists add` and `add` print only the new id; the
  remaining writes print nothing on success. **Writes, `--json` mode:** the
  two add commands print `{"id": "…"}`; the remaining writes print
  `{"ok": true}`.
- **`lists`, human mode:** a `tabwriter` table with the header
  `ID NAME PENDING COMPLETE`, one row per list; an empty result prints
  nothing. **`lists`, JSON:** `[{"id", "name", "pending", "complete",
  "created_at"}]`.
- **`tasks`, human mode (tree, the default):** the §6 sections, headers
  `Pending (N)` / `Complete (N)` where N is the section's row count — a
  section header appears only when the section has rows, and a list with no
  tasks prints nothing at all. Rows follow §12's fixed layout (indent,
  expand glyph, checkbox, title, and the trailing ` (NN%)` progress suffix
  whenever `store.DerivedProgress` reports a percentage). `--flat` prints
  `<id>\t<status>\t<title>` per line instead — the greppable view, no
  headers. **`tasks`, JSON:** the same preorder array in both modes —
  `[{"id", "parent_id", "title", "status", "progress", "depth"}]` —
  `--flat` changes only the human rendering. Depth starts at 0 for a root;
  `parent_id` + `depth` let a caller reassemble the tree.
- **`show`, human mode:** labeled lines (`Title:`, `ID:`, `List:`, `Status:`,
  `Progress:`, `Notes:` with each line indented two spaces, then
  `Children (N):` and the §12 tree when there are any). The `Progress:` line
  spells out a subtasks task with no children as `subtasks (simple)` rather
  than a misleading `(0%)` (§3). **`show`, JSON:** the task's fields
  (`id`, `list_id`, `title`, `notes`, `status`, `created_at`, `updated_at`,
  `completed_at` as unix seconds), its `progress`, and `children` as the
  same row array `tasks` emits, depth relative to the shown task.
- **`progress` JSON:** `{"kind", "percent", "display_as_simple"}`.
  `kind` is the stored `progress_kind`; `percent` is the displayed value —
  the stored percent for `percentage`, the derived ratio for `subtasks` —
  and is `null` whenever the kind has nothing to display; `display_as_simple`
  reports the §3 zero-children subtasks fallback.
- **`search`, human mode:** a `tabwriter` table with the header
  `ID LIST TITLE`. **Ranking:** `store.SearchTasks` returns LIKE candidates;
  title matches are ranked by `sahilm/fuzzy` score, then candidates that
  matched only on notes follow in store order — a notes hit is a real hit,
  just weaker than a title one. **`search`, JSON:**
  `[{"id", "list_id", "list_name", "title", "status", "progress"}]`.
- **Empty results, human mode:** a read command whose result is empty prints
  nothing (exit 0); JSON mode prints `[]`. A caller that needs to
distinguish "no data" from "failed" reads the exit code, never the bytes.
- **Human output is plain text — no ANSI escapes** — so a script can capture
  any read command's stdout without stripping styling.

`complete mv <task-id> --parent <task-id-or-empty>` (re-parent a task,
without the ±1-level restriction §4 puts on the TUI's *add* flow — a CLI
re-parent is a deliberate restructure, not the inline-add gesture that rule
exists to keep predictable) is useful but not required for a first alpha; it
is scheduled as phase 9, not blocking anything before it. See
`docs/ROADMAP.md`.

## 10. Package layout

Mirrors stack-stitcher's split between the Bubble Tea half and the
non-Bubble-Tea half, with one addition (`cli`) for the second front end:

```
main.go              # cobra root: no subcommand -> launch TUI; else dispatch
src/
├── model/           # AppModel: Init/Update/View, the top-level Bubble Tea model
├── components/      # one package per leaf model (tasktree, listspanel, addinput,
│                     # detailsmodal, themepickermodal, searchmodal, listnamemodal,
│                     # confirmmodal, helpoverlay, keybindingbar)
│   └── chrome/       # shared rendering: PanelFrame, tree-row rendering, the
│                     # progress pill, KeyHints, Spinner — ported from stack-stitcher
│                     # where the helper is domain-agnostic, written fresh where it isn't
├── cmds/            # message types and the tea.Cmds that produce them
├── apptypes/        # List, Task, Status, ProgressKind — the shapes components pass around
├── keys/            # the one keymap package; see stack-stitcher's for why there's exactly one
├── store/           # SQLite schema, migrations, and every read/write function —
│                     # the only package that imports database/sql
├── cli/             # one file per subcommand group; each is a thin adapter from
│                     # cobra flags to a store call and a --json-aware printer
├── appstyles/       # Theme type + the 14-theme registry, ported from stack-stitcher
├── config/          # ~/.config/complete/config.yaml
└── constants/       # layout widths, focusable-zone ids, branding
```

**`src/store` is the only package that imports `database/sql` or
`modernc.org/sqlite`.** Both `src/model` (the TUI) and `src/cli` (the CLI)
depend on `store` and nothing deeper; neither ever builds a SQL string.
**`src/cli` never imports `src/model`, and `src/model` never imports
`src/cli`** — they are siblings over the same `store`, not layered on each
other, which is the structural expression of "neither front end is
secondary" from §1. `main.go` is the one file that imports both, to decide
which to run.

## 11. Theming

Ported from stack-stitcher's `src/appstyles` near verbatim — the `Theme`
struct, `newTheme`'s tier-derivation math, the `InkOn` contrast helper, and
the picker's live-preview-on-cursor-move mechanic. Read
[stack-stitcher's `Theme.go`](https://github.com/filipemolina/stack-stitcher/blob/main/src/appstyles/Theme.go)
and copy its structure; do not redesign the derivation math, it's already
been tuned across 14 imported palettes (see stack-stitcher's `docs/DESIGN.md`
§"Color lives on a Theme" for the reasoning behind `Lighten`/`Darken` and why
`Modal` needs to clear `BackgroundElevated` by a minimum margin).

**What changes:** the four status-color fields are domain colors, so they're
renamed to match this app's domain instead of Docker's:

| stack-stitcher | Chore Completer | Same hex per theme |
| --- | --- | --- |
| `StatusRunning` | `StatusComplete` | yes |
| `StatusStopped` | `StatusPending` | yes |
| `StatusStarting` | `StatusInProgress` | yes |
| `StatusError` | `StatusOverdue` *(reserved; unused until a due-date feature exists — see `docs/ROADMAP.md`)* | yes |

Every hex value in the 14-theme registry (`stitcher-dark`, `stitcher-ember`,
`stitcher-slate`, `stitcher-day`, plus the ten imported community palettes)
carries over unchanged — same accent, same text/panel/modal bases, same
status colors under their new field names. This is deliberate: a person who
runs both apps should see the same "Tokyo Night" render the same way in
either, because it's the same theme, not a reinterpretation of one.
`DefaultTheme` becomes `"complete-dark"` (the renamed `stitcher-dark`); adjust
every `Name` string and registry key from `stitcher-*` to `complete-*`
accordingly, since the name is user-visible in the theme picker.

## 12. Visual coherence: the UI contract

This section exists because "pick something reasonable and be consistent"
is not a instruction that survives being executed by several different,
disconnected contributors across nine phases. Every visual detail below is
**decided, not suggested** — a fixed number, a fixed glyph, a fixed rule —
specifically so phase 4's task tree and phase 6's lists panel, built weeks
apart with no memory of each other, render as one app rather than two
apps sharing a color scheme. Where stack-stitcher already solved the same
problem, the answer is ported outright, including its exact numbers, for the
same "these are sister apps" reason §11 gives for porting hex values
unchanged.

**If a UI element needs a visual detail this section doesn't specify — a
glyph, a spacing value, a color-tier choice — add it here, in this section,
in the same commit that introduces the element.** Do not decide it locally
inside a component and move on; a decision made only in one component's code
is a decision the next component's author cannot find.

**→ For the hardened, actionable verification checklist,** read
[`docs/UI_INSTRUCTIONS.md`](UI_INSTRUCTIONS.md). Before marking a component
complete, run the verification script: `scripts/verify-ui-component.sh <component-path>`.

### Background tiers, and sealing them

Ported unchanged from stack-stitcher's `docs/DESIGN.md` §"Background tiers,
and sealing them" — read that section once for the full reasoning (the SGR-
reset mechanics that make an unsealed tier bleed the terminal's own
background through). The tiers, mapped to this app's own surfaces:

| Tier | Field | Where, in this app |
| --- | --- | --- |
| 1 | terminal default | outside the app — never drawn on |
| 2 | `BackgroundContent` | the outermost frame, if one exists (gutter between the lists panel and the main panel) |
| 3 | `BackgroundPanel` | the lists panel and the main panel, unfocused |
| 4 | `BackgroundElevated` | whichever zone (§5) currently has focus |
| — | `ModalBg` | every modal (theme picker, confirm, list-name, details screen if built as a modal — §Phase 7) **and the row the cursor sits on in the task tree** — an active row is its own register, not a tint of the panel it's in, the same reasoning stack-stitcher applies to an active list row |
| — | `BackgroundRecessed` | empty-state cards (§Empty states, below) — equal to `PanelBg`, the un-raised base |

**Every tier must be sealed.** Anything that draws text — a tree row, the
add input, a list row, a modal's body — needs an explicit background, and
`lipgloss.JoinVertical`/`JoinHorizontal` pad shorter siblings with bare,
unstyled spaces that must themselves carry a background or the terminal's
own color shows through. Seal innermost first: a tree row seals itself,
then the panel it sits in seals what's left, then (if a tier-2 frame exists)
the outermost pass seals last. Port stack-stitcher's
`appstyles.HasBackgroundBleed` assertion and its background test suite
(`docs/plans/phase-3-tui-shell.md` step 9's verification) — this is not
optional polish, it's the mechanical check that catches a missing
`Background()` call before it ships.

### Focus is shown by lifting a tier, never by changing box size or border weight

A zone's box is exactly the same width and height whether or not it has
focus. What changes is the fill: `BackgroundPanel` (tier 3) unfocused,
`BackgroundElevated` (tier 4) focused. **Do not indicate focus with a
heavier border, a different border color, or a resized box** — any of those
shifts the layout of everything around it by at least one cell, which is
exactly the kind of thing that looks fine in isolation and wrong the moment
two zones are on screen having made different choices. One function,
`chrome.PanelBg(isFocused bool) color.Color`, is the only place this
decision is made; every zone calls it rather than branching on focus itself.

### One shared frame: `chrome.PanelFrame`

All three focusable zones (§5) — the lists panel, the task tree, the add
input — render inside the same frame helper, with the same padding:
**1 row vertical, 2 columns horizontal** (`lipgloss.NewStyle().Padding(1,
2)`), the exact padding stack-stitcher's own `PanelFrame`/`ListWrapperStyle`
use. No component sets its own padding value. This is what keeps a title's
left edge, a checkbox's left edge, and the add input's left edge all landing
on the same column when the zones stack vertically in the main panel — three
components independently choosing "close enough" padding is how that
alignment quietly breaks.

### Truncation: one function, built in phase 3, used everywhere from the start

**`chrome.Truncate(s string, width int) string`** exists by the end of phase
3 (`docs/plans/phase-3-tui-shell.md`), not phase 9 — every component that
renders user-supplied text (a task title, a list name, a note preview) calls
it from the moment that component exists, so there is never a window where
different components truncate differently because "polish comes later."
Rule: cut to `width - 1` display cells and append a single `…`, never mid-
escape-sequence, using the same `ansi`-aware width measurement
stack-stitcher's `chrome.Truncate` does (a plain byte-slice truncate can
split a multi-byte rune or an ANSI sequence, corrupting the rest of the
line — ported reasoning, see that project's `docs/DESIGN.md` under "Narrow
terminals: shed whole things"). **Never truncate a unit to a fragment** — if
a row genuinely has no room for any of a title, show nothing of it (clip the
row) rather than one or two letters followed by `…`.

What phase 9 still owns: *shedding whole optional elements* (a trailing
percentage, a key-hint) under extreme narrowness, which is a different,
coarser mechanism layered on top of truncation, not a replacement for having
truncation from the start.

### The glyph vocabulary

One table. A component does not invent a symbol not listed here; if a new
one is needed, it's added here first.

| Meaning | Glyph | Notes |
| --- | --- | --- |
| Task: pending | `[ ]` | |
| Task: in progress | `[~]` | Used for all three progress kinds (§3) alike — the trailing percentage (below), not the checkbox, is what distinguishes them. |
| Task: complete | `[x]` | Title renders in `TextMuted`, not `TextPrimary`, once complete — see Typography below. |
| Node has children, expanded | `▾` | One column wide, placed immediately before the checkbox. |
| Node has children, collapsed | `▸` | Same column. |
| Node is a leaf | *(one blank space)* | Occupies the same column so every row's checkbox lands in the same position regardless of whether the row above or below it has an expand glyph. |
| Add-input level: sibling (default) | `-` | §4. |
| Add-input level: child | `+` | §4. |
| Add-input level: parent-of-selection | `^` | §4. |
| Trailing derived/percentage progress | ` (NN%)` | In `TextMuted`, appended directly after the title with one leading space; omitted entirely when `DerivedProgress` reports `displayAsSimple` (§3) — never rendered as `(0%)` in that case. |

**Row layout, left to right, fixed order:** `{2 spaces × depth}{expand-glyph-or-blank}{space}{checkbox}{space}{title}{progress suffix if any}`. Depth-0 example with children, expanded, pending: `▾ [ ] Buy paint for the fence`. A leaf at depth 1, complete: `   [x] Choose color` (two spaces of indent, one blank leaf-column, one separating space — four columns before the checkbox).

Section headers (`Pending`, `Complete` — §6) render as `{bold TextPrimary}
{section name} {dim count in parens}`, e.g. **Pending** `(3)` — the same
"name, then a muted count" shape the lists panel already uses for a list's
own row, so the two surfaces read as one convention rather than two.

### Typography: which text tier, when

Three tiers exist (`TextPrimary`, `TextMuted`, `TextDim`); which one a piece
of text uses is a rule, not a per-component judgment call:

- **`TextPrimary`** — the thing being read: a pending or in-progress task's
  title, a list's name, notes body text, a modal's main content.
- **`TextMuted`** — secondary, still-relevant metadata: a completed task's
  title (this is the one deliberate exception to "titles are always
  primary" — completion is exactly the state where a title becomes
  secondary information), section-header counts, the trailing progress
  percentage, a list's pending/complete counts in the lists panel.
- **`TextDim`** — inert or placeholder text: an empty-state's message, a
  disabled key hint, the add-input's placeholder text before anything is
  typed.

Do not introduce a fourth informal tier (a hand-picked opacity, a literal
gray hex) for "something in between" — if the three don't cover a case,
that's a signal to reconsider the case, not to add a color.

### Empty states: one recessed-card pattern

Every empty state (§6: no tasks in `Pending`; a lists panel with no lists
yet) is the same shape: a box on the `BackgroundRecessed` tier, rimmed with
`BorderCard` (not `BorderDefault` — see §11's inherited reasoning: a border
has to contrast with the surface it wraps, and `BorderDefault` moves *toward*
`BackgroundRecessed` rather than away from it), `Padding(1, 2)` matching
`PanelFrame`, one line of `TextDim` guidance text, left-aligned. Do not
center empty-state text and do not give it its own bespoke padding —
reusing the exact `PanelFrame` numbers is what makes it read as "this panel,
currently empty" rather than a different kind of surface that happens to be
nearby.

### The chrome-package contract

Before a leaf component (`docs/DESIGN.md` §10) is considered done, it
satisfies all of the following. Treat this as a literal checklist, not
prose to have read once:

1. Every color it draws with is read from `appstyles.Active.*` at render
   time — never a cached package-level color, never a literal hex.
2. Its outer box is built with `chrome.PanelFrame` (or, for a modal, the
   equivalent shared modal-frame helper phase 3 establishes) — it does not
   set its own padding, border style, or corner treatment.
3. Any user-supplied text it renders goes through `chrome.Truncate`.
4. It seals its own background tier before returning its content to
   whatever composes it (§Background tiers, above).
5. Any glyph or symbol it needs is one of the ones listed under §The glyph
   vocabulary — or that table was extended, in the same change, to add it.
6. Focus, if it applies to this component, is shown exactly per §Focus is
   shown by lifting a tier — nothing else changes between focused and
   unfocused.

A component that satisfies 1–6 cannot visually drift from the rest of the
app no matter which phase or which contributor built it — that is the
entire purpose of making the checklist mechanical rather than a matter of
taste.

**For a hardened, actionable version of this checklist with verification
steps, bash commands to check each rule, and examples,** see
[`docs/UI_INSTRUCTIONS.md`](UI_INSTRUCTIONS.md). Use
`scripts/verify-ui-component.sh` to check a component against all six rules
before marking it complete.

## 13. Testing

The store package is the one clear advantage this project has over
stack-stitcher's testing story: stack-stitcher's `utils` package shells out to
`docker`, which can't be unit-tested without either a running daemon or a
mock; this project's `store` package talks to a real SQLite file created
fresh in a temp directory per test, so **every** state-machine rule in §3 —
including the auto-completion cascade and the zero-children fallback — is a
plain Go test with no TUI, no terminal, and no mocking required. Write those
tests directly against `store`, not against the CLI or the TUI, wherever the
assertion is about data rather than rendering.

Above that, follow stack-stitcher's tiers as documented in its
`CONTRIBUTING.md`: components take a message and hand back a model, assert on
the result; a component's rendering is a string (`ansi.Strip(m.View().Content)`)
worth asserting on for layout and styling; an end-to-end rig
(`src/model/rig_test.go`'s pattern, adapted) is the only way to test a full
keystroke-to-render flow and should stay the exception, not the default, for
the same reason it's rare there — it's timing-based and has to wait for
output rather than sleep and hope.
