---
title: Architecture
description: How the layers fit together — the two halves, the request/response split, the poll loop, and the concurrency model.
sidebar:
  order: 2
---

Farol's architecture is the answer to one question: *how do a human TUI and an agent-driven CLI stay two views of one store without drifting apart?* The answer, stated in `docs/DESIGN.md` §10, is a strict layering with one boundary.

## Two halves, one store

The package layout is split into two halves plus one addition:

- **The Bubble Tea half** — `src/model`, `src/components`, `src/cmds`, `src/keys`, `src/appstyles`, `src/constants`. This is the TUI: models, rendering, messages, keybindings, theming, layout constants.
- **The non-Bubble-Tea half** — `src/store`, `src/apptypes`, `src/config`. This is the data layer: SQLite, the shapes components pass around, and user preferences. No terminal, no Bubble Tea.
- **`src/cli`** — the agent-facing front end, one file per subcommand group.

The boundary between the halves is `src/store`:

> **`src/store` is the only package that imports `database/sql` or `modernc.org/sqlite`.** `src/model` (the TUI) and `src/cli` (the CLI) both depend on `store` and nothing deeper; neither ever builds a SQL string. — `docs/DESIGN.md` §10

`src/apptypes` is the shape language that crosses the boundary. The store returns its own row types (`store.Task`, `store.List`); `cmds.RefreshLists` and `cmds.RefreshTasks` convert them to `apptypes` at the boundary, and components never hold a `store` type. The conversion is a function (`apptypes.FromStore`), not a type alias, so a field added to one side cannot silently leak to the other.

## Siblings, not layers

`src/model` and `src/cli` are **siblings over the same `store`, not layered on each other** (`docs/DESIGN.md` §10). Neither is "the real app" with the other bolted on:

- `src/model` never imports `src/cli`.
- `src/cli` imports `src/model` in exactly one place — the cobra root command's `RunE`, which launches the TUI when no subcommand is given. CLI subcommands themselves never touch `model`.
- Both import `src/store` and nothing deeper.

This is the structural expression of "neither front end is secondary" (`docs/DESIGN.md` §1): the TUI does not shell out to the CLI, and the CLI is not a read-only reporting layer over a TUI-owned database. Both call the same `store` functions, and a write from either is visible to the other within one poll tick.

## main.go: the one decision

`main.go` is a two-line shim:

```go
func main() {
	os.Exit(cli.Execute(os.Args[1:]))
}
```

The TUI-vs-CLI decision lives in the cobra root command (`src/cli/root.go`), which `cli.Execute` runs:

- **No subcommand** → launch the TUI: `store.Open(config.DBPath())`, `config.LoadConfig()`, `appstyles.SetTheme(cfg.Theme)`, then `model.GetInitialModel(s, cfg)` and `tea.NewProgram(m).Run()`.
- **A single ULID-looking argument** (`^[0-9A-Z]{1,26}$`) → mark that task complete, the founding example the whole project was scoped around.
- **Anything else** → dispatch to a subcommand (`lists`, `tasks`, `add`, `show`, `search`, `inbox`, `work`, `export`, `import`, …).

`cli.Execute` owns the exit-code contract: `0` success, `1` domain failure, `2` usage error (`docs/DESIGN.md` §9).

## The request/response split

Inside the TUI, components never touch the store. The pattern, described in `docs/DESIGN.md` §3 and §5, is:

1. A component (say, the task tree) matches a keypress against a binding in `src/keys` and **emits an intent** — a message type from `src/cmds`, e.g. `cmds.ToggleTaskMsg{TaskID: …}`.
2. `AppModel` (the top-level model in `src/model`) receives the message, **resolves it against the store** — calling `store.Toggle(taskID)`, `store.DeleteTask(id)`, `store.UnassignTask(...)`, and so on — and reports failures through `lastError`.
3. AppModel then issues a refresh command (`cmds.RefreshTasks`), which re-queries the store and routes the fresh rows back to the component.

The tree only asks; AppModel executes. This is the same shape for `space` (toggle complete), `d` (delete), `u`/`U` (release assignment), `enter` (open Details), and every other store-backed write. The rule that keeps the CLI and TUI from diverging on a write path: **every mutation goes through `src/store`**, and neither `src/cli` nor `src/model` contains logic that decides *whether* a transition is allowed — only logic that calls the `store` function and reports the result.

