---
title: Core concepts
description: The mental model — the Tea loop, focus, the esc ladder, keyboard ownership, and the status/progress state machine.
sidebar:
  order: 4
---

This page is the mental model you need before touching the TUI half. The rules here are the ones `docs/DESIGN.md` states as contracts rather than suggestions — the places where a plausible, intuitive implementation is the wrong one.

## The Tea loop: everything is a message

The TUI runs on Bubble Tea v2's Elm-style loop. A program is a `tea.Model` with three methods:

- `Init() tea.Cmd` — the commands that start the app (for farol: the poll tick, the first lists refresh, the startup focus broadcast).
- `Update(msg tea.Msg) (tea.Model, tea.Cmd)` — every event is a message: a keypress (`tea.KeyPressMsg`), a window resize (`tea.WindowSizeMsg`), a poll tick (`cmds.PollTickMsg`), a refresh result (`cmds.RefreshListsMsg`), an intent from a component (`cmds.ToggleTaskMsg`). The model returns a new model and possibly more commands.
- `View() tea.View` — renders the current state to a string (in v2, a `tea.View` struct whose `Content` field is the string).

There is no shared mutable state and no direct function calls between components: components communicate by emitting messages, and `AppModel` routes them. That is the request/response split described in [Architecture](/contributors/architecture/).

## AppModel: the one model that owns everything

`AppModel` (`src/model`) is the top-level model. It owns:

- the `store` handle and the `config`,
- the terminal dimensions (the only place that reads `tea.WindowSizeMsg`),
- the poll loop (re-issuing `PollTick` on every `PollTickMsg`),
- navigation and focus (`focusedZone`, `listsPanelVisible`, `detailsPanelVisible`),
- the active list and the lists it holds,
- modals (`activeModal`) and the Details modal,
- `lastError`, the one channel every failed action reports through.

Components are constructed in `GetInitialModel` and stored in `m.components`. The task tree is the startup focus zone, broadcast at startup so its keys work from the first frame.

## Focus: two zones, not three

There are exactly **two keyboard focus targets** (`docs/DESIGN.md` §5):

- **The Lists panel** — present in the `tab`/`shift+tab` cycle only while *rendered*. `L` toggles the stored preference; a separate derived predicate (`listsPanelRendered()`) decides whether the panel actually occupies width this frame, and that predicate — not the raw preference — governs focus, the footer, and rendering. On the first window-size message the preference is seeded from terminal width (auto-shows at 120 columns or wider); after that, `L` is the sole authority.
- **The task tree** — the Tasks surface's Pending/Complete sections, one flat keyboard-navigable cursor across both.

`tab`/`shift+tab` cycle **only through the targets currently visible** (`focusableZones()` in `src/model/Update.go` computes the cycle at runtime). With no side panel open the cycle is a single zone, so the footer does not advertise tab/shift+tab at all.

**Details is a modal, not a body surface.** Opening Details (`enter` on a selected task) layers a centered modal over the body — sized to about 90% of each axis — and takes focus. It is never in the `tab` cycle; it is entered and left by explicit open/close transitions.

## The esc ladder

`esc` is the most overloaded key in the app — six jobs, resolved through a strict "ladder of claims" (`docs/DESIGN.md` §5). Each surface that might own esc is checked in a fixed order, and the first one that claims it gets it. **The order is the contract**; checking it out of sequence silently breaks whichever claim got skipped:

1. **A modal** (theme picker, confirm, list-name) closes itself first — modals intercept every keypress at the top of `Update`.
2. **The Details modal**, while visible, owns every keypress ahead of AppModel's normal `Back` case: closing a clean modal, or raising the inline "Discard changes? (y/n)" prompt on a dirty one.
3. **The focused panel's own `KeepsEsc` claim**: the task tree while typing or applying a `/` filter, or while inline-creating a task; the lists panel while its own filter is open or applied — clearing the filter, not closing the panel.
4. **Closing the Lists panel**, when it is focused and visible and did not already claim esc at step 3 — the panel is a transient picker, and esc is its cancel. This must stay below step 3, or a filtered lists panel would close instead of clearing its query.
5. Otherwise, a **no-op**.

## Keyboard ownership

While a modal, the Details modal, a filter input, or the inline create input is open, **it owns the keyboard** (`docs/DESIGN.md` §5). The consequences:

