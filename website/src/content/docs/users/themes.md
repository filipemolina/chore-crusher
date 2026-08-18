---
title: Themes
description: The fourteen themes, how to switch them, and how to set your default.
sidebar:
  order: 8
---

farol ships with fourteen themes: four farol themes and ten imported community palettes.

![The theme picker, previewing a theme live over the task tree](/screenshot-theme.png)

| Theme | Notes |
| --- | --- |
| `farol-dark` | **The default.** The brand pair's dark member — deep-navy surfaces, amber accent, cream body text, built from the logo's own colors. |
| `farol-ember` | A dark theme with a warm brown-black base and an amber accent. |
| `farol-slate` | A refined dark theme with golden accents on a blue-black base. |
| `farol-day` | The light theme — the brand turned inside out: cream surfaces, navy ink, a darkened lamp-amber accent. |
| `catppuccin-mocha` | |
| `gruvbox-dark` | |
| `tokyo-night` | |
| `nord` | |
| `dracula` | |
| `solarized-dark` | |
| `one-dark` | |
| `everforest-dark` | |
| `rose-pine` | |
| `kanagawa-wave` | |

The imported palettes carry their original accent, text, panel, and status colors unchanged: a person who knows "Tokyo Night" sees it render the same way here, because it is the same theme, not a reinterpretation of one.

## Switching themes

Press `T` to open the theme picker. It lists every registered theme with the active one marked. Cursor movement previews the theme live — the entire UI behind the modal repaints on each keystroke. `Enter` applies and persists your choice; `esc` cancels.

## Setting the default

The confirmed choice is written to `~/.config/farol/config.yaml`:

```yaml
theme: gruvbox-dark
```

The saved theme is loaded before the program starts, so a chosen theme survives the process. A missing config, or a `theme:` value that names no registered theme, falls back to the default (`farol-dark`).

## How themes work

Every color the app draws with is a field on a `Theme`, not a hex value scattered through a component. Each theme is built from a handful of base colors — accent, text, panel, modal, danger, and the four status colors — with everything else derived by lightening (dark themes) or darkening (light themes) the panel base. Adding a theme is choosing those base colors, not hand-tuning thirty derived ones.

The four status colors are domain vocabulary shared across themes: `StatusComplete` (done), `StatusPending` (not started), `StatusInProgress` (active), and `StatusOverdue` (reserved — not used in this app). The farol themes share one set of status and danger colors on purpose; an imported scheme brings its own, because a farol green dropped into Gruvbox would read as the one thing on screen that is not Gruvbox. What stays constant is the *mapping*, not the hex.
