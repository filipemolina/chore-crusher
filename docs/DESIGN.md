# Design

The guiding decisions of Farol, written down so a contributor —
human or agent — has a north star instead of a one-line feature description to
extrapolate from. Where a rule below looks oddly specific, it is specific on
purpose: it was written to close off a plausible wrong implementation, not to
describe the obvious one.

This document is self-contained: every rule below carries its own reasoning,
and nothing here sends you to another repository's documentation to find out
*why*. Farol shares its architecture with one sister project (they
were built by the same author, one after the other), and several patterns
were ported from it; where that provenance matters, the relevant reasoning is
written out in full in this file instead of being left as a pointer.

## 1. What this app is, and isn't

Farol is a to-do list manager with one store and two ways to reach it: a
terminal UI for the human at the keyboard, and a CLI that is the single
agent-facing front end (scripts and coding agents, including agents that use
Farol as their todo store). Neither is secondary — they simply serve different
users. The TUI does not shell out to the CLI, and the CLI is not a read-only
reporting layer bolted onto a TUI-owned database — both talk to the same
`store` package (§8), and a write from either is visible to the other within
one poll tick (§7).

The MCP server (`farol mcp`) is deprecated and being removed in favour of the
CLI. The CLI is the only agent surface that new features should target; the
MCP server stays wired only until it is deleted, and must not gain new
behaviour.

It is **not**:

- A sync client. No CalDAV, no Todoist, no Nextcloud Tasks. One local SQLite
  file is the entire backend. If sync matters later, it is a sync of that
  file (or an export from it), not a second source of truth — see
  `docs/ROADMAP.md`'s post-alpha list for where that idea is parked and why.
- A project-management tool. There IS an **assignee** and a **priority**
  field (§2, §3) — but as *coordination* primitives for multiple agents,
  not project-management creep: assignment answers "which agent owns this
  task right now", and priority answers "which task should an agent pick
  next". There is still no due date and no Gantt view, and neither is
  planned. §8 explains why the schema is shaped to add columns without a
  migration disaster, the way migration 0006 does for these two.
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
  assignee       text not null default ''
                 -- agent tag ("pi", "claude", …) holding this task;
                 -- '' = no assignment. NOT presence: AgentActivity (0002)
                 -- is a 120-second heartbeat; this column has no TTL and
                 -- changes on explicit assign/unassign/complete, or when
                 -- the holding MCP session ends (§3).
  assigned_at    integer          -- unix seconds; null unless assignee != ''
  priority       text not null default 'none'
                 -- 'none' | 'low' | 'medium' | 'high'. Stored, displayed,
                 -- and used by next_task's ordering (§9) — and by nothing
                 -- else: priority does NOT re-sort the tree, which stays
                 -- ordered by `position` alone.
                 -- Migration: 0006
