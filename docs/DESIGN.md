# Design

The guiding decisions of Chore Crusher, written down so a contributor —
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

> Note (inline-creation refactor): several sections below — §4, §5, §6, and §12,
> plus `docs/plans/phase-4-task-tree.md`, `docs/plans/phase-5-add-input.md`, and
> `docs/plans/stack-stitcher-sister-tui.md` — describe the add input as a
> **bottom-pinned footer** and `COMPONENT_ADD_INPUT` as a separate focus zone.
> That design has been **superseded**: the add input is now an inline row inside
> the task tree (see `docs/plan/task-row-redesign-and-inline-creation.md`),
> startup focus is `COMPONENT_TASK_TREE`, and `addinput` is no longer composed by
> `taskspanel`. Those sections are retained for history; implement against the
> inline plan doc as the source of truth.

## 1. What this app is, and isn't

Chore Crusher is a to-do list manager with two front ends over one store: a
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
  created_by    text not null default ''
                -- declared owner tag ("pi", "claude", …); empty = owned by
                -- nobody. Only the MCP server reads it (§9); the CLI and TUI
                -- write '' and ignore it.

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
CLI as arguments (`crush <task-id>`) and printed by `add`. A ULID
is a stable, copy-pasteable, sortable-by-creation-time string that never
collides across a `list add` and a concurrent `task add` from two processes —
an autoincrement id needs the database to hand it out, which is fine, but a
ULID lets `store.NewTaskID()` be generated before the transaction opens,
which matters for §7's transaction-shape rule. Ids are **not** meant to be
typed from memory; the CLI accepts an unambiguous *prefix* of an id
(§9, `resolveID`) so a human or an agent copying an 8-character prefix from
`crush tasks` output doesn't have to paste the full 26 characters.

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

