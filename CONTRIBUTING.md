# Contributing

Farol is a keyboard-first task manager for humans and AI agents
working from one store: a TUI, a CLI, and an MCP server over a single SQLite
database. This file is for human contributors. If you are an AI coding agent,
read [`AGENTS.md`](AGENTS.md) instead; it has the operational rules, the
known hallucination traps in this stack, and the verification habits this
project expects.

## Quick start

Prerequisites: Go 1.26 or newer. No CGO required.

```bash
go build ./...
make dev             # launches the TUI against your real store
go run main.go --help   # the CLI reference
```

For throwaway data, point `XDG_DATA_HOME` and `XDG_CONFIG_HOME` at a temp
directory, the way `demo/seed.sh` does.

## The docs

Everything a contributor needs is in the repository. Read these before
writing code:

- [`docs/DESIGN.md`](docs/DESIGN.md): the specification, covering the data model,
  status/progress state machine, level rules for adding a task, focus and
  keybinding contract, theming, storage, and the full CLI spec. If a change
  contradicts it, either the change is wrong or the document needs updating
  first, as its own commit.
- [`docs/ROADMAP.md`](docs/ROADMAP.md): what has shipped and what it found,
  what is ahead, and the decisions already taken.
- [`docs/STATUS.md`](docs/STATUS.md): current verification state and known
  caveats.
- [`docs/UI_INSTRUCTIONS.md`](docs/UI_INSTRUCTIONS.md): the visual-coherence
  checklist for UI components, verified mechanically by
  `scripts/verify-ui-component.sh`.
- [`docs/COMPONENT_CHECKLIST.md`](docs/COMPONENT_CHECKLIST.md): the
  per-component build checklist.

## The loop

```bash
go build ./...
go vet ./...
go test ./...
gofmt -l .          # must print nothing
```

CI runs exactly these on every pull request, `go test -race` included.
Keep every commit green, not just the branch tip.

## Testing

`src/store` is plain Go: no terminal, no Bubble Tea, a real SQLite file in a
temp directory per test. This is where the state-machine rules from
`docs/DESIGN.md` §3 get their tests.

Above `store`, test in three tiers: components take a message and hand back a
model (assert on the result directly); rendering is a string
(`ansi.Strip(m.View().Content)`, worth asserting on for layout); an
end-to-end rig is the only way to test a full keystroke-to-render flow and
should stay rare, because it's timing-based.

## Style

- Comments say why, not what. The code already says what happens; a comment
  earns its place by recording a decision, a constraint, or a trap. Design
  reasoning belongs in `docs/DESIGN.md`, at length; a code comment is for
  the reasoning that's only legible next to the line it explains.
- Constructors are always `New`; the exported type is always `Model` for a
  Bubble Tea component.
- Commit messages: a short summary line, then prose explaining why, not what.

## Glossary

Fixed vocabulary, so the same thing has the same name in every file. If a
concept needs a new name, change it here first and grep for the old name
across `docs/` and `src/` in the same commit.

| Term | Means | Does not mean |
| --- | --- | --- |
| **List** | A `List` row, a named container of tasks. | A `bubbles/list.Model` (that's "the lists panel's inner list," or just "a `list.Model`" when the distinction matters). |
| **Task** | A `Task` row, at any depth. | "Todo," "item," "entry": pick Task and keep it everywhere, including comments and UI copy. |
| **Subtask** | A Task with a non-nil `parent_id`. Not a separate Go type. | A fixed second level of nesting; nesting is unbounded (`docs/DESIGN.md` §2). |
| **Root task** | A Task with a nil `parent_id` (depth 0). | |
| **Status** | One of `pending` / `in_progress` / `complete`, the `Task.status` column. | "Progress," which is the separate `progress_kind`/`progress_pct` pair that only applies while status is `in_progress`. |
| **Progress kind** | One of `none` / `simple` / `subtasks` / `percentage`, `Task.progress_kind`. | A synonym for status. |
| **Cascade** | The downward propagation `store.Complete` performs onto every descendant (`docs/DESIGN.md` §3). | Any recursive walk; `recomputeAncestors` walks *upward* and is never called "cascade" in these docs. |
| **Zone** | One of the focusable regions in `docs/DESIGN.md` §5. | "Panel," "pane," "component": those still apply, but "zone" is reserved for the focus-cycle concept (`focusableZones()`). |
| **Level offset** | The `{-1, 0, +1}` state the inline create input tracks relative to the current selection (`docs/DESIGN.md` §4). | "Indent level" or "depth" alone; level offset is always relative to the current selection. |
| **Tier** | One of the background-color layers a surface renders on (`BackgroundContent`/`Panel`/`Elevated`/`Recessed`, plus `ModalBg`, `docs/DESIGN.md` §12). | A synonym for "zone": a zone is a focus-cycle concept, a tier is a paint concept. |

## The layout

```
main.go              # cobra root: no subcommand launches the TUI, else dispatch
src/
├── model/           # AppModel: Init/Update/View, the top-level Bubble Tea model
├── components/      # one package per leaf model (tasktree, taskspanel,
│                     # listspanel, detailspanel, themepickermodal, searchpicker,
│                     # listnamemodal, confirmmodal, helpoverlay, keybindingbar,
│                     # mainmenu)
│   └── chrome/       # shared rendering: PanelFrame, tree-row rendering, the
│                     # progress pill, KeyHints, Spinner
├── cmds/             # message types, and the tea.Cmds that produce them
├── apptypes/         # List, Task, Status, ProgressKind and their list-item wrappers
├── keys/             # the one keymap package: see the rule above
├── store/            # SQLite schema, migrations, and every read/write function
├── cli/              # one file per subcommand group, thin cobra-to-store adapters
├── mcpserver/        # the MCP server (`farol mcp`): tools mirror the CLI but
│                     # talk directly to store (docs/DESIGN.md §9)
├── banner/           # the figlet wordmark rendered with the active theme
├── appstyles/        # Theme type + the 14-theme registry (docs/DESIGN.md §11)
├── config/           # ~/.config/farol/config.yaml
└── constants/        # layout widths, focusable-zone ids, branding
docs/
├── DESIGN.md             # why: the specification
├── ROADMAP.md            # order: what shipped, what it found, what is ahead
├── STATUS.md             # current verification state and known caveats
├── UI_INSTRUCTIONS.md    # the hardened UI coherence checklist
└── COMPONENT_CHECKLIST.md# the per-component build checklist
scripts/
└── verify-ui-component.sh  # mechanically checks the six UI_INSTRUCTIONS rules
```

## How to contribute

1. Find something to work on: the app's own task lists (Bugs, Features, UI)
   or a GitHub issue.
2. Create a branch, and commit in small logical commits (`area: description`,
   e.g. `keys: add the panel-cycle binding`). Keep every commit green.
3. Open a pull request. CI runs build, vet, the full test suite with `-race`,
   and a hard gofmt check ("Check formatting").

## Releases

Maintainer-only. Pushing a `v*` tag builds and drafts a release through
GoReleaser (`.github/workflows/release.yml`); `.goreleaser.yaml` holds the
matrix config (CGO_ENABLED=0, cross-compiled linux/darwin × amd64/arm64).
See `docs/DESIGN.md` §8 for why the driver choice keeps that cross-compile
matrix possible.
