---
title: The theme system
description: How the 14 themes are built from a handful of base colors — the Theme struct, tier derivation, and the single Active theme.
sidebar:
  order: 6
---

Every color the app draws with is a field on `appstyles.Theme` — not a hex value scattered through a component. The theme system lives in `src/appstyles` (`Theme.go`, `Styles.go`, and the tests), and its rules are stated in `docs/DESIGN.md` §11.

## The `Theme` struct

A `Theme` is the app's complete visual vocabulary, resolved to concrete values. One field per semantic token:

| Field | Meaning |
| --- | --- |
| `Name`, `Dark` | The theme's name; `Dark` says which way the tiers derive |
| `Accent` | The brand color: focus, the wordmark, title chips. Does not vary with `Dark` |
| `TextPrimary`, `TextMuted`, `TextDim` | The three text tiers, most to least emphasis |
| `PanelBg` | The base surface color every background tier derives from; also `BackgroundRecessed`'s value |
| `BackgroundContent`, `BackgroundPanel`, `BackgroundElevated` | Tiers 2/3/4: the frame, an unfocused panel, a focused panel |
| `BackgroundRecessed` | Below the panel tier, for insets like empty-state cards — equal to `PanelBg` by construction |
| `ModalBg` | Every modal — and an active list row — drawn on a distinct register, not derived from `PanelBg` |
| `BorderDefault`, `BorderCard` | Rims for ordinary panels and recessed surfaces |
| `StatusComplete`, `StatusPending`, `StatusInProgress`, `StatusOverdue` | Domain colors: done, not started, active, and *(reserved — not used in this app)* |
| `Danger` | App-level alert chrome: the error banner, an inline validation message |
| `InkOnLight`, `InkOnDark` | The two fixed inks for text on status pills |

A `Theme` is inert data, not a service: nothing here reads the terminal or does I/O.

## `newTheme`: everything else derives

`newTheme(p themeParams)` builds a full `Theme` from a handful of base colors — `Accent`, `Text`, `Panel`, `Modal`, `Danger`, the four status colors, and the `Dark` flag. **Adding a theme is choosing those base colors, not hand-tuning thirty derived ones.** The derivation:

- `TextMuted = lower(Text, 0.2)`, `TextDim = lower(Text, 0.3)`
- `BackgroundContent = raise(Panel, 0.04)`, `BackgroundPanel = raise(Panel, 0.08)`, `BackgroundElevated = raise(Panel, 0.12)`, `BackgroundRecessed = Panel`
- `BorderDefault = lower(Panel, 0.3)`, `BorderCard = raise(Panel, 0.18)`
- `InkOnLight = #151520`, `InkOnDark = #FAFAFA` — fixed regardless of `Dark`

Every registered theme is built through `newTheme` rather than a bare struct literal, so a registered theme can't leave a field zero-valued — which matters because a nil `color.Color` renders as no SGR at all, i.e. a background-bleed bug.

## Dark themes lighten; light themes darken

`raise`/`lower` pick `lipgloss.Lighten` or `lipgloss.Darken` based on `Dark`:

- A **dark theme** raises a surface's attention by **lightening** it — further from the near-black base, toward the light text sitting on it.
- A **light theme** raises attention by **darkening** it — further from the near-white base, toward the dark text sitting on it.

Both apply the same deltas, so the tiers stay proportional to each other; only the direction flips. The asymmetry that drives every imported palette: `Lighten` is additive (adds `255 × percent` to each channel) while `Darken` is multiplicative (scales each channel by `1 − percent`). For a dark theme the raise is a fixed climb independent of the base; for a light theme the step shrinks as the base approaches white. The consequence for imported color schemes: **set `Panel` to that scheme's deepest background tier**, so the raised tiers land back inside the scheme's own background range.

## The elevation ladder

The focused/unfocused panel step is the focus signal — a surface's box is exactly the same width and height whether or not it has focus; what changes is the fill (`BackgroundPanel` unfocused, `BackgroundElevated` focused). The ladder:

