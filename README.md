# Chore Completer

**A terminal to-do list you can watch — and your coding agent can drive.**

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
![Status](https://img.shields.io/badge/status-scaffolding-lightgrey)

Chore Completer is a keyboard-driven terminal UI for to-do lists, paired with a
full command-line interface for the exact same operations. Leave the TUI open
in a pane while an agent — Claude Code, Pi, a shell script, whatever runs
`complete` — adds, completes, and updates tasks from the command line, and
watch it happen live. Nothing here is agent-specific plumbing bolted onto a
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

**Two panels.** The lists panel starts hidden — `L` toggles it — because most
sessions live inside one list, and list management (`n` new, `d` delete, `R`
rename) is enabled only while that panel is visible and focused, so those
letters are free for task-level use everywhere else. The main panel holds two
sections, **Pending** and **Complete**, and a text input fixed to the bottom
for adding tasks.

**Nested tasks, added inline.** Type a title and hit `enter` to add it as a
sibling of whatever is selected. `tab` before you hit enter adds it as a
*child* of the selection instead — the input nudges right and its leading
glyph changes to say so. `shift+tab` goes the other way: a sibling of the
selection's *parent*. You can move at most one level either side of the
selected task per keystroke of travel; see
[`docs/DESIGN.md` §Adding a task](docs/DESIGN.md#adding-a-task-the-level-rules)
for the exact rule, because "one level" turned out to need a precise
definition once someone builds it.

**A status model richer than a checkbox.** A task is `pending`, `complete`, or
`in_progress` — and `in_progress` comes in three flavors: a plain flag, a
user-set percentage, or a percentage *derived from its subtasks*
(`completed / total`, and reaching 100% that way promotes the task straight to
`complete`). Full rules, including what happens when a derived task has no
subtasks yet, are in `docs/DESIGN.md` — this is the part most likely to be
implemented wrong from a one-line description, so it isn't left as one.

**A details screen per task.** `enter` on a selected task opens it: title,
a notes text box, and its progress settings. `space` toggles complete/pending
without leaving the list.

**Fuzzy search, two scopes.** `/` filters the current list in place. `F`
opens a picker across every list, GNOME-Tasks style, returning both the list
and the task.

**Fourteen themes**, ported from stack-stitcher's registry — same hex values,
same live-preview picker (`T`), same persisted choice
(`~/.config/complete/config.yaml`).

## The CLI

Every read and write the TUI can do, the CLI can do in one invocation — this
is the point of the project, not an add-on:

```bash
complete                                    # opens the TUI
complete lists                              # id, name, pending/complete counts
complete lists add "Home renovation"        # prints the new list's id
complete tasks <list-id>                    # tree view, indented
complete add <list-id> "Buy paint" --parent <task-id>
complete show <task-id> --json              # title, notes, status, progress, children
complete complete <task-id>                 # marks complete, cascades to subtasks
complete progress <task-id> --mode percentage --percent 60
complete search "paint" --json
```

`--json` on every read command, on both success and failure, so a script or
an agent parses one shape either way. Full contract:
[`docs/DESIGN.md` §The CLI contract](docs/DESIGN.md#the-cli-contract).

## Status

Scaffolding only — phase 0 of `docs/ROADMAP.md`. `go build ./...`, `go vet
./...`, and `go test ./...` all pass against a placeholder `main.go` that
answers `--version` and nothing else; there is no database, no CLI surface
beyond that, and no TUI yet. What exists is the plan a contributor (human or
agent) builds the rest from:

- [`docs/DESIGN.md`](docs/DESIGN.md) — the data model, the state machine, the
  keybinding and focus contract, theming, storage, and the full CLI spec.
  *Why* things are shaped the way they are.
- [`docs/ROADMAP.md`](docs/ROADMAP.md) — the ordered phases from an empty repo
  to a usable alpha, and the decisions already settled that a phase should not
  re-open.
- [`docs/plans/`](docs/plans/) — one file per phase, step by step.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — the build/test loop and the rules
  that keep a contributor (especially an unsupervised one) from drifting from
  the plan above.

## Built with (planned)

Go, [Bubble Tea](https://github.com/charmbracelet/bubbletea) /
[Lip Gloss](https://github.com/charmbracelet/lipgloss) for the UI,
[Cobra](https://github.com/spf13/cobra) for the CLI surface, and
[modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (pure Go, no CGO) for
storage — one file, one schema, safe under a TUI and a CLI process touching it
at once. See `docs/DESIGN.md` for why each of these and not the obvious
alternative.

## License

[MIT](LICENSE). © 2026 Filipe Molina.
