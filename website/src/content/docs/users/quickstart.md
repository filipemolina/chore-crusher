---
title: Quickstart
description: From installed to your first task in under a minute — and then the same loop from an agent's shell.
sidebar:
  order: 3
---

This walks through both surfaces back to back: the human loop first, at the keyboard, then the same loop from a coding agent's shell. The point is to *see* the two views over one store, not just read about them.

Prerequisite: `farol` on your `$PATH`. See [Installation](/users/installation/) if it isn't.

## The human loop

### 1. Launch

```bash
farol
```

With no arguments, `farol` opens the TUI. The first run creates one list for you automatically, named **Inbox** — a name worth keeping, not a placeholder.

### 2. Look around

The TUI has two body surfaces:

- **Lists** (left) — your lists with their pending and complete counts. It opens itself automatically on terminals 120 columns or wider; press `L` to toggle it any time.
- **Tasks** (right) — the active list's task tree, split into `Pending` and `Complete` sections.

A footer bar at the bottom advertises the keys live right now; `?` opens the help overlay listing every key in the app.

### 3. Do something

With the task tree focused:

| Key | Action |
| --- | --- |
| `↑` `↓` `k` `j` | Move the cursor |
| `n` | Create a new task (inline, right in the tree) |
| `[` `]` | While creating: outdent / indent — sibling, child, or parent of the selection |
| `enter` | Submit the new task, or open the selected task's details |
| `space` | Toggle the selected task complete / pending (completing cascades to descendants) |
| `←` `→` `h` `l` | Collapse / expand a task with children |
| `d` | Delete the selected task (confirm-guarded) |

Add a couple of tasks, nest one under another with `]`, and complete a parent to see the cascade land on its children.

### 4. Quit

`q` quits from the task tree or the Lists panel. `ctrl+c` always quits — even from a modal or a text input, where `q` cannot.

## The agent loop

Same store, different surface. Open a second terminal — leave the TUI running in the first one so you can watch — and do the whole loop from the shell.

### 1. Take an identity

The agent's tag is whatever you put in `FAROL_AGENT`. It shows up on rows the agent is holding.

```bash
export FAROL_AGENT=claude
```

If you skip this, farol generates a per-process tag. That works, but you will not recognize it in the TUI.

### 2. Read the whole board

```bash
farol inbox --json
```

`farol inbox` is the start-of-session context call: it returns the agent's own list plus every other list in the store, each with pending and complete counts and the top pending tasks. One JSON value, one read.

### 3. Grab the top task

```bash
farol next <list-id> --json
```

`farol next` picks the highest-priority eligible task (priority, then tree order), assigns it to the calling agent atomically, and prints it. Watch the TUI — the row lights up with the `@claude` tag and a spinner within a second.

### 4. Move it

```bash
farol progress <task-id> --mode percentage --percent 50
farol comment <task-id> "wired the callback, tests passing"
farol <task-id>                     # mark complete — the founding shortcut
```

Every `<task-id>` accepts an unambiguous prefix of the full ULID, so eight characters copied out of any JSON response is enough. Watch the row in the TUI — the percentage moves, the comment lands, and then the task cascades into the Complete section.

### 5. Release presence on the way out

```bash
farol release --all
```

Completing a task auto-unassigns it, but a session that ends without releasing leaves any *other* claim (`farol claim`, or an assignment still in progress) sitting there. `release --all` drops every claim the agent holds.

## That's it

That is the whole loop, from both sides. The next thing to read depends on which side you are on:

- The keyboard, in depth: [The TUI](/users/tui/).
- Setting up an agent to drive farol: [Working with coding agents](/users/agents/).
- Every command and its JSON shape: [The CLI](/users/cli/).
