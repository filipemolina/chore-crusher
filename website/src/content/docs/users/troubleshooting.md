---
title: Troubleshooting
description: Common problems and their fixes — terminal size, themes, auto-completion, JSON output, and stale assignments.
sidebar:
  order: 10
---

## "Terminal too small"

Below **40 columns by 10 rows** farol stops attempting the layout and renders a single centred `Terminal too small` line — nothing else, not even an open modal. Grow the terminal back over the threshold and the real layout returns on the next resize. 40 columns is where a task row still seats a checkbox and a title at its minimum width.

## The theme I set is not applied

The theme is read from `~/.config/farol/config.yaml` (or `$XDG_CONFIG_HOME/farol/config.yaml`) at startup. A missing config falls back to the default (`farol-dark`), and an unknown theme name is skipped the same way.

- Check the file exists and the `theme:` value matches a registered theme name exactly — see [Themes](/users/themes/).
- Set it from inside the app instead: `T`, pick a theme, `Enter` — that writes the config for you.

## A task is not auto-completing at 100%

Auto-completion is deliberately asymmetric:

- A task in **`subtasks`** mode auto-completes the moment every direct child is complete — that's a verified fact.
- A task in **`percentage`** mode at 100 does **not** auto-complete — it's a claim, not a verified fact. Completing is a separate, explicit action: `space` in the TUI, `farol <id>` on the CLI.

If a subtasks-mode parent is not completing, check its *direct* children (not the whole subtree) — the percentage is derived only from children one level down, and a task with zero children displays and behaves as `simple` until the first child exists.

## The CLI's `--json` output looks wrong

In `--json` mode stdout is **always exactly one JSON value**, whether the command succeeded or failed. On failure it is `{"error": "…"}`; on success it is the command's payload. Nothing else is written to stdout in that mode.

To tell the two apart, read the **exit code**: `0` success, `1` a domain failure (id not found, invalid state transition), `2` a usage error. An empty read result prints `[]` (or nothing in human mode) with exit 0 — that's "no data", not "failed".

## A red `@tag` badge won't go away

A `@tag` in red means **stale assignment**: the task's assignee has no live presence claim — an agent session that was killed before it could release its work. Assignment has no expiry and no background sweeper, so nothing auto-releases it; the red badge is the signal for a human to decide.

`u` releases the selected task's assignment (unconditional, prompts for nothing). `U` releases every assignment in the active list, through a confirm modal. Completing a task also auto-unassigns it and every descendant the cascade completes.

## Something looks stale

The TUI re-polls the store every second (configurable via `poll_interval_ms`), so a write from the CLI — or from another terminal — appears within one poll tick. If a panel still looks wrong, the cursor follows the previously selected task by id, so a CLI insert or delete that shifts every row index doesn't move what you were looking at.

## Still stuck?

Open an issue at [github.com/filipemolina/farol](https://github.com/filipemolina/farol). Include the output of `farol --version` (or the commit hash it reports on an unstamped build) and what you were doing when it failed.