```mermaid
sequenceDiagram
    participant User
    participant Tree as tasktree (component)
    participant App as AppModel (src/model)
    participant Store as src/store
    participant DB as SQLite (WAL)

    User->>Tree: space
    Tree->>App: ToggleTaskMsg{TaskID}
    App->>Store: store.Toggle(taskID)
    Store->>DB: BEGIN; UPDATE Task SET status='complete' ...; COMMIT
    Store-->>App: nil (or error → lastError)
    App->>App: cmds.RefreshTasks(store, activeListID)
    App->>Store: store.ListTasks(listID) + store.ListWork()
    Store-->>App: rows
    App-->>Tree: RefreshTasksMsg{Rows}
    Tree-->>User: re-render
```

## The poll loop: how the TUI sees the CLI's writes

There is no daemon, no socket, no file watcher — **the TUI polls** (`docs/DESIGN.md` §7).

`cmds.PollTick` wraps `tea.Tick(interval, …)`; the interval comes from `config.PollInterval(cfg)`, which is the config's `poll_interval_ms` when set and **1000 ms** by default. On every `PollTickMsg`, `AppModel`:

1. Re-issues `PollTick` — that self-rescheduling is what makes the poll recurring for the life of the app.
2. Re-runs exactly two queries: **list summaries** (`cmds.RefreshLists` → `store.ListLists`, plus `store.ListWork` for live agent claims and `store.ClaimedTaskListIDs`) and the **active list's task tree** (`cmds.RefreshTasks` → `store.ListTasks`, `store.ListWork`, `store.TaskIDsWithComments`).
3. While the Details modal is open, also refreshes it (`cmds.RefreshDetails`), so external CLI writes stay current.

The results are diffed against what's rendered. On no change, nothing re-renders. The diff matters for one thing that isn't free: **cursor position**. A poll that finds the previously selected task still present keeps the cursor on it — matched by **id, not row index**, because a CLI insert or delete during the interval can move every row index without moving the task the user was looking at. A poll that finds the selected task gone moves the cursor to the nearest surviving row.

```mermaid
sequenceDiagram
    participant Tick as tea.Tick
    participant App as AppModel
    participant Store as src/store
    participant DB as SQLite

    loop every poll_interval_ms (default 1000)
        Tick->>App: PollTickMsg
        App->>App: re-issue PollTick (self-rescheduling)
        App->>Store: ListLists() + ListWork() + ClaimedTaskListIDs()
        Store->>DB: SELECT ...
        DB-->>Store: rows
        Store-->>App: RefreshListsMsg
        App->>Store: ListTasks(activeListID) + ListWork() + TaskIDsWithComments()
        Store-->>App: RefreshTasksMsg
        App-->>App: diff vs rendered; keep cursor by id
    end
```

The very first load is animated, later polls are not: `GetInitialModel` does no database work, so Bubble Tea paints the first frame before the opening `RefreshLists` query completes. The Tasks panel renders a sealed `Loading` label with an animated ellipsis until that first refresh lands.

## Concurrency: one reader, short writers

The TUI and the CLI share one SQLite file across processes, so the concurrency model is explicit (`docs/DESIGN.md` §7–8):

- **The TUI holds a read connection for the process's lifetime.** Every poll is a `SELECT`, full stop.
- **All writes go through `store` functions**, each wrapping one short transaction that opens, writes, commits, and returns. A rapid-fire agent loop calling `farol <task-id>` in a shell `for` loop is never waiting behind the TUI, and the TUI is never waiting behind it.
- **SQLite's WAL journal mode** is what lets a long-lived reader and a short writer coexist without blocking each other. The DSN carries it: `?_journal_mode=WAL&_foreign_keys=1&_busy_timeout=5000`. Do not disable it.
- Within one process, `store.Open` sets `SetMaxOpenConns(1)`, serializing access on a single connection so two agent writes dispatched in one batch cannot contend for the WAL writer lock.

## The one-resolution rule

**`store.Open` is the one function that opens the database** (`docs/DESIGN.md` §8). Every caller — `main.go`'s TUI path and every CLI subcommand — calls it. A second `sql.DB` opened anywhere else is a subtle, load-bearing regression: a connection that forgets the WAL pragma would stall the TUI's next poll for the length of a write. The same "one resolution, passed down" shape applies to migrations — applied inside `store.Open`, idempotently, on every invocation of the binary — and to the config path, which `src/config` resolves once so the TUI and every subcommand agree on where the database lives.