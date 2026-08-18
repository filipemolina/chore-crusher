---
title: Configuration
description: The config file, its fields, and where the SQLite store lives.
sidebar:
  order: 9
---

farol is deliberately low-configuration. There is one persisted preference file with two optional fields, and the database location follows the XDG conventions.

## The config file

farol reads `~/.config/farol/config.yaml` (or `$XDG_CONFIG_HOME/farol/config.yaml` when `XDG_CONFIG_HOME` is set). A missing file is not an error — it falls back to the defaults. A malformed file is reported rather than silently ignored.

```yaml
# ~/.config/farol/config.yaml
theme: farol-dark
poll_interval_ms: 1000
```

| Field | Meaning | Default |
| --- | --- | --- |
| `theme` | The theme to start with. Any of the [14 themes](/users/themes/). | `farol-dark` |
| `poll_interval_ms` | How often the TUI re-polls the store, in milliseconds. | `1000` |

Both fields are optional: a missing file or a missing field falls back to the compiled default. The theme is written here when you confirm a choice in the theme picker (`T`), and loaded before the program starts. The poll interval is how quickly a write from the CLI becomes visible in the TUI — one second by default, since a local SQLite read costs microseconds.

## The database

The store is a single SQLite file at:

```
$XDG_DATA_HOME/farol/farol.db
```

falling back to `~/.local/share/farol/farol.db` when `XDG_DATA_HOME` is unset. It is opened in **WAL journal mode**, which is what lets the TUI's long-lived read connection and a CLI process's short write transaction coexist without blocking each other.

The database is already per-OS-user: the path derives from `$XDG_DATA_HOME` (or `~/.local/share`), which is per-account by definition. Two OS users on the same machine get independent databases with no extra code.

For throwaway data, point `XDG_DATA_HOME` and `XDG_CONFIG_HOME` at a temp directory — the way the demo scripts do — and farol will use those instead.

## The agent identity

`FAROL_AGENT` is the one environment variable that matters at runtime. It is the tag every write is attributed to, and the identity `farol assign` and `farol next` act as. When it is unset, each `farol` process invents its own `agent-<6 hex>` tag — so two consecutive commands act as two different agents. For an agent (or a script) that wants stable identity, export it once per session:

```bash
export FAROL_AGENT=claude
```

The TUI reads `FAROL_AGENT` too, so a human running `farol` also has an identity — useful for `u` / `U` (releasing another agent's stale assignment) and for `farol lists --mine`. See [Working with coding agents](/users/agents/) for the whole model.