```

Why ULIDs and not autoincrement integers: task and list ids are handed to the
CLI as arguments (`farol <task-id>`) and printed by `add`. A ULID
is a stable, copy-pasteable, sortable-by-creation-time string that never
collides across a `list add` and a concurrent `task add` from two processes —
an autoincrement id needs the database to hand it out, which is fine, but a
ULID lets `store.NewTaskID()` be generated before the transaction opens,
which matters for §7's transaction-shape rule. Ids are **not** meant to be
typed from memory; the CLI accepts an unambiguous *prefix* of an id
(§9, `resolveID`) so a human or an agent copying an 8-character prefix from
`farol tasks` output doesn't have to paste the full 26 characters.

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
  (`space` in the TUI, `farol <id>` on the CLI) even at 100%.
  If this surprises a future contributor enough to want to change it, that's
  a product decision to raise, not a bug to silently fix — it was chosen
  specifically to keep the one auto-promotion path (verified subtask
  completion) the only one, rather than accumulating several slightly
  different auto-complete triggers that a reader has to hold in their head.

**Completing cascades down; reopening does not.** Marking a task `complete`
(`space`, or `farol <id>`) sets every descendant, at every depth,
to `complete` too — a `complete` task with a `pending` grandchild is a state
this app does not allow to exist, because the two-list split (§6) would then
have to explain why a "done" tree still has visibly undone rows in it.
Reopening a task (`farol reopen <id>`, or `space` again on an already
complete task) does **not** cascade to children — it returns *only that task*
to `pending`. This is intentionally lossy: the task's prior `progress_kind`
and `progress_pct` are not restored, because tracking "what it was before
completion, one level deep" is a second piece of state for a rare path
(un-completing) that would otherwise need its own tests and its own edge
cases (what if it was `subtasks`-derived and a child changed while it sat
complete?). If this bites someone in practice, revisit it — but start from
`pending`, not from resurrected history.

**Agent activity is orthogonal to this machine.** A task or list can be claimed by an MCP agent (auto-claimed on every task write that leaves a task behind — status, progress, comment, add, edit and grab — but **not** `delete_task`, which would claim a row that no longer exists; the durable, explicit grab is `assign_task`, §9) without changing its `status` — the claim is a UI signal (a spinner in the TUI), not a state transition. Claiming a task does not move it from `pending` to `in_progress`; completing a task does not release an agent's claim. Status and progress writes by the same agent refresh (extend) its live claim's `acquired_at` — a write-heartbeat; they never create or release claims. **A grab is a task write, so `assign_task` and `next_task` auto-claim the task they hand you**, and the payload they return already reports `assignee_live: true`: without that, an agent would read its own just-grabbed task back as abandoned by the §3 rule below, and the TUI's stale tier would light up on work nobody has let go of. `assign_task(release=true)` is the opposite — letting go — and claims nothing. The `AgentActivity` table stores which agent is on which entity and when; it is read by the same 1s poll that reads lists and tasks (§7), but it does not interact with the status machine above. Claims expire after `WorkTTL` (120s) of inactivity; the MCP server also calls `store.ReleaseAgentClaims` when the MCP session ends (client disconnect), so the exiting agent's own spinners vanish immediately rather than waiting for TTL, while other agents' claims remain unaffected.

**Assignment is a third axis, orthogonal to both the status machine and presence.** `Task.assignee` (§2) has no TTL and no background sweeper — it changes only when someone explicitly assigns, unassigns or completes the task (§9 `assign_task` / `next_task`), **or when the session holding it ends**. It is not the same thing as the spinner above: presence says an agent is at the keyboard *right now*; assignment says who *owns* this work. **An assignment lives for the session that made it.** An MCP session's identity is unique to its process unless `FAROL_AGENT` pins it (§9), so a tag that will never return must not hold work forever: on shutdown the server releases its own claims and its own assignments, and removes the Inbox it auto-created if that Inbox is empty. Completing a task auto-unassigns it and every descendant the cascade completes — one less step for an agent to forget.

That leaves exactly one way for an assignment to outlive its owner: a session killed hard enough that it never runs its shutdown path. That case is what the reads and the TUI still describe — **`assignee != ''` and no live presence claim (`assignee_live: false`) means the work is abandoned**, and it is the only signal the stale-assignment tier needs. Nothing auto-releases it: reads report enough for a human to decide, and the human releases it from the TUI, per task or per list. Expect that tier to be rare rather than routine.

**The store owns every transition.** None of the above should be duplicated
in both `store` and `cli` (or `store` and `components`). `store.Complete`,
`store.Reopen`, `store.SetProgress` are the only three functions that write
`status`/`progress_kind`/`progress_pct`, and every caller — CLI subcommand or
TUI keypress handler — goes through them. There is exactly one write path per
kind of change; the invariant is enforced by Go visibility (these three are
the only exported mutators), which is what makes it hold without anyone
having to remember it.

## 4. Adding a task: the level rules

The inline create row — a card spliced into the tree itself, not a pinned
footer — adds a task relative to whatever is selected in the tree. Call the selected task's depth `L` (root-level tasks are `L = 0`).
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
  `L` is the sole authority and a resize never reverses a toggle — but `L` is
  not the only way to flip `listsPanelVisible` off. The Lists panel is a
  transient picker: committing a list with **`enter`** and cancelling with
  **`esc`** (see the esc precedence order below) both close it and move focus
  to Tasks exactly as `L` would, through the same `closeListsPanel` path
  (`src/model/Update.go`) — so a `enter`/`esc` close is indistinguishable from
  an `L` close afterward, including staying closed across a later resize.
  Selecting a list moves the highlight live as the cursor moves (no store
  write), so `enter`/`esc` never need to change *which* list is active —
  only whether the panel is in the way. When the
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
whole modal. It renders **no line-number gutter**: that is a code-editor
affordance, and these notes are a few lines of prose nobody cites by line, so
the column only spent width and made the field read as an editor pane. The
`┃` prompt is the field's left edge; the line the cursor is on lifts to
`BackgroundElevated` while Notes holds the keyboard (§12). A note taller than
the cap **opens on its first line**, with the cursor parked there — a reader
opening a note starts at its beginning, and typing lands where the cursor is
visible rather than off-screen at the end. The comment thread renders as selectable cards (the shared row-card
chrome, §12); `↑`/`↓` move the highlight and `y` copies the highlighted
comment's id to the system clipboard.

`tab`/`shift+tab` cycle **only through the targets currently visible** —
the lists panel is skipped entirely from the cycle while hidden
(`focusableZones()` in `src/model/Update.go` computes the cycle at runtime)
rather than being a focusable-but-inert stop. Do not implement "hidden but
still tab-able to an invisible panel"; that produces a focus ring with a
silent dead stop in it. The footer follows the same rule: with no side panel
open the cycle is a single zone, so the keybinding bar does not advertise
tab/shift+tab (a dead hint: nothing to cycle), and the hints return the
moment the lists panel opens (`GlobalsFor`/`Active` in `src/keys/Keys.go`
drop the pair on `ListsPanelVisible == false`).

`[`/`]` restructure the *selected* task — not just the create-mode level
selector (§4). Outdent `[` (move the selected task out from under its
parent, becoming the parent's next sibling) and indent `]` (move it under
its previous sibling, as that sibling's last child); both are no-ops at
their boundaries (a root task cannot be outdented; a first sibling has
nothing to hang under). Indent additionally obeys §3: a pending task never
moves under a complete sibling. While the inline create input is active the
same two keys are the create-mode level selector (§4) instead. **While the
inline create input is live, creating a task focuses only the text input:**
`tab`/`shift+tab` do NOT cycle focus to another panel and `?` types a literal
instead of opening help, so a half-typed title can never be stranded on
another panel mid-entry. `esc` is the way out (it cancels, or parks on an
empty list). Once the input is parked or closed, tab cycles the panels again
as usual. The same single-active-element rule holds visually: while the
create row is on screen, no task row draws the selected treatment (accent
bar, ModalBg, accent spinner) — the previously selected task keeps its
selection state but renders unselected, so the input is the only "selected"
thing on the panel. The highlight returns the moment the input closes. This
reversed an earlier design ("tab keeps cycling even while the
create row has the keyboard") — the cycle itself was harmless, but focus
leaving the input mid-entry was a constant footgun during rapid task entry,
and the draft-preservation it relied on was invisible state. The tree is the
startup focus and is broadcast as such at startup (phase-3 Init), so its keys
work from the first frame rather than only after a focus change.

**Vim and arrow bindings both work, always, on the task tree:** `↑`/`k` up,
`↓`/`j` down (moving the cursor across every *visible* row — a collapsed
node's children are not visible rows and are skipped), `←`/`h` collapses the
selected node if it has children and is expanded, else moves selection to its
parent; `→`/`l` expands the selected node if it has children and is
collapsed, else moves selection to its first child. This is the same
convention as `nnn`, `lf`, and most terminal file managers with a tree pane —
picked because it is already muscle memory for the audience, not invented
for this app.

**Collapse is deep; expand is shallow — two halves of one invariant, not
independently choosable.** Collapsing a node hides its *entire* subtree,
every depth, not just its direct children: every descendant's own collapse
state is set at the same time (`tasktree.collapseDeep`), walking forward
through `m.rows`' depth-first preorder while `Depth` stays greater than the
collapsed node's own. Expanding a node reveals *only* its direct children —
grandchildren stay collapsed, and have to be expanded themselves to appear.
**Collapsing resets, it does not remember**: a descendant's collapsed flag is
set unconditionally, never preserved from before, so a later shallow expand
of an ancestor can never reveal more than exactly one level — not "however
many levels happened to be open before that ancestor was collapsed." The
alternative (each node remembers its own expansion state across a collapse)
was considered and rejected: it would make expand's behavior depend on
history the user can't see — sometimes revealing one level, sometimes six —
which is precisely the inconsistency the shallow-expand rule exists to
remove. This decision is final; a future contributor proposing "restore
prior expansion on re-expand" is proposing the rejected alternative, not a
new idea. If the currently selected row is a descendant hidden by a
collapse, selection moves to the collapsed node itself — the nearest row
that stayed visible — rather than pointing at a row no longer on screen.

**`g`** jumps the cursor to the first row and **`G`** to the last,
**`pgup`** and **`pgdown`** move it one viewport height up/down (clamped to
the row bounds). `home`/`g` and `end`/`G` are both accepted, and `pgup`/
`pgdown` mirror the lists panel's `ListKeyMap()` so the two panels agree on
movement — `left`/`right` are deliberately **not** borrowed here, since the
tree reserves them for expand/collapse (§5).

**`alt+↑`/`alt+k` and `alt+↓`/`alt+j` move the selected task** up or down
*within its own status run* — the gesture never crosses the Pending/Complete
boundary, so a task cannot be moved out of the section it belongs in
without being crushed or un-crushed first (§3, §6). The modifier choice
follows vim's `alt+k`/`alt+j` convention (a plain `k`/`j` moves the cursor;
`alt` makes it move the *thing under* the cursor, the same handshape VS
Code's `alt+↑`/`alt+↓` uses for moving lines) — one key with a modifier,
not a second unmodified key that would steal a character vim users expect
to type.

**`u` releases the selected task's assignment, and `U` releases every
assignment in the active list.** Assignment has **no TTL and no background
sweeper** (§3): a session releases its own work on the way out, so these two
keys are the only thing in the app that frees a task whose agent went away
*without* getting to run its shutdown path — which is why the stale tier
(§12) marks such a task rather than the app quietly reclaiming it. The
release is **unconditional**: it clears an assignment held by any agent, since
a stale one is by definition held by someone who is not the person at the
keyboard, and a release that refused a foreign holder would leave abandoned
work stuck forever. `u` prompts for nothing — nothing is destroyed and
re-assigning restores it — while `U` goes through the same confirm modal every
other bulk action uses (§9), with the dialog counting the assignments it is
about to clear; it can free work several agents hold at once. The shifted form
takes the whole-list action, the same way `L` takes the panel-wide one over
the tree's own `l`. The tree only asks (`UnassignTaskMsg` / `ReleaseListMsg`);
AppModel calls `store.UnassignTask` / `store.UnassignList` and refreshes,
the same request/response split `space` and `d` use.

**`space` toggles complete/pending** on the selected task, from wherever the
tree has focus — it does not open anything and does not move the cursor.
**`enter`** on a selected tree row opens the Details modal — so it can't
also mean "toggle complete"; the two are deliberately different keys because
"open a thing" and "flip a checkbox" are different enough actions that
collapsing them into one key is what makes an app feel like a demo rather
than a tool. When one key serves two surfaces it is tempting to alias it to
both verbs, but here the verbs genuinely differ, so they are two different
bindings from the start rather than one alias split apart later.

Inside the Details modal: **`ctrl+s`** saves title, notes, progress and
priority changes,
closes the modal, returns focus to the task tree, and refreshes its rows;
**`tab`**/**`shift+tab`** cycle between the title editor, the notes editor, the
progress selector, the priority selector, and the comment thread (every zone is always in the cycle —
the comment thread is reachable even while empty, since `c` adds the first
comment from there); **`←`/`→`** (or `h`/`l`) cycle through the three progress
modes (`simple`/`subtasks`/`percentage`) while the progress selector is focused —
the modal shows those modes under plain-language labels ("in progress (flag)",
"from subtasks", "percentage"), never their stored names, which stay the CLI and
MCP vocabulary (§9); in `percentage` mode **digits** type the value directly and
**`↑`/`↓`** step it by 5, clamped to 0–100 (typed input reports an out-of-range
error instead of clamping — the user can see what they typed, unlike a held
arrow key), and both affordances are advertised in the modal's hint line only
while that mode is selected, since neither does anything in the other two;
**`←`/`→`** (or `h`/`l`) cycle the **priority** through the four values §2
locks while the priority selector is focused — in rank order
(`none` → `low` → `medium` → `high`), wrapping, so `→` always means "more
important" until it comes back round; it is one binding
(`keys.Details.CyclePriority`) carrying both directions, because a four-step
rank with no way back is a control the user has to loop three times to undo.
The zone shows `none` where the task row's badge shows nothing (§12): a field
being edited has to display the value it holds. The rank is written through
`store.SetPriority` on `ctrl+s` and **only when it changed** — that store
function rejects the zero value and bumps `updated_at`, so a save that touched
only the title must not write the priority at all;
**`↑`/`↓`** move the comment highlight, **`y`** copies the highlighted
comment's id to the system clipboard, and **`d`** deletes it — routed through
the same confirm modal every other destructive action uses (§9), with the
dialog quoting the comment's own text so the highlight can never be
mistaken — while the comment thread is focused; **`ctrl+y`** copies the open
task's id from any zone (a TUI has no reliable text
selection under mouse reporting, so the id is shown below the title and a key is
the real copy affordance); `esc` closes a clean modal immediately, and on a dirty
one shows the inline `Discard changes? (y/n)` prompt — `y` closes and discards,
`n` keeps editing. **`enter` is deliberately unbound at that prompt**: the
confirm modal can bind it because it has a visible yes/no selection for `enter`
to act on, and this prompt has none, so binding it would leave unsaved edits one
stray keystroke from gone. The help overlay lists the prompt's own `y`/`n` entry
for that reason, rather than letting the Overlays scope's "enter confirm" — true
of the modals that do have that selection — imply it works here too. No path
silently discards edits. The label of the focused zone
(Title/Notes/Progress/Comments) is bolded onto `TextPrimary`, the focused field
itself lifts to `BackgroundElevated` (§12), and every footer hint bolds its key
ahead of a muted description.

While the modal is open the **global footer renders nothing at all** — a blank
full-width bar, so the layout height does not move. The modal carries its own
hint line beside the controls it describes, and that line is the only one on
screen; it never spells its own wording, taking every hint from the binding in
`src/keys` so the modal and the help overlay cannot describe a key two ways.
A key is advertised only in the zone where it is actually live.

That line is always exactly one row. The modal's content column is fixed-width,
so a hint line too long to fit would wrap and grow the modal by a row; whole
hints shed from the tail instead, the same way the footer bar sheds (§5). Each
zone therefore lists its hints in priority order — the ways out of the zone
(`tab`, `esc`), then how to commit (`ctrl+s`), then the input methods for the
value the zone edits, then the extras — so what a narrow terminal drops is what
a user stuck in that zone would need last.

The task **title** is an editable single-line `textinput`, first in the tab
cycle; a save writes it through `store.RenameTask`, and a title cleared to
whitespace is refused in place (the store forbids an empty title) rather than
closing on a write that would fail.

Adding a comment is an **inline compose card**, opened with **`c`** from the
comment thread — the same "fake card while adding" shape the task tree uses for
inline task creation. The card renders at the foot of the thread styled like the
selected comment card, showing the OS author and a single-line `textinput`. That
input paints on the **card's** tier (`BackgroundElevated`), never the modal's —
it has no bare-modal row to sit on, and sealing it onto `ModalBg` cuts a
modal-coloured stripe through the card that reads as the card being broken. Both
sides take that tier from one place so they cannot drift apart.
**`enter`** posts it and **`esc`** cancels (a terminal cannot reliably
distinguish `ctrl+enter` from `enter`, so `enter` is the submit key — `ctrl+enter`
is accepted as an alias). Comments are short status/handoff notes, so one line is
sufficient. The compose draft is **transient**:
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
way. The filter is **live**: it re-narrows on every keystroke, the way `F`
does, so the query never sits typed-but-inert waiting on a commit. `enter`
still applies the query — it blurs the input and leaves the filtered view
active so the cursor can walk the results — and `esc` clears it. The two
states differ only in where the keyboard is, never in what is on screen.

**`F`** opens the cross-list search picker (phase 8): a text input searches
every list live, ranking title matches before notes-only hits, and showing
each result as `<list> › <task>`. `enter` on a result jumps to that task —
switching the active list when the match lives elsewhere — and `esc` cancels.

**`d` deletes the selected task** (or list, when the lists panel is focused),
prompting for confirmation first (docs/DESIGN.md §9: destructive TUI ops need
a confirm modal). The tree emits `DeleteTaskMsg`; AppModel opens a confirm modal;
accepting runs `store.DeleteTask` and refreshes the rows. List delete
(`L` panel, `d`) follows the same confirm-modal pattern.

**`q` quits, and `ctrl+c` always quits.** `q` is the ordinary exit and
`ctrl+c` the escape hatch that yields to nothing — so both are advertised as
"quit" rather than one of them as "force quit", which made the only documented
way out sound like an emergency. Because `q` is a printable character it is
handled *after* everything that could be typing one: a modal or the Details
modal owns the keyboard and swallows it; the inline create row and a `/` filter
being typed both take it as a literal `q`; and it quits only from the task tree
or the lists panel with none of those active. That is the same precedence
`keys.Active` already uses to decide what is live, so what the footer
advertises and what `q` does cannot drift apart. There is no "really quit?"
confirmation: every write is already on disk (§8), so leaving costs nothing.
`q` is deliberately left unbound on the lists panel's inner bubbles list
(`ListKeyMap()`), so the global handler is the only thing that answers to it.

**`?` opens the help overlay, and it lists EVERY key in the app on every
screen.** Not the keys live on the screen it was opened from — the whole
catalog, always the same scopes in the same order, built by `keys.Catalog` from
the same binding structs the handlers match against. Keys the user cannot press
right now are **dimmed** rather than omitted, and the overlay carries a legend
saying so, since a dimmed row otherwise reads as "removed" rather than "not
here".

The scopes came and went with the context once — `Lists` only while the lists
panel was visible, `Task Tree` only with an active list, `Creating`/`Filter`/
`Task Tree` as three mutually exclusive branches. That makes the overlay
useless for its actual job: a key you can only read about once you have already
found the surface it belongs to is not documented at all. The same failure, in
its worst form, is what shipped — `n` (new task) was bound, handled, advertised
in the footer and named by the empty state, but appeared nowhere in the overlay,
so help taught a reader how to create a *list* and not how to create a *task*.
`src/components/helpoverlay/coverage_test.go` reflects over every keymap struct
and fails if any binding is missing from the rendered overlay; that guard, not
review discipline, is what keeps this true.

Where a key does something its help text cannot carry, the scope gets a
one-line **`Note`** (`keys.Scope.Note`) — that is how `L` says it also moves
focus into the panel it reveals, and how the Overlays scope says the Details
discard prompt answers to `y`/`n` alone. Saying so in the section is the
alternative to omitting the key.

Listing the whole app is more than an 80x24 terminal holds, so the scope
content is **windowed and scrolls with `↑`/`↓`** (`keys.Overlay.Navigation`,
which every overlay already advertised for exactly this and now carries the
keystrokes to match), with the same `N above` / `N below` counts the task
tree's pinned section headers use (§12). The window height is *measured* from
the assembled chrome rather than counted from a constant, because several of
those pieces wrap at some widths and not others.

**Task renaming** in the TUI is done in the Details modal: its Title field is an
editable input (first in the tab cycle), saved with `ctrl+s` through
`store.RenameTask` — see the Details modal keys above.

**`esc` is the most overloaded key in the app** — six jobs (cancel an inline
create, clear the task-tree filter, clear the lists-panel filter, close a
modal, discard a dirty Details edit, close the Lists panel) resolved through
a strict "ladder of claims": each surface that might own esc is checked in a
fixed order, and the first one that claims it gets it. It is one switch case
(`keys.Global.Back`), checked in this order —
the order is the contract; checking it out of sequence silently breaks
whichever claim got skipped:

1. **A modal** (theme picker, confirm, list-name) closes itself first — modals
   intercept every keypress at the top of `Update`, so by the time esc
   reaches AppModel's own handling, no modal is open.
2. **The Details modal**, while visible, owns every keypress ahead of
   AppModel's normal Back case (it is tracked outside `activeModal`, for its
   poll-refresh-while-open behavior, so it sits just after the modal check).
   Its own handler takes esc: closing a clean modal, or raising the inline
   "Discard changes? (y/n)" prompt on a dirty one — once that prompt is up,
   only `y`/`n` resolve it (see the Details modal keys above).
3. **The focused panel's own `KeepsEsc` claim**: the task tree while typing
   or applying a `/` filter, or while inline-creating a task (§8); the lists
   panel while its own filter is open or applied — clearing the filter, not
   closing the panel. A user who just filtered the list must not lose the
   whole panel to their very next esc.
4. **Closing the Lists panel**, when it is focused and visible and did not
   already claim esc at step 3: `listsPanelVisible` goes false and focus
   returns to Tasks, through the same `closeListsPanel` path `enter` uses to
   commit a selection (§5 above) — the panel is a transient picker, and esc
   is its cancel. This must stay below step 3, or a filtered lists panel
   would close instead of clearing its query — the worse-bug-than-the-fix
   failure mode this ordering exists to prevent.
5. Otherwise, a no-op.

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
under `Complete` is, by invariant, 100% complete rows all the way down — the
section header is a true claim, not an approximation. A tree under `Pending`
can and will contain a mix: a `pending` parent can have `complete` children
sitting inline (checked, perhaps struck through) underneath it, still nested
in place. **Do not move a completed subtask out to the `Complete` section
while its parent is still pending** — that would separate a task from the
tree it belongs to for a reader trying to see the shape of remaining work,
which defeats the reason the tree exists at all.

A list with no tasks yet auto-shows the inline "new task" input as its only
row, under the `Pending` header — the input creates a pending task, so it
belongs to the section the task will land in — with one line of `TextDim`
guidance beneath it: **"type a title and press enter"**. The same rule holds
for a list whose tasks are all complete: the input opens under `Pending`
(even though that section is empty) rather than after the `Complete` section,
so it never sits at the bottom of the list while the task it creates lands at
the top. And a root-append draft (one with no anchor, i.e. appending at the
end of the pending section) renders its input at the end of `Pending`,
above the rule and `Complete`, for the same reason: the input always appears
in the section the new task will join.

**That is the empty list's only appearance.** `esc` *parks* the input rather
than closing it: the draft is discarded and the input is **blurred**, but the
row stays exactly where it is, so the surface renders identically before and
after esc. (An empty input draws its placeholder, not a cursor, so parked and
live are indistinguishable on screen.) `n` makes it live again.

Blurring is the part that matters beyond appearance: a live create input owns
the keyboard, and the input on an empty list can no longer be closed, so
without parking `q`, `L`, `t` and `/` would be dead forever on the first screen
a new user sees. `KeepsEsc`, `OwnsKeyboard` and `IsCreating` all key off the
*live* state, not off the row being rendered, which also keeps the footer from
advertising `enter create` for an input that would ignore it.

This replaced an earlier design in which esc swapped the input for a recessed
"No tasks yet. Press n to create one." card. One condition drew two different
screens, and the card explaining how to add a task appeared only *after* the
user dismissed the thing it was telling them to open. The guidance now sits
beside the input instead of in a card that replaces it. The `Complete`
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
existing `↑`/`↓`/`j`/`k` walk, plus `g`/`G` for first/last and `pgup`/`pgdown` for
one viewport-height jumps (§5) — there is no mouse wheel and no horizontal
scroll (both out of scope). The window is computed
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

There is no daemon, no socket, no file watcher — the TUI polls.

`tea.Tick` fires every `poll_interval_ms` (config default: **1000** — a local
SQLite read costs microseconds, so there is no reason to make a human wait
several seconds to see their own agent's last completion) and dispatches a
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
nearest surviving row — when the ground moves, keep the user on the closest
thing to where they were, never dump the cursor to a fixed position.

**Agent activity claims are read by the same poll.** Each `RefreshLists` and `RefreshTasks` call also runs `store.ListWork()` to fetch the current set of live agent claims (entities claimed within the `WorkTTL` window). The returned `[]AgentActivity` travels with the refresh messages so the task tree and lists panel can render a spinner on claimed rows. `RefreshLists` additionally computes the set of lists with any live task claim (`ClaimedTaskListIDs`) and carries it on the message, so the lists panel shows a spinner on a list row when an agent is working inside it — not only when the list itself is claimed. The same 1s poll tick governs both data and claims — no separate IPC or interval is needed.

**The very first load is animated, later polls are not.** `GetInitialModel`
does no database work: it constructs the components and returns immediately with
no active list, so Bubble Tea can paint the first frame before the opening
`RefreshLists` query completes. That first `RefreshListsMsg` also decides
*which* list is active: it reopens the list the user last had active, persisted
in the `Setting` table (§8) on every switch. The stored id wins when it still
exists; the first list is the fallback for a first run, an empty store, or a
stored id whose list was deleted in the meantime. The fallback choice is itself
persisted, so a relaunch never silently forgets where the user landed. Until
that first `RefreshListsMsg` arrives —
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
farol <task-id> in a shell `for` loop is never waiting behind the TUI, and
the TUI is never waiting behind it either. SQLite's WAL mode (§8) is what
makes concurrent readers and a writer not block each other; do not disable it.

## 8. Storage and concurrency

**modernc.org/sqlite** — pure Go, no CGO. This keeps the build story simple:
`CGO_ENABLED=0`, cross-compiled linux/darwin ×
amd64/arm64 by the same GoReleaser shape (`.goreleaser.yaml` carries the
exact config forward). A CGO-based SQLite driver would be the
more common choice by download count, but it would make this the one thing
in the whole toolchain that needs a C compiler to cross-compile, for no
capability this app uses that the pure-Go driver lacks.

**One file:** `$XDG_DATA_HOME/farol/farol.db` (falling back to
`~/.local/share/farol/farol.db`), opened in WAL journal mode. WAL is
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
no-op, not an error). One function decides the schema is current, called from
one place (`store.Open`), rather than each caller assuming someone else
already did it — "one resolution, passed down".

**Config** (`~/.config/farol/config.yaml`, or `$XDG_CONFIG_HOME`) holds
exactly two fields at launch, in a struct designed to grow — add a field,
tag it, and `LoadConfig`/`SaveConfig` round-trip it automatically:

```yaml
theme: farol-dark
poll_interval_ms: 1000
```

Both optional; a missing file or a missing field falls back to the compiled
default, and a malformed file is reported rather than silently ignored.
The `src/config` package doc comment carries the full contract.

**App state** (as opposed to user preferences) lives in a `Setting`
key/value table in the same SQLite file (migration `0007_settings.sql`):
one row per key, values TEXT, read and written only through `store`
`GetSetting`/`SetSetting` (an upsert, like every other store mutator). The
one key today is `last_list_id`, the list the TUI reopens at startup (§7).
It is written only when the active list actually changes, never by the
poll: the poll stays a pure read, per the concurrency rule above, and a
1s-tick UPSERT would turn the TUI into a writer on every tick for no user
benefit.

## 9. The CLI contract

Built on [Cobra](https://github.com/spf13/cobra). Every subcommand that reads
data accepts `--json`; every subcommand, reading or writing, reports failure
the same way regardless of `--json`. This uniformity is the whole point of
having a contract document at all — an agent that has read one subcommand's
`--help` should be able to predict the shape of every other one's output and
errors without reading the rest.

**Output shape, human mode (default):** a write command that succeeds prints
nothing but the one piece of information a script might want to capture
(`farol lists add` prints the new list's id and nothing else; `farol add` prints the new task's id and nothing else). A read command prints a
formatted table or tree to stdout. Any failure prints one line to stderr,
prefixed `farol: `, and the process exits non-zero.

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

**Destructive commands need `--force`.** `farol lists rm`, `farol rm`
(task), and `farol comment rm` refuse to run without `--force`. The TUI's
equivalent actions go through a confirm modal; the CLI has no modal to route
through and
no human to ask, so the flag *is* the confirmation. This is the one place
the CLI is deliberately less convenient than the TUI, on purpose: an agent's
typo in a task id should not have the blast radius of an unrecoverable
delete with no prompt at all.

**Comment deletion and its ownership rule.** Comments were append-only
(§2); `store.DeleteComment` hard-deletes by id, with no soft-delete or
tombstone. The CLI (`farol comment rm <comment-id> --force`) and the TUI
(the Details modal's comments zone) are unenforced like every other
human-facing delete — either may remove any comment. The MCP `comment`
tool is the one gated surface: its `delete=true` mode
refuses unless the comment's `author`
equals the calling session's identity, with the same error shape the list
ownership gate (`requireWritable`) uses — `comment <id> is owned by <author>
— you may only delete your own comments`. The `comment` tool also merges
the old `add_comment`; deletion alone stays gated by
`requireOwnComment`, and posting a comment is never blocked by assignment.
This is a narrower rule than list
ownership: it keys off the individual comment's author, not the list's
`created_by`, so an agent can always delete its own comment even on a list
it does not own.

**Full subcommand list**, grouped by the thing they act on. `<id>` accepts a
prefix (see above) throughout.

```
farol                                          launch the TUI
farol lists                                    list all lists
farol lists add <name>                         create a list; prints its id
farol lists rename <list-id> <name>            rename a list
farol lists rm <list-id> --force               delete a list and its tasks

