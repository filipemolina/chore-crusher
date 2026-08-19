---
title: Storage and concurrency
description: The SQLite store — the driver, migrations, the data model, ULIDs, and the one-resolution rule.
sidebar:
  order: 7
---

The entire backend is one local SQLite file. `docs/DESIGN.md` §8 is the specification; `src/store` is the only package that implements it.

## The driver: `modernc.org/sqlite`

`modernc.org/sqlite` is a pure-Go SQLite driver — no CGO. That keeps the build story simple: `CGO_ENABLED=0`, cross-compiled linux/darwin × amd64/arm64 by the same GoReleaser shape. A CGO-based driver would be the more common choice by download count, but it would make this the one thing in the whole toolchain that needs a C compiler to cross-compile, for no capability this app uses that the pure-Go driver lacks.

Two traps to know before touching `src/store`:

- **The registered driver name is `"sqlite"`, not `"sqlite3"`.** `sql.Open("sqlite3", …)` fails at runtime with "unknown driver" — it survives `go build` and `go vet` and shows up only when `store.Open` is actually called.
- **`modernc.org/sqlite` pins an exact `modernc.org/libc` version**, and the two are not independently upgradable. Never run a broad `go get -u ./...` or `go get -u all`; bump `modernc.org/sqlite` by name and let `go mod tidy` resolve `libc` from *its* `go.mod`.

## One file, per OS user

The database lives at **`$XDG_DATA_HOME/farol/farol.db`**, falling back to `~/.local/share/farol/farol.db`, opened in **WAL journal mode**. WAL is what lets the TUI's long-lived read connection and a CLI process's short write transaction coexist without either blocking the other — the default rollback-journal mode takes an exclusive lock for the duration of a write, which would stall the TUI's next poll.

The database is **already per-OS-user**: the path derives from `$XDG_DATA_HOME` (or `~/.local/share`), which is per-account by definition. Two OS users on the same machine get independent databases with no extra code.

## The one-resolution rule

**`store.Open` is the one function that opens the database.** Every caller — `main.go`'s TUI path and every CLI subcommand — calls it. Do not open a second `sql.DB` anywhere; a second connection that forgets the WAL pragma is a subtle, load-bearing regression, not a stylistic one.

`store.Open` does three things:

1. Creates the parent directory if missing.
2. Opens the connection with the DSN carrying connection-level settings the driver parses directly off the query string: `?_journal_mode=WAL&_foreign_keys=1&_busy_timeout=5000`.
3. Applies pending migrations, then runs the one-time list-owner backfill.

It also sets `SetMaxOpenConns(1)`, serializing all access within the process on a single connection — the WAL writer lock is process-wide, and with an unlimited pool two agent writes dispatched in one batch would contend for it. Cross-process safety still relies on `_busy_timeout=5000`.

## Migrations

Migrations are numbered `.sql` files embedded via `embed.FS` (`src/store/migrations/0001_init.sql`, `0002_*.sql`, …), applied in order inside `store.Open`, tracked by a `schema_migrations(version integer primary key)` table. Every invocation of the binary — TUI or any CLI subcommand — runs this before touching data, idempotently (a migration that's already applied is a no-op, not an error).

| Migration | Adds |
| --- | --- |
| `0001_init.sql` | The migration tracker, `List`, `Task`, and the tree-read indexes |
| `0002_agent_activity.sql` | Per-entity "an agent is working on this" claims (the TUI spinner), expiring after `WorkTTL` (120 s) |
| `0003_list_owner.sql` | The `created_by` owner tag on lists |
| `0004_task_comments.sql` | The per-task comment thread and the per-list `comments_disabled` flag |
| `0005_list_collaborative.sql` | The per-list opt-in `collaborative` flag |
| `0006_assignment_and_priority.sql` | Durable `assignee`/`assigned_at` and `priority` |
| `0007_settings.sql` | The `Setting` key/value table for app state (e.g. `last_list_id`) |
| `0008_task_attachments.sql` | File-path attachments on tasks |
| `0009_list_archived.sql` | The nullable `archived_at` timestamp on lists |

## The data model

Two entities, `List` and `Task`. A `Task` belongs to exactly one `List` and has at most one parent `Task`; nesting depth is not capped in the schema.

**`List`** — `id` (ULID, primary key), `name`, `created_at` (unix seconds), `position` (manual ordering among lists), `created_by` (declared owner tag; empty = owned by nobody), `archived_at` (unix seconds; null = active — the default. A timestamp rather than a bool so the archive page can sort by archive date for free).

**`Task`** — `id` (ULID), `list_id` (references `List(id)`, on delete cascade), `parent_id` (references `Task(id)`; null = root-level task), `title`, `notes`, `status` (`pending` | `in_progress` | `complete`), `progress_kind` (`none` | `simple` | `subtasks` | `percentage`), `progress_pct` (0–100, set only when `progress_kind='percentage'`), `position` (manual ordering among siblings), `created_at`, `updated_at`, `completed_at` (null unless `status='complete'`), `assignee` (agent tag; `''` = no assignment, no TTL), `assigned_at` (null unless `assignee != ''`), `priority` (`none` | `low` | `medium` | `high`).

## ULIDs for ids

Task and list ids are handed to the CLI as arguments (`farol <task-id>`) and printed by `add`. A ULID (`store.NewID`) is a stable, copy-pasteable, sortable-by-creation-time string that never collides across a `list add` and a concurrent `task add` from two processes. It is 26 Crockford-base32 characters (minus the confusable `I`, `L`, `O`, `U`): the first 10 encode the Unix-millisecond timestamp, the last 16 encode 80 random bits. A ULID also lets `CreateTask` generate the id **before** its transaction opens, so the caller gets it back without a re-query.

Ids are **not** meant to be typed from memory. The CLI accepts an unambiguous *prefix* of an id, resolved by `store.ResolveID` against the relevant table. A prefix matching zero rows is a not-found error; one matching more than one row is an **ambiguous error** — a caller must never resolve that by guessing (`docs/DESIGN.md` §9).

## The store owns every transition

`store.Complete`, `store.Reopen`, and `store.SetProgress` are the only three functions that write `status`/`progress_kind`/`progress_pct`, and every caller — CLI subcommand or TUI keypress handler — goes through them. `store.Toggle` delegates to whichever of the two applies. The invariant is enforced by Go visibility: these three are the only exported mutators. The full state machine — auto-completion of `subtasks` parents, the zero-children fallback, the cascade down / no-cascade-up asymmetry — lives here, in `src/store/state.go`, not in any front end. See [Core concepts](/contributors/core-concepts/) for the rules.

## Config

User preferences live in **`~/.config/farol/config.yaml`** (or `$XDG_CONFIG_HOME`), read and written by `src/config`:

```yaml
theme: farol-dark
poll_interval_ms: 1000
```

Both fields are optional; a missing file or a missing field falls back to the compiled default, and a malformed file is reported rather than silently ignored. The struct is designed to grow — add a field, tag it, and `LoadConfig`/`SaveConfig` round-trip it automatically.

App state (as opposed to user preferences) lives in the `Setting` key/value table in the same SQLite file (migration `0007_settings.sql`), read and written only through `store.GetSetting`/`store.SetSetting`. The one key today is `last_list_id`, the list the TUI reopens at startup — written only when the active list actually changes, never by the poll, so the poll stays a pure read.