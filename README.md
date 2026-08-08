# Chore Crusher

**A terminal to-do list you can watch — and your coding agent can drive.**

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
![Status](https://img.shields.io/badge/status-v0.1.0-blue)

Chore Crusher is a keyboard-driven terminal UI for to-do lists, paired with a
full command-line interface for the exact same operations. Leave the TUI open
in a pane while an agent — Claude Code, Pi, a shell script, whatever runs
`crush` — adds, completes, and updates tasks from the command line, and
watch it happen live. When an agent claims a task or list through the MCP
server, the TUI draws a live spinner on that row — you see *which* row the
agent is on, not just that something changed. Nothing here is agent-specific
plumbing bolted onto a
human app, or a human UI bolted onto an agent tool: the TUI and the CLI are two
views of one store, and every state change either can make, the other can see
within a second.

This is a **sister project to
[stack-stitcher](https://github.com/filipemolina/stack-stitcher)**: same
language, same UI toolkit, same visual language (the theme registry is ported
near verbatim, including the switcher), same architectural discipline — one
keymap package, one Theme in effect, panels sized by a broadcast layout,
request/response commands for every write. Read
[`docs/DESIGN.md`](docs/DESIGN.md) before writing code; it is this project's
`DESIGN.md`, written the same way stack-stitcher's is: it records *why*, not
just what.

## Why this exists

Terminal to-do managers already exist, and most of them are good at what they
do. None of them are good at *this*:

- **Taskwarrior** is the mature, scriptable standard, but subtasks are a
  workaround (dependency links, or the third-party `subtaskwarrior` project),
  not a first-class tree — and its CLI grammar is its own DSL to learn.
- **vit** and **taskwarrior-tui** are TUIs *over* Taskwarrior, not
  general-purpose to-do stores with their own CLI contract.
- **dstask**, **todo.txt-cli** are CLI-first and scriptable, but flat — no
  nesting, no derived progress — and have no live TUI dashboard to leave open.
- **Backlog.md** and **claude-task-master** are the closest in spirit — built
  for AI coding agents, git-native, hierarchical — but they render as
  Markdown/Kanban-in-a-terminal-wrapper, not a persistent, low-latency,
  keyboard-driven TUI a human parks in a pane and glances at.
- **Claude Code's own task tool** (`TodoWrite`/`TaskCreate` et al.) is scoped
  to one session, has no CLI, and nothing to watch it with outside the agent's
  own transcript.

The gap is the combination: **a tree-structured to-do store, with a status
model richer than done/not-done, driven equally well by a human in a TUI and a
script issuing one command at a time — and a dashboard that shows either one's
changes without a restart.** That's what this project is.

It is also, on its own merits, a daily to-do app: two-pane layout (lists on
the left, tasks on the right), vim and arrow-key navigation, fuzzy search,
themes. The agent-facing CLI is not a special mode: it is the same store the
TUI reads, exercised without opening a screen.

Inspired by the shape of GNOME's [Tasks](https://flathub.org/en/apps/dev.edfloreshz.Tasks)
app (lists on the left, tasks on the right, one thing selected at a time) —
this project takes that shape and adds nesting, a derived-progress status
model, and the CLI/agent contract Tasks has no reason to have.

## What it does (planned)

**Two panels.** The lists panel starts hidden on a narrow terminal and shown
on a wide one (120+ columns) — `L` toggles it from either starting point, and
so does picking a list with `enter` or backing out with `esc`, both of which
close the panel and hand focus to the tasks panel. Once toggled, the panel
stays however you left it; a resize never reopens or recloses it on its own.
List management (`n` new, `d` delete, `R` rename) is enabled only while that
panel is visible and focused, so those letters are free for task-level use
everywhere else. The main panel holds two sections, **Pending** and
**Complete**, and a text input fixed to the bottom for adding tasks.

**Nested tasks, added inline.** Type a title and hit `enter` to add it as a
sibling of whatever is selected. `]` before you hit enter adds it as a
*child* of the selection instead — the input nudges right and its leading
glyph changes to say so. `[` goes the other way: a sibling of the
selection's *parent* — and it is a no-op when the new task would land at
the root, because nothing sits above the root. You can move at most one
level either side of the selected task per keystroke of travel; see
[`docs/DESIGN.md` §Adding a task](docs/DESIGN.md#adding-a-task-the-level-rules)
for the exact rule, because "one level" turned out to need a precise
definition once someone builds it.

**A status model richer than a checkbox.** A task is `pending`, `crush`, or
`in_progress` — and `in_progress` comes in three flavors: a plain flag, a
user-set percentage, or a percentage *derived from its subtasks*
(`completed / total`, and reaching 100% that way promotes the task straight to
`crush`). Full rules, including what happens when a derived task has no
subtasks yet, are in `docs/DESIGN.md` — this is the part most likely to be
implemented wrong from a one-line description, so it isn't left as one.

**A details screen per task.** `enter` on a selected task opens it: title,
a notes text box, and its progress settings. `space` toggles complete/pending
without leaving the list.

**Fuzzy search, two scopes.** `/` filters the current list in place. `F`
opens a picker across every list, GNOME-Tasks style, returning both the list
and the task.

**Live agent presence.** An agent driving the MCP server calls `claim_work`
on a task or list while it works; the TUI renders an animated spinner on that
row. Any task write — add, rename, notes, comment, move, or a status/
progress change — lights the spinner too (server-managed `autoClaim`), so
`claim_work` is only needed when an agent wants a claim *before* it starts
writing. The spinner clears when the claim is released or expires after
two minutes of silence.
Status and progress writes keep a live claim alive (a write-heartbeat), and
the skill tells the agent to re-claim after a pause. The lists panel shows
the spinner on any list an agent is working inside, not just on lists claimed
as a whole. The
pane stays open, and you see *which* row the agent is on — not just that the
store changed.

**Fourteen themes**, ported from stack-stitcher's registry — same hex values,
same live-preview picker (`T`), same persisted choice
(`~/.config/chore-crusher/config.yaml`).

## The CLI

Every read and write the TUI can do, the CLI can do in one invocation — this
is the point of the project, not an add-on:

```bash
crush                                       # opens the TUI
crush lists                                 # id, name, pending/complete counts
crush lists add "Home renovation"           # prints the new list's id
crush tasks <list-id>                       # tree view, indented
crush add <list-id> "Buy paint" --parent <task-id>
crush show <task-id> --json                 # title, notes, status, progress, children
crush <task-id>                             # marks complete, cascades to subtasks
crush progress <task-id> --mode percentage --percent 60
crush search "paint" --json
```

`--json` on every read command, on both success and failure, so a script or
an agent parses one shape either way. Full contract:
[`docs/DESIGN.md` §The CLI contract](docs/DESIGN.md#the-cli-contract).

## The MCP server

`crush mcp` runs a [Model Context Protocol](https://modelcontextprotocol.io)
server over stdin/stdout — the same store, exposed to an agent as a surface it
can discover without reading source. Two jobs, one board:

1. **Agent todo store** — the agent tracks its own work on a list named after
   its tag (`my_list` get-or-creates `<tag>: Inbox`).
2. **Human dispatcher** — you create tasks in the TUI or CLI and tell the
   agent to work them; it may complete and set progress anywhere, and may
   restructure only lists it owns.

### Surface

- **12 tools** — the agent-facing surface, mirroring the CLI but talking
  directly to `src/store` and returning the same `--json` shapes. The session
  opener `my_list` returns your list plus every foreign list with
  pending/complete counts; `show_task(ids)` returns full details (including
  comments) for 1–50 tasks in one call; `list_tasks(list_id, since=…)`
  incrementally checks whether a list moved since your last call (scoped per
  list via `updated_at`); `set_status(ids, status?, progress?, percent?,
  comment?)` is the one status/progress write, collapsing N writes into one
  and reopening a complete task before applying progress; `edit_task` merges
  rename + notes + move into a single call; and `claim_work` carries the
  presence trio (working/inspecting, plus `release=true` to stop the
  spinner). All names are server-prefixed (`chore_crusher_<name>`).
- **2 resources** — read-only, URI-addressed, auto-listed by MCP hosts:
  `crush:///inbox` (one-shot start-of-session context: your list + every
  foreign list with top pending tasks and notes) and `crush://work` (the live
  claim set). Five others — `crush:///lists`, `crush:///lists/{id}`,
  `crush:///lists/{id}/tasks`, `crush:///tasks/{id}` and
  `crush:///search/{query}` — were removed as row-for-row duplicates of
  `my_list` / `list_tasks` / `show_task` / `search_tasks`
  (`docs/plan/mcp-assignment-and-priorities.md` §8).
- **2 prompts** — canned workflows with current state already filled in:
  `crush_inbox` (one-shot opener: read `crush:///inbox` and pick the next
  task — fewer round-trips than `my_list` + `list_tasks` + `show_task`
  fan-out; carries the full working loop), and `crush_breakdown` (decompose a
  task into subtasks).

Destructive tools require `force=true`, and every response — success or error
— is one JSON shape, mirroring the CLI contract.

### Ownership and identity

Every list carries a declared owner (`created_by`). The server acts under the
tag in the `CRUSH_AGENT` environment variable (default `agent`), which you set
per server in the MCP client config:

```json
{
  "mcpServers": {
    "chore-crusher": {
      "command": "crush",
      "args": ["mcp"],
      "env": { "CRUSH_AGENT": "pi" }
    }
  }
}
```

Structural writes (adding, renaming, moving or deleting lists, tasks and
notes) are refused on a list owned by another agent or on an untagged one;
reads and status/progress writes are open on every list. The CLI and the TUI
are human surfaces and stay unenforced — so you can drop tasks onto an
agent-owned list from the TUI without fighting the gate.

Omit `agent_id` on `claim_work` — it defaults to the server's own identity
(`CRUSH_AGENT`): status/progress writes keep a live claim alive, but only
when the claim's agent matches the server identity. Set it explicitly only
for a different label (demos, multi-claim); the heartbeat will not refresh
that claim.

Full contract:
[`docs/DESIGN.md` §The CLI contract](docs/DESIGN.md#the-cli-contract).

### Plans and next steps

MCP work is tracked as its own track on the roadmap:

| Status | Plan |
| --- | --- |
| ✅ Done | [`docs/plan/mcp-server-enhancement.md`](docs/plan/mcp-server-enhancement.md) — presence, resources, prompts |
| ✅ Done | [`docs/plan/agent-presence-heartbeat.md`](docs/plan/agent-presence-heartbeat.md) — write-heartbeat + lists-panel aggregation |
| ✅ Done | [`MCP_COMFORT_PLAN.md`](MCP_COMFORT_PLAN.md) — busy-fix, children, `my_list`, always-on instructions |
| ✅ Done | [`docs/plan/list-ownership-enforcement.md`](docs/plan/list-ownership-enforcement.md) — `created_by` enforcement |
| ✅ Done | [`docs/plan/mcp-agent-todo-hardening.md`](docs/plan/mcp-agent-todo-hardening.md) — identity/claim defaults, `my_list` owner match, CLI handoff |

## Status

Alpha shipped — phases 0–9 of `docs/ROADMAP.md` are complete and tagged
`v0.1.0`. The TUI, the CLI, and the MCP server all talk to the same SQLite
store; choose whichever surface fits the caller. The MCP track above is the
active post-alpha focus (agent todo store + human dispatch).

- [`docs/DESIGN.md`](docs/DESIGN.md) — the data model, the state machine, the
  keybinding and focus contract, theming, storage, the CLI contract, and the
  MCP server. *Why* things are shaped the way they are.
- [`docs/ROADMAP.md`](docs/ROADMAP.md) — the shipped alpha, the MCP track, and
  the live post-alpha backlog.
- [`docs/plans/`](docs/plans/) — one file per shipped alpha phase.
- [`docs/plan/`](docs/plan/) — post-alpha feature plans (MCP, UI, …).
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — the build/test loop and the rules
  that keep a contributor (especially an unsupervised one) from drifting from
  the plan above.

## Built with

Go, [Bubble Tea](https://github.com/charmbracelet/bubbletea) /
[Lip Gloss](https://github.com/charmbracelet/lipgloss) for the UI,
[Cobra](https://github.com/spf13/cobra) for the CLI surface,
[modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (pure Go, no CGO) for
storage, and the
[Model Context Protocol Go SDK](https://github.com/modelcontextprotocol/go-sdk)
for the MCP server — one file, one schema, safe under a TUI, a CLI process,
and an MCP client touching it at once. See `docs/DESIGN.md` for why each of
these and not the obvious alternative.

## License

[MIT](LICENSE). © 2026 Filipe Molina.