farol tasks <list-id> [--status pending|in_progress|complete|all] [--flat]
                                                   list tasks (tree by default)
farol add <list-id> <title> [--parent <task-id>] [--notes <text>]
                                                   add a task; prints its id
farol show <task-id>                           title, notes, status, progress, children
farol rename <task-id> <title>                 rename a task
farol notes <task-id> <text>                   replace a task's notes (whole text, not append)
farol <task-id>                                mark complete (cascades to descendants)
farol reopen <task-id>                         mark pending (does not cascade)
farol toggle <task-id>                         complete <-> reopen, whichever applies
farol progress <task-id> --mode simple
farol progress <task-id> --mode percentage --percent <0-100>
farol progress <task-id> --mode subtasks
farol assign <task-id> [--force]               assign to the current agent; --force takes it from another
farol unassign <task-id>                       release the current agent's assignment on the task
farol unassign --list <list-id>                release the assignment on every task in the list
farol priority <task-id> --level none|low|medium|high
                                                   set a task's priority
farol mv <task-id> [--parent <task-id>]        re-parent a task; empty --parent moves it to the list root
farol rm <task-id> --force                     delete a task and its descendants
farol comment rm <comment-id> --force          delete a comment
farol search <query> [--list <list-id>]        fuzzy search across titles (+ notes)

