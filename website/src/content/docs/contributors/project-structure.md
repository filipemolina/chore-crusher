---
title: Project structure
description: The src/ tree, per-package responsibilities, and the two single-source-of-truth seams.
sidebar:
  order: 3
---

The layout is the architecture made visible. `docs/DESIGN.md` §10 and `CONTRIBUTING.md` both carry it; this is the same tree with each package's job spelled out.

```
main.go              # entry point: forwards to cli.Execute (exit-code contract)
src/
├── model/           # AppModel: Init/Update/View, the top-level Bubble Tea model
├── components/      # one package per leaf model
│   └── chrome/       # shared rendering: PanelFrame, tree-row, progress pill, KeyHints, Spinner
├── cmds/            # message types and the tea.Cmds that produce them
├── apptypes/        # List, Task, Status, ProgressKind — the shapes components pass around
├── keys/            # the one keymap package: every key declared exactly once
├── store/           # SQLite schema, migrations, every read/write function
├── cli/             # one file per subcommand group, thin cobra-to-store adapters
├── banner/          # the figlet wordmark rendered with the active theme
├── appstyles/       # Theme type + the 14-theme registry
├── config/          # ~/.config/farol/config.yaml + XDG directory resolution
└── constants/       # layout widths, focusable-zone ids, branding, version
```

## Per-package responsibilities

### `main.go`

The entry point. It does one thing: `os.Exit(cli.Execute(os.Args[1:]))`. The TUI-vs-CLI decision lives in the cobra root command inside `src/cli` (see [Architecture](/contributors/architecture/)).

### `src/model` — AppModel

The top-level Bubble Tea model: `Init`, `Update`, `View`. It owns the store handle, the config, the terminal dimensions, the poll loop, navigation, focus, and modals. Components never read `tea.WindowSizeMsg` — this is the only place that does, and it broadcasts the derived layout. It is also where the request/response split lands: components emit intents (`cmds.*Msg`), AppModel resolves them against the store and refreshes.

### `src/components` — one package per leaf model

Each leaf model is its own package, named `…modal` or `…panel` or `…tree`:

`tasktree`, `taskspanel`, `listspanel`, `detailspanel`, `themepickermodal`, `searchpicker`, `listnamemodal`, `confirmmodal`, `helpoverlay`, `keybindingbar`, `mainmenu`, `aboutmodal`, `importexportmodal`.

The convention (`CONTRIBUTING.md`): **constructors are always `New`; the exported type is always `Model`**, so callers read as `listnamemodal.New(...)` and assert on `listnamemodal.Model`.

### `src/components/chrome` — shared rendering

Rendering and layout shared by more than one leaf component: `PanelFrame`, tree-row rendering, the progress pill, `KeyHints`, `Spinner`, `Truncate`, `EmptyStateCard`, `Scrim`, `SealInput`. A helper earns its way in here by having a **second caller** — the chrome-package contract (`docs/DESIGN.md` §12) is what keeps components built months apart looking like one app.

### `src/cmds` — messages and commands

Every message type the app passes between components, and the `tea.Cmd`s that produce them: `PollTick`, `RefreshLists`, `RefreshTasks`, `ToggleTask`, `DeleteTask`, `SetFocus`, `SetBodyLayout`, `OpenDetails`, and the rest. This is the vocabulary of the request/response split — a component emits a `cmds.*Msg`, AppModel handles it.

### `src/apptypes` — the shape language

`List`, `Task`, `Status`, `ProgressKind`, `Priority`, `AgentActivity`, `Comment`, `Attachment`, and the list-item wrappers `bubbles/list.Item` needs. These are converted from `src/store`'s row types **at the store boundary** — `apptypes` does not import `database/sql`. `apptypes.Flatten` is the one tree-flattening both the CLI and the TUI render from, so the two surfaces cannot drift apart.

### `src/keys` — the one keymap package

Every keybinding in the app, declared exactly once. Components match against these bindings; the footer and the help overlay render from them. See [The keybinding system](/contributors/keybinding-system/).

### `src/store` — the data layer

SQLite schema, embedded migrations, and every read/write function — including the full status/progress state machine (`docs/DESIGN.md` §3). **The only package that imports `database/sql` or `modernc.org/sqlite`.** See [Storage and concurrency](/contributors/storage/).

### `src/cli` — the agent-facing front end

One file per subcommand group (`lists.go`, `tasks.go`, `search.go`, `inbox.go`, `work.go`, `presence.go`, `export.go`, `import.go`, `skill.go`, …), each a thin adapter from Cobra flags to a `store` call and a `--json`-aware printer. The root command also hosts the TUI launch. See [Architecture](/contributors/architecture/) for the sibling relationship with `src/model`.

### `src/banner` — the wordmark

The figlet wordmark rendered with the active theme.

### `src/appstyles` — theming

The `Theme` type and the 14-theme registry, built by `newTheme` from a handful of base colors. See [The theme system](/contributors/theme-system/).

### `src/config` — preferences and paths

`~/.config/farol/config.yaml` (or `$XDG_CONFIG_HOME`): exactly two fields today — `theme` and `poll_interval_ms` — in a struct designed to grow. The package also owns user-directory resolution for the data side (`config.DBPath()`), so the TUI and every CLI subcommand agree on where the database lives without each re-deriving XDG rules.

### `src/constants` — the fixed numbers

Layout widths (`LEFT_PANEL_WIDTH`, `MIN_PANEL_WIDTH`, `AUTO_SHOW_LISTS_MIN_WIDTH`, `MIN_TERMINAL_WIDTH`/`HEIGHT`, `HEADER_HEIGHT`, `FOOTER_HEIGHT`, `BODY_GUTTER_WIDTH`), focusable-zone ids (`COMPONENT_LISTS_PANEL`, `COMPONENT_TASK_TREE`, …), branding (`WORDMARK`, `APP_NAME`, `DEFAULT_LIST_NAME`), and the version-resolution logic.

## The two seams

Two packages are single sources of truth, and the whole design depends on them staying that way:

1. **`src/keys` is the single declaration of every keybinding.** A key declared anywhere else is a key the help overlay can't advertise and the next contributor can't discover. The footer, the help overlay, and every component's handler all read from the same binding structs.
2. **`appstyles.Active` is the single source of color.** Every color the app draws with is a field on `appstyles.Theme`, and every call site reads `appstyles.Active.*` fresh at render time — never a cached package-level color, never a literal hex. Assign a different registered theme to `Active` and the next frame repaints.

Both seams are enforced mechanically, not by review discipline: `src/components/helpoverlay/coverage_test.go` fails if any binding is missing from the rendered overlay, and `src/appstyles`'s tests (`Background_test.go`, `Foreground_test.go`, `Contrast_test.go`) assert that rendered frames seal their backgrounds, draw no default foreground, and keep the elevation tiers separated.