---
title: Working with coding agents
description: The whole story — farol skill, FAROL_AGENT, the working loop, presence vs. assignment vs. ownership, subtree reservation, and the traps an agent hits on its first run.
sidebar:
  order: 6
---

farol was built so a coding agent can drive it. This is the page for the human doing the setup and for the person answering "so why not just an MCP server / a markdown checklist / the agent's own todo list?". If you want the shape of every command, jump to [The CLI](/users/cli/); this page is why you'd string them together.

## Ten seconds: teach the agent

```bash
farol skill > .agent/skills/farol.md
```

`farol skill` prints the complete agent-facing reference as markdown — the identity contract, the session-opener reads, every write, the presence-versus-assignment distinction, the ownership gate, and the traps. Pipe it into your agent's context (a skill file, a system prompt, a preface to the first turn) and it knows the whole API. If the CLI changes, so does the output — the reference is generated from the same source the code is.

There is no other setup. No MCP server to install, no config file to write, no daemon to start.

## Identity: FAROL_AGENT

Every write is attributed to an agent identity. It comes from the `FAROL_AGENT` environment variable.

```bash
export FAROL_AGENT=claude
```

That tag is what appears on the row in the TUI when the agent claims a task, and it is how `farol next` and `farol assign` know who is asking. **If you do not set it, every `farol` process invents its own `agent-<6 hex>` tag** — so two consecutive commands act as two different agents and never see each other's assignments or presence. Set it once per session and forget about it.

The tag has no registration and no permission model. Any string that is a valid identifier works. Two agents wanting to distinguish themselves pick two different tags; two shells wanting to act as the same agent share one tag.

## The working loop

Everything the agent does fits in these calls. Copy this into a runbook once and keep it there.

```bash
export FAROL_AGENT=claude

# 1. Read the whole board.
farol inbox --json

# 2. Grab the top eligible task on a list.
farol next <list-id> --json

# 3. Report progress as it happens.
farol progress <task-id> --mode percentage --percent 50
farol comment <task-id> "callback verified against staging"

# 4. Mark it complete when done.
farol <task-id>

# 5. On the way out, drop every claim.
farol release --all
```

That is the loop. The five commands above are the reason `farol skill` exists — they are the shape of a session, in order.

### `farol inbox` — the session opener

One call returns the agent's own list plus every other list in the store, each with pending and complete counts and the top pending tasks in each. That is the whole board, in a single JSON value.

```json
{
  "mine": {"list": {...}, "pending": [...]},
  "foreign_lists": [
    {"list": {...}, "pending_count": 7, "complete_count": 2, "pending": [...]}
  ]
}
```

`farol inbox --include notes` widens the top-pending rows to carry their notes bodies too, for when the agent needs to reason about the *content* of what's ahead, not just the titles.

### `farol next` — the work queue