farol mcp                                      run the MCP server on stdin/stdout

farol --version
```

> **Deprecated.** The MCP server is being removed in favour of the CLI
> (cli-first migration). The CLI is the only agent-facing front end that new
> work should target; this section is retained only until the server is
> deleted, and describes behaviour that will go away. New agent features
> belong on the CLI (the command reference below, the parity gap table in the
> migration plan, and the `--json` contract).

**`farol mcp`** runs a Model Context Protocol server over stdin/stdout. The
tools it exposes mirror the CLI subcommands and return the same JSON shapes
that `--json` would emit on the command line, so an agent host can call
`farol` operations as native tool calls instead of spawning the CLI per
operation. The server is a thin adapter over `src/store` in
`src/mcpserver`, not a layer on `src/cli`.

**The MCP tool surface is exactly twelve tools**, pinned by
`TestMCPToolSurface` in `src/mcpserver/server_test.go`: `my_list`,
`list_tasks`, `show_task`, `search_tasks`, `add_task`, `edit_task`,
`delete_task`, `set_status`, `assign_task`, `next_task`, `comment`,
`add_list`. This count is deliberate and is not a minimum — it trades tool
count against call count, and **call count wins** (§2 of the assignment
plan): `set_status` absorbs `complete_task`/`reopen_task`/`set_progress`,
`assign_task` replaces `claim_work`, `comment` merges
`add_comment`/`delete_comment`, and `list_tasks` absorbs `list_changes` via
its `since` parameter. Nothing is deleted that a caller still needs.

The task read rows (`list_tasks`, `show_task`, `search_tasks`) carry an
`assignee`, `assigned_at`, `assignee_live` and `priority` field on every
row, so a task's ownership and importance are visible without a second
round-trip; `assignee_live` is computed per call from `store.ListWork` and
is `false` for the stale-assignment tier (§3).

**MCP rows are a superset of the CLI's `--json` shapes** (CONTRIBUTING rule
6): `my_list` adds `position` and `created_by` to the `lists` rows it
returns (`mine` + `foreign_lists`). **The read-only resources no longer
mirror the tools.** Five resources that duplicated a tool row-for-row —
`farol:///lists`, `farol:///lists/{id}`, `farol:///lists/{id}/tasks`,
`farol:///tasks/{id}` and `farol:///search/{query}` — were deleted:
keeping them meant every
field added to a task had to be added in three places or the surfaces
drifted, and MCP hosts do not auto-read resources, so they cost maintenance
and bought nothing at runtime. Only `farol:///inbox` (a composed shape with
no tool equivalent) and `farol://work` (presence) remain, and
`TestMCPResourcesListed` pins that set at exactly two with zero templates.
A new field belongs on the tool. The server-side tests that pin the
MCP shapes live in `src/mcpserver/server_test.go`. The task read shapes —
`show_task`/`farol show`, `list_tasks`/`farol tasks`, `search_tasks`/`farol
search` — carry `list_owner` on every row (the parent list's `created_by`,
`""` for an unowned list), so an agent holding a task id knows at a glance
whether its list is writable without a separate `my_list` round-trip
(CONTRIBUTING rule 6: the CLI `--json` shapes gained the same additive
field).

**MCP agent-optimised extensions.** The task read shapes gained
`has_notes` and `notes_len` on every row: `has_notes` is `false` when the
notes body is empty, so an agent can skip a `show_task` call entirely.
`list_tasks(list_id, status?, since?, include?)` filters **per task** (not
per tree root): `status` defaults to `open` (= pending + in_progress) —
plus `pending`, `in_progress`, `complete`, `all` — and a row kept only as
ancestor context comes back with `context_only: true`. `since` (unix
seconds) absorbs the old `list_changes` tool, and passing it **widens the
default `status` to `all`** — `list_changes` had no status filter, so a
change feed that defaulted to `open` would be blind to a task being
completed, the most common change there is. An explicit `status` still
wins. `include` accepts `notes`,
`comments`, or both; inlined bodies are **never cut mid-text**: a byte
budget (`notesBudget`) caps the whole response, an over-budget row is
dropped whole, and its id goes into the `elided` array of the return
object `{"tasks": [...], "elided": [...], "budget_exceeded": false}`.
Only rows that actually carry a body are charged to the budget or named
in `elided`, since `elided` exists to be re-fetched with `show_task`.
The old `notes_truncated` flag is gone — mid-sentence truncation was the
bug being fixed. `show_task(ids)` is the batch equivalent for cross-list
workflows: up to 50 task ids return their **entire subtree** — every
descendant's full notes and comments, uncapped. Unresolvable entries are
returned as `{id, error}`. The `progress` field is
omitted on rows where the task has no progress, cutting typical row size by
~25%. `my_list` now returns `{mine: {id,name,pending,complete},
foreign_lists: [{id,name,pending,complete,created_by}]}`, merging `my_list`
+ `list_lists` into a single session-opening call. Status and progress
writes auto-claim the task under the writing agent's identity (best-effort,
non-stealing) — that is the presence heartbeat of §3, and it is distinct
from the durable `assign_task` that replaces `claim_work`. Every other task
write — `add_task`, `comment`, `edit_task` — auto-claims the touched task too;
`delete_task` does not (the task no longer exists), and `DeleteTask` clears
any claim rows on the deleted subtree so a removed task cannot keep a spinner
alive. The
`farol:///inbox` resource and `farol_inbox` prompt
deliver all of the above as a single read for start-of-session triage.

**`set_status` is the one status/progress write.** It takes 1–50 ids in
one call and accepts `status?` (`pending` | `in_progress` | `complete`),
`progress?`, `percent?`, an optional `comment`, and `force?`; at least one
of `status`/`progress`/`comment` is required. Per id, applied in this
order: assignment guard (a task held by another agent is refused unless
`force`) → reopen if needed → progress → status → comment. One call
replaces the old `set_progress` + `complete_task` + `reopen_task` and
fixes a documented gotcha: progress on a complete task no longer errors,
because the reopen happens first —
`set_status(ids, status='in_progress', progress='percentage', percent=10)`
just works. `status='complete'` cascades and auto-unassigns; `status='pending'`
reopens without cascading. Returns one
`{id, ok:true}` / `{id, error}` row per id in input order (a bad id does not
stop the rest — `batchApply`).
The rename/notes/re-parent/to-root structural edits stay in `edit_task`
(cross-list or ownership-gated, it stays single-task). Each touched
task is auto-claimed under the writing agent's identity, same as the
single-task writes.

**Priority is an agent-settable field.** `add_task(priority?)` defaults it to
`none`; `edit_task(priority?)` re-ranks an existing task. Both are gated as
structural edits — re-ranking is a steer about what should be picked up next,
so it is refused on a list the caller does not own, exactly like a rename.
An **omitted** `priority` on `edit_task` leaves the stored value alone: the
parameter's *presence* is what means "set it", never its emptiness, because
`store.SetPriority` rejects `""` and a rename must not silently clear a
`high` someone set. The
value is validated before either tool writes anything, so a rejected priority
never leaves a created task or a completed rename behind an error.

**Why the surface merges tools rather than adding them.** `set_status`
absorbed the separate status, progress and batch-update tools; `edit_task`
absorbed rename, set-notes and move; `comment` absorbed add-comment and
delete-comment. Each pair differed only in which field it wrote, and every
extra tool is a permanent cost — it sits in the agent's context on every
single turn, whether or not it is used, and it is one more name the agent has
to choose correctly. The count that actually matters is *calls*: an agent
touching fifty tasks should make one call, not fifty. The surface therefore
trades tool count against call count, and call count wins.

