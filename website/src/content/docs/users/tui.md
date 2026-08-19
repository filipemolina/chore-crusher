---
title: The TUI
description: The farol terminal UI in depth — the task tree, the Details modal, progress modes, lists, search, and how presence surfaces on a row.
sidebar:
  order: 4
---

This page is the reference for the person at the keyboard. If you are setting up an agent to drive farol from a shell, see [Working with coding agents](/users/agents/); if you want the shape of every command, see [The CLI](/users/cli/).

The TUI is two body surfaces — **Lists** on the left, **Tasks** on the right — plus a **Details modal** that layers over both, and a footer bar that advertises the keys live right now. `tab`/`shift+tab` cycle focus between the surfaces that are actually visible; `↑`/`↓` (or `k`/`j`) move the cursor.

## The task tree

The Tasks panel is a tree, not a flat list. Tasks nest to any depth, and the tree renders them indented, with an expand/collapse marker on any node that has children.

![The task tree: nested tasks with expand/collapse markers](/screenshot-tree.png)

- `↑`/`↓` (or `k`/`j`) move the cursor across every *visible* row — a collapsed node's children are not visible rows and are skipped.
- `→`/`l` expands the selected node if it has children and is collapsed, else moves to its first child.
- `←`/`h` collapses the selected node if it has children and is expanded, else moves to its parent.
- `g`/`G` jump to the first/last row; `pgup`/`pgdown` move one viewport height.
- Collapse is **deep** — it hides the entire subtree — and expand is **shallow** — it reveals only direct children. Grandchildren stay collapsed until expanded themselves.

## Pending and Complete

The tree is split into two sections, headed `Pending` and `Complete`. One cursor walks both, pending rows first. Which section a tree renders under is decided by the **root** task's own status alone: a pending root renders under `Pending` with its whole visible subtree — including any complete children sitting inline, still nested in place. A complete root renders under `Complete`, and because completing cascades to every descendant, that section is complete all the way down.

Crossing the section boundary wraps — `↓` from the last pending row lands on the first complete row, and `↑` from the first complete row returns to the last pending one.

## Adding tasks

`n` opens the inline create row — a card spliced into the tree itself, not a pinned footer. Type a title and press `enter` to submit, `esc` to cancel.

While the input is open, `[` and `]` change **where** the new task lands, relative to whatever is selected:

| Keystrokes so far | New task's parent | New task's depth | Glyph |
| --- | --- | --- | --- |
| none (default) | the selected task's parent (a sibling of the selection) | same as selection | `-` |
| one `]` | the selected task itself | one deeper | `+` |
| one `[` | the selected task's parent's parent | one above | `^` |

Further presses in the same direction do not go further — the range is clamped to exactly one level either side of the selection. `[` is a no-op on a root-level task, because there is no level above root.

![The inline create row, adding a task at the selected level](/screenshot-add.png)

After a successful add, selection moves to the new task and the level resets to the default. Selecting a different task while the input has unsent text also resets the level — the indicator always describes a relationship to the *current* selection.

## The Details modal

`enter` on a selected task opens the Details modal — a centered overlay sized to most of the screen. It holds six zones, cycled with `tab`/`shift+tab`:

- **Title** — an editable single-line field, saved through the same rename path the CLI uses. A `@<task-id>` mention is validated the same way it is in Notes, but the field always shows raw text — it does not render the resolved title inline the way Notes does.
- **Notes** — a textarea that grows with its content, capped so comment cards stay visible. It opens on its first line, cursor parked at the start. Unfocused, a `@<task-id>` reference in the text renders as the mentioned task's title in the accent color (`[deleted task]` if the target is gone); focus the zone to edit and the raw `@<ULID>` text is what you see and type. A mention has to spell out the full 26-character ULID — not a prefix — so writing one by hand means copying an id out of `farol show --json` or a search result rather than typing it from memory.
- **Progress** — the progress-mode selector (see below).
- **Priority** — cycles `none` → `low` → `medium` → `high` with `←`/`→` (or `h`/`l`), wrapping.
- **Comments** — the comment thread, rendered as selectable cards.
- **Attachments** — the task's file attachments, one path per line.

`ctrl+s` saves title, notes, progress, and priority changes, closes the modal, and returns focus to the tree. `esc` closes a clean modal immediately; on a dirty one it raises an inline `Discard changes? (y/n)` prompt — `y` discards, `n` keeps editing, and `enter` is deliberately unbound there so a stray keystroke can never throw away unsaved edits. `ctrl+y` copies the open task's id from any zone.

### Comments

`c` opens an inline compose card at the foot of the thread; `enter` posts the comment, `esc` cancels. Comments are short status or handoff notes — one line is sufficient. A `@<task-id>` mention is validated the same way it is in Notes, but a comment card shows raw text, not the resolved title. `↑`/`↓` move the highlight, `y` copies the highlighted comment's id, and `d` deletes it through the same confirm modal every destructive action uses, quoting the comment's own text.

### Attachments

The Attachments zone lists the task's files, one path per line, highlighting the selected one. `↑`/`↓` (or `k`/`j`) move the highlight; `d` deletes the highlighted attachment through the same confirm modal every destructive action uses.

The zone is view-and-delete only — there is no TUI action to add an attachment. Attaching a file happens on the CLI:

```bash
farol attach <task-id> <path>              # a local file, stored by path (not copied)
cat notes.txt | farol attach <task-id>     # from stdin
farol attach <task-id> https://example.com/spec.pdf  # downloaded and stored
```