`farol next <list-id>` picks the highest-priority eligible task (priority: `high` > `medium` > `low` > `none`; then tree order), assigns it to the calling agent atomically, and prints it as JSON. Eligible means: pending, not already assigned to someone else, and no ancestor or descendant assigned to someone else (see [Subtree reservation](#subtree-reservation) below).

An empty board is a normal state, not an error. `farol next` on a list with no eligible task returns `null` with exit code `0`.

### `farol work` — see who is on what

`farol work` lists every live presence claim, across every list, right now.

```bash
$ farol work --json
[
  {"entity_type": "task", "entity_id": "01AS1F3K…", "agent_id": "claude", "kind": "working", "acquired_at": 1723856400},
  {"entity_type": "task", "entity_id": "01AS22PJ…", "agent_id": "codex",  "kind": "working", "acquired_at": 1723856410}
]
```

An empty result is `[]`, exit `0`.

### `farol release --all` — clean exit

Every write auto-claims presence for the calling agent, so a spinner lights on the row without the agent doing anything for it. A claim's TTL is 120 seconds; if the agent leaves without releasing, the claim naturally times out. But assignment does not — it has no expiry and no background sweeper. `farol release --all` clears every presence claim the agent holds; completing a task auto-unassigns it (and every descendant the cascade completes), but any *other* assignment sits there until released explicitly.

## Presence vs. assignment vs. ownership

Three concepts, three axes. Do not confuse them.

| Axis | Set by | Cleared by | What it means |
| --- | --- | --- | --- |
| **Presence** | any write (auto) or `farol claim` | `farol release`, `--all`, or a 120s TTL | *"an agent is here right now."* The TUI spinner. Never blocks another agent. |
| **Assignment** | `farol assign` or `farol next` | `farol unassign`, completion, or the human's `u`/`U` in the TUI | *"this agent owns this task."* Reserves the subtree. |
| **Ownership** | list creation with `--owner`, or `farol lists rename` | list rename with an empty owner | *"this agent owns this whole list."* Gates structural writes on the list (see [The ownership gate](#the-ownership-gate) below). |

Presence is a UI signal; assignment is a reservation; ownership is a structural gate. Reading them:

- `farol show <task-id>` carries `assignee` and `assignee_live` (an assignee that has no live presence — the red-badge case in the TUI).
- `farol work` lists live presence claims only.
- `farol lists --mine` / `--foreign` filter lists by ownership.

## Subtree reservation

The single fact that makes multi-agent coordination on one list *safe*: assigning a task reserves its whole subtree.

If `claude` holds `Ship auth v2`, then no other agent can take:

- an **ancestor** of it (there is none here — it is a root — but if it had one, that would be locked too),
- a **descendant** of it (`Wire the OAuth callback`, `Migrate the sessions table`, or any of their children),
- or the task itself.

`farol next` respects that rule when picking; `farol assign` refuses with an error naming the holder and how long they've held it; `farol assign --force` takes it, reassigns, and writes a takeover comment on the task so the audit trail is not silent.

That is how two agents work the same list without ever ending up on the same work twice.

## The ownership gate

Structural writes — `add`, `rename`, `notes`, `mv`, `priority`, `rm` — refuse to run on a list owned by another agent unless you pass `--force`. Status writes, progress writes, assignment, comments, and every read are ungated: any agent may cooperate on the work without needing permission to reshape it.

An untagged list (created by a human in the TUI, no `--owner`) is foreign to every agent: read + status/progress only. The human can mark a list *collaborative* through the Rename modal (`R`, then `space`) to open structural writes to any agent — the switch is a single boolean stored on the list row.

## Change detection

A polling agent — a background watcher, a status bot, a CI hook — needs a cheap way to ask "what changed since I last looked?". That is `--since`:

```bash
farol tasks <list-id> --since <unix-seconds>
farol diff <list-id> --since <unix-seconds>
```

Both return only rows whose `updated_at` moved past the given timestamp, and widen the default status filter to `all`. Deletions are not representable by a row filter — diff the id sets between two reads to detect those.

## Why no MCP server

farol shipped an MCP server in an earlier version and retired it. The reasoning is stated plainly in the code and the README: running a subprocess and a JSON-RPC handshake so an agent can invoke a command it could already invoke is a moving part with nothing to show for it.

Every capability the MCP tools exposed — `farol_inbox`, `farol_breakdown`, presence and assignment — is now a plain `farol` subcommand. Any agent that can run a shell command can drive farol. Any language that can `exec` a subprocess and parse JSON can wrap it. The MCP layer is not on the roadmap; the CLI is the API.

If your agent framework prefers MCP, wrap the CLI: a one-file shim that shells out and forwards the JSON.

## Traps on the first run

Every trap below has caught someone. They are worth learning once.

- **You forgot `FAROL_AGENT`.** Every command is a new random agent, so your second `farol` doesn't see the assignment your first `farol` made. Fix: `export FAROL_AGENT=<tag>` once per session.
- **You copied too short an id prefix.** An 8-character ULID prefix is almost always unambiguous; a 4-character one often isn't. Ambiguous prefixes are a hard error (exit `1`), not a silent pick — copy more of the id.
- **You expected `--force` behavior by default.** `farol rm`, `farol lists rm`, and `farol comment rm` refuse without `--force`. The flag *is* the confirm modal.
- **You tried to write on a foreign list without `--force`.** `add`/`rename`/`notes`/`mv`/`priority`/`rm` on a list owned by another agent is refused. Either the list needs an owner change, or the human needs to mark it collaborative, or the write needs `--force`.
- **You expected a percentage at 100 to auto-complete.** `percentage` at 100 does not — it's an estimate, not a fact. Use `subtasks` mode (where the store can verify), or call `farol <id>` explicitly.
- **You confused presence with assignment.** A presence claim (from `farol claim`, or the auto-claim on any write) lights the TUI spinner. It does *not* move a task to `in_progress` and it is *not* ownership. Use `farol assign` (or `farol next`) to actually own a task.
- **You are calling `farol show <bad-id>`.** A single bad id is a hard failure (exit `1`); with multiple ids, a bad id returns a per-row `{id, error}` and the rest still succeed. That is deliberate — bulk callers do not want one typo to lose the batch.

## A minimal wrapper, in shell

For the reader who wants to see it as ~10 lines of shell. This is what a "poll the store, work the top task, loop" agent looks like without any framework.

```bash
#!/usr/bin/env bash
set -euo pipefail
export FAROL_AGENT=${FAROL_AGENT:-worker}
LIST=$1

while :; do
  task=$(farol next "$LIST" --json)
  [[ "$task" == "null" ]] && { sleep 5; continue; }
  id=$(jq -r '.id' <<<"$task")
  title=$(jq -r '.title' <<<"$task")

  farol comment "$id" "$FAROL_AGENT picked up: $title"
  # ... do the work ...
  farol "$id"
done
```

That is the whole loop. Every capability on this page is one `farol` call away.

## Where to next

- [The CLI](/users/cli/) — every command with its `--json` shape and exit code
- [The TUI](/users/tui/) — what the human sees while the agent works
- [Troubleshooting](/users/troubleshooting/) — including the red-badge stale-assignment case