**`assign_task` is the durable grab.** `assign_task(ids, release?, force?)`
takes 1–50 ids and assigns each to the calling session's identity. An
explicit `agent_id` parameter is **rejected**, the same way `comment`'s
`author` is: an agent may only assign work to itself, and assigning work
*to* another agent is a human action taken from the TUI. `release: true`
unassigns, and succeeds silently when the caller did not hold the task.
Assignment reserves the **subtree**: it is refused when any ancestor or any
descendant is held by a different agent, so two agents cannot own a parent
and its child. A conflict without `force` is refused with an error naming
the holder and the age — `task 000037YRRJNE is assigned to "claude" (2h14m
ago, no live session)` — and `force: true` takes it, reassigns, and writes
a takeover comment recording who took what from whom. That is the
refuse-with-override rule of §3: a silent steal is worse than no steal. On
a successful assign the tool returns the **full `show_task` payload** for
each id rather than `{ok: true}` — grabbing a task and reading it are one
call, which is the whole point.

**`next_task` is the anti-race primitive.** `next_task(list_id)` atomically
selects the top eligible task, assigns it to the caller, and returns its
full `show_task` payload — the same shape `assign_task` returns. Eligible
means all of: `status != 'complete'`, `assignee = ''`, no ancestor assigned
to a different agent, and no descendant assigned to a different agent.
Ordering is **`priority` descending** (`high` > `medium` > `low` > `none`),
then depth-first preorder position — the same order `list_tasks` returns —
so preorder breaks ties between tasks of equal priority, and the tree the
TUI and CLI render is never itself re-sorted (§2). This is the *only* place
priority changes behaviour. Nothing eligible is **not an
error**: the tool returns `{ok: false, reason: "no eligible task in this
list"}`, because an empty board is a normal state, not a failure. Splitting
this into "read the list, then assign one" would be inherently racy across
two calls; as one atomic conditional update it cannot be raced at all —
which matters because the store file is shared across processes (TUI, CLI,
and every MCP session), where in-process serialisation buys nothing. With
`my_list` it makes session open two calls: what boards exist, then here is
your task and everything about it.

**Change-detection is folded into `list_tasks` via `since`.**
`list_tasks(list_id, since=<unix>)` returns only tasks whose `updated_at`
is strictly greater — newly
created, status/progress edited, renamed, re-noted, re-parented, or newly
commented (`AddComment` bumps `updated_at`, so new comments surface too).
Completions surface because `since` widens the default `status` to `all`
(above); pass `status` explicitly to narrow the feed.
The rows use the exact same shape as `list_tasks` (`has_notes`/`notes_len`,
omitted-empty `progress`), so `include=['notes']` inlines bodies identically.
Deletions are not representable by a row filter — a removed task is simply
absent; an agent that must detect deletions diffs id sets against its last
`list_tasks`. `updated_at` now means "last activity, including comments".
The standalone `list_changes` tool no longer exists — the `since` parameter
is now part of `list_tasks`, by the merge rule above: it returned the same
rows, filtered.

**List ownership, and what the MCP server refuses.** Every `List` carries a
`created_by` tag (§2). The MCP server resolves its own identity once at start
from the `FAROL_AGENT` environment variable; the *human* may set it per server
in the MCP client config to get a tag that is stable across runs. **When it is
unset the identity is generated per process** — `agent-` plus six random hex
digits — never a shared constant. A constant default was a real bug rather than
a convenience: identity is what every cross-agent guard compares on, so two
unconfigured clients acted as one agent and overwrote each other's work with no
refusal, no `force` and no takeover comment. Either way one stdio session is one
identity, and no tool lets an agent change it. A generated tag is only coherent
because assignments do not outlive their session (§3), and it necessarily
satisfies the `created_by` pattern below, since the server writes it there
itself. A list is writable by that
session when `created_by` equals the identity, **or** when the list's
`collaborative` flag is set (below). The structural tools — `add_task`, `edit_task`, `delete_task` — resolve
their id first (the §9 prefix rule still applies), then refuse a foreign list
with one error shape: `list <id> is owned by <owner> — you may read it and
update task status/progress only`. `edit_task`'s re-parent checks the list it
would move *into* — the parent's list when a `parent` is given, the task's own
list when `to_root` is set — before any write, so a refused move never
half-happens.
(`store.Reparent` independently rejects a parent on a different list, so on a
move that would otherwise succeed the two are the same list; the owner check
runs first and reports ownership rather than the cross-list error.)

**Status and progress writes are never list-gated.** `set_status` works on
every list, as do all reads, the surviving resources, and the prompts.
What is gated is *assignment*: `assign_task`, and any `set_status`/`edit_task`/
`delete_task` on a task whose `assignee` is another agent, are refused
unless `force=true` — the forced write performs the change, reassigns, and
records a takeover comment — the refuse-with-override rule (§3). An **empty** `created_by`
means owned by nobody, which makes the list foreign to *every* identity: an
untagged list — the shape `farol lists add` and the TUI create — is read +
status/progress only for all agents, and only a human can restructure it.
`add_list` defaults `created_by` to the session identity and accepts an
explicit tag matching `^[A-Za-z0-9_-]{1,32}$`; `my_list` reports `created_by`
on every row. Ownership is adopted from the `<tag>: <name>`
naming convention in two places: a rename into a
tag adopts the owner **in the same write** (`store.RenameList` — the human's
`farol lists rename Groceries "pi: Groceries"` handoff path takes effect
immediately), and `farol lists add --owner <tag>` provisions an owned list
from the start. One idempotent backfill pass at `store.Open` catches
anything that predates both. The adoption cannot tell intent: any `^tag:`
prefix is adopted, so a human list named `Note: buy milk` becomes owned by
tag `Note` — an accepted false-positive class. The
inverse is deliberately *not* done: `GetOrCreateAgentList` and
`my_list` match `created_by` only, never the name, so an untagged
`pi: ...` list is never silently adopted.

**`collaborative` is an explicit, list-level, opt-in override of the owner
check above** — not "untagged means collaborative" (that would silently open
every existing human list at once), and not per-task (list-level is the unit
the ownership model already uses everywhere; a per-task flag would multiply
the states an agent has to reason about). It defaults to `false` for every
existing and new list, human-set only — the same shape `comments_disabled`
uses (`src/store/migrations/0005_list_collaborative.sql`,
`store.SetCollaborative`), and, like ownership itself, unenforced on the
CLI/TUI side: a human may always restructure their own list regardless of
this flag. A human sets it from the TUI's list-rename modal (`R` in the
Lists panel); the Lists panel marks a collaborative row so it reads
differently from one an agent cannot restructure (§12). `my_list`'s
`foreign_lists` carries `collaborative` next to `created_by`, so an agent can
tell before it tries rather than discovering it from a refusal.

Enforcement lives in `src/mcpserver` alone (the `requireWritable` helper) —
the store stays a dumb data layer and the CLI and TUI stay unenforced, which
is the deliberate front-end divergence CONTRIBUTING rule 5 asks to be written
down. Identity is self-declared and unauthenticated: this is cooperative
trust between agents, not a security boundary. When comments arrive they join
the owner-only bucket behind the same `requireWritable` helper.

**Output shapes, pinned.** The subcommand list above fixes *which* commands
and flags exist; this fixes *what each prints*. The shapes below were
settled in phase 2 and are part of the contract,
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
  `[{"id", "parent_id", "title", "status", "progress", "depth",
  "assignee", "priority"}]` —
  `--flat` changes only the human rendering. Depth starts at 0 for a root;
  `parent_id` + `depth` let a caller reassemble the tree; `assignee` is `""`
  when unassigned; `priority` is one of `none`/`low`/`medium`/`high`.
- **`show`, human mode:** labeled lines (`Title:`, `ID:`, `List:`, `Status:`,
  `Progress:`, `Notes:` with each line indented two spaces, then
  `Children (N):` and the §12 tree when there are any). The `Progress:` line
  spells out a subtasks task with no children as `subtasks (simple)` rather
  than a misleading `(0%)` (§3). **`show`, JSON:** the task's fields
  (`id`, `list_id`, `title`, `notes`, `status`, `created_at`, `updated_at`,
  `completed_at` as unix seconds, plus `assignee`, `assigned_at` as unix
  seconds or null, and `priority`), its `progress`, and `children` as the
  same row array `tasks` emits, depth relative to the shown task.
