# Farol

**A terminal to-do list you can watch, and your agent can steer.**

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
![Status](https://img.shields.io/badge/status-v0.3.0-blue)

<p align="center"><img src="./assets/farol-banner.svg" alt="Farol" width="760"></p>

[**Demo**](#demo) • [**Get Started**](#get-started) • [**Usage**](#usage) • [**For Agents**](#for-coding-agents)

---

## Why Farol for Agents?

Farol isn't just a todo list with a CLI bolted on—it's built from the ground up for **human + agent collaboration**. Here's why it works for agent workflows:

**1. One store, two views.** The TUI and CLI share a single SQLite store. When an agent completes a task via `farol <id>`, the TUI updates within a second. No sync, no polling loops, no "did it save?" uncertainty.

**2. Agent-first CLI with a real JSON contract.** Every subcommand supports `--json` and emits exactly one JSON value on stdout (success or error). No mixed stdout/stderr, no parsing text. `farol show <id> --json` returns the full task tree; `farol tasks <list> --json` returns the full tree. Errors are `{"error": "..."}` with consistent exit codes (0 success, 1 domain error, 2 usage).

**3. Agent identity is first-class.** Set `FAROL_AGENT=claude` (or any tag) and the agent's presence appears as a live spinner on the task row in the TUI. The human sees exactly which task the agent is working on *right now*. `farol work --json` shows all live claims across the store.

**4. `farol next` is the agent's work queue.** It returns the highest-priority eligible task (priority → tree order), auto-claims it, and returns the task JSON. The agent works it, updates progress, then `farol release <id>` (or `farol unassign <id>`) releases it. No manual queue management.

**5. `farol skill` prints a ready-to-paste agent skill file** (markdown) with the full command reference — the identity contract, the working loop, the presence-versus-assignment distinction, and the traps an agent hits on its first run. Drop it into an agent's context and it knows the whole API.

**6. `farol inbox` is the start-of-session context call.** One read returns the agent's own list plus every other list in the store, with pending and complete counts and the top pending tasks in each. The whole board in a single JSON value.

**7. Assignment reserves the subtree.** Taking a task with `farol next` or `farol assign` locks its whole subtree: no other agent can claim an ancestor or a descendant of it. That is how two agents working the same list never end up doing the same work twice.

**8. Structural writes are gated by list ownership.** `add`, `mv`, `rm`, `rename` refuse to run on a list another agent owns; status, progress, and comments stay open to everyone. Agents cooperate on work without reshaping each other's boards.

**9. ID prefixes everywhere.** Every `<task-id>` and `<list-id>` argument accepts an unambiguous ULID prefix. Agents (and humans) copy 8 chars from `farol tasks --json` and it just works.

**10. No server process.** The CLI talks directly to the SQLite store. No daemon, no socket, no extra infrastructure. `farol` is a single binary. Farol shipped an MCP server and retired it — a subprocess and a JSON-RPC handshake to run commands the agent could already run is a moving part with nothing to show for it.

**11. Live presence is visible to humans.** When an agent claims a task, the TUI shows a spinner with the agent's tag on that row. The human sees *which* task is being worked, not just "something changed."

**12. A change feed for polling agents.** `farol diff <list-id> --since <unix-seconds>` returns every task added or changed since a timestamp. Cheap to call, easy to loop on.

**13. Export/Import for handoffs.** `farol export --json` dumps the entire store (or one list) as versioned JSON. `farol import` restores it. Perfect for checkpointing agent sessions or moving work between machines.

**Bottom line:** Farol treats the agent as a first-class user, not an afterthought. The CLI is the agent's API; the TUI is the human's dashboard. They share one truth.

---

## Demo

![Farol Demo](demo/demo.gif)

## Features

- **Watch an agent work.** Leave the TUI open in a pane while
  Claude Code, Pi, or a shell script adds, completes, and updates tasks.
  Watch it happen live. When an agent claims a task, you see **which** row, 
  not just that something changed.

- **A UI that serves both humans and agents.** No agent-specific plumbing
  bolted onto a human app, or a human UI bolted onto an agent tool. The TUI
  and the CLI are two views of **one store**. Any state change one makes, the
  other sees within a second.

- **Tree-structured with derived progress.** Nest tasks to any depth and watch
  percentages compute automatically.

---

## Origin story

This project started from a real workflow problem. When I work with AI coding agents, I found myself jumping between windows constantly. Open a todo app. Add a task when an idea pops into my head. Wait for the agent to finish what it was doing. Check the todo app. Talk to the agent about the next thing. Then manually check that task off my list. As more tasks piled up, that loop got messy fast.

There has to be a better way to do this (I'm absolutely certain there is). Well, anyways... building this has been fun, and I learned a lot about Go along the way. I plan to keep adding features / squashing bugs because this is something genuinely helpful to me. These are the things I think Farol actually brings to the table, beyond being a fast terminal todo list:

## What it does

| Feature | Description |
|---------|-------------|
| **Two-pane layout** | Tasks on the left, lists on the right (toggles with `L`) |
| **Vim + arrow keys** | navigate, `space` toggle, `/` fuzzy search, `F` global search |
| **Nested tasks** | `]` to add a child, `[` to add a sibling of parent |
| **Status model** | `pending`, `in_progress`, `complete` with user % or derived % |
| **Live agent presence** | Animated spinner lights on task writes. You see exactly what's working. |
| **4-value priority** | `high` > `medium` > `low` > `none` (drives `next_task` ordering) |
| **Agent CLI** | a single agent-facing front end: every operation is a `farol` subcommand emitting one JSON value with `--json` |
| **Notes, comments, attachments** | Long-form notes per task, threaded comments, and file attachments (path, stdin, or URL) added via `farol attach`; view and delete them in the details modal |
| **Change feed** | `farol diff <list-id> --since <unix-seconds>` returns tasks added or changed since a timestamp — cheap to poll |
| **Export / Import** | `farol export` / `farol import` (or `e` / `i` in the Lists panel) move lists and tasks between stores as versioned JSON |
| **Themes** | 14 themes: four of the app's own (`farol-*`) plus ten imported community palettes (see `docs/DESIGN.md` §11) |

---

### Screenshots

| Main view | Add task | Search |
|-----------|----------|--------|
|![Main view](demo/screenshot-main.png)|![Add task](demo/screenshot-add.png)|![Search](demo/screenshot-search.png)|
|Tasks (left) + lists (right)|Inline add with level indicator|Global search picker|

| Theme picker | Help | Complete section |
|--------------|------|------------------|
|![Theme picker](demo/screenshot-theme.png)|![Help](demo/screenshot-help.png)|![Complete](demo/screenshot-complete.png)|
|Live theme preview|Full keybinding catalog|Tasks cascade to Complete on `space`|

---

## Get started

### Installation

```bash
# Using Go
go install github.com/filipemolina/farol@latest

# Or build from source
git clone https://github.com/filipemolina/farol.git
cd farol
make build  # installs to ~/go/bin/farol

# Or download a pre-built binary
# See https://github.com/filipemolina/farol/releases
```

### Launch

```bash
farol              # opens the TUI
farol --help       # shows all CLI commands
```

### First run

On first launch, Farol creates:
- Data: `~/.local/share/farol/farol.db` (SQLite store)
- Config: `~/.config/farol/config.yaml` (theme, layout)

The default list `Inbox` is created automatically; the app opens on the
`farol-dusk` theme (switchable with `T`).

---

## Usage

### The TUI

| Keystroke | Action |
|-----------|--------|
| `↑` / `↓` | Navigate tasks |
| `←` / `→` | Collapse or expand the task tree |
| `tab` / `shift+tab` | Cycle panels (tasks and lists). Locked while typing a new task: focus stays on the text input |
| `space` | Toggle task complete (cascades to descendants) |
| `enter` | Show task details |
| `esc` | Close details, picker, or cancel |
| `n` | Start adding a new task (inline) |
| `d` | Delete the selected task |
| `]` | Indent the selected task (re-parent under previous sibling) |
| `[` | Outdent the selected task (promote out of its parent) |
| `alt+↑` / `alt+↓` | Move the selected task up or down among its siblings |
| `u` | Release your assignment on the selected task |
| `U` | Release every assignment you hold on the current list |
| `ctrl+y` | Copy the selected task's id |
| `s` | Sort the current list |
| `/` | Filter current list (fuzzy search) |
| `F` | Global search across all lists |
| `e` | Export the store or highlighted list to JSON |
| `i` | Import lists from a JSON file |
| `T` | Toggle theme picker |
| `L` | Toggle lists panel visibility |
| `a` | Open the About modal |
| `?` | Show help overlay |
| `q` / `Ctrl+C` | Quit |

The details modal (`enter` on a task) has one more tab-cycled zone beyond
Title/Notes/Progress/Priority/Comments: **Attachments**, listing the task's
files. `↑`/`↓` (or `k`/`j`) move the highlight and `d` deletes the selected
one (confirm-guarded). There is no key to *add* an attachment — that's
CLI-only, via `farol attach` (see below).

### The CLI

Every TUI operation is available via CLI commands (`farol --help` lists them all):

```bash
# Lists
farol lists                       # list all lists with counts
farol lists add "Home"            # create a new list
farol lists rename <id> "Garden"  # rename a list
farol lists rm <id>               # delete a list and its tasks

# Tasks
farol tasks <list-id>             # show tasks in a list (tree view)
farol add <list-id> "Buy paint"   # add a root task
farol add <list-id> "Mix colors" --parent <task-id>  # add a subtask
farol show <task-id> --json       # show full task details
farol <task-id>                   # mark task complete (cascades)
farol complete <task-id> [<task-id> ...]  # mark one or more tasks complete (cascades)
farol toggle <task-id>            # complete <-> reopen, whichever applies
farol reopen <task-id> [<task-id> ...]    # reopen one or more complete tasks
farol rename <task-id> "New name" # rename a task
farol notes <task-id> "text..."   # replace a task's notes (whole text)
farol mv <task-id> --parent <id>  # re-parent a task (or --root)
farol rm <task-id>                # delete a task and descendants
farol diff <list-id> --since <unix-seconds>   # tasks added or changed since a timestamp

# Progress & priority
farol progress <task-id> --mode percentage --percent 60
farol progress <task-id> --mode subtasks   # derive % from children
farol progress <task-id> --mode simple     # plain in_progress flag
farol priority <task-id> high    # none | low | medium | high

# Comments
farol comment <task-id> "note"    # add a comment to a task; prints its id
farol comment rm <comment-id>     # delete a comment

# Attachments (CLI-only — adding a file has no TUI keybinding)
farol attach <task-id> <path>     # attach a local file, stored by path (not copied)
farol attach <task-id>            # attach from stdin, e.g. `cat file | farol attach <task-id>`
farol attach <task-id> <url>      # attach an http(s) URL; it's downloaded and stored
farol attachments <task-id>       # list a task's attachments
farol detach <attachment-id>      # remove an attachment

# Assignment & presence
farol next <list-id>              # grab the top eligible task (auto-claims it)
farol assign <task-id>            # assign to current agent; --force takes it
farol unassign <task-id>          # release the current agent's assignment
farol unassign --list <list-id>   # release every task you hold on that list
farol claim <task-id|list-id>     # explicit presence claim (--kind working|inspecting)
farol release <task-id|list-id>   # release presence; --all clears every claim
farol work                        # list every live presence claim

# Agent onboarding
farol inbox                       # your list + every other list, with top pending tasks
farol skill                       # print the agent skill file (markdown)

# Search
farol search "paint"              # fuzzy search across titles and notes
farol search "deck" --json       # JSON output

# Export & import
farol export [list-id] [--out <file>]  # export the whole store or one list
farol import <file> [--list <id>]      # import lists and tasks from an export file

# Global
farol --help                      # full CLI reference
```

### For coding agents

Agents drive Farol through its CLI: every operation is a `farol` subcommand,
and `--json` gives machine-readable output. There is no separate server process
to manage. Agent identity for presence (the TUI spinner) comes from the
`FAROL_AGENT` environment variable; when unset it is generated per process.

```bash
# Teach it: emit the full agent reference and drop it into the agent's context
farol skill > .agent/skills/farol.md

# The whole working loop — plain shell calls:
export FAROL_AGENT=claude
farol inbox --json                       # start-of-session context
farol next <list-id> --json              # grab the top eligible task (auto-claims it)
farol progress <task-id> --mode percentage --percent 50
farol comment <task-id> "tests green"
farol <task-id>                          # mark complete when done
farol release --all                      # drop every presence claim
```

This project ships a CLI-first agent surface: every operation is a `farol`
subcommand, and `--json` emits one JSON value per call, so an agent drives the
store without a server process. The MCP server that previously mirrored these
commands was retired in the cli-first migration — a subprocess and a JSON-RPC
handshake to run commands the agent could already run is a moving part with
nothing to show for it.

`farol skill` is the canonical onboarding: it prints the identity contract, the
working loop, the presence-versus-assignment distinction, the ownership gate,
and the traps an agent hits on its first run. Pipe it into an agent's context
and it knows the whole API.

---

## Project status

Alpha shipped. Phases 0–9 of [`docs/ROADMAP.md`](docs/ROADMAP.md) are
complete (tagged `v0.1.0`). Post-alpha work is at `v0.3.0`.

See [`docs/STATUS.md`](docs/STATUS.md) for what each phase changed and why.

---

## Built with

- [Go](https://go.dev)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) /
  [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- [Cobra](https://github.com/spf13/cobra)
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite): pure Go, no CGO

See [`docs/DESIGN.md`](docs/DESIGN.md) for the architectural rationale.

---

## Contributing

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) before writing code. It is
stricter than typical because this project expects unsupervised agents to
work from the docs alone. No back-and-forth review needed.

If you are an AI coding agent, read [`AGENTS.md`](AGENTS.md) instead: it has
the operational rules, the known hallucination traps in this stack, and the
verification habits the project expects.

---

## License

[MIT](LICENSE). © 2026 Filipe Molina.

---

**Questions?** Open an issue with "[Question]" in the title, or ask in a
discussion.
