# Contributing

This project expects a lot of its contributors to be unsupervised coding
agents working from these documents alone, without the back-and-forth a human
maintainer would normally provide in a PR review. That shapes this file: it
is stricter and more explicit than a typical `CONTRIBUTING.md`, on purpose.
If something here feels over-specified, it's because the alternative was
tried in spirit on stack-stitcher (this project's sister — see below) and
under-specification is what produced the need for a document like
`docs/DESIGN.md` in the first place.

## Read this before writing any code

In this order:

1. [`docs/DESIGN.md`](docs/DESIGN.md) — the data model, the status/progress
   state machine, the level rules for adding a task, the focus and
   keybinding contract, theming, storage, and the full CLI spec. This is not
   background reading; it is the specification. If your change contradicts
   something in here, either the change is wrong or the document needs to be
   updated *first*, as its own commit, with the reasoning for the change
   written into it.
2. [`docs/ROADMAP.md`](docs/ROADMAP.md) — which phase you're in, what it
   depends on, and the decisions already taken that your phase should not
   re-litigate.
3. The specific file in [`docs/plans/`](docs/plans/) for your phase — the
   step-by-step "how."
4. **For UI components:** [`docs/UI_INSTRUCTIONS.md`](docs/UI_INSTRUCTIONS.md) —
   the hardened checklist for visual coherence. Before marking a component
   complete, run through the six rules and the verification script
   (`scripts/verify-ui-component.sh <component-path>`). This is not optional
   polish; it's the mechanical check that keeps all components looking like
   one app rather than a collection of bespoke pieces.
5. [stack-stitcher's `docs/DESIGN.md`](https://github.com/filipemolina/stack-stitcher/blob/main/docs/DESIGN.md)
   and [its `CONTRIBUTING.md`](https://github.com/filipemolina/stack-stitcher/blob/main/CONTRIBUTING.md) —
   this project's architecture is deliberately copied from that one (theming,
   the single-keymap-package discipline, request/response commands, panel
   layout broadcast). When this project's own docs are silent on a structural
   question, that project's answer is the default, not a blank page to
   improvise on.

## Rules for not deviating or hallucinating

These are the specific failure modes this file exists to prevent, stated as
rules rather than left implicit:

- **Do not invent a keybinding.** Every key lives in `src/keys`, exactly
  once, the same discipline stack-stitcher's `src/keys/Keys.go` documents at
  the top of that file. If a feature needs a key that isn't in
  `docs/DESIGN.md`'s keybinding sections, add it to `src/keys` and to
  `docs/DESIGN.md` in the same commit — not just to the component that
  happens to handle it. A key that works but isn't declared there is a key
  the help overlay can't advertise and the next contributor can't discover.
- **Do not invent a visual detail.** A glyph, an indent width, a padding
  value, which text tier a label uses, how an empty state looks — all of it
  is fixed in `docs/DESIGN.md` §12, not left to the component writing it. If
  something genuinely isn't covered there, add it to §12 in the same commit —
  the same discipline the keybinding rule above requires. Run the §12
  "chrome-package
  contract" checklist against any new component before calling it done — a
  component that builds its own padding or picks its own color instead of
  going through `chrome` is the single most common way two phases built
  months apart stop looking like one app.
- **Do not add a dependency not named in `docs/DESIGN.md` or a phase plan.**
  The stack is Bubble Tea v2 / Lip Gloss v2 / Bubbles v2, Cobra, and
  `modernc.org/sqlite`. If a task seems to need something else (a YAML
  library, a different fuzzy-match package, a TUI table widget), that is a
  signal to re-read `docs/DESIGN.md` for whether the existing stack already
  covers it before reaching for something new — `sahilm/fuzzy` in particular
  is already a transitive dependency via `bubbles/list`'s filter, so it does
  not need to be added again as if it weren't already available.
- **Do not implement the status/progress state machine from memory of a
  similar app.** `docs/DESIGN.md` §3 is specific about the auto-completion
  asymmetry (subtask-derived 100% auto-completes; user-set percentage 100%
  does not) precisely because the intuitive, symmetric version is wrong for
  this app. If your change touches `status`, `progress_kind`, or
  `progress_pct`, re-read that section immediately before writing the code,
  every time — not just the first time.
- **Do not let the CLI and the TUI diverge on a write path.** Every mutation
  goes through `src/store`; neither `src/cli` nor `src/model` should contain
  logic that decides *whether* a transition is allowed — only logic that
  calls the `store` function and reports the result. If you find yourself
  writing an `if` that checks `progress_kind` outside `src/store`, stop and
  move it into `store`.
- **Do not guess at a `--json` shape.** `docs/DESIGN.md` §9 states the
  contract: exactly one JSON value on stdout, success or failure, nothing
  else on stdout in that mode. If a subcommand's exact JSON field names
  aren't pinned down yet in that section, propose the shape as part of the
  same commit that implements it, and update `docs/DESIGN.md` to match —
  don't ship an undocumented shape and leave a future caller to reverse-
  engineer it from the source.
- **Do not resolve an ambiguous id by guessing.** `store.ResolveID` returns
  an error on an ambiguous prefix match; no caller should catch that error
  and silently pick the first result.
- **When a plan file and `docs/DESIGN.md` disagree, `docs/DESIGN.md` wins**,
  and the plan file is out of date and should be corrected in the same PR.
  The reverse should never happen silently — if implementing a plan reveals
  that `docs/DESIGN.md` itself is wrong or underspecified, fix
  `docs/DESIGN.md` and explain why in the commit message, rather than
  quietly implementing something else and letting the two drift.

## Known hallucination traps in this stack

These are not style preferences — they are places where a plausible,
confident, wrong answer is *more* likely than a correct one, because the
correct answer contradicts what dominates public training data. Each one
below has been verified directly against the installed module source, not
recalled from memory, specifically so this list can be trusted more than the
instinct it's warning you about.

### Bubble Tea v2 / Bubbles v2 / Lip Gloss v2 are not the library you remember

`charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, and `charm.land/lipgloss/v2`
are a breaking rewrite of the `github.com/charmbracelet/{bubbletea,bubbles,lipgloss}`
v1 APIs that make up the overwhelming majority of public Bubble Tea code —
tutorials, Stack Overflow answers, and almost certainly most of whatever
pattern your training data associates with "Bubble Tea." A symbol that *feels*
right from memory is more likely a v1 symbol that does not exist in v2 than a
correct guess.

**Before writing the first line of code that touches any of these three
packages, read that package's own upgrade guide.** Each project ships one,
written by its maintainers explicitly for this situation — the file opens
with "written for both humans and LLM-assisted migration tools." Once
`go mod download` (or a `go build`) has fetched the dependency, it's sitting
in your module cache:

```
$(go env GOPATH)/pkg/mod/charm.land/bubbletea/v2@<version>/UPGRADE_GUIDE_V2.md
$(go env GOPATH)/pkg/mod/charm.land/bubbles/v2@<version>/UPGRADE_GUIDE_V2.md
$(go env GOPATH)/pkg/mod/charm.land/lipgloss/v2@<version>/UPGRADE_GUIDE_V2.md
```

(`ls $(go env GOPATH)/pkg/mod/charm.land/bubbletea/v2*` to find the exact
version directory — it changes as the dependency is updated.) Read the whole
guide for whichever package you're about to use, not just the excerpt below;
the excerpt only covers what's highest-impact and easiest to get wrong
silently, not everything that changed.

**Verified, highest-impact differences** (confirmed against the v2.0.x source
in this project's module cache — re-verify against `go doc` if the pinned
version has moved on):

- **`tea.Model.View()` returns a `tea.View` struct, not a `string`.** The
  struct's `Content` field is what used to be the return value; construct one
  with `tea.NewView(s)` or `tea.View{Content: s}`. Writing `func (m Model)
  View() string` will not even satisfy the `tea.Model` interface — you'll get
  a compile error, which is one of the better outcomes on this list, but only
  if you don't first "fix" it by trying to make `View()` return a string
  through some wrapper.
- **`tea.KeyMsg` was renamed `tea.KeyPressMsg`.** Every `case tea.KeyMsg:` in
  a remembered v1 example needs to become `case tea.KeyPressMsg:`.
- **`key.Matches` is generic** (`func Matches[Key fmt.Stringer](k Key, b
  ...Binding) bool`), but you call it exactly as before —
  `key.Matches(msg, someBinding)`. If a call site doesn't compile, the type
  being matched needs a `String() string` method, not a different call
  shape.
- **`lipgloss.Color(s string)` returns `image/color.Color`, not a
  string-backed `lipgloss.Color` type.** This is exactly why
  `docs/DESIGN.md` §11's `Theme` struct types every color field as
  `color.Color` — matching that, not "simplifying" it back to `string`, is
  the point.
- **Exported `Width`/`Height` fields became getter/setter method pairs**
  across `textinput`, `textarea`, `list`, `viewport`, `progress`, and
  `table`: `m.SetWidth(40)` / `m.Width()`, not `m.Width = 40`.
- **`DefaultStyles()` (and similar) now take an explicit `isDark bool`.**
  Lip Gloss v2 removed automatic terminal-background adaptation
  (`AdaptiveColor` is gone); nothing auto-detects light vs. dark anymore.
  This project's theme system (`docs/DESIGN.md` §11) supplies its own colors
  regardless, so this mostly matters for any stock Bubbles component styling
  you leave at its default.
- **`DefaultKeyMap` moved from a package-level `var` to a `func
  DefaultKeyMap() KeyMap`** in `textinput`, `textarea`, and `paginator` —
  call it, don't reference it as a value.
- **`NewModel(...)` constructors are gone; use `New(...)`** everywhere they
  existed in v1 (`help`, `list`, `paginator`, `spinner`, `textinput`).

If a call site referencing one of these packages doesn't compile, the fix is
almost always in the upgrade guide's tables, not in reverting to the v1
import path — the v1 path isn't in `go.mod` and reaching for it is a sign to
stop and read the guide, not to add the dependency.

### modernc.org/sqlite (the `store` package's driver)

- **The registered `database/sql` driver name is `"sqlite"`, not
  `"sqlite3"`.** `sql.Open("sqlite3", …)` is the name the *different*,
  CGO-based `mattn/go-sqlite3` driver registers — and it's what nearly every
  public Go+SQLite example uses, which is exactly why this is easy to get
  wrong on autopilot. This project's `go.mod` does not depend on
  `mattn/go-sqlite3` at all; `sql.Open("sqlite3", …)` will fail at runtime
  with "unknown driver," not at compile time, so it's a mistake that survives
  `go build` and `go vet` and shows up only when `store.Open` is actually
  called.
- **Prefer DSN query parameters over separate `PRAGMA` statements** for
  connection-level settings this driver parses directly off the connection
  string — `?_journal_mode=WAL&_foreign_keys=1&_busy_timeout=5000` — rather
  than a `PRAGMA journal_mode=WAL` exec after opening. Verified against the
  driver's own DSN parsing in `conn.go`/`driver.go`; if a parameter's exact
  spelling matters and doesn't match what's in the vendored source under
  `$(go env GOPATH)/pkg/mod/modernc.org/sqlite@<version>/`, trust the source
  over this file — driver versions move.
- **`modernc.org/sqlite` pins an exact `modernc.org/libc` version**, and the
  two are not independently upgradable — the transpiled SQLite C code in
  `modernc.org/sqlite` is generated against that specific `libc`. **Never run
  a broad `go get -u ./...` or `go get -u all`** once this dependency is in
  `go.mod`; bump `modernc.org/sqlite` by name and let `go mod tidy` resolve
  `libc` from *its* `go.mod`, never the reverse. This package ships its own
  `CLAUDE.md` at its module root (`$(go env GOPATH)/pkg/mod/modernc.org/sqlite@<version>/CLAUDE.md`)
  with more implementation detail than belongs here — read it before
  touching anything in `store` that isn't a plain query, e.g. connection
  hooks, custom functions, or vtabs (none of which the current plan needs,
  but a future contributor tempted to add one should read that file first).

### Cobra

Lower risk — the API most training data agrees on here is also the current
one. The one place worth double-checking is the flag-grouping helpers this
project's CLI spec relies on
(`docs/plans/phase-2-cli.md` step 5: `MarkFlagsMutuallyExclusive`,
`MarkFlagsRequiredTogether`) — both take a variadic list of flag *names*
(strings), not a single name or the flag objects themselves. Run `go doc
github.com/spf13/cobra.Command.MarkFlagsMutuallyExclusive` if a call site
doesn't compile, rather than guessing at the signature a second time from
memory.

## How to verify instead of guessing

General habits, not specific to this stack, that catch a wrong assumption
before it becomes ten more lines built on top of it:

- **Before calling a dependency function you didn't write this session,
  confirm its signature with `go doc <package>.<Symbol>`.** It reads the
  actual installed version, not whatever a training corpus happened to
  describe, and it's faster than writing code and finding out from a compile
  error.
- **Before assuming a helper doesn't exist in this repo and writing your own,
  `grep -rn` for it first.** `docs/DESIGN.md` and the phase plans repeatedly
  point at shared functions (the tree-flattening logic mentioned in both
  `docs/plans/phase-2-cli.md` and `docs/plans/phase-4-task-tree.md`, the three
  `store` mutators in `docs/DESIGN.md` §3) that a *later* phase's author
  needs to find, not reinvent — and a plan file can tell you such a thing
  should exist without knowing the exact name an earlier phase gave it.
- **Work in the smallest increment you can verify, and verify it before
  writing the next one.** Write one function (or one tight group of related
  ones), then run `go build ./... && go vet ./...` immediately — `go test
  ./...` too, if it has a test yet. A model's error rate per line written is
  roughly constant regardless of how careful the instructions are; the lever
  that actually works is shrinking how many unverified lines can pile up
  before the first check catches a mistake, not writing any single line more
  carefully.
- **For a function whose correctness is a matter of exact rules rather than
  judgment** — `docs/DESIGN.md` §3's state machine, §4's level-offset table —
  **write the test from the rule's own worked examples before or alongside
  the implementation.** Several phase plans hand you the table already
  (`docs/plans/phase-1-storage.md`'s required-coverage list,
  `docs/plans/phase-5-add-input.md` step 2's `nextOffset` table); treat a
  passing suite as the definition of "implemented," not a read-through of
  the code that looks right.
- **When a plan file and the repository disagree — it references a function
  that doesn't exist, or a file that was never created — stop and reconcile
  before working around it.** Check `docs/ROADMAP.md` for whether an earlier
  phase was skipped. If the plan itself is simply wrong, fix the plan file in
  the same change; don't invent an undocumented workaround that the next
  reader has no way to discover.
- **Prefer a pure function with a table-driven test over a stateful method
  with none, whenever they're equivalent.** A table of (input → expected
  output) can be checked mechanically, row by row; a stateful `Update`
  handler's correctness has to be reasoned about from a diff.
  `docs/plans/phase-5-add-input.md`'s `nextOffset` function is the template —
  follow that shape for other small, rule-heavy pieces of logic as they come
  up (row-flattening's depth/collapse bookkeeping is another candidate).

## Glossary

Fixed vocabulary, so the same thing has the same name in every file. If a
concept below needs a new name, change it here first and grep for the old
name across `docs/` and `src/` in the same commit — a term that drifts
between "task" and "item" and "todo" across files is indistinguishable, to
the next reader, from three different things.

| Term | Means | Does not mean |
| --- | --- | --- |
| **List** | A `List` row — a named container of tasks. | A `bubbles/list.Model` (that's "the lists panel's inner list," or just "a `list.Model`" when the distinction matters). |
| **Task** | A `Task` row, at any depth. | "Todo," "item," "entry" — pick Task and keep it everywhere, including comments and UI copy. |
| **Subtask** | A Task with a non-nil `parent_id`. Not a separate Go type — `apptypes.Task` and `store`'s task rows have no `Subtask` type. | A fixed second level of nesting — nesting is unbounded (`docs/DESIGN.md` §2); "subtask" describes a relationship (child of), not a depth. |
| **Root task** | A Task with a nil `parent_id` (depth 0). | — |
| **Status** | One of `pending` / `in_progress` / `complete` — the `Task.status` column. | "Progress," which is the separate `progress_kind`/`progress_pct` pair that only applies while status is `in_progress`. |
| **Progress kind** | One of `none` / `simple` / `subtasks` / `percentage` — `Task.progress_kind`. | A synonym for status. |
| **Cascade** | The specific downward propagation `store.Complete` performs onto every descendant (`docs/DESIGN.md` §3). | Any recursive walk — `recomputeAncestors` walks *upward* and is never called "cascade" in these docs, to keep the direction unambiguous from the word alone. |
| **Zone** | One of the three focusable regions in `docs/DESIGN.md` §5: the lists panel, the task tree, the add input. | "Panel," "pane," "component" — those all still apply to the same things, but "zone" is reserved for the focus-cycle concept specifically (`focusableZones()`). |
| **Level offset** | The `{-1, 0, +1}` state the add input tracks relative to the current selection (`docs/DESIGN.md` §4). | "Indent level" or "depth" alone — those describe a task's absolute position in the tree; level offset is always relative to whatever's currently selected. |
| **Tier** | One of the four background-color layers a surface renders on (`BackgroundContent`/`Panel`/`Elevated`/`Recessed`, plus `ModalBg` — `docs/DESIGN.md` §12). | A synonym for "zone" — a zone is a focus-cycle concept (which of three things `tab` lands on); a tier is a paint concept (which background color a given pixel-region uses). A single zone's fill still changes tier when it gains or loses focus. |

## The layout

```
main.go              # cobra root — no subcommand launches the TUI, else dispatch
src/
├── model/           # AppModel: Init/Update/View, the top-level Bubble Tea model
├── components/      # one package per leaf model: tasktree, listspanel, addinput,
│                     # detailsmodal, themepickermodal, searchmodal, listnamemodal,
│                     # confirmmodal, helpoverlay, keybindingbar
│   └── chrome/       # shared rendering: PanelFrame, tree-row rendering, the
│                     # progress pill, KeyHints, Spinner
├── cmds/             # message types, and the tea.Cmds that produce them
├── apptypes/         # List, Task, Status, ProgressKind and their list-item wrappers
├── keys/             # the one keymap package — see the rule above
├── store/            # SQLite schema, migrations, and every read/write function
├── cli/              # one file per subcommand group, thin cobra-to-store adapters
├── appstyles/        # Theme type + the 14-theme registry (ported from stack-stitcher)
├── config/           # ~/.config/complete/config.yaml
└── constants/        # layout widths, focusable-zone ids, branding
docs/
├── DESIGN.md         # why — the specification
├── ROADMAP.md        # order — which phase, what it depends on
└── plans/            # how — one file per phase
```

## The loop

Once phase 0 has landed (`docs/plans/phase-0-scaffolding.md`):

```bash
make dev            # go run . (launches the TUI against a scratch database)
go build ./...
go vet ./...
go test ./...
gofmt -l .          # must print nothing
```

CI runs exactly these on every pull request, `go test -race` included —
`src/store` opens real SQLite connections and the TUI's poll loop runs on its
own goroutine, both worth checking under the race detector. Keep every commit
green, not just the branch tip.

## Testing

`src/store` is plain Go — no terminal, no Bubble Tea, a real SQLite file in a
temp directory per test. This is where the state-machine rules from
`docs/DESIGN.md` §3 get their tests, and there is no excuse for a change to
that section landing without one: create the scenario (a task in `subtasks`
mode, complete every child, assert the parent auto-completed; a task in
`percentage` mode at 100, assert it did *not* auto-complete), assert the
result.

Above `store`, follow stack-stitcher's tiers: components take a message and
hand back a model (assert on the result directly); rendering is a string
(`ansi.Strip(m.View().Content)`, worth asserting on for layout); an
end-to-end rig is the only way to test a full keystroke-to-render flow and
should stay rare, because it's timing-based.

## Style

- **Comments say why, not what.** The code already says what happens; a
  comment earns its place by recording a decision, a constraint, or a trap —
  the kind of thing a future edit would otherwise "fix" back into a bug.
  `docs/DESIGN.md` is where a *design* decision's reasoning belongs, at
  length; a code comment is for the reasoning that's only legible standing
  next to the specific line it explains.
- **Constructors are always `New`; the exported type is always `Model`** for
  a Bubble Tea component, so callers read as `addinput.New(...)` and assert
  on `addinput.Model` — the same convention stack-stitcher's `components`
  tree uses throughout.
- Commit messages: a short summary line, then prose explaining why, not what
  — match the register in stack-stitcher's `git log` if you want a model to
  follow.

## Releases

Maintainer-only. Once phase 0's GoReleaser config is in place, pushing a `v*`
tag builds and drafts a release the same way stack-stitcher's does — see
`docs/plans/phase-0-scaffolding.md` for the exact config, carried over with
the binary name changed from `stitch` to `complete`.