| Tier | Field | Derivation |
| --- | --- | --- |
| 2 | `BackgroundContent` | `raise(Panel, 0.04)` |
| 3 | `BackgroundPanel` | `raise(Panel, 0.08)` |
| 4 | `BackgroundElevated` | `raise(Panel, 0.12)` |
| — | `BackgroundRecessed` | `Panel` (un-raised base) |
| — | `ModalBg` | not a raise of the panel ladder at all |

**`ModalBg` must clear `BackgroundElevated` by ≥14 per channel** — guarded by `TestElevationSeparation` in `src/appstyles/Contrast_test.go` — or the modal disappears into the focused panel.

## Ink on status pills

`InkOnLight`/`InkOnDark` are the one deliberate exception to "derived from base colors": they do not vary with a theme's `Dark` flag, because a status pill's own fill does not vary with the app's theme either. The text that reads legibly on a given fill has to stay legible whichever theme is active, not follow `TextPrimary`, which flips. `appstyles.InkOn(fill)` picks whichever of the two fixed inks has better contrast on the fill at hand, and `Contrast_test.go` verifies the result clears its threshold on every status pill, the accent chip, and the error banner for every registered theme.

## `appstyles.Active`: the one theme in effect

`appstyles.Active` is the one `Theme` in effect. **Every call site reads it fresh** — `appstyles.Active.TextPrimary`, say — rather than caching a color at package init. That is the whole of what lets a later switch actually repaint: assign a different registered `Theme` to `Active` and the next frame draws it. Styles composed of more than one field are functions (e.g. `appstyles.NormalTitle`) for the same reason, not package-level `var`s — a `var` built at init freezes whichever theme was active when the package loaded.

`appstyles.SetTheme(name)` assigns a new active theme by name and returns `false` if the name is not in the registry. The theme picker (`T`) applies themes live as the cursor moves and persists the chosen name via `config.SaveConfig`; the TUI's boot path applies the saved `theme:` before the first frame, with an unknown name falling back to the default.

## The 14 themes

The registry (`appstyles.Themes`) holds 14 themes: four of this app's own plus ten imported community palettes. The imported palettes carry their original accent, text/panel/modal bases, and status hexes unchanged — a person who knows "Tokyo Night" should see it render the same way here, because it is the same theme, not a reinterpretation of one.

| Theme | Kind |
| --- | --- |
| `farol-dark` | The brand pair's dark member: deep-navy surfaces, amber accent, cream body text — the Night Watch icon itself |
| `farol-ember` | Dark, warm brown-black base, amber accent |
| `farol-slate` | Dark, blue-black base with golden accents |
| `farol-day` | The brand pair's light member: cream surfaces, navy ink, darkened lamp-amber accent |
| `catppuccin-mocha`, `gruvbox-dark`, `tokyo-night`, `nord`, `dracula`, `solarized-dark`, `one-dark`, `everforest-dark`, `rose-pine`, `kanagawa-wave` | Imported community palettes |

**The fresh-install default is `"farol-dark"`** — `DefaultTheme` names it, and a config with no `theme:` preference activates it. Every other registered theme stays selectable through the `T` picker and as a saved `theme:` value.

## Adding a theme

Adding a theme is choosing the base colors, not hand-tuning the derived ones:

1. Pick a name and the `Dark` flag.
2. Choose the base colors: `Accent`, `Text`, `Panel` (the scheme's deepest background tier), `Modal` (must clear `BackgroundElevated` by ≥14 per channel), `Danger`, and the four status colors.
3. Register it in `appstyles.Themes` through `newTheme(themeParams{...})`.
4. Run the appstyles tests — `Contrast_test.go` (elevation separation, ink contrast on every pill), `Background_test.go` (no background bleed), `Foreground_test.go` (no default foreground) — across the new theme.

Do not redesign the derivation math — it is already tuned across all 14 registered palettes (`docs/DESIGN.md` §11).