**States.** A task's `status` is one of `pending`, `in_progress`, `crush`.
`progress_kind` only has meaning while `status = in_progress`; it is `none`
for `pending` and `crush` tasks (§3 keeps this an invariant the store
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

- `subtasks` reaching 100% (every direct child `crush`) **promotes the
  parent to `crush` automatically.** This is a verified fact — if every
  child is done, the parent claiming otherwise would be a lie the store can
  see through — so the store does not wait for a human or a script to say so.
  This check re-runs on every child completion and must walk upward: completing
  a leaf can crush its parent, which can crush *its* parent, and so on.
  Implement this as a single `recomputeAncestors(taskID)` walk after any
  status write, not as a special case bolted onto `Complete()` alone —
  `Reopen()` and `SetProgress()` can also change whether a parent's derived
  condition holds.
- `percentage` reaching 100 **does not** auto-complete. It's a claim, not a
  verified fact, and the store has no way to distinguish "I meant it" from
  "I typed 100 out of habit." Completing is a separate, explicit action
  (`space` in the TUI, `crush <id>` on the CLI) even at 100%.
  If this surprises a future contributor enough to want to change it, that's
  a product decision to raise, not a bug to silently fix — it was chosen
  specifically to keep the one auto-promotion path (verified subtask
  completion) the only one, rather than accumulating several slightly
  different auto-complete triggers that a reader has to hold in their head.

**Completing cascades down; reopening does not.** Marking a task `crush`
(`space`, or `crush <id>`) sets every descendant, at every depth,
to `crush` too — a `crush` task with a `pending` grandchild is a state
this app does not allow to exist, because the two-list split (§6) would then
have to explain why a "done" tree still has visibly undone rows in it.
Reopening a task (`crush reopen <id>`, or `space` again on an already
complete task) does **not** cascade to children — it returns *only that task*
to `pending`. This is intentionally lossy: the task's prior `progress_kind`
and `progress_pct` are not restored, because tracking "what it was before
completion, one level deep" is a second piece of state for a rare path
(un-completing) that would otherwise need its own tests and its own edge
cases (what if it was `subtasks`-derived and a child changed while it sat
complete?). If this bites someone in practice, revisit it — but start from
`pending`, not from resurrected history.

**Agent activity is orthogonal to this machine.** A task or list can be claimed by an MCP agent (`claim_work`) without changing its `status` — the claim is a UI signal (a spinner in the TUI), not a state transition. Claiming a task does not move it from `pending` to `in_progress`; completing a task does not release an agent's claim. Status and progress writes by the same agent refresh (extend) its live claim's `acquired_at` — a write-heartbeat (docs/plan/agent-presence-heartbeat.md §3.2); they never create or release claims. The `AgentActivity` table (§3.5 of `mcp-server-enhancement.md`) stores which agent is on which entity and when; it is read by the same 1s poll that reads lists and tasks (§7), but it does not interact with the status machine above. Claims expire after `WorkTTL` (120s) of inactivity; the MCP server also calls `store.ReleaseAllClaims` when the MCP session ends (client disconnect), so a dead agent's spinners vanish immediately rather than waiting for TTL (hardening plan H13).

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
Before the input is submitted, `[` (outdent) and `]` (indent) change **where** the new
task lands, and the input's leading glyph and indentation reflect the current
choice:

| Keystrokes so far | New task's parent | New task's depth | Glyph |
| --- | --- | --- | --- |
| none (default) | selected task's parent (i.e., a sibling of the selection) | `L` | `-` |
| one `]` | the selected task itself | `L + 1` | `+` |
| one `[` (only if `L > 0`) | the selected task's parent's parent | `L - 1` | `^` |

Further presses in the same direction **do not go further** — the range is
clamped to exactly one level either side of the selection, always. This is
the literal rule ("one level deeper, one level above") and it is a clamp, not
a wrap: pressing `]` three times behaves the same as pressing it once.
`[` is a no-op (not an error, not a beep) when `L = 0`, because there
is no level above root.

Where the new task is **inserted** among its new siblings: immediately after
the reference task that determined its parent (the selection itself for the
default and `tab` cases; the selection's parent for the `shift+tab` case).
This keeps rapid entry predictable — hitting `enter` repeatedly at the
default level appends a flat run of siblings in the order typed, which is the
behavior anyone coming from Workflowy or a plain bullet list already expects.

**State resets, every time.** After a successful add, selection moves to the
newly created task, and the level state resets to default (`-`,
sibling-of-whatever-is-now-selected) — it does not stay at `+` just because
the last add was a child. Selecting a *different* task (arrow keys, `j`/`k`,
mouse if that ever lands) while the input has unsent text also resets the
level to default for the newly selected task; the level indicator describes
a relationship to the *current* selection, and an old relationship to a task
that's no longer selected would be meaningless. `esc` while the input has
text clears the text and resets the level, and does nothing else — the input
itself never hides, unlike a modal, so `esc` here has exactly one job.

## 5. Navigation and focus

Two keyboard focus targets, not three — this app
has a sidebar that can be entirely absent from the cycle:

- **Lists panel** — present in the cycle only while *rendered* (`L` toggles
  the preference; see §6). `listsPanelVisible` is the user's stored preference;
  a separate derived predicate, `listsPanelRendered()` (preference on **and**
  the layout gave it width this frame), decides whether it actually occupies
  the row — and that predicate, not the raw preference, governs focus, the
  footer, and rendering, so focus can never land on a zero-width panel. On the
  **first** window-size message the preference is seeded from terminal width:
  Lists auto-shows at `AUTO_SHOW_LISTS_MIN_WIDTH` (120 columns) or wider and
  starts hidden below it — spending your time in one list is still the premise,
  but a wide terminal has room for both. That seeding happens once; afterward
  `L` is the sole authority and a resize never reverses a toggle. When the
  terminal is too narrow for any sidebar (`MIN_PANEL_WIDTH + BODY_GUTTER_WIDTH`)
  Lists yields its width without changing the preference and focus falls back
  to Tasks; it reappears on a later resize if the preference is still on.
  Opening it by width never steals focus — the first focus is always Tasks.
- **Task tree** — the Tasks surface's Pending/Complete sections, one flat
  keyboard-navigable cursor across both (see §6 for why the split is visual
  section headers, not two independently-focusable lists). Inline creation
  lives inside the tree, so there is no separate add-input focus zone.

The body pairs **Tasks** with *at most one* side surface — **Lists** — the two
body shapes are Tasks alone or Tasks + Lists. When Lists is hidden, Tasks fills
the body width; when it shows, a tier-2 gutter separates the two equal-height
surfaces. Tasks is elevated while the task tree has keyboard focus. Moving focus
between surfaces must not change the Tasks surface's title, padding, gap, or
dimensions.

**Details is a modal, not a body surface.** Opening Details (`enter` on a
selected task, §5's key list) layers a centered modal over the body — sized to
most of the screen (about 90% of each axis) — and takes focus so its keys own
the keyboard while it is up; closing it (clean `esc`, a discarded edit, or a
completed save) restores focus to the task tree. It does **not** disturb the
Lists/Tasks split beneath it: the body layout is unchanged while the modal is
open, and it never takes a body column (`DetailsWidth` is always 0). Details is
never in the `tab`/`shift+tab` focus cycle — it is entered and left by the
explicit open/close transitions. It was briefly a thin side surface; that could
never be made wide enough to show notes and comments without crushing the Tasks
list beside it, so it is a modal again.

Inside the Details modal the notes textarea grows with its content but is capped
so at least one or two comment cards stay visible — notes never swallow the
whole modal. The comment thread renders as selectable cards (the shared row-card
chrome, §12); `↑`/`↓` move the highlight and `y` copies the highlighted
comment's id to the system clipboard.

`tab`/`shift+tab` cycle **only through the targets currently visible** —
the lists panel is skipped entirely from the cycle while hidden, the same way
stack-stitcher's nav bar is permanently absent from its own cycle
(`constants.FocusableComponents`) rather than being a focusable-but-inert
stop. Do not implement "hidden but still tab-able to an invisible panel";
that produces a focus ring with a silent dead stop in it.

`[`/`]` restructure the *selected* task — not just the create-mode level
selector (§4). Outdent `[` (move the selected task out from under its
parent, becoming the parent's next sibling) and indent `]` (move it under
its previous sibling, as that sibling's last child); both are no-ops at
their boundaries (a root task cannot be outdented; a first sibling has
nothing to hang under). Indent additionally obeys §3: a pending task never
moves under a complete sibling. While the inline create input is active the
same two keys are the create-mode level selector (§4) instead. `tab`/
`shift+tab` keep cycling focus between the two
panels even while the create row or the `/` filter has the keyboard — they
are focus keys, not characters, so they never compete with the level
selector — and the draft is preserved while focus is elsewhere; typing
resumes when focus returns to the tree. The tree is the startup focus
and is broadcast as such at startup (phase-3 Init), so its keys work
from the first frame rather than only after a focus change.

**Vim and arrow bindings both work, always, on the task tree:** `↑`/`k` up,
`↓`/`j` down (moving the cursor across every *visible* row — a collapsed
node's children are not visible rows and are skipped), `←`/`h` collapses the
selected node if it has children and is expanded, else moves selection to its
parent; `→`/`l` expands the selected node if it has children and is
collapsed, else moves selection to its first child. This is the same
convention as `nnn`, `lf`, and most terminal file managers with a tree pane —
picked because it is already muscle memory for the audience, not invented
for this app.

**`alt+↑`/`alt+k` and `alt+↓`/`alt+j` move the selected task** up or down
*within its own status run* — the gesture never crosses the Pending/Complete
boundary, so a task cannot be moved out of the section it belongs in
without being crushed or un-crushed first (§3, §6). The modifier choice
follows vim's `alt+k`/`alt+j` convention (a plain `k`/`j` moves the cursor;
`alt` makes it move the *thing under* the cursor, the same handshape VS
Code's `alt+↑`/`alt+↓` uses for moving lines) — one key with a modifier,
not a second unmodified key that would steal a character vim users expect
to type.

**`space` toggles complete/pending** on the selected task, from wherever the
tree has focus — it does not open anything and does not move the cursor.
**`enter`** on a selected tree row opens the Details modal — so it can't
also mean "toggle complete"; the two are deliberately different keys because
"open a thing" and "flip a checkbox" are different enough actions that
collapsing them into one key is what makes an app feel like a demo rather
than a tool. Note the asymmetry with stack-stitcher, which binds `Select` to
*both* `space` and `enter` — that works there because both mean "start"; here
they must mean two different things, so they are two different bindings from
the start rather than one alias split apart later.

Inside the Details modal: **`ctrl+s`** saves title, notes and progress changes,
closes the modal, returns focus to the task tree, and refreshes its rows;
**`tab`**/**`shift+tab`** cycle between the title editor, the notes editor, the
progress selector, and the comment thread (every zone is always in the cycle —
the comment thread is reachable even while empty, since `c` adds the first
comment from there); **`←`/`→`** (or `h`/`l`) cycle through the three progress
modes (`simple`/`subtasks`/`percentage`) while the progress selector is focused;
**`↑`/`↓`** move the comment highlight and **`y`** copies the highlighted
comment's id to the system clipboard while the comment thread is focused;
**`ctrl+y`** copies the open task's id from any zone (a TUI has no reliable text
selection under mouse reporting, so the id is shown below the title and a key is
the real copy affordance); `esc` closes a clean modal immediately, and on a dirty
one shows the inline `Discard changes? (y/n)` prompt — `y` closes and discards,
`n` keeps editing. No path silently discards edits. The label of the focused
zone (Title/Notes/Progress/Comments) is bolded onto `TextPrimary`, and every
footer hint bolds its key ahead of a muted description.

The task **title** is an editable single-line `textinput`, first in the tab
cycle; a save writes it through `store.RenameTask`, and a title cleared to
whitespace is refused in place (the store forbids an empty title) rather than
closing on a write that would fail.

Adding a comment is an **inline compose card**, opened with **`c`** from the
comment thread — the same "fake card while adding" shape the task tree uses for
inline task creation. The card renders at the foot of the thread styled like the
selected comment card, showing the OS author and a single-line `textinput`;
**`enter`** posts it and **`esc`** cancels (a terminal cannot reliably
distinguish `ctrl+enter` from `enter`, so `enter` is the submit key — `ctrl+enter`
is accepted as an alias). Comments are short status/handoff notes, so one line is
sufficient (`docs/plan/task-comments.md` §6). The compose draft is **transient**:
it is not part of the task's saved fields, so `ctrl+s` never posts it and `esc`
on the modal (outside the card) needs no discard prompt for it. While the card is
open the draft survives a poll refresh — the input is cleared only when the card
closes — so a background refresh can no longer wipe a half-typed comment. Posting
a comment appends it to the visible thread immediately (no modal reload); the next
refresh reconciles the authoritative ordering. Below the progress zone, each
comment renders as a selectable card in the shared row-card chrome (§12) — a
`{author} · {timestamp}` header in `TextDim`, a blank spacer, then the note in
`TextPrimary` wrapped to the card width, oldest first — with the highlighted card
lifted onto the elevated tier under an accent bar. The comment write path goes
through `store.AddComment`, which enforces the per-list `comments_disabled` flag
and surfaces its error as an inline message rather than posting.

**`/?`** enters a local fuzzy filter, and its target follows focus: the
**task tree** filter (phase 8) narrows the current list's rows in place to
each match plus its ancestor chain, so a matched leaf never loses its
parent rows; the **lists panel** filter narrows the visible lists the same
way. `enter` applies the query and leaves the filtered view active;
`esc` clears it.

**`F`** opens the cross-list search picker (phase 8): a text input searches
every list live, ranking title matches before notes-only hits, and showing
each result as `<list> › <task>`. `enter` on a result jumps to that task —
switching the active list when the match lives elsewhere — and `esc` cancels.

**`d` deletes the selected task** (or list, when the lists panel is focused),
prompting for confirmation first (docs/DESIGN.md §9: destructive TUI ops need
a confirm modal). The tree emits `DeleteTaskMsg`; AppModel opens a confirm modal;
accepting runs `store.DeleteTask` and refreshes the rows. List delete
(`L` panel, `d`) follows the same confirm-modal pattern.

**Task renaming** in the TUI is done in the Details modal: its Title field is an
editable input (first in the tab cycle), saved with `ctrl+s` through
`store.RenameTask` — see the Details modal keys above.

`esc` follows the "ladder of claims" stack-stitcher documents: a modal
(theme picker, confirm, list-name) closes itself first — it intercepts
all keypresses at the top of `Update`, so by the time esc reaches AppModel's
own handler no modal is open. Next, while the **Details** modal is visible it
owns every keypress (it is tracked separately from `activeModal` — for its
poll-refresh-while-open — so it sits just after the modal check): its own handler
takes `esc` — closing a clean modal, or opening the discard prompt on a dirty
one — before AppModel's normal Back case runs.
Then the focused panel claims esc if it declared `KeepsEsc`: the tree while
typing in or applying a `/` filter, or inline-creating (§8); the lists panel
while its filter is open or applied. After that, a no-op. The ladder is one
switch case (`keys.Global.Back`) that checks `KeepsEsc` on the focused
component. Keep this ladder tested against its claims in order — checking
claims in the wrong order silently breaks whichever claim got skipped.

## 6. The main panel: Pending and Complete

The right panel is two sections, headed `Pending` and `Complete`, not two
independently scrollable/focusable lists. One cursor moves through both,
`Pending` first, and each section keeps its own order: the cursor walks the
*pending rows in their store order, then the complete rows in theirs*, so a
complete task sitting between two pending ones in the store (it was crushed
in place, §3) never hijacks a pending row's cursor position. Crossing the
section boundary wraps — `↓` from the last pending row lands on the first
complete row, `↑` from the first complete row returns to the last pending
row — and the two ends clamp (no wrap from before the first pending or
after the last complete). The cursor is a flat walk over the concatenation
`pending + complete`, not a per-section index; "own order" is about which
rows the walk skips, not about separate focusables (that would reintroduce
the exact dead-stop focus ring §5's tab rule forbids).

**Which section a task-tree renders under is decided by the *root* task's own
`status` alone**, not by scanning descendants: a root-level task with
`status != complete` renders (with its whole visible subtree) under
`Pending`; a root-level task with `status = complete` renders under
`Complete`. Because completing cascades to every descendant (§3), a tree
under `Complete` is, by invariant, 100% crush rows all the way down — the
section header is a true claim, not an approximation. A tree under `Pending`
can and will contain a mix: a `pending` parent can have `crush` children
sitting inline (checked, perhaps struck through) underneath it, still nested
in place. **Do not move a completed subtask out to the `Complete` section
while its parent is still pending** — that would separate a task from the
tree it belongs to for a reader trying to see the shape of remaining work,
which defeats the reason the tree exists at all.

A list with no tasks yet auto-shows the inline "new task" input as its only
row, under the `Pending` header — the input creates a pending task, so it
belongs to the section the task will land in. Once the user leaves it with
esc, the standard recessed empty-state card takes its place ("No tasks yet.
Press n to create one." — see §12 "Empty states"), and the input only comes
back via `n` or by deleting every remaining task in the list. The `Complete`
header is omitted entirely when empty; an empty `Complete` section with
nothing under it is noise no reader needs.

**Vertical rhythm inside the sections:** one blank line separates each
section header from its first row (and from the row above it), and one
blank line sits below the last pending row before the `Complete` header —
the two sections read as two blocks instead of one unbroken column, and a
pending list's bottom edge doesn't collide with the complete header. The
blank line after a section's last row is part of the section, not the next
one: it renders even when the next section is empty.

**Scrolling is selection-driven, not a second navigation mode.** When the two
sections together are taller than the panel body, the tree renders only a
vertical window of its lines and shifts that window the minimum needed to keep
the selected row (or, mid-create, the inline input row) inside it, under the
existing `↑`/`↓`/`j`/`k` walk — there is no mouse wheel, no page-up/page-down
binding, and no horizontal scroll (all out of scope). The window is computed
from one **line plan** (`[]panelLine` in `tasktree`, each entry a single display
line tagged with its task id or blank for chrome) that the renderer and the
scroll math share, so header/blank/rule counts are never duplicated in the
scroll logic. The scroll offset lives in the tree model and is advanced by a
Bubble Tea update — never by rendering — after navigation, refresh, filter
change, collapse/expand, create start/cancel/confirm, and a layout resize; a
resize that shrinks the panel clamps a now-too-large offset, and an empty or
no-selection state resets it to the top. The window always paints its full
height: any short remainder is filled with the panel-tier background so a
partly-filled panel never bleeds. The `Lists` panel keeps its existing
Bubbles-list scrolling unchanged — one scrolling system per panel.

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

**Agent activity claims are read by the same poll.** Each `RefreshLists` and `RefreshTasks` call also runs `store.ListWork()` to fetch the current set of live agent claims (entities claimed within the `WorkTTL` window). The returned `[]AgentActivity` travels with the refresh messages so the task tree and lists panel can render a spinner on claimed rows. `RefreshLists` additionally computes the set of lists with any live task claim (`ClaimedTaskListIDs`) and carries it on the message, so the lists panel shows a spinner on a list row when an agent is working inside it — not only when the list itself is claimed. The same 1s poll tick governs both data and claims — no separate IPC or interval is needed.

**The very first load is animated, later polls are not.** `GetInitialModel`
does no database work: it constructs the components and returns immediately with
no active list, so Bubble Tea can paint the first frame before the opening
`RefreshLists` query completes. Until that first `RefreshListsMsg` arrives —
success *or* error — the Tasks panel renders a sealed `Loading` label with an
animated ellipsis (Bubbles' `spinner.Ellipsis` frames `""`, `.`, `..`, `...`,
so the animation adds no ambiguous emoji width). The animation lives only in the
`taskspanel` initial-load state; the first refresh leaves that state permanently
and no later `PollTickMsg`/`RefreshListsMsg` cycle ever restores it. The ellipsis
is drawn through a render-time `Foreground(appstyles.Active.Accent)` style read
fresh each frame — no theme color is cached on the spinner, so a live theme
switch mid-load repaints it. The empty-store default-list creation that the old
constructor did synchronously now happens on that first refresh (§5): a
successful first load still ends with an active list.

**The TUI never holds a write transaction, and no poll tick is allowed to
block on one.** It opens a read connection at startup and keeps it; every
poll is a `SELECT`, full stop. All writes — from the TUI's own keypress
handlers as much as from an external CLI invocation — go through the same
`store` functions the CLI uses (§8), each wrapping one short transaction that
opens, writes, commits, and returns, so a rapid-fire agent loop calling
crush <task-id> in a shell `for` loop is never waiting behind the TUI, and
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

**One file:** `$XDG_DATA_HOME/chore-crusher/chore-crusher.db` (falling back to
`~/.local/share/chore-crusher/chore-crusher.db`), opened in WAL journal mode. WAL is
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

The database is **already per-OS-user**: the path derives from `$XDG_DATA_HOME`
(or `~/.local/share`), which is per-account by definition. Two OS users on the
same machine get independent databases with no extra code — the
"separate SQLite databases by user" ask is satisfied as-is, and the CLI/TUI
is not multi-tenant within one OS account.

**Migrations** are numbered `.sql` files embedded via `embed.FS`
(`store/migrations/0001_init.sql`, `0002_*.sql`, …), applied in order inside
`store.Open`, tracked by a `schema_migrations(version integer primary key)`
table. Every invocation of the binary — TUI or any CLI subcommand — runs this
before touching data, idempotently (a migration that's already applied is a
no-op, not an error). This is the same "one resolution, passed down" instinct
stack-stitcher applies to compose-file discovery: one function decides the
schema is current, called from one place, rather than each caller assuming
someone else already did it.

**Config** (`~/.config/chore-crusher/config.yaml`, or `$XDG_CONFIG_HOME`) holds
exactly two fields at launch, in the same struct-designed-to-grow shape as
stack-stitcher's `config.Config`:

```yaml
theme: crush-dark
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
(`crush lists add` prints the new list's id and nothing else; `crush add` prints the new task's id and nothing else). A read command prints a
formatted table or tree to stdout. Any failure prints one line to stderr,
prefixed `crush: `, and the process exits non-zero.

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

**Destructive commands need `--force`.** `crush lists rm` and `crush rm`
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
crush                                          launch the TUI
crush lists                                    list all lists
crush lists add <name>                         create a list; prints its id
crush lists rename <list-id> <name>            rename a list
crush lists rm <list-id> --force               delete a list and its tasks

crush tasks <list-id> [--status pending|in_progress|complete|all] [--flat]
                                                   list tasks (tree by default)
crush add <list-id> <title> [--parent <task-id>] [--notes <text>]
                                                   add a task; prints its id
crush show <task-id>                           title, notes, status, progress, children
crush rename <task-id> <title>                 rename a task
crush notes <task-id> <text>                   replace a task's notes (whole text, not append)
crush <task-id>                                mark complete (cascades to descendants)
crush reopen <task-id>                         mark pending (does not cascade)
crush toggle <task-id>                         complete <-> reopen, whichever applies
crush progress <task-id> --mode simple
crush progress <task-id> --mode percentage --percent <0-100>
crush progress <task-id> --mode subtasks
crush mv <task-id> [--parent <task-id>]        re-parent a task; empty --parent moves it to the list root
crush rm <task-id> --force                     delete a task and its descendants
crush search <query> [--list <list-id>]        fuzzy search across titles (+ notes)

crush mcp                                      run the MCP server on stdin/stdout

crush --version
```

**`crush mcp`** runs a Model Context Protocol server over stdin/stdout. The
tools it exposes mirror the CLI subcommands and return the same JSON shapes
that `--json` would emit on the command line, so an agent host can call
`crush` operations as native tool calls instead of spawning the CLI per
operation. The server is a thin adapter over `src/store` in
`src/mcpserver`, not a layer on `src/cli`, preserving the "two front ends
over one store" rule from §1 and §10.

**MCP rows are a superset of the CLI's `--json` shapes** (CONTRIBUTING rule
6): `my_list` adds `position` and `created_by` to the `lists` rows it
returns (`mine` + `foreign_lists`), and the read-only resources mirror the
tools row-for-row (including `created_by` on `crush:///lists` and
`crush:///lists/{id}`). Both surfaces read the same
store rows (`ListLists`, `ListTasks`), so a field added to one appears in
the other (hardening plan §4.5, §4.7); the server-side tests that pin the
MCP shapes live in `src/mcpserver/server_test.go`. The task read shapes —
`show_task`/`crush show`, `list_tasks`/`crush tasks`, `search_tasks`/`crush
search` — carry `list_owner` on every row (the parent list's `created_by`,
`""` for an unowned list), so an agent holding a task id knows at a glance
whether its list is writable without a separate `my_list` round-trip
(CONTRIBUTING rule 6: the CLI `--json` shapes gained the same additive
field).

**MCP agent-optimised extensions.** The task read shapes gained
`has_notes` and `notes_len` on every row: `has_notes` is `false` when the
notes body is empty, so an agent can skip a `show_task` call entirely.
`list_tasks` accepts an optional `include` parameter (`["notes"]`) that
inlines the notes body per row, capped at 2000 characters with
`notes_truncated=true` when trimmed — one call replaces N `show_task`
follow-ups. `show_task(ids)` is the batch equivalent for cross-list
workflows: up to 50 task ids return their full details (including comments)
in one call, with unresolvable entries returned as `{id, error}`. The `progress` field is
omitted on rows where the task has no progress, cutting typical row size by
~25%. `my_list` now returns `{mine: {id,name,pending,complete},
foreign_lists: [{id,name,pending,complete,created_by}]}`, merging `my_list`
+ `list_lists` into a single session-opening call. Status and progress
writes auto-claim the task under the writing agent's identity (best-effort,
non-stealing), so `claim_work` is only needed when an agent wants a claim
before writing. Every other task write — `add_task`, `add_comment`, `edit_task` — auto-claims
the touched task too;
`delete_task` does not (the task no longer exists), and `DeleteTask` clears
any claim rows on the deleted subtree so a removed task cannot keep a spinner
alive. Full rationale: `docs/plan/mcp-presence-on-all-writes.md`. The
`crush:///inbox` resource and `crush_inbox` prompt
deliver all of the above as a single read for start-of-session triage.

**Batch status/progress writes.** `set_progress(ids)`, `complete_task(ids)`,
and `reopen_task(ids)` each take 1–50 task ids and return one
`{id, ok:true}` / `{id, error}` row per id in input order (a bad id does not
stop the rest — not a transaction). `edit_task` covers the
rename/notes/re-parent/to-root structural edits in a single call (destructive
or ownership-gated, it stays single-task). Each touched task is auto-claimed
under the writing agent's identity, same as the single-task writes. Full
rationale: `docs/plan/mcp-batch-writes.md` and
`docs/plan/mcp-tool-consolidation.md`.

**`list_changes`** lets an agent cheaply check "did anything change since I last
looked?" on a single list: pass the unix timestamp of your previous call as
`since` and it returns only tasks whose `updated_at` is strictly greater (newly
created, status/progress edited, renamed, re-noted, re-parented, or newly
commented — `AddComment` bumps `updated_at`, so new comments surface too). The
rows use the exact same shape as `list_tasks` (`has_notes`/`notes_len`,
omitted-empty `progress`), so `include=['notes']` inlines bodies identically.
Deletions are not representable by a row filter — a removed task is simply
absent; an agent that must detect deletions diffs id sets against its last
`list_tasks`. `updated_at` now means "last activity, including comments"
(`docs/plan/mcp-list-changes-since.md` §1). Full rationale:
`docs/plan/mcp-list-changes-since.md`.

**List ownership, and what the MCP server refuses.** Every `List` carries a
`created_by` tag (§2). The MCP server reads its own identity once at start
from the `CRUSH_AGENT` environment variable (default `"agent"`); the *human*
sets it per server in the MCP client config, so one stdio session is one
identity and no tool lets an agent change it. A list is writable by that
session only when `created_by` equals the identity. The structural tools — `add_task`, `edit_task`, `delete_task` — resolve
their id first (the §9 prefix rule still applies), then refuse a foreign list
with one error shape: `list <id> is owned by <owner> — you may read it and
update task status/progress only`. `edit_task`'s re-parent checks the list it
would move *into* — the parent's list when a `parent` is given, the task's own
list when `to_root` is set — before any write, so a refused move never
half-happens.
(`store.Reparent` independently rejects a parent on a different list, so on a
move that would otherwise succeed the two are the same list; the owner check
runs first and reports ownership rather than the cross-list error.)

**Status and progress are never gated.** `complete_task`, `reopen_task`,
and `set_progress` work on every list, as do all reads, `claim_work`, the
resources, and the prompts. An **empty** `created_by`
means owned by nobody, which makes the list foreign to *every* identity: an
untagged list — the shape `crush lists add` and the TUI create — is read +
status/progress only for all agents, and only a human can restructure it.
`add_list` defaults `created_by` to the session identity and accepts an
explicit tag matching `^[A-Za-z0-9_-]{1,32}$`; `my_list` and `crush:///lists`
report `created_by` on every row. Ownership is adopted from the `<tag>: <name>`
naming convention in two places (hardening plan §4.6–4.7): a rename into a
tag adopts the owner **in the same write** (`store.RenameList` — the human's
`crush lists rename Groceries "pi: Groceries"` handoff path takes effect
immediately), and `crush lists add --owner <tag>` provisions an owned list
from the start. One idempotent backfill pass at `store.Open` catches
anything that predates both. The adoption cannot tell intent: any `^tag:`
prefix is adopted, so a human list named `Note: buy milk` becomes owned by
tag `Note` — an accepted false-positive class (hardening plan §4.10). The
inverse is deliberately *not* done: `GetOrCreateAgentList` and
`my_list` match `created_by` only, never the name, so an untagged
`pi: ...` list is never silently adopted.

Enforcement lives in `src/mcpserver` alone (the `requireWritable` helper) —
the store stays a dumb data layer and the CLI and TUI stay unenforced, which
is the deliberate front-end divergence CONTRIBUTING rule 5 asks to be written
down. Identity is self-declared and unauthenticated: this is cooperative
trust between agents, not a security boundary. When comments arrive they join
the owner-only bucket behind the same `requireWritable` helper. Full
rationale and the rejected alternatives:
`docs/plan/list-ownership-enforcement.md`.

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

`crush mv <task-id> [--parent <task-id>]` (re-parent a task) is the one
CLI re-parent, without the ±1-level restriction §4 puts on the TUI's *add*
flow — a CLI re-parent is a deliberate restructure, not the inline-add
gesture that rule exists to keep predictable. The task stays in its current
list (a cross-list parent is rejected) and is appended to the end of the
new parent's children, closing the gap it leaves behind. **An empty
`--parent` — the flag's default, so omitting it entirely — moves the task to
the list root**; that is the documented representation of "no parent". `mv`
rejects a target that would create a cycle (reparenting a task under its own
descendant, which would break the tree walks in `store`), and moving a
non-complete task under a complete parent, which §3 forbids to exist —
complete the task first, then move.

## 10. Package layout

Mirrors stack-stitcher's split between the Bubble Tea half and the
non-Bubble-Tea half, with one addition (`cli`) for the second front end:

```
main.go              # cobra root: no subcommand -> launch TUI; else dispatch
src/
├── model/           # AppModel: Init/Update/View, the top-level Bubble Tea model
├── components/      # one package per leaf model (tasktree, listspanel, addinput,
│                     # detailspanel, themepickermodal, searchmodal, listnamemodal,
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
├── mcpserver/       # Model Context Protocol server; tools mirror the CLI but
│                     # talk directly to src/store, not to src/cli
├── appstyles/       # Theme type + the 14-theme registry, ported from stack-stitcher
├── config/          # ~/.config/chore-crusher/config.yaml
└── constants/       # layout widths, focusable-zone ids, branding
```

**`src/store` is the only package that imports `database/sql` or
`modernc.org/sqlite`.** `src/model` (the TUI), `src/cli` (the CLI), and
`src/mcpserver` (the MCP server) all depend on `store` and nothing deeper;
none of them ever builds a SQL string. **`src/cli`, `src/model`, and
`src/mcpserver` are siblings over the same `store`, not layered on each
other**, which is the structural expression of "neither front end is
secondary" from §1. `main.go` is the one file that imports the CLI and TUI
to decide which to run, and `src/mcpserver` is reached through the
`crush mcp` subcommand.

## 11. Theming

Ported from stack-stitcher's `src/appstyles` near verbatim — the `Theme`
struct, `newTheme`'s tier-derivation math, the `InkOn` contrast helper, and
the picker's live-preview-on-cursor-move mechanic. Read
[stack-stitcher's `Theme.go`](https://github.com/filipemolina/stack-stitcher/blob/main/src/appstyles/Theme.go)
and copy its structure; do not redesign the derivation math, it's already
been tuned across 14 imported palettes (see stack-stitcher's `docs/DESIGN.md`
§"Color lives on a Theme" for the reasoning behind `Lighten`/`Darken` and why
`Modal` needs to clear `BackgroundElevated` by a minimum margin).

**Elevation ladder (the `raise` coefficients in `newTheme`).** The focused/
unfocused panel step is the focus signal (see §12 "Focus is shown by lifting a
tier"), but the step's *contrast ratio* is inherently small for every theme:
`BackgroundContent = raise(Panel, 0.04)`, `BackgroundPanel = raise(Panel,
0.08)`, `BackgroundElevated = raise(Panel, 0.12)`, `BackgroundRecessed =
Panel` (un-raised base). Both elevated and panel derive from the same base by
`Lighten` (dark) / `Darken` (light), so for a near-black base (dark themes)
the additive step near black is tiny, and for a near-white base (crush-day,
the lone light theme) a larger coefficient *darkens* elevated toward the
*lighter* panel, shrinking the ratio. Measured: the elevated-vs-panel step is
~1.10-1.17 for every theme and is capped at ~1.2 for crush-day under the
additive ladder — a geometric ladder was prototyped and hit the same cap, so
the base palette, not the step function, is the binding variable. The nominal
1.35 target for this step is therefore unreachable without moving the base
colors and/or relaxing the `TextPrimary on elevated ≥ 4.5` ceiling, both out
of scope for the focus bug. The genuinely perceptible, theme-independent focus
signal is the **selected-row** contrast (`ModalBg` for the focused panel's
active row vs `BackgroundElevated` for an unfocused panel's remembered row),
which `chrome.ListRowBg` produces and which measures ~9.5:1 on crush-day —
that is the fix that makes focus obvious, not the panel-surface step.

**Text colors that sit on the elevated tier.** Three imported palettes ship a
body `Text` too dim for a brightened elevated tier — `one-dark` (`#ABB2BF`),
`solarized-dark` (`#93A1A1`), and `everforest-dark` (`#D3C6AA`) — and were the
only themes failing `TextPrimary on elevated ≥ 4.5` once the ladder was
widened. Their `Text` is lightened to `#C6CDD7`, `#BCC4CF`, and `#E4D9C0`
respectively (chosen as the dimmest grey that clears 4.5 on elevated *and* on
panel). This changes those three themes' body-text luminance slightly; every
other theme's `Text` is untouched.

**What changes:** the four status-color fields are domain colors, so they're
renamed to match this app's domain instead of Docker's:

| stack-stitcher | Chore Crusher | Same hex per theme |
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
Every `Name` string and registry key is adjusted from `stitcher-*` to
`crush-*`, since the name is user-visible in the theme picker (`crush-dark`
is the renamed `stitcher-dark`). **The fresh-install default is
`"catppuccin-mocha"`** — `DefaultTheme` names it, and a config with no
`theme:` preference activates it; every other registered theme (including
`crush-dark`) stays selectable through the `T` picker and as a saved
`theme:` value.

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
| 3 | `BackgroundPanel` | the Lists and Tasks surfaces, when unfocused (`raise(Panel, 0.08)`) |
| 4 | `BackgroundElevated` | Lists when it has focus, or Tasks while its task-tree or add-input control has focus (§5), and the highlighted comment card in the Details modal (`raise(Panel, 0.12)`) |
| — | `ModalBg` | every modal (theme picker, confirm, list-name, **and the Details modal**) **and the row the cursor sits on in the task tree** — an active row is its own register, not a tint of the panel it's in, the same reasoning stack-stitcher applies to an active list row |
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

A rendered surface's box is exactly the same width and height whether or not
it has focus. What changes is the fill: `BackgroundPanel` (tier 3) unfocused,
`BackgroundElevated` (tier 4) focused. **Do not indicate focus with a
heavier border, a different border color, or a resized box** — any of those
shifts the layout of everything around it by at least one cell, which is
exactly the kind of thing that looks fine in isolation and wrong the moment
two surfaces are on screen having made different choices. One function,
`chrome.PanelBg(isFocused bool) color.Color`, is the only place this
decision is made; each surface calls it rather than branching on focus itself.
The selected task row still uses `ModalBg` as its separate active-register
signal, and the input caret identifies the active control inside Tasks.

### Two shared frames: `chrome.PanelFrame`

`chrome.PanelFrame` owns the body frames: **Lists** and **Tasks**. It renders
those exact labels through `appstyles.NormalTitle()` as an accent chip with a
two-column left gutter, then one blank chrome row before the body. (The Details
surface is no longer one of these — it is a modal, wrapped in
`chrome.ModalSurface`, sized to most of the screen and layered over the body;
see §5.) The frame has **1 row vertical and 2 columns horizontal** padding
(`lipgloss.NewStyle().Padding(1, 2)`), matching stack-stitcher's `PanelFrame`.
No component sets its own panel padding value or panel border.

The **Tasks** frame additionally shows the active list's name on its title
row: bold, in the panel's primary text color, right-aligned so it lands on the
body's right edge opposite the "Tasks" chip (`chrome.PanelFrameWithRightTitle`).
The name is truncated (`chrome.Truncate`) to the space the chip leaves, so it
never widens the frame; when there is no active list the row is just the
"Tasks" chip, unchanged. Lists passes no right label.

The frame receives the total surface box. It alone derives its inner body
width and height, so callers never subtract frame padding twice. Tasks
composes its raw task tree above its raw add-input footer inside that one
inner box: the tree clips or scrolls above, while the unframed one-row input
is pinned to the bottom with the same Tasks background and no divider. The
inner renderers receive their supplied dimensions and background; they do not
add a title, frame, elevation, or local padding.

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

Phase 9 implements that for the task-tree row as an explicit drop order
(`tasktree.taskRowDropOrder`, enforced by `tasktree.fitTitleAndSuffix`): the
indent, collapse marker and checkbox are the row's identity and are never
shed; the title is truncated grapheme-safely by `chrome.Truncate` and kept to
the last; the trailing percentage is the one whole unit the row gives up, and
it is **never** rendered as a fragment — when there is no room for the whole
unit, its columns are handed back to the title, so a task's name always
survives at least as well as its optional progress figure. `TUI`'s width-sweep
test (`tasktree.View_test.go`) asserts, for a sweep of panel widths, that a row
never overflows and the percentage is shed whole or not at all.

### The glyph vocabulary

One table. A component does not invent a symbol not listed here; if a new
one is needed, it's added here first.

| Meaning | Glyph | Notes |
| --- | --- | --- |
| Task: pending | `◻` | Text-presentation square — the checkbox character Claude Code's todo lists use (`figures.squareSmall`, verified from its source, 2026-08-03). Single display cell, unlike the emoji ⬜ (2 cells). |
| Task: in progress | `◻` | The same text-presentation square as pending — no dedicated glyph; the `IN PROGRESS` label and bar colour set the row apart. Used for all three progress kinds (§3) alike — the trailing percentage (below), not the checkbox, is what distinguishes them. |
| Task: crush | `◼` | Filled square (`figures.squareSmallFilled`), tinted `StatusComplete`; title renders in `TextMuted`, not `TextPrimary`, once crush — see Typography below. |
| Node has children, expanded | `▾` | One column wide, appended to the *end* of the title (see Row layout below). |
| Node has children, collapsed | `▸` | Same column, same position — the marker never occupies a leading column, so a parent's title starts at its own depth. |
| Node is a leaf | *(no glyph)* | Nothing appended; the title simply has no trailing marker. |
| Task has detail text | `🗎` | U+1F5CE DOCUMENT, left half of the fixed two-cell trailing icon column, immediately right of the status column, in `TextMuted`; the column is reserved on every row and the notes cell is rendered blank when `Notes` is empty, so noted and un-noted rows keep the same right edge. The column is two cells because it pairs with the comments glyph (below). Measures one cell in go-runewidth, but it is an emoji codepoint: emoji-capable terminal fonts may render it two cells or tofu — accepted tradeoff, the `✎`/`ⓘ` alternatives were rejected in favour of the literal "document" reading (2026-08-03). |
| Task has comments | `🗨` | U+1F5E8 LEFT SPEECH BUBBLE, right half of the fixed two-cell trailing icon column, in `TextMuted`; the cell is blank when the task has no comments. `💬` (U+1F4AC) was the natural choice but measures two cells in go-runewidth (v0.0.23), which would have widened the column past its partner glyph — `🗨` is the one-cell form (2026-08-06). Absent a comment the cell is blank. `HasComments` is set per-row by `RefreshTasks` from `store.TaskIDsWithComments` (Commit 4 of `docs/plan/task-comments.md`). |
| Row card: active bar | `▌` | Left edge marker on lists and task rows. Accent when the row is selected (or the inline input is active), otherwise the row's own status color — see Row layout below. |
| Add-input level: sibling (default) | `-` | §4. |
| Add-input level: child | `+` | §4. |
| Add-input level: parent-of-selection | `^` | §4. |
| Trailing derived/percentage progress | ` (NN%)` | In `TextDim`, rendered in the row's right-aligned block immediately before the status; omitted entirely when `DerivedProgress` reports `displayAsSimple` (§3) — never rendered as `(0%)` in that case. |
| Agent is working | `⠋⠙⠹⠸⠼⠴⠦⠧` | 1-cell braille spinner, animated via `AnimTickMsg` (§3.5 of `mcp-server-enhancement.md`); draws `Accent` when the row is focused/selected, `TextDim` otherwise. Appended to the right-aligned block after the status label when the row's entity is claimed. The `Spinner(frame int)` function lives in `src/components/chrome/Spinner.go`; no component invents its own glyph. |

**Task rows are full-width cards** (docs/plan/task-row-cards-and-status.md):
a `▌` bar column, then `{2 spaces × depth}{checkbox}{space}{title}` on the
left and the right-aligned `{progress}{space}{status}` block, then the fixed two-cell trailing icon column (`{🗎}{🗨}`, each cell blank when its indicator is absent) at
the line's end — the bar and checkbox sit flush, and every level of depth
indents the *whole card* by two columns, so a subtask's bar steps right and no
continuous vertical bar line forms. A parent's title carries the
expand/collapse marker (`▾`/`▸`, one column, no space) at its end — before
the right block when the title is long enough to reach it, dropped
entirely when the title is shed for narrowness — and the whole right block
sheds as a unit before the title does. The status label sits in a
**fixed-width column** (the longest label, `IN PROGRESS`, 11 columns) with the
label right-aligned inside it, so `PENDING` / `IN PROGRESS` / `COMPLETE` all
end at the same column across rows; the document glyph (or, with no notes, its
reserved blank cell) is the row's last cell, right of that column. The card spans
the panel body width with `Padding(0, 1, 0, 0)` — no vertical padding,
content-height, so a one-line title makes a one-line card — and the
selected row's `ModalBg` covers the full card, not just the text run.
Status labels are all caps — `PENDING` in `TextMuted`, `IN PROGRESS` in
`StatusInProgress`, `COMPLETE` in `StatusComplete` — and the bar is accent
when the row is selected, the row's own status color otherwise. Under
narrowness the progress sheds before the status, both whole; the title and
checkbox are never shed. Depth-0 pending example (parent row):
`▌◻ Buy paint for the fence ▾             PENDING`.

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
  secondary information), section-header counts, a list's pending/complete
  counts in the lists panel.
- **`TextDim`** — inert or placeholder text: an empty-state's message, a
  disabled key hint, the add-input's placeholder text before anything is
  typed, the trailing progress percentage in a task row.
- **Status labels** — the one place a status token, not a text tier, styles
  text: `PENDING` renders in `TextMuted`, `IN PROGRESS` in
  `StatusInProgress`, `COMPLETE` in `StatusComplete` (the same tokens the
  checkbox already draws). The three text tiers carry no semantic
  success/warning color, which is why the theme holds these tokens
  separately (docs/plan/task-row-cards-and-status.md).

Do not introduce a fourth informal tier (a hand-picked opacity, a literal
gray hex) for "something in between" — if the three don't cover a case,
that's a signal to reconsider the case, not to add a color.

### Empty states: one recessed-card pattern

Every empty state (an empty Tasks surface after the inline input was
dismissed; a lists panel with no lists yet) is the same shape: a box on the
`BackgroundRecessed` tier, rimmed with
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
2. Its outer surface is built with `chrome.PanelFrame` (or, for a modal,
   the equivalent shared modal-frame helper phase 3 establishes) — it does
   not set its own padding, border style, or corner treatment. A leaf inside
   the aggregate Tasks surface does not create a second frame; its parent
   frames and seals the aggregate while the leaf uses the supplied dimensions
   and background.
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