- **`assign` / `unassign` / `priority` JSON:** each echoes the field it
  wrote, so a caller never needs a follow-up `show` to confirm what landed.
  `assign`: `{"ok": true, "assignee": "pi"}` — the tag the task now belongs
  to, which is the caller's own identity whether or not `--force` was needed.
  `unassign <task-id>`: `{"ok": true, "assignee": ""}`, the same shape with
  the field cleared. `unassign --list <list-id>` acts on a list and so has no
  single task to report: `{"ok": true, "released": <n>}`, the number of tasks
  whose assignment was cleared (`store.UnassignList`'s count), which is `0`
  when the list held none — not an error. `priority`:
  `{"ok": true, "priority": "high"}`. Failures use the §9 error shape above
  (`{"error": …}` and exit 1) like every other command, including a takeover
  refused for want of `--force`.
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
  `[{"id", "list_id", "list_name", "title", "status", "progress",
  "assignee", "priority"}]`.
- **Empty results, human mode:** a read command whose result is empty prints
  nothing (exit 0); JSON mode prints `[]`. A caller that needs to
distinguish "no data" from "failed" reads the exit code, never the bytes.
- **Human output is plain text — no ANSI escapes** — so a script can capture
  any read command's stdout without stripping styling.

`farol mv <task-id> [--parent <task-id>]` (re-parent a task) is the one
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

Two halves: the Bubble Tea half (`model`, `components`, `cmds`, `keys`,
`appstyles`, `constants`) and the non-Bubble-Tea half (`store`, `apptypes`,
`config`), with one addition (`cli`) for the agent-facing front end:

```
main.go              # cobra root: no subcommand -> launch TUI; else dispatch
src/
├── model/           # AppModel: Init/Update/View, the top-level Bubble Tea model
├── components/      # one package per leaf model (tasktree, taskspanel,
│                     # listspanel, detailspanel, themepickermodal, searchpicker,
│                     # listnamemodal, confirmmodal, helpoverlay, keybindingbar,
│                     # mainmenu)
│   └── chrome/       # shared rendering: PanelFrame, tree-row rendering, the
│                     # progress pill, KeyHints, Spinner — a helper earns its
│                     # way in here by having a second caller (§12, chrome contract)
├── cmds/            # message types and the tea.Cmds that produce them
├── apptypes/        # List, Task, Status, ProgressKind — the shapes components pass around
├── keys/            # the one keymap package: every key declared exactly once,
│                     # footer and help overlay render from these bindings
├── store/           # SQLite schema, migrations, and every read/write function —
│                     # the only package that imports database/sql
├── cli/             # one file per subcommand group; each is a thin adapter from
│                     # cobra flags to a store call and a --json-aware printer
├── appstyles/       # Theme type + the 14-theme registry (§11)
├── config/          # ~/.config/farol/config.yaml
└── constants/       # layout widths, focusable-zone ids, branding
```

**`src/store` is the only package that imports `database/sql` or
`modernc.org/sqlite`.** `src/model` (the TUI) and `src/cli` (the CLI) both
depend on `store` and nothing deeper; neither ever builds a SQL string.
**`src/cli` and `src/model` are siblings over the same `store`, not layered on
each other**, which is the structural expression of "neither front end is
secondary" from §1. `main.go` is the one file that imports the CLI and TUI
to decide which to run.

The MCP server (`src/mcpserver`, reached via `farol mcp`) is deprecated and is
being removed; see the cli-first migration plan. Do not add new code to it.

## 11. Theming

The `Theme` struct, `newTheme`'s tier-derivation math, the `InkOn` contrast
helper, and the picker's live-preview-on-cursor-move mechanic live in
`src/appstyles` and are documented there and below. Do not redesign the
derivation math — it is already tuned across all 14 registered palettes.
The reasoning behind `Lighten`/`Darken` and why `Modal` needs to clear
`BackgroundElevated` by a minimum margin:

**Color lives on a Theme.** Every color the app draws with is a field on
`appstyles.Theme`, not a hex value scattered through a component.
`appstyles.Active` is the one `Theme` in effect; every call site reads it
fresh — `appstyles.Active.TextPrimary`, say — rather than caching a color at
package init, which is what lets a later switch actually repaint: assign a
different registered `Theme` to `Active` and the next frame draws it. Styles
composed of more than one field are functions (e.g. `appstyles.NormalTitle`)
for the same reason, not package-level `var`s: a `var` built at init freezes
whichever theme was active when the package loaded.

`appstyles.Themes` is the registry, built by `appstyles.newTheme` from a
handful of base colors — `Accent`, the text/panel/modal bases, the four
status colors — with everything else derived by `Lighten`/`Darken`. A dark
theme raises a tier's attention by lightening it, a light theme by darkening
it. Adding a theme is choosing those base colors, not hand-tuning thirty
derived ones.

**The asymmetry that drives every imported palette:** tier derivation is
`lipgloss.Lighten` for dark themes and `lipgloss.Darken` for light ones.
`Lighten` is additive — it adds `255 × percent` to each channel — while
`Darken` is multiplicative — it scales each channel by `1 − percent`. For a
dark theme the raise is a fixed climb independent of the base; for a light
theme the step shrinks as the base approaches white. The consequence for
imported colour schemes: **set `Panel` to that scheme's deepest background
tier**, so the raised tiers (`BackgroundContent` +4%, `BackgroundPanel` +8%,
`BackgroundElevated` +12%) land back inside the scheme's own background
range. `ModalBg` is not a raise of the panel ladder at all — it must clear
`BackgroundElevated` by ≥14 per channel (guarded by
`TestElevationSeparation` in `src/appstyles/Contrast_test.go`) or the modal
disappears into the focused panel.

`InkOnLight`/`InkOnDark` are the one deliberate exception to "derived from
base colors": they do not vary with a theme's `Dark` flag, because a status
pill's own fill does not vary with the app's theme either — the text that
reads legibly on a given fill has to stay legible whichever theme is active,
not follow `TextPrimary`, which flips. The `appstyles.InkOn(fill)` helper
picks whichever of the two fixed inks has better contrast on the fill at
hand, and `Contrast_test.go` verifies the result clears its threshold on
every status pill, the accent chip, and the error banner for every
registered theme.

**Elevation ladder (the `raise` coefficients in `newTheme`).** The focused/
unfocused panel step is the focus signal (see §12 "Focus is shown by lifting a
tier"), but the step's *contrast ratio* is inherently small for every theme:
`BackgroundContent = raise(Panel, 0.04)`, `BackgroundPanel = raise(Panel,
0.08)`, `BackgroundElevated = raise(Panel, 0.12)`, `BackgroundRecessed =
Panel` (un-raised base). Both elevated and panel derive from the same base by
`Lighten` (dark) / `Darken` (light), so for a near-black base (dark themes)
the additive step near black is tiny, and for a near-white base (farol-day,
the lone light theme) a larger coefficient *darkens* elevated toward the
*lighter* panel, shrinking the ratio. Measured: the elevated-vs-panel step is
~1.10-1.17 for every theme and is capped at ~1.2 for farol-day under the
additive ladder — a geometric ladder was prototyped and hit the same cap, so
the base palette, not the step function, is the binding variable. The nominal
1.35 target for this step is therefore unreachable without moving the base
colors and/or relaxing the `TextPrimary on elevated ≥ 4.5` ceiling, both out
of scope for the focus bug. The genuinely perceptible, theme-independent focus
signal is the **selected-row** contrast (`ModalBg` for the focused panel's
active row vs `BackgroundElevated` for an unfocused panel's remembered row),
which `chrome.ListRowBg` produces and which measures ~9.5:1 on farol-day —
that is the fix that makes focus obvious, not the panel-surface step.

**Text colors that sit on the elevated tier.** Three imported palettes ship a
body `Text` too dim for a brightened elevated tier — `one-dark` (`#ABB2BF`),
`solarized-dark` (`#93A1A1`), and `everforest-dark` (`#D3C6AA`) — and were the
only themes failing `TextPrimary on elevated ≥ 4.5` once the ladder was
widened. Their `Text` is lightened to `#C6CDD7`, `#BCC4CF`, and `#E4D9C0`
respectively (chosen as the dimmest grey that clears 4.5 on elevated *and* on
panel). This changes those three themes' body-text luminance slightly; every
other theme's `Text` is untouched.

**What changes:** the four status-color fields are domain colors, renamed to
match this app's domain:

| Field | Meaning |
| --- | --- |
| `StatusComplete` | done |
| `StatusPending` | not started |
| `StatusInProgress` | active |
| `StatusOverdue` | *(reserved; unused until a due-date feature exists — see `docs/ROADMAP.md`)* |

The registry holds 14 themes: four of this app's own — `farol-dark`,
`farol-ember`, `farol-slate`, `farol-day` — plus ten imported community
palettes (catppuccin-mocha, gruvbox-dark, tokyo-night, nord, dracula,
solarized-dark, one-dark, everforest-dark, rose-pine, kanagawa-wave). The
imported palettes carry their original accent, text/panel/modal bases and
status hexes unchanged: a person who knows "Tokyo Night" should see it
render the same way here, because it is the same theme, not a
reinterpretation of one. **The fresh-install default is
`"farol-dark"`** — `DefaultTheme` names it, and a config with no
`theme:` preference activates it; every other registered theme (including
`gruvbox-dark`) stays selectable through the `T` picker and as a saved
`theme:` value. The default is the brand pair's dark member below, so a
fresh install opens in the logo's own colors.

**`farol-dark` and `farol-day` are the brand pair** (2026-08-10): both are
built from the logo family's three colors — the Night Watch icon's navy
(`#0E1B30` deep / `#22385C` mid), the lamp's amber `#F0B263`, and the
wordmark's cream `#F5EDE4`. The dark theme is the icon itself: deep-navy
surfaces, amber accent, cream body text. The light theme is the lockup
reversed — cream surfaces, navy ink, a darkened lamp-amber accent
(`#A06A0E`, the darkest amber that still reads as the lamp against cream).
Both keep the shared status/danger hexes (darkened for the light theme,
per the washout rule above), and pending carries a navy tint in both,
mirroring how `farol-ember`'s pending carries a warm one. They replaced the
palettes previously copied from stack-stitcher's `stitcher-dark` /
`stitcher-day`.

## 12. Visual coherence: the UI contract

This section exists because "pick something reasonable and be consistent"
is not a instruction that survives being executed by several different,
disconnected contributors across nine phases. Every visual detail below is
**decided, not suggested** — a fixed number, a fixed glyph, a fixed rule —
specifically so phase 4's task tree and phase 6's lists panel, built weeks
apart with no memory of each other, render as one app rather than two
apps sharing a color scheme.

**If a UI element needs a visual detail this section doesn't specify — a
glyph, a spacing value, a color-tier choice — add it here, in this section,
in the same commit that introduces the element.** Do not decide it locally
inside a component and move on; a decision made only in one component's code
is a decision the next component's author cannot find.

**→ For the hardened, actionable verification checklist,** read
[`docs/UI_INSTRUCTIONS.md`](UI_INSTRUCTIONS.md). Before marking a component
complete, run the verification script: `scripts/verify-ui-component.sh <component-path>`.

### Background tiers, and sealing them

Sections are separated by background color rather than by borders. The tiers
are `Theme` fields, read through `appstyles.Active`:

| Tier | Field | Where, in this app |
| --- | --- | --- |
| 1 | terminal default | outside the app — never drawn on |
| 2 | `BackgroundContent` | the outermost frame, if one exists (gutter between the lists panel and the main panel) |
| 3 | `BackgroundPanel` | the Lists and Tasks surfaces, when unfocused (`raise(Panel, 0.08)`) |
| 4 | `BackgroundElevated` | Lists when it has focus, or Tasks while its task-tree or add-input control has focus (§5), and the highlighted comment card in the Details modal (`raise(Panel, 0.12)`) |
| — | `ModalBg` | every modal (theme picker, confirm, list-name, **and the Details modal**) **and the row the cursor sits on in the task tree** — an active row is its own register, not a tint of the panel it's in |
| — | `BackgroundRecessed` | empty-state cards (§Empty states, below) — equal to `PanelBg`, the un-raised base |

**Every tier must be sealed.** A terminal's SGR reset clears the background
until the next SGR, and lipgloss closes each styled run with a reset — so
any unstyled text later on the same line renders on the terminal's own
own color shows through. Anything that draws text — a tree row, the inline
create input, a list row, a modal's body — needs an explicit background, and
`lipgloss.JoinVertical`/`JoinHorizontal` pad shorter siblings with bare,
unstyled spaces that must themselves carry a background or the terminal's
own color shows through. Wrapping the result in a `Background()` style does
not help, because a style only paints the padding it adds itself. Two rules
follow:

1. **Anything that draws text needs an explicit background**, including
   buttons, cards and list rows. A run with no background set is the notch.
   Components that sit inside a panel take that panel's tier as a parameter
   instead of picking a tint of their own, so they stay flush when focus
   lifts the panel.
2. **Seal innermost-first.** A tree row seals itself,
then the panel it sits in seals what's left, then (if a tier-2 frame exists)
the outermost pass seals last. Sealing only at the outer tier would flatten
the inner ones — the active row's distinct surface would be repainted
to the panel color. `appstyles.FillBackground` is the sealing function.

`appstyles.HasBackgroundBleed` is the matching assertion — tests apply it to
fully rendered frames and component bodies (see
`src/model/layout_test.go`, `src/components/tasktree/scroll_test.go`,
`src/components/detailspanel/Model_test.go` and the per-theme checks in
`src/appstyles/Background_test.go`). This is not
optional polish, it's the mechanical check that catches a missing
`Background()` call before it ships.

### Foreground tiers: never draw in the terminal's default color

Background sealing alone is not enough: a glyph with no foreground SGR in
effect draws in whatever the user's terminal calls "normal text", and nearly
every terminal default is light. On the dark themes that reads fine; on a
light theme it vanishes — `farol-day` made the bug visible, with pending
task titles rendering white on warm off-white. The rule is the foreground
analogue of the sealing rule:

1. **Every glyph draws from an `appstyles.Active` tier.** A row title, an
   expand/collapse marker, an empty-state message, a footer hint — all of it
   carries an explicit `Foreground()`. A styled run ends in a reset, so an
   unstyled glyph appended after a styled one falls back to the default just
   as badly (the row's `▾`/`▸` marker is styled with the row's own title
   tier for exactly this reason).
2. **Widgets from Bubbles are not theme-aware and must be sealed.** A raw
   `textinput.New()` carries no foreground on its focused text and a
   hardcoded ANSI white on its blurred text and default `> ` prompt, and
   `list.New()` ships dark-assumed styles for its filter bar. Every input is
   therefore sealed every render, not once at construction, so a theme
   switch cannot leave a stale palette on it: `chrome.SealInput` for the
   standalone text inputs (the task tree's `/` filter and inline create,
   the search picker, the list-name modal, the Details modal's title and
   compose fields), `chrome.SealListFilter` for a bubbles list's built-in
   filter bar (the lists panel). Each takes the surface the input sits on,
   the way a panel passes its tier down; inputs that clear the default
   prompt (the task tree's filter bar is `/` + query + suffix, nothing
   else) do so at construction.

`appstyles.HasDefaultForeground` is the matching assertion — tests apply it
to fully rendered frames under both the light and the default theme and to
individual rows (`src/model/foreground_test.go`,
`src/components/tasktree/foreground_test.go`, with the SGR state machine
itself pinned in `src/appstyles/Foreground_test.go`). It cannot repaint a
frame the way `FillBackground` can — a missing foreground has to be fixed at
the source — so it exists purely as the mechanical check that catches a
dropped `Foreground()` call before it ships.

### Modal scrim

Every modal composites over the rest of the screen (`AppModel.overlayModal`,
used for the Details panel and every `activeModal`: confirm, help, theme
picker, search picker). Before laying the modal on top, `overlayModal` runs
the page underneath through `chrome.Scrim`, which dims the whole page to one
flat `TextDim`-on-`BackgroundContent` tier. Without it, a modal only paints
the columns its own box occupies — any page content in the cells the box
doesn't cover keeps its original styling, and right-aligned row content (a
status pill's tail, an empty-state card's border) ends up reading as
distinct fragments stuck to the modal's edge.

`chrome.Scrim` cannot use a translucent overlay layer: lipgloss's compositor
has no alpha blending, a layer always draws opaquely. It also cannot get
away with wrapping the page in an outer `Background()` style, the way the
outermost frame seal above does — that only paints cells the page left
unstyled, leaving every glyph's own embedded color (a status pill's fill, a
row's `ModalBg`) untouched underneath. Instead `chrome.Scrim` strips every
SGR the page carries (`ansi.Strip`) and re-renders the resulting plain text
in the one muted tier, which recolors every cell uniformly because nothing
embedded survives to compete with it. Every modal gets this through
`overlayModal`; no modal implements its own dimming.

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

The same lift marks the spot the keyboard is acting on *inside* a surface,
not just the surface itself: the notes textarea's cursor line and the Details
modal's percentage value both sit on `BackgroundElevated` while their zone has
focus. A value the user can edit therefore looks lit and reads in
`TextPrimary`, where a value they cannot — a `subtasks`-derived percentage, or
`(no children)` — stays a muted parenthetical annotation. That contrast is the
whole signal that a field takes input; there is no caret glyph, and a component
must not invent one.

### Two shared frames: `chrome.PanelFrame`

`chrome.PanelFrame` owns the body frames: **Lists** and **Tasks**. It renders
those exact labels through `appstyles.NormalTitle()` as an accent chip with a
two-column left gutter, then one blank chrome row before the body. (The Details
surface is no longer one of these — it is a modal, wrapped in
`chrome.ModalSurface`, sized to most of the screen and layered over the body;
see §5.) The frame has **1 row vertical and 2 columns horizontal** padding
(`lipgloss.NewStyle().Padding(1, 2)`).
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
3, not phase 9 — every component that
renders user-supplied text (a task title, a list name, a note preview) calls
it from the moment that component exists, so there is never a window where
different components truncate differently because "polish comes later."
Rule: cut to `width - 1` display cells and append a single `…`, never mid-
escape-sequence, using an `ansi`-aware width measurement (a plain byte-slice
truncate can split a multi-byte rune or an ANSI sequence, corrupting the rest
of the line). **Never truncate a unit to a fragment** — if
a row genuinely has no room for any of a title, show nothing of it (clip the
row) rather than one or two letters followed by `…`.

What phase 9 still owns: *shedding whole optional elements* (a trailing
percentage, a key-hint) under extreme narrowness, which is a different,
coarser mechanism layered on top of truncation, not a replacement for having
truncation from the start.

### The task row's width budget: reserve the fixed things first

`tasktree.computeTaskRowCols` is where a row decides what fits. The order it
works in is the whole point: **the fixed-width cells are reserved first and
the title takes the remainder**, never the reverse. Sizing the title first and
letting the status group have what is left is what produced
`◻ Reach the fe…45%  IN PROGRESS` — an ellipsised title welded to a
percentage, on a row whose right edge landed three columns past every other
row's.

Two rules follow from that ordering:

- **A gutter of one column** (`titleGutter`) is reserved at the end of the
  title cell, so an ellipsised title can never touch the cell after it. It is
  reserved *inside* the title cell rather than added as a column of its own,
  which is what keeps the status and icon columns at the fixed offsets every
  row aligns on. The status label is right-aligned in its fixed column, so
  labels of different lengths start at different columns and **end** at the
  same one; the icon column after it is what makes every row's right edge
  identical.
- **A title floor of 12 columns** (`titleFloor`). When the reserved cells
  would squeeze the title below it, the passengers shed whole — never as
  fragments — in this order: the **status+icon block and the priority badge
  together first**, then the **assignee badge**, then the agent-spinner
  unit, and the **percentage last**, which sheds only to stop
  the row overflowing. Priority sheds with the status, not after the
  assignee: since the priority badge renders immediately left of the status
  label they read as one state group, and a narrow row keeping the badge
  while dropping the label would read as a floating `● HIGH` with no state
  next to it. At 40 columns the question "who is actively working" and "who
  owns it" still outlive "what should I pick up next", and the assignee is
  still readable in the Details modal and over every CLI and MCP read (§9).
  That order is deliberate and is the reverse of what the
  row layout originally specified: the first version budgeted for overflow
  alone, with no notion of a floor, so a narrow
  row spent eleven columns on `IN PROGRESS` while the title shrank to a stub.
  Dropping the label costs the user nothing — the row still carries its status
  in the `◻`/`◼` glyph, in its foreground colour, and in the Pending/Complete
  section it sits in — whereas the percentage appears nowhere else on the row.
  Those two plan files are superseded on this point; this section is the rule.

The indent, collapse marker and checkbox are the row's identity and are never
shed, and the title is never dropped below one column. `tasktree`'s width-sweep
tests assert, across a sweep of panel widths, that a row never overflows, that
every unit is shed whole or not at all, that an ellipsised title always has a
blank cell after it, and that every row ends its status label at the same
column.

### Below the minimum size: one line, not a broken layout

The smallest terminal the app supports is **40 columns by 10 rows**
(`constants.MIN_TERMINAL_WIDTH` / `MIN_TERMINAL_HEIGHT`). 40 columns is where a
task row still seats a checkbox, a title at its `titleFloor`, and a status
label; 10 rows is a header, a footer, a section header, and enough body left to
be worth drawing.

Below **either** dimension the app stops attempting the layout and renders a
single line — **`Terminal too small`** — centred on both axes on the frame's
tier-2 background. Nothing else renders: not the header, not the footer, not an
open modal. The decision lives in exactly one predicate
(`AppModel.terminalTooSmall`), which `View` consults before it composes
anything, so no surface gets to decide this for itself. The predicate answers
`false` until the first `WindowSizeMsg` has arrived, so the pre-layout frame —
width and height still 0 — is not mistaken for a tiny terminal. It is not
sticky: growing the terminal back over the threshold restores the real layout
on the next resize.

### The glyph vocabulary

One table. A component does not invent a symbol not listed here; if a new
one is needed, it's added here first.

| Meaning | Glyph | Notes |
| --- | --- | --- |
| Task: pending | `◻` | Text-presentation square — the checkbox character Claude Code's todo lists use (`figures.squareSmall`, verified from its source, 2026-08-03). Single display cell, unlike the emoji ⬜ (2 cells). |
| Task: in progress | `◻` | The same text-presentation square as pending — no dedicated glyph; the `IN PROGRESS` label and bar colour set the row apart. Used for all three progress kinds (§3) alike — the trailing percentage (below), not the checkbox, is what distinguishes them. |
| Task: farol | `◼` | Filled square (`figures.squareSmallFilled`), tinted `StatusComplete`; title renders in `TextMuted`, not `TextPrimary`, once farol — see Typography below. |
| Node has children, expanded | `▾` | One column wide, appended to the *end* of the title (see Row layout below). |
| Node has children, collapsed | `▸` | Same column, same position — the marker never occupies a leading column, so a parent's title starts at its own depth. |
| Node is a leaf | *(no glyph)* | Nothing appended; the title simply has no trailing marker. |
| Task has detail text | `🗎` | U+1F5CE DOCUMENT, left half of the fixed two-cell trailing icon column, immediately right of the status column, in `TextMuted`; the column is reserved on every row and the notes cell is rendered blank when `Notes` is empty, so noted and un-noted rows keep the same right edge. The column is two cells because it pairs with the comments glyph (below). Measures one cell in go-runewidth, but it is an emoji codepoint: emoji-capable terminal fonts may render it two cells or tofu — accepted tradeoff, the `✎`/`ⓘ` alternatives were rejected in favour of the literal "document" reading (2026-08-03). |
| Task has comments | `🗨` | U+1F5E8 LEFT SPEECH BUBBLE, right half of the fixed two-cell trailing icon column, in `TextMuted`; the cell is blank when the task has no comments. `💬` (U+1F4AC) was the natural choice but measures two cells in go-runewidth (v0.0.23), which would have widened the column past its partner glyph — `🗨` is the one-cell form (2026-08-06). Absent a comment the cell is blank. `HasComments` is set per-row by `RefreshTasks` from `store.TaskIDsWithComments`. |
| Row card: active bar | `▌` | Left edge marker on lists and task rows. Accent when the row is selected (or the inline input is active), otherwise the row's own status color — see Row layout below. |
| Add-input level: sibling (default) | `-` | §4. |
| Add-input level: child | `+` | §4. |
| Add-input level: parent-of-selection | `^` | §4. |
| Trailing derived/percentage progress | ` (NN%)` | In `TextDim`, rendered at the start of the row's right-aligned block, immediately left of the agent spinner; omitted entirely when `DerivedProgress` reports `displayAsSimple` (§3), never rendered as `(0%)` in that case. |
| Task priority | ` ● HIGH` / ` ● MED` / ` ● LOW` | All caps, like the status label, with a coloured rank dot, in a content-width cell in the right-aligned block immediately left of the status label. **`none` renders nothing at all**, since most tasks are `none` and a badge on every row is noise rather than information, so the cell is not reserved and rows do not align on it, unlike the fixed status column. The badge is drawn in the theme's status tokens rather than its text tiers: `high` on `StatusOverdue` (red), `medium` on `StatusInProgress` (amber), `low` on `StatusPending` (grey). The colour ladder is the signal; the all-caps label keeps the status label's register so the badge reads as a sibling of the status it sits next to. `tasktree.priorityLabel`/`priorityFg`. |
| Task assignee | ` @tag` | The durable holder of a task (§3), in the right-aligned block immediately right of the agent spinner; the two are adjacent on purpose, because "assigned, but nobody is here" is only legible as a gap between them. The tag is clipped through `chrome.Truncate` to seven cells so one long agent identity cannot push the right block across the row, and an unassigned task renders nothing. `tasktree.assigneeBadge`. |
| Task assignee: stale | ` @tag` in `StatusOverdue` | The **stale-assignment tier**: `assignee != ""` **and** no live presence claim by that agent (§3). Assignment has no TTL and no background sweeper, and a session releases its own work as it exits, so this badge marks the one case left: a session killed before it could clean up. It is the only thing on screen that distinguishes abandoned work from work merely owned, and the human's `u`/`U` release keys (§5) are the only thing that clears it. Rare by design, not routine. `StatusOverdue` is reused rather than a new token added — it is the same "a human needs to look at this" tier the Details modal and the search picker already draw their error lines in. The live-agent set is read **once per refresh** from the activity set the poll already carries, never per row. `tasktree.assigneeFg`. |
| Agent is working | `⠋⠙⠹⠸⠼⠴⠦⠧` | 1-cell braille spinner, animated via `AnimTickMsg`; draws `Accent` when the row is focused/selected, `TextDim` otherwise. Rendered in the right-aligned block immediately left of the assignee badge when the row's entity is claimed. The `Spinner(frame int)` function lives in `src/components/chrome/Spinner.go`; no component invents its own glyph. |
| List is collaborative | ` · shared` | Appended to a lists-panel row's count line (`N pending · M done`) when `List.Collaborative` is true — plain text in the same `TextDim` tier the count line already renders in, not a new glyph, so it needs no dedicated symbol here. `src/components/listspanel/View.go`'s `listDelegate.Render`. The rename modal's collaborative toggle (below) reuses the task row's own `◻`/`◼` checkbox glyphs for the same flag, rather than inventing a `[ ]`/`[x]` of its own. |

**Task rows are full-width cards**:
a `▌` bar column, then `{2 spaces × depth}{checkbox}{space}{title}` on the
left and the right-aligned `{progress}{agent spinner}{assignee}{priority}{status}` block, then the fixed two-cell trailing icon column (`{🗎}{🗨}`, each cell blank when its indicator is absent) at
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
narrowness the status sheds before the progress, both whole; the title and
checkbox are never shed. Depth-0 pending example (parent row):
`▌◻ Buy paint for the fence ▾             PENDING`.

Section headers (`Pending`, `Complete` — §6) render as `{bold TextPrimary}
{section name} {dim count in parens}`, e.g. **Pending** `(3)` — the same
"name, then a muted count" shape the lists panel already uses for a list's
own row, so the two surfaces read as one convention rather than two.

### Section headers: pinned and overflow-marked

When the cursor's section header has scrolled off the top of the task-tree
viewport, it is **pinned** as a fixed line at the top of the window so the
section name and count stay in view while scrolling (renderWindow in
`src/components/tasktree/View.go`). The pin reduces the content area by one
line: `scrollFor` recomputes the offset with `height-1` so the selected row
remains visible in the shrunken window. A header that is already at the top
of the viewport is not duplicated — the pin only takes effect when the header
is above the scroll offset.

The pinned header carries an **overflow suffix** rendered in `TextDim`,
separated from the header by two spaces: the count of task rows hidden above
the viewport (`N above`), the count hidden below (`N below`), or both joined
by ` . ` — e.g. **Pending** `(3)` `  5 below`, or **Pending** `(30)` `  11 above . 5 below`.
When the full section fits within the viewport (no rows hidden on either
side), the suffix is omitted entirely. The counts span only task rows —
not chrome lines like section rules, blank spacers, the filter bar, or the
inline create row.

The lists panel mirrors the same convention: the bubbles list's built-in
pagination dots (which render as `••••…` at the bottom of a multi-page list)
are disabled (`SetShowPagination(false)` in `src/components/listspanel/Model.go`),
and an "N below" line in `TextDim` replaces them via `chrome.PanelBodyWithFooter`
in `View()`, so both surfaces show the same "N below" overflow signal in the
same tier.

### The task tree's filter bar

The `/` filter bar replaces the section headers while the filter is active
(§8). It opens with a bold `TextPrimary` `/`, then the query — the live input
while typing, the same text in `TextMuted` once `enter` has applied it — then
a `TextDim` suffix two spaces along: the **match count**, then two more
spaces, then `esc to clear`. The count is `N matches`, singular `1 match`,
and it counts *directly matched rows only* — the ancestors kept for tree
context are not matches and never inflate it. Both the count and the hint
appear from the first character typed, not on commit: they are what tell a
user a query has gone too narrow before they abandon it. With `/` open but
nothing typed there is no query to count, so the bar shows `esc to clear`
alone.

Inside a directly-matched row, the characters the query matched are drawn in
`Accent` **bold**; the rest of the title keeps the colour the row would have
had anyway. The offsets come from `sahilm/fuzzy`'s `MatchedIndexes` — the
same `Find` call that decided the row matched, so the highlight can never
disagree with the match — and adjacent matched characters coalesce into one
span rather than being styled one rune at a time. A row visible only as an
**ancestor** of a match is not highlighted: it renders elided as
`[…] <title>` in the recessed dim card, which is what distinguishes "this
matched" from "this is here for context".

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
  separately.

Do not introduce a fourth informal tier (a hand-picked opacity, a literal
gray hex) for "something in between" — if the three don't cover a case,
that's a signal to reconsider the case, not to add a color.

### Empty states: one recessed-card pattern

An empty *task list* is not one of these: it renders its inline input and the
guidance beside it (§6), which is what gives that condition exactly one
appearance. `EmptyStateCard` is for the states with nothing to type into —
a filter that matched nothing, a lists panel with no lists yet.

Every empty state that does use it is the same shape: a box on the
`BackgroundRecessed` tier, rimmed with
`BorderCard` (not `BorderDefault` — see §11's inherited reasoning: a border
has to contrast with the surface it wraps, and `BorderDefault` moves *toward*
`BackgroundRecessed` rather than away from it), `Padding(1, 2)` matching
`PanelFrame`, `TextDim` guidance text, left-aligned. Do not
center empty-state text and do not give it its own bespoke padding —
reusing the exact `PanelFrame` numbers is what makes it read as "this panel,
currently empty" rather than a different kind of surface that happens to be
nearby.

**The card is sized to its message, then centered in the space it is given.**
It does not stretch to fill that space: a two-line message inside a 28-row
rounded box reads as a large broken panel rather than a note. Only the *box* is
centered — the text inside it stays left-aligned, per the rule above.
`chrome.EmptyStateCard` still returns a block occupying the caller's full
width × height, so the caller composes one thing; the space around the card is
painted in the **caller's own tier**, not the recessed one, because that space
belongs to the panel and not to the card. That is why the function takes the
surrounding background as a parameter rather than assuming a tier.

### First run

The store starts empty, so the TUI creates one list for itself. It is named
**`Inbox`** (`constants.DEFAULT_LIST_NAME`) — a name, not a placeholder. "New
List" described the list's age rather than its contents, read as an unfinished
setup step the user was expected to complete, and the only way to correct it is
`R` in the Lists panel: a panel a first-run user may not have discovered, and
one that is hidden entirely below `AUTO_SHOW_LISTS_MIN_WIDTH`. The name a user
is handed without being asked has to be one worth keeping. It matches what the
MCP server already names an agent's own default list (`<tag>: Inbox`,
`store.GetOrCreateAgentList`).

This is only the *auto-created* name, used on first run and whenever every list
has been deleted. A list the user creates with `n` is named by the user in
`listnamemodal` and shares no constant with it. First run does not ask for a
name: `Inbox` is the decision, not a question.

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

`src/store` talks to a real SQLite file created fresh in a temp directory
per test, so **every** state-machine rule in §3 —
including the auto-completion cascade and the zero-children fallback — is a
plain Go test with no TUI, no terminal, and no mocking required. Write those
tests directly against `store`, not against the CLI or the TUI, wherever the
assertion is about data rather than rendering.

Above that, test in three tiers:

1. **Model tests** — a component takes a message and hands back a model;
   assert on the result directly. `src/model`'s tests build the full
   `AppModel` against a real store in a temp directory (`newTestModel` in
   `src/model/refresh_test.go`) and drive keypresses through `Update`, which
   covers keystroke-to-state flows without a terminal.
2. **Render tests** — a component's rendering is a string
   (`ansi.Strip(m.View().Content)`), worth asserting on for layout and
   styling; `chrome`-level checks like `appstyles.HasBackgroundBleed` belong
   here too.
3. **End-to-end rigs** — a real Bubble Tea program driven over a fake
   terminal are the only way to test a full keystroke-to-render flow. Keep
   them the exception, not the default: they are timing-based and must wait
   for specific output rather than sleep and hope.

The same three tiers are listed in `CONTRIBUTING.md`'s Testing section; keep
the two in step if you change either.
