---
title: Contributing
description: How to find work, commit, and open a PR — plus the glossary and the chrome-package contract.
sidebar:
  order: 10
---

This page is the how-to for contributing to farol. The full operational rules live in `CONTRIBUTING.md` (for humans) and `AGENTS.md` (for AI coding agents); the specification is `docs/DESIGN.md`.

## Read `docs/DESIGN.md` first

Everything a contributor needs is in the repository. Read these before writing code:

- **`docs/DESIGN.md`** — the specification: the data model, the status/progress state machine, the level rules for adding a task, the focus and keybinding contract, theming, storage, and the full CLI spec. If a change contradicts it, either the change is wrong or the document needs updating *first*, as its own commit, with the reasoning written into it.
- **`docs/ROADMAP.md`** — what has shipped and what it found, what is still ahead, and the decisions already taken that a change should not re-litigate.
- **`docs/UI_INSTRUCTIONS.md`** — the hardened visual-coherence checklist, verified mechanically by `scripts/verify-ui-component.sh`.

When anything disagrees with `docs/DESIGN.md`, **`docs/DESIGN.md` wins**: an issue, a review comment, a task description, your own working notes. Those are all narrower than the contract and routinely out of date against it. The reverse should never happen silently: if the work reveals that `docs/DESIGN.md` itself is wrong or underspecified, fix `docs/DESIGN.md` and explain why in the commit message, rather than quietly implementing something else and letting the two drift.

## Find something to work on

- **The app's own task lists** — farol tracks its own work (Bugs, Features, UI lists). Read them with `farol inbox` at the start of a session.
- **A GitHub issue** — the project's issue tracker.

Keep the status of what you take current as you work: assign what you take, flip it to `in_progress` with a progress percentage, advance the percentage at milestones, comment at decision points, and complete it when done. The human watches the TUI; the statuses are the report.

## Create a branch, commit small

1. Create a branch.
2. Commit in small logical commits: `area: description` (e.g. `keys: add the panel-cycle binding`).
3. **Keep every commit green** — `go build ./... && go vet ./... && go test ./...` passing at every commit, not just at the branch tip. CI runs build, vet, the full test suite with `-race`, and a hard gofmt check ("Check formatting").

## Open a pull request

Open a PR against `main`. CI runs automatically on every pull request. If a commit fails or hooks reject it, fix the issue and create a new commit; do not amend the failed commit.

## The glossary

Fixed vocabulary, from `CONTRIBUTING.md`, so the same thing has the same name in every file. If a concept needs a new name, change it in the glossary first and grep for the old name across `docs/` and `src/` in the same commit.

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

## The chrome-package contract

Before a leaf component (`docs/DESIGN.md` §10) is considered done, it satisfies all of the following. Treat this as a literal checklist, not prose to have read once:

1. **Every color it draws with is read from `appstyles.Active.*` at render time** — never a cached package-level color, never a literal hex.
2. **Its outer surface is built with `chrome.PanelFrame`** (or, for a modal, the equivalent shared modal-frame helper) — it does not set its own padding, border style, or corner treatment. A leaf inside the aggregate Tasks surface does not create a second frame; its parent frames and seals the aggregate while the leaf uses the supplied dimensions and background.
3. **Any user-supplied text it renders goes through `chrome.Truncate`.**
4. **It seals its own background tier** before returning its content to whatever composes it.
5. **Any glyph or symbol it needs is one of the ones listed in `docs/DESIGN.md` §12's glyph vocabulary** — or that table was extended, in the same change, to add it.
6. **Focus, if it applies to this component, is shown exactly per §12** — by lifting a tier; nothing else changes between focused and unfocused.

A component that satisfies 1–6 cannot visually drift from the rest of the app no matter which phase or which contributor built it — that is the entire purpose of making the checklist mechanical rather than a matter of taste.

For the hardened, actionable version of this checklist with verification steps and bash commands to check each rule, read `docs/UI_INSTRUCTIONS.md` and run `scripts/verify-ui-component.sh <component-path>` before marking a component complete. The mechanical assertions behind it — `HasBackgroundBleed`, `HasDefaultForeground`, `TestElevationSeparation`, the help-overlay coverage test — are described in [Testing](/contributors/testing/).