- `q` is a printable character, so it is handled *after* everything that could be typing one: a modal swallows it, the inline create row and a `/` filter take it as a literal `q`, and it quits only from the task tree or the lists panel with none of those active. `ctrl+c` always quits — the escape hatch that yields to nothing.
- While the inline create input is live, `tab`/`shift+tab` do **not** cycle focus to another panel and `?` types a literal instead of opening help — a half-typed title can never be stranded on another panel mid-entry.
- `keys.Active` decides what is live, so what the footer advertises and what `q` does cannot drift apart.

## The layout

The screen is three stacked regions (`src/model/View.go`):

- **Header** — one row: the main menu bar with the wordmark.
- **Body** — the Tasks surface, plus at most one side surface (Lists), separated by a tier-2 gutter. Tasks alone, or Tasks + Lists; never three panels.
- **Footer** — one row: the keybinding bar, rendered from `keys.Active`/`GlobalsFor` for the current context.

Below the minimum supported size (40 columns × 10 rows) the app stops attempting the layout and renders a single centred `Terminal too small` line. The decision lives in exactly one predicate (`AppModel.terminalTooSmall`).

## The status/progress state machine

This is the section most likely to be implemented from intuition rather than from what's written (`docs/DESIGN.md` §3). Read the whole thing before writing `store` code that touches `status` or `progress_kind`.

**States.** A task's `status` is one of `pending`, `in_progress`, `complete`. `progress_kind` only has meaning while `status = in_progress`; it is `none` for pending and complete tasks — an invariant the store enforces, not a convention callers remember.

**The three flavors of `in_progress`:**

| Kind | Meaning |
| --- | --- |
| `simple` | No number. The task is being worked on; that's the whole claim. |
| `percentage` | A user- or agent-set integer 0–100 (`progress_pct`). An honest estimate, not a fact the store can verify. |
| `subtasks` | `progress_pct` is **not stored**; it is computed on read as `round(100 × complete_children / total_children)` over the task's *direct* children only. A fact the store can verify. |

Two edge rules that intuition gets wrong:

- **Switching a task to `subtasks` mode with zero children is not an error.** It displays and behaves as `simple` — no percentage, no auto-completion — until it has at least one child. The stored `progress_kind` stays `subtasks`; only the display and completion behavior falls back. Do not "fix" this by rewriting `progress_kind` to `simple`.
- **A task that gains its first subtask is auto-switched to `subtasks` mode.** When any add path inserts a task under a parent whose `progress_kind` is still `none`, the store switches the parent to `subtasks` through the same `SetProgress` write path a user uses by hand — which also starts the parent (a pending parent becomes `in_progress`). An explicit kind is never overridden, and a complete parent is never touched.

## Auto-completion is asymmetric — deliberately

- **`subtasks` reaching 100% (every direct child complete) promotes the parent to `complete` automatically.** This is a verified fact — if every child is done, the parent claiming otherwise would be a lie the store can see through. The check re-runs on every child completion and walks upward (`recomputeAncestors`): completing a leaf can complete its parent, which can complete *its* parent, and so on.
- **`percentage` reaching 100 does not auto-complete.** It's a claim, not a verified fact, and the store has no way to distinguish "I meant it" from "I typed 100 out of habit." Completing is a separate, explicit action (`space` in the TUI, `farol <id>` on the CLI) even at 100%.

If this asymmetry surprises you enough to want to change it, that's a product decision to raise, not a bug to silently fix — it was chosen specifically to keep the one auto-promotion path (verified subtask completion) the only one.

## Completing cascades down; reopening does not

- **Completing** a task (`space`, or `farol <id>`) sets every descendant, at every depth, to `complete` too — a `complete` task with a `pending` grandchild is a state this app does not allow to exist. Completing also clears the assignment of the task and everything the cascade completes.
- **Reopening** a task (`farol reopen <id>`, or `space` again on an already-complete task) does **not** cascade to children — it returns *only that task* to `pending`. This is intentionally lossy: the task's prior `progress_kind` and `progress_pct` are not restored.

## The store owns every transition

None of the above is duplicated in `src/cli` or in components. `store.Complete`, `store.Reopen`, and `store.SetProgress` are the **only three functions** that write `status`/`progress_kind`/`progress_pct`, and every caller — CLI subcommand or TUI keypress handler — goes through them. There is exactly one write path per kind of change, enforced by Go visibility (these three are the only exported mutators). `store.Toggle` delegates to whichever of the two applies, so the CLI's `toggle` and the TUI's `space` can never each decide which direction a toggle goes.