---
title: Keybindings
description: The complete keybinding reference, organized by scope. Every key is declared once in src/keys and rendered by the footer and the ? overlay.
sidebar:
  order: 7
---

Every key in farol is declared exactly once, in `src/keys/Keys.go`. The footer bar and the `?` help overlay render from that same declaration, so what they advertise is what the handlers do — they cannot drift apart.

`?` lists every key in the app on every screen, with the ones that do nothing right now dimmed. The tables below are the full reference, in the same scope order the overlay uses.

## Global

Work anywhere that no overlay owns the keyboard.

| Key | Action |
| --- | --- |
| `tab` / `shift+tab` | Cycle focus between the visible surfaces (Lists ↔ Tasks). Dropped from the footer while the inline create input is live — focus is locked to the input then. |
| `L` | Toggle the Lists panel; opening it also moves focus into it. |
| `q` | Quit — yields to anything typing a `q` (a modal, the inline create row, a filter). |
| `ctrl+c` | Force quit — yields to nothing, so it quits from a modal or a text input alike. |
| `esc` | Back — a ladder of claims: closes a modal, clears a filter being typed, clears an applied filter, closes the Lists panel. |
| `?` | Help overlay. |
| `T` | Theme picker (live preview; `Enter` applies and persists). |
| `/` | Local fuzzy filter — the task tree when the tree is focused, the Lists panel when the panel is. |
| `F` | Cross-list search picker. |
| `ctrl+y` | Copy the selected item's id (task or list) to the system clipboard. |
| `s` | Cycle the task tree's sort mode: manual → priority → created → updated → alpha. |
| `a` | About modal. |

## Task Tree

Act on the selected task in the tree.

| Key | Action |
| --- | --- |
| `↑` `↓` `k` `j` | Move the cursor (across every visible row). |
| `→` `l` | Expand the selected node if it has children and is collapsed, else move to its first child. |
| `←` `h` | Collapse the selected node if it has children and is expanded, else move to its parent. |
| `space` | Toggle complete / pending (completing cascades to descendants; reopening does not). |
| `enter` | Open the Details modal. |
| `n` | New task — the inline create row. |
| `d` | Delete the selected task (confirm-guarded). |
| `[` | Outdent the selected task — move it out from under its parent. |
| `]` | Indent the selected task — move it under its previous sibling. |
| `alt+↑` `alt+k` | Move the selected task up within its own Pending/Complete section. |
| `alt+↓` `alt+j` | Move the selected task down within its own section. |
| `u` | Release the selected task's assignment. |
| `U` | Release every assignment in the active list (confirm-guarded). |
| `g` `home` | Jump to the first row. |
| `G` `end` | Jump to the last row. |
| `pgup` | Move one viewport height up. |
| `pgdown` | Move one viewport height down. |

`u` releases the selected task's assignment and `U` releases every assignment in the list — an assignment has no expiry, so these are the only thing that frees a task whose agent went away.

## Creating a task

Act inside the inline create row, and only there — the input owns the whole keyboard while it is open.

| Key | Action |
| --- | --- |
| `enter` | Submit the new task. |
| `esc` | Cancel (clears the draft and resets the level). |
| `[` | Outdent — the new task lands one level above the selection (`^`). |
| `]` | Indent — the new task lands as a child of the selection (`+`). |

`[` and `]` set the new task's level while the input is open. Focus is locked to the input: `tab`/`shift+tab` do not cycle panels, and `?` types a literal.

## Filtering

Act while a `/` filter is open or applied.

| Key | Action |
| --- | --- |
| `/` | Enter the local fuzzy filter on the focused panel. |
| `enter` | Apply the query and leave the filtered view active. |
| `esc` | Clear the filter. |

`/` filters the focused panel; `F` searches across every list.

## Lists

Act on the Lists panel, which must be visible and focused.

| Key | Action |
| --- | --- |
| `↑` `↓` `k` `j` | Move the highlight. |
| `enter` | Select the highlighted list and close the panel. |
| `esc` | Close the panel (after first clearing an active filter). |
| `n` | New list. |
| `R` | Rename the highlighted list. |
| `d` | Delete the highlighted list (confirm-guarded). |
| `alt+↑` `alt+k` | Move the highlighted list up in the ordering. |
| `alt+↓` `alt+j` | Move the highlighted list down. |
| `e` | Export — the highlighted list, or the whole store. |
| `i` | Import lists from a JSON file. |
| `/` | Filter the panel's lists. |

`L` shows the Lists panel and moves focus into it; `tab` moves focus back. `enter` and `esc` also close it, on the selected list and on cancel respectively.

## Renaming a list

Act inside the list-name modal's Rename mode.

| Key | Action |
| --- | --- |
| `enter` | Confirm the new name. |
| `esc` | Cancel. |
| `tab` | Next field (name → collaborative toggle). |
| `space` | Toggle the collaborative flag — any agent may restructure the list. |

`tab` and `space` only apply in Rename mode; New-list mode is a bare text input.

## Details

Act inside the Details modal, which owns the keyboard while open.

| Key | Action |
| --- | --- |
| `ctrl+s` | Save title, notes, progress, and priority changes; close the modal. |
| `tab` / `shift+tab` | Cycle the zones: Title → Notes → Progress → Priority → Comments. |
| `←` `→` `h` `l` | Cycle the progress modes (`simple` / `subtasks` / `percentage`) — Progress zone. |
| `←` `→` `h` `l` | Cycle the priority (`none` → `low` → `medium` → `high`, wrapping) — Priority zone. |
| `↑` `↓` | Step the percentage by 5, clamped to 0–100 — percentage mode only. |
| `0`–`9` | Type a percentage directly — percentage mode only. |
| `ctrl+y` | Copy the open task's id — works from every zone. |
| `c` | Add a comment (opens the inline compose card) — Comments zone. |
| `enter` | Post the comment — compose card. |
| `y` | Copy the highlighted comment's id — Comments zone. |
| `d` | Delete the highlighted comment (confirm-guarded) — Comments zone. |
| `esc` | Close a clean modal; on a dirty one, raise the `Discard changes? (y/n)` prompt. |

The `Discard changes?` prompt answers to `y` and `n` only — it has no visible default for `enter` to act on.

## Overlays

The keys every modal answers to.

| Key | Action |
| --- | --- |
| `enter` | Confirm / submit. |
| `esc` | Cancel. |
| `y` `Y` | Yes. |
| `n` `N` | No. |
| `↑` `↓` `k` `j` | Navigate (the help overlay scrolls its catalog with these). |

## Design notes

- **`q` yields, `ctrl+c` does not.** `q` is a printable character, so it is handled after everything that could be typing one — a modal or the Details modal swallows it, the inline create row and a `/` filter take it as a literal `q`, and it quits only from the task tree or the lists panel with none of those active. `ctrl+c` is the escape hatch that yields to nothing.
- **`esc` is the most overloaded key in the app** — six jobs, resolved through a strict ladder of claims: a modal closes itself first, then the Details modal, then a focused panel's own claim (clearing a filter, cancelling a create), then closing the Lists panel.
- **The footer sheds, never wraps.** On a narrow terminal the keybinding bar drops whole hints in a declared priority order rather than wrapping to a second line — and `?` lists every key, so a shed hint is hidden, not lost.
