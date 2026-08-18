---
title: The CLI
description: farol's CLI — the contract, the id-prefix rule, worked examples, and every subcommand with its --json shape and exit code.
sidebar:
  order: 5
---

The CLI is not a companion to the TUI. It is farol's **API**: the surface every script, every automation, every coding agent uses to read and write the store. This page has two halves — the contract (read it once, once is enough), and the reference (look up the shape of a specific command).

If you want a tour of *using* the CLI as an agent — the working loop, session openers, presence — read [Working with coding agents](/users/agents/) instead. This page is the shape of every command; that page is why you'd string them together.

## The contract

Six rules govern every subcommand. An agent that has read one command's `--help` predicts the shape of every other one's output and errors.

### 1. `--json` prints exactly one JSON value on stdout

In `--json` mode, stdout is **always exactly one JSON value**, whether the command succeeded or failed — `{"error": "list not found: 01ARZ…"}` on failure, the command's payload on success. Nothing else is written to stdout in that mode; diagnostic text goes to stderr. A caller never has to check two streams: parse stdout, then read the exit code to know whether what you parsed was the payload or the error.

### 2. Exit codes are stable

| Code | Meaning |
| --- | --- |
| `0` | Success. |
| `1` | Domain failure — id not found, invalid state transition, validation failure. |
| `2` | Usage error — malformed flag, missing argument, unknown subcommand (Cobra's default). |

An empty read result — no tasks match, no claims are held — prints `[]` in `--json` mode with exit `0`. That is "no data", not "failed". Never confuse the two by parsing the bytes; read the exit code.

### 3. Human output is plain text, no ANSI

A read command's human-mode output is a plain formatted table or tree — no colour, no bold, no cursor moves. A script can capture stdout without stripping anything. Errors, in human mode, print one line prefixed `farol: ` to stderr.

### 4. ID prefixes are first-class

Every `<list-id>` and `<task-id>` argument accepts an **unambiguous prefix** of the full ULID. An ambiguous prefix — one that matches more than one row — is a domain error (exit `1`), never a silent pick of the first match.

That is what makes the CLI usable: eight characters copied out of any `--json` response is enough, in every argument, everywhere.

### 5. Destructive commands need `--force`

`farol lists rm`, `farol rm` (task), and `farol comment rm` refuse to run without `--force`. The TUI's equivalent actions go through a confirm modal; the CLI has no modal and no human to ask, so the flag *is* the confirmation. An agent's typo in a task id should not have the blast radius of an unrecoverable delete with no prompt at all.

### 6. Writes echo what they wrote

A write command that succeeds does not go silent. The two add commands print `{"id": "…"}`; every other write prints `{"ok": true}` plus any field it changed — `{"ok": true, "assignee": "pi"}`, `{"ok": true, "priority": "high"}`, `{"ok": true, "released": 3}`. A caller never needs a follow-up `show` to confirm what landed.

## Worked example

The founding shortcut, and the reason the whole binary exists:

```bash
farol <task-id>
```

That marks the task complete, cascades to every descendant, and prints its id on stdout. `farol` is the verb; `<task-id>` is the object. The rest of the CLI is subcommands.

A slightly fuller session — read, act, verify:

```bash
$ farol lists
ID       NAME     PENDING  COMPLETE
01ARZ…   Inbox    3        1
01AS1…   api      7        2

$ farol tasks 01AS1 --json | jq '.[0]'
{
  "id": "01AS1F3K…",
  "parent_id": null,
  "title": "Ship auth v2",
  "status": "pending",
  "progress": { "kind": "subtasks", "percent": 20, "display_as_simple": false },
  "depth": 0,
  "assignee": "",
  "priority": "high"
}

$ farol assign 01AS1F3K --json
{"ok": true, "assignee": "you"}

$ farol progress 01AS1F3K --mode percentage --percent 50 --json
{"ok": true, "kind": "percentage", "percent": 50, "display_as_simple": false}

$ farol 01AS1F3K --json
{"id": "01AS1F3K…"}
```

Every id in that transcript is an 8-character prefix. Everything after the prefix — the store — resolved the rest.

## Reference

### Launch

| Command | What it does |
| --- | --- |
| `farol` | Launch the TUI. |
| `farol <task-id>` | Mark the task complete (cascades to descendants) — the founding shortcut. |
| `farol --version` | Print the version. |

### Lists

| Command | What it does |
| --- | --- |
| `farol lists` | List all lists (`ID NAME PENDING COMPLETE`). `--mine` / `--foreign` filter by the current agent's ownership. |
| `farol lists add <name>` | Create a list; prints its id. `--owner <tag>` provisions it for an agent up front. |
| `farol lists rename <list-id> <name>` | Rename a list. |
| `farol lists rm <list-id> --force` | Delete a list and its tasks. |

### Tasks

| Command | What it does |
| --- | --- |
| `farol tasks <list-id>` | List a list's tasks as a tree, with `Pending (N)` / `Complete (N)` sections. `--status pending\|in_progress\|complete\|all` filters by root status; `--flat` prints `id<TAB>status<TAB>title` per line. |
| `farol add <list-id> <title>` | Add a task; prints its id. `--parent <task-id>` makes it a subtask, `--notes <text>` sets notes, `--force` allows adding to a list owned by another agent or by nobody. |
| `farol show <task-id> [<task-id> …]` | Show one or more tasks (up to 50): title, notes, status, progress, children, comments, attachments. |
| `farol rename <task-id> <title>` | Rename a task. |
| `farol notes <task-id> <text>` | Replace a task's notes (whole text, not append). |
| `farol complete <task-id> [<task-id> …]` | Mark one or more tasks complete (cascades to descendants). `farol <task-id>` is the single-task shorthand. |
| `farol reopen <task-id> [<task-id> …]` | Mark one or more tasks pending (does not cascade). |
| `farol toggle <task-id>` | Complete ↔ reopen, whichever applies. |
| `farol comment <task-id> <note>` | Add a comment; prints its id. |
| `farol comment rm <comment-id> --force` | Delete a comment. |

### Progress

| Command | What it does |
| --- | --- |
| `farol progress <task-id> --mode simple` | Mark the task in progress with no number. |
| `farol progress <task-id> --mode percentage --percent <0-100>` | Set a user- or agent-set percentage. |
| `farol progress <task-id> --mode subtasks` | Derive the percentage from the task's direct children. |

Setting progress starts the task — a pending task becomes `in_progress`. The `--json` shape is `{"kind", "percent", "display_as_simple"}`.

### Assignment and priority

| Command | What it does |
| --- | --- |
| `farol assign <task-id> [--force]` | Assign the task to the current agent (`FAROL_AGENT`, or the per-process default). `--force` takes it from another holder. |
| `farol unassign <task-id>` | Release the current agent's assignment on the task. |
| `farol unassign --list <list-id>` | Release the assignment on every task in the list. |
| `farol priority <task-id> --level none\|low\|medium\|high` | Set a task's priority. |

Assignment reserves the subtree: it is refused when any ancestor or descendant is held by a different agent. A conflict without `--force` is refused with an error naming the holder and the age; `--force` takes it, reassigns, and writes a takeover comment. The whole model is in [Working with coding agents](/users/agents/).

### Move and delete

| Command | What it does |
| --- | --- |
| `farol mv <task-id> [--parent <task-id>]` | Re-parent a task. An empty `--parent` — the flag's default, so omitting it entirely — moves the task to the list root. A cross-list parent is rejected, as is a move that would create a cycle or put a non-complete task under a complete parent. |
| `farol rm <task-id> --force` | Delete a task and its descendants. |

### Attachments

| Command | What it does |
| --- | --- |
| `farol attach <task-id> <path>` | Attach a file to a task. |
| `farol attachments <task-id>` | List a task's attachments. |
| `farol detach <attachment-id>` | Remove an attachment. |

### Search

| Command | What it does |
| --- | --- |
| `farol search <query> [--list <list-id>]` | Fuzzy search across titles and notes. Title matches are ranked by fuzzy score first, then notes-only hits in store order. |

### Export and import

| Command | What it does |
| --- | --- |
| `farol export [list-id] [--out <file>]` | Export the whole store, or one list, to a versioned JSON document. With `--out`, writes the file and prints a one-line summary; without, prints the document to stdout. |
| `farol import <file> [--list <list-id>]` | Import lists and tasks from a farol export file. Additive: it recreates each list with fresh ids and never overwrites existing data. A file whose `version` does not match is a domain error. |

### Session openers and presence

| Command | What it does |
| --- | --- |
| `farol inbox [--include notes]` | Start-of-session context: your list plus every foreign list, each with its top 20 pending tasks. Read-only and non-interactive — claims no presence. |
| `farol work` | Live presence claims: who is working on which task or list right now. Read-only. |
| `farol claim <task-id\|list-id> [--kind working\|inspecting]` | Claim presence on an entity (lights the TUI spinner). A claim held by another agent is a domain error — never silently stolen. |
| `farol release <task-id\|list-id> [--all]` | Release presence on an entity, or `--all` to clear every claim this agent holds. A no-op when the agent does not hold the claim. |
| `farol next <list-id>` | Grab and show the top eligible task — highest priority, then tree order — assigning it to the current agent atomically. An empty board is a normal state, not an error. |
| `farol diff <list-id> [--since <unix-seconds>]` | Return tasks added or changed since a timestamp. Cheap to loop on for a polling agent. |
| `farol skill` | Print the agent command reference (markdown) — the identity contract, the write surface, and the `--json` contract. Pipe into your agent's context. |

Presence is orthogonal to assignment: a claim lights the TUI spinner but does not move a task to `in_progress` and is not ownership. Who *owns* a task is the row's `assignee` field, a separate axis.

## JSON shapes, pinned

The shapes below are part of the contract:

- **Writes**: `{"id": "…"}` for the two add commands, `{"ok": true, …}` for every other write, always echoing the field it changed.
- **`lists`**: `[{"id", "name", "pending", "complete", "created_at"}]`.
- **`tasks`**: a flat preorder array — `[{"id", "parent_id", "title", "status", "progress", "depth", "assignee", "priority"}]` — in both `--flat` and tree modes. `parent_id` + `depth` let a caller reassemble the tree.
- **`show`**: the task's fields, its `progress`, and `children` as the same row array `tasks` emits, depth relative to the shown task.
- **`assign` / `unassign` / `priority`**: each echoes the field it wrote — `{"ok": true, "assignee": "pi"}`, `{"ok": true, "assignee": ""}`, `{"ok": true, "released": <n>}`, `{"ok": true, "priority": "high"}` — so a caller never needs a follow-up `show`.
- **`progress`**: `{"kind", "percent", "display_as_simple"}` — `percent` is `null` whenever the kind has nothing to display.
- **`search`**: `[{"id", "list_id", "list_name", "title", "status", "progress", "assignee", "priority"}]`.
- **`export`**: `{ "version": 1, "lists": [ … ] }` — the same document shape whether one list or the whole store.
- **`work`**: `[{"id", "entity_type", "entity_id", "agent_id", "kind", "acquired_at"}]` — a bare array, so an empty claim set is `[]`.
- **Empty results**: `[]` in `--json` mode, nothing in human mode; exit `0` in both. Read the exit code to distinguish empty from failed.