farol never reads or copies a local file you attach — it stores the path as given, so opening it is on you. Stdin and URL sources are the exception: those are materialized under `$XDG_DATA_HOME/farol/attachments` so the content has somewhere to live. See [The CLI](/users/cli/#attachments) for the full command reference.

## Progress modes

While a task is `in_progress`, it carries one of three progress kinds, cycled with `←`/`→` (or `h`/`l`) in the Details modal's Progress zone:

| Mode | Meaning | Percentage |
| --- | --- | --- |
| `simple` | The task is being worked on; that's the whole claim. | none |
| `percentage` | A user- or agent-set integer 0–100. An honest estimate, not a fact the store can verify. | typed with digits, or stepped ±5 with `↑`/`↓` |
| `subtasks` | Derived from the task's direct children: `round(100 × complete / total)`. A fact the store can verify. | computed, not stored |

A task switched to `subtasks` mode with zero children is not an error — it displays and behaves as `simple` until the first child exists. And a task that gains its first subtask is auto-switched to `subtasks` mode, which also starts it: a parent with subtasks is a parent that has started.

## Auto-completion

The two derived-vs-declared kinds behave differently on purpose:

- **`subtasks` reaching 100%** — every direct child complete — **auto-completes the parent**, and the check walks upward: completing a leaf can complete its parent, which can complete *its* parent, and so on.
- **`percentage` reaching 100 does not auto-complete.** It's a claim, not a verified fact. Completing is a separate, explicit action (`space` in the TUI, `farol <id>` on the CLI) even at 100%.

## Completing and reopening

`space` toggles the selected task complete or pending. Completing **cascades down**: every descendant, at every depth, is set to complete too — a complete task with a pending grandchild is a state farol does not allow to exist. Reopening (`space` again on a complete task) does **not** cascade — it returns only that task to `pending`, and its prior progress kind and percentage are not restored.

## The Lists panel

`L` toggles the Lists panel and moves focus into it. It is a transient picker: `enter` commits the highlighted list and closes the panel, `esc` cancels — both indistinguishable from an `L` close afterward. Selecting a list moves the highlight live as the cursor moves, so `enter`/`esc` never need to change which list is active.

![The Lists panel open beside the task tree](/screenshot-split.png)

Inside the panel: `n` creates a list (named in a modal), `R` renames the highlighted list (with a `space` toggle to mark it collaborative — any agent may restructure it), `d` deletes it (confirm-guarded), and `alt+↑`/`alt+↓` reorder it. `/` filters the panel's lists the same way `/` filters the tree.

## Filtering and search

- **`/`** enters a local fuzzy filter whose target follows focus: it narrows the task tree's rows in place to each match plus its ancestor chain, or the lists panel's rows the same way. The filter is live — it re-narrows on every keystroke. `enter` applies the query and leaves the filtered view active so the cursor can walk the results; `esc` clears it. Matched characters are highlighted in the accent color; rows kept only as ancestors render elided as `[…] <title>`.
- **`F`** opens the cross-list search picker: a text input searches every list live, ranking title matches before notes-only hits, and showing each result as `<list> › <task>`. `enter` on a result jumps to that task, switching the active list when the match lives elsewhere; `esc` cancels.

## Deleting

`d` deletes the selected task (or list, when the Lists panel is focused), always through a confirm modal first. Accepting runs the same store delete the CLI's `farol rm --force` runs.

## Moving and restructuring

- **`alt+↑`/`alt+k` and `alt+↓`/`alt+j`** move the selected task up or down *within its own status run* — the gesture never crosses the Pending/Complete boundary, so a task cannot be moved out of the section it belongs in.
- **`[`/`]`** restructure the selected task: `[` outdents it out from under its parent (becoming the parent's next sibling), `]` indents it under its previous sibling. Both are no-ops at their boundaries, and a pending task never moves under a complete sibling.

## Seeing what an agent is doing

The TUI is your dashboard on the agents' work. Two things surface on a task row:

- **A live spinner and an `@tag`** — when an agent has claimed presence on the task or holds an assignment on it. Two agents at once are two visible spinners on two rows.
- **A red `@tag`** — a stale assignment: the row's assignee has no live presence, meaning an agent session ended without releasing. Assignment has no expiry and no background sweeper, so the red badge is the only signal that separates abandoned work from work merely owned.

![A task row showing an agent's assignment and live presence](/screenshot-agent.png)

Two keys on the tree free assignments:

- **`u`** releases the selected task's assignment — unconditionally, since a stale one is by definition held by someone who is not at the keyboard. It prompts for nothing; re-assigning restores it.
- **`U`** releases every assignment in the active list, through the same confirm modal every bulk action uses, counting the assignments it is about to clear.

The full story — presence vs. assignment vs. ownership, subtree reservation, why an agent's assignment cannot be silently stolen — is in [Working with coding agents](/users/agents/).

## Export and import

With the Lists panel focused, **`e`** opens the export modal — defaulted to "this list" when one is highlighted, "whole store" otherwise — and **`i`** opens the import modal, which takes a source file path and recreates each list additively with fresh ids. Both use the same store-level export/import the CLI's `farol export`/`farol import` use, and failures surface in an error strip between the body and the footer.

## The help overlay

`?` opens the help overlay, and it lists **every key in the app on every screen** — not just the keys live on the screen it was opened from. Keys you cannot press right now are dimmed rather than omitted, with a legend saying so. The overlay scrolls with `↑`/`↓`, and a coverage test fails the build if any declared key is missing from it.

## The theme picker

`T` opens the theme picker. Cursor movement previews the theme live — the entire UI behind the modal repaints on each keystroke. `Enter` applies and persists your choice to the config file; `esc` cancels. See [Themes](/users/themes/) for the full list.
