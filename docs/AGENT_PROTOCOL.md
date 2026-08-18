# Agent interaction protocol

The contract for an agent (or script) driving Farol through its CLI. Farol is
a to-do list with one store and two front ends: a TUI for the human and a CLI
that is the single agent-facing front end. This protocol is the minimal loop
for taking, tracking, and releasing work without colliding with other agents.
`farol agent help` prints the same protocol from the CLI, and `farol skill` is
the full command reference; this document is the canonical, longer-form
version of both.

The six commands below are the whole loop. Everything else in `farol skill`
(read the inbox, comments, the ownership gate, `--json` shapes) layers on top
of this.

## 1. Set your identity first

```bash
export FAROL_AGENT=<unique-tag>
```

Set it **before any farol command**, once per session. The tag is your
identity on two axes:

- **Presence** — the live-claim spinner the TUI shows on what you are
  touching, and the `agent_id` rows `farol work` reports.
- **Assignment** — the `assignee` field on a task, the durable
  "who owns this work" reservation that stops two agents researching the same
  thing.

Without it the CLI falls back to the **shared** tag `agent`, so every
unconfigured agent acts as one agent and overwrites each other's work with no
refusal and no audit trail. Pick a tag that is unique to you and stable for
the whole session — a short name (`pi`, `claude`) or a session id both work.

## 2. Grab the top task

```bash
farol next <list-id> --json
```

`next` is the anti-race grab: it **atomically** selects the top eligible task
in the list, assigns it to you, and claims presence on it — all in one call —
and returns the task's full payload (the same shape `farol show <id> --json`
returns), so grabbing a task and reading it are one call.

Eligibility means all of: `status != complete`, `assignee == ''`, and no
ancestor or descendant assigned to a different agent. Ordering is `priority`
descending (`high` > `medium` > `low` > `none`), then tree (preorder)
position — so the highest-priority task an agent can take is always the one
handed back, and two agents calling `next` on the same list cannot race each
other to the same task.

An empty or exhausted list is **not an error**: it returns
`{"ok": false, "reason": "no eligible task in this list"}` (exit 0). Branch on
that, don't treat it as a failure.

## 3. Update progress as you work

```bash
farol progress <id> --mode <mode> [--percent N]
```

Keep the task's status current as you work — that is the report the human
reads in the TUI. `--mode` is one of:

| mode | meaning | extra |
| --- | --- | --- |
| `simple` | in progress, no number | — |
| `percentage` | an honest 0–100 estimate | requires `--percent <0-100>` |
| `subtasks` | derived from completed children | — |

Setting progress on a pending task starts it (moves it to `in_progress`).
`percentage` reaching 100 does **not** auto-complete; completing is a separate
explicit action (`farol complete <id>`), except in `subtasks` mode, where a
fully-completed child set promotes the parent automatically.

## 4. Complete when done

```bash
farol complete <id> [<id> ...]
```

Marking a task complete cascades to every descendant and auto-unassigns the
task and the whole cascade — so a completed task needs no explicit `unassign`.
`farol <id>` is a shorthand for the same single-task action. Completing is the
explicit counterpart to setting progress: `percentage` at 100 does not
auto-complete, and only `subtasks` mode promotes a parent when all its
children are done.

## 5. Release when done

```bash
farol unassign <id>
```

Clears your assignment on the task, so the next agent's `next` can pick it up
again. `farol unassign --list <list-id>` releases every task in a list.
Completing a task auto-unassigns it and every descendant the cascade
completes, so a completed task needs no explicit unassign — one less step to
forget.

## 6. See who is working on what

```bash
farol work --json
```

Lists every live presence claim: who is on which task or list **right now**.
`--json` emits a bare array of
`{id, entity_type, entity_id, agent_id, kind, acquired_at}` where
`acquired_at` is within the store's 120-second `WorkTTL` — the exact set the
TUI shows a spinner for. `farol work` in human mode prints the same set as a
plain table with the holding agent, the entity, its title, the claim kind, and
the claim's age. An empty claim set is `[]`/nothing — a normal state.

## Presence vs. assignment

The protocol leans on both axes, and confusing them is the most common agent
mistake:

- **Presence** (`farol work`, the auto-claim on writes, `farol claim` /
  `farol release`) is a **UI signal** — the spinner — and **expires** after
  120 seconds of inactivity. It does NOT move a task to `in_progress` and it
  is NOT ownership.
- **Assignment** (`farol next` / `farol assign` / `farol unassign`) is
  **durable ownership** with no TTL; it changes only on an explicit
  assign/unassign/complete. It is what `next` checks for eligibility and what
  stops two agents colliding.

Claiming a task does not assign it, and assigning does not claim it. `farol
show` carries both (`assignee`, `assignee_live`) so you can read either axis.

## Where the rest lives

- `farol skill` — the full command reference: identity, the inbox, the write
  surface, the list-ownership gate, the `--json` contract, and the gotchas.
- `farol inbox` — the start-of-session read: your list plus every foreign
  list, each with its top pending tasks and notes inlined.
- `docs/DESIGN.md` §9 — the complete CLI contract (every command, every
  pinned JSON shape, exit codes).
- `docs/AGENTS.md` — how to work this repository's own task list as an agent.
