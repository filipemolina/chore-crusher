# Chore Crusher

**A terminal to-do list you can watch — and your coding agent can drive.**

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
![Status](https://img.shields.io/badge/status-v0.1.0-blue)

[**Demo**](#demo) • [**Get Started**](#get-started) • [**Usage**](#usage) • [**For Agents**](#for-coding-agents)

---

## Why this exists

Terminal to-do managers already exist, and most of them are good at what they do.
None of them are good at **this**:

- **You want to watch an agent work** — Leave the TUI open in a pane while
  Claude Code, Pi, or a shell script adds, completes, and updates tasks.
  Watch it happen live. When an agent claims a task, the TUI draws a spinner
  on that row — you see **which** row, not just that something changed.

- **A UI that serves both humans and agents** — No agent-specific plumbing
  bolted onto a human app, or a human UI bolted onto an agent tool. The TUI
  and the CLI are two views of **one store**, and every state change either
  makes, the other can see within a second.

- **Tree-structured with derived progress** —nest tasks to any depth and watch
  percentages compute automatically (`completed / total`), so long tasks
  naturally break down while you work.

**This is the gap.** Taskwarrior is scriptable but has a custom DSL. DSTask
and todo.txt are CLI-first but flat. Backlog.md and claude-task-master are
agent-native but render as Markdown, not a persistent TUI you can leave open.
Claude Code's task tool is scoped to one session, has no CLI, and nothing to
watch it with outside the transcript.

Chore Crusher deliver**s the combination**: a keyboard-driven terminal UI with
a server that speaks JSON and MCP, both accessing the same SQLite store,
updated live in both views.

---

## What it does

| Feature | Description |
|---------|-------------|
| **Two-pane layout** | Lists on the left, tasks on the right (toggles with `L`) |
| **Vim + arrow keys** | navigate, `space` toggle, `/` fuzzy search, `F` global search |
| **Nested tasks** | `]` to add a child, `[` to add a sibling of parent |
| **Status model** | `pending`, `in_progress`, `complete` with user % or derived % |
| **Live agent presence** | Animated spinner lights on task writes — you see exactly what's working |
| **4-value priority** | `high` > `medium` > `low` > `none` (drives `next_task` ordering) |
| **MCP server** | 12 tools + 2 resources + 2 prompts for discoverable agent integration |
| **Themes** | 14 themes ported from [stack-stitcher](https://github.com/filipemolina/stack-stitcher) |

**A note about design** — This project is a sister to
[stack-stitcher](https://github.com/filipemolina/stack-stitcher): same
language, same UI toolkit, same visual language (the theme registry is ported
near verbatim), same architectural discipline: one keymap package, one Theme
in effect, panels sized by a broadcast layout. Read
[`docs/DESIGN.md`](docs/DESIGN.md) before writing code — it is this project's
DESIGN.md, written the same way stack-stitcher's is: it records *why*, not
just what.

---

## Demo

Watch an agent complete a task, add a nested subtask, change the theme, and
search globally — all in under 30 seconds.

![Chore Crusher Demo](demo/demo.gif)

**What you're seeing:**
1. Complete a task + descendants at once (`space`) → cascades back to pending
2. Add a nested task (`n` → `]` → `Enter`) — grows as a child of "Plan the garden"
3. Switch themes live (`T` → cursor → `Esc`) — 14 themes including catppuccin-mocha
4. Global search (`F` → type) — finds "trellis" across all lists and jumps to it

See the [VHS tape](demo/demo.tape) for the exact keystrokes.

### Screenshots

| Main view | Add task | Search | Theme picker |
|-----------|----------|--------|--------------|
|![Main view](demo/screenshot-main.png)|![Add task](demo/screenshot-add.png)|![Search](demo/screenshot-search.png)|![Theme picker](demo/screenshot-theme.png)|
|`Lists` + `Tasks` panels|Inline add with level indicator|Global search picker|Live theme preview |

| Help | Complete section |
|------|------------------|
|![Help](demo/screenshot-help.png)|![Complete](demo/screenshot-complete.png)|

---

## Get started

### Installation

```bash
# Using Go
go install github.com/filipemolina/chore-crusher@latest

# Or build from source
git clone https://github.com/filipemolina/chore-crusher.git
cd chore-crusher
make build  # installs to ~/go/bin/crush

# Or download a pre-built binary
# See https://github.com/filipemolina/chore-crusher/releases
```

### Launch

```bash
crush              # opens the TUI
crush --help       # shows all CLI commands
```

### First run

On first launch, Chore Crusher creates:
- Data: `~/.local/share/chore-crusher/chore-crusher.db` (SQLite store)
- Config: `~/.config/chore-crusher/config.yaml` (theme, layout)

The default list `Inbox` is created automatically under `catppuccin-mocha`
theme.

---

## Usage

### The TUI (terminal UI)

| Keystroke | Action |
|-----------|--------|
| `↑` / `↓` | Navigate tasks |
| `←` / `→` | Navigate panels (lists ↔ tasks) or collapse/expand |
| `space` | Toggle task complete (cascades to descendants) |
| `enter` | Show task details |
| `esc` | Close details panel / switch back to lists panel |
| `n` | Start adding a new task (inline) |
| `]` | Set next added task as **child** of selected |
| `[` | Set next added task as **sibling of parent** |
| `/` | Filter current list (fuzzy search) |
| `F` | Global search across all lists |
| `T` | Toggle theme picker |
| `L` | Toggle lists panel visibility |
| `?` | Show help overlay |
| `q` / `Ctrl+C` | Quit |

### The CLI (command line interface)

Every TUI operation is available via CLI commands:

```bash
# Lists
crush lists                       # list all lists with counts
crush lists add "Home"            # create a new list
crush lists rename <id> "Garden"  # rename a list
crush lists delete <id>           # delete a list

# Tasks
crush tasks <list-id>             # show tasks in a list (tree view)
crush add <list-id> "Buy paint"   # add a root task
crush add <list-id> "Mix colors" --parent <task-id>  # add a subtask
crush show <task-id> --json       # show full task details
crush <task-id>                   # mark task complete (cascades)
crush reopen <task-id>            # reopen a complete task

# Progress
crush progress <task-id> --mode percentage --percent 60
crush progress <task-id> --mode subtasks   # derive % from children
crush progress <task-id> --mode simple     # plain in_progress flag

# Search
crush search "paint"              # fuzzy search
crush search "deck" --json        # JSON output

# Global
crush --help                      # full CLI reference
```

### The MCP server (for coding agents)

Run `crush mcp` to start an MCP server that agents can discover and use:

```bash
# Claude Code (claude.json)
{
  "mcpServers": [
    {
      "name": "chore-crusher",
      "command": "crush",
      "args": ["mcp"],
      "env": { "CRUSH_AGENT": "claude" }
    }
  ]
}

# Cursor (settings.json)
{
  "mcpServers": {
    "chore-crusher": {
      "command": "crush",
      "args": ["mcp"],
      "env": { "CRUSH_AGENT": "cursor" }
    }
  }
}
```

### For coding agents

Chore Crusher doubles as an agent's todo store. Every MCP tool maps to a CLI
command, exposing:

- **12 tools** — `list_tasks`, `show_task`, `assign_task`, `next_task`,
  `set_status`, `edit_task`, `add_task`, `comment`, `search_tasks`, etc.
- **2 resources** — `crush:///inbox`, `crush://work`
- **2 prompts** — `crush_inbox`, `crush_breakdown`

The agent acts under an identity tag (configured via `CRUSH_AGENT`). Each
agent gets its own automatically-named list (`<tag>: Inbox`) and can only
modify lists it owns — reads are open across all lists.

**Working loop:**
1. `my_list` — get your list plus all others with counts
2. `list_tasks` on your list
3. `next_task(<list_id>)` — atomically grab the top-priority task
4. `set_status(ids, progress=...)` — start working (lights your spinner)
5. Update as you go — progress writes keep your claim alive

Full MCP contract: [`docs/DESIGN.md`](docs/DESIGN.md) §7 and §9.

---

## Project status

 Alpha shipped — phases 0–9 of [`docs/ROADMAP.md`](docs/ROADMAP.md) are
complete and tagged `v0.1.0`.

See [`docs/STATUS.md`](docs/STATUS.md) for what each phase changed and why.

---

## Built with

- [Go](https://go.dev) — the language
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) /
  [Lip Gloss](https://github.com/charmbracelet/lipgloss) — the TUI toolkit
- [Cobra](https://github.com/spf13/cobra) — the CLI surface
- [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) — pure Go, no CGO
- [Model Context Protocol Go SDK](https://github.com/modelcontextprotocol/go-sdk) — MCP server

See [`docs/DESIGN.md`](docs/DESIGN.md) for the architectural rationale.

---

## Contributing

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) before writing code. It is
stricter than typical because this project expects unsupervised agents to
work from the docs alone — no back-and-forth review.

---

## License

[MIT](LICENSE). © 2026 Filipe Molina.

---

**Questions?** Open an issue with "[Question]" in the title, or ask in a
discussion.
