---
title: Testing
description: How to test a TUI — plain-Go store tests, the three tiers above the store, table-driven state-machine tests, and VHS.
sidebar:
  order: 8
---

The testing philosophy, stated in `docs/DESIGN.md` §13 and `CONTRIBUTING.md`: test the data rules directly against `store`, and above the store test in three tiers. The same three tiers are listed in both documents; keep the two in step if you change either.

## `src/store`: plain Go, no terminal

`src/store` talks to a real SQLite file created fresh in a temp directory per test — no terminal, no Bubble Tea, no mocking. This is where the state-machine rules from `docs/DESIGN.md` §3 get their tests, and there is no excuse for a change to that section landing without one: create the scenario (a task in `subtasks` mode, complete every child, assert the parent auto-completed; a task in `percentage` mode at 100, assert it did *not* auto-complete), assert the result.

Write those tests directly against `store`, not against the CLI or the TUI, wherever the assertion is about data rather than rendering. The store's own suite covers the state machine (`state_test.go`), lists (`lists_test.go`), tasks (`tasks_test.go`), assignment (`assignment_test.go`), comments (`comments_test.go`), activity (`activity_test.go`), resolve (`resolve_test.go`), ULIDs (`ulid_test.go`), settings, export, and search.

## The three tiers above the store

1. **Model tests** — a component takes a message and hands back a model; assert on the result directly. `src/model`'s tests build the full `AppModel` against a real store in a temp directory (`newTestModel` in `src/model/refresh_test.go`) and drive keypresses through `Update`, which covers keystroke-to-state flows without a terminal.
2. **Render tests** — a component's rendering is a string (`ansi.Strip(m.View().Content)`), worth asserting on for layout and styling. `chrome`-level checks like `appstyles.HasBackgroundBleed` belong here too.
3. **End-to-end rigs** — a real Bubble Tea program driven over a fake terminal are the only way to test a full keystroke-to-render flow. Keep them the exception, not the default: they are timing-based and must wait for specific output rather than sleep and hope.

## State-machine tests: table-driven from the rules

For a function whose correctness is a matter of exact rules rather than judgment, write the test from the rule's own worked examples before or alongside the implementation. `docs/DESIGN.md` §3 and §4 state their rules as tables, which transcribe directly into a table-driven test; treat a passing suite as the definition of "implemented," not a read-through of the code that looks right.

The template to follow: `computeTaskRowCols` in `src/components/tasktree/View.go` — the whole column drop order is a pure function over widths, and `TestComputeTaskRowColsDropOrder` checks it row by row. Prefer a pure function with a table-driven test over a stateful method with none, whenever they're equivalent.

## Concurrency: `go test -race`

CI runs `go test -race ./...` on every pull request. It matters here for two reasons: `src/store` opens real SQLite connections, and the TUI's poll loop runs on its own goroutine — both worth checking under the race detector. The Makefile's `test` target runs the race detector over `src/store` and `src/cli` locally.

## The mechanical chrome checks

The visual-coherence rules in `docs/DESIGN.md` §12 are enforced by assertions, not review:

| Assertion | What it catches | Where it's applied |
| --- | --- | --- |
| `appstyles.HasBackgroundBleed` | A missing `Background()` call — an unstyled cell showing the terminal's own color | `src/model/layout_test.go`, `src/components/tasktree/scroll_test.go`, `src/components/detailspanel/Model_test.go`, and the per-theme checks in `src/appstyles/Background_test.go` |
| `appstyles.HasDefaultForeground` | A dropped `Foreground()` call — a glyph drawing in the terminal's default color | `src/model/foreground_test.go`, `src/components/tasktree/foreground_test.go`, with the SGR state machine itself pinned in `src/appstyles/Foreground_test.go` |
| `TestElevationSeparation` | `ModalBg` not clearing `BackgroundElevated` by ≥14 per channel | `src/appstyles/Contrast_test.go` |
| Ink-contrast checks | Status-pill text failing its contrast threshold | `src/appstyles/Contrast_test.go`, across every registered theme |
| `TestOverlayDocumentsEveryBinding` | A key declared in `src/keys` but missing from the rendered help overlay | `src/components/helpoverlay/coverage_test.go` |

## VHS for visual verification

The demo directory (`demo/`) records the app with [VHS](https://github.com/charmbracelet/vhs): a tape (`demo/*.tape`) drives a real terminal session, and `demo/seed.sh` seeds a deterministic store under `/tmp/farol-demo` so the recording neither depends on nor clobbers the real store. `make demo` builds the binary, seeds, and records the demo GIF and screenshots. The tapes set the same XDG dirs as the seed script, so a run is reproducible from a clean checkout.

## The poll loop's own test

One behavior worth knowing about: `src/model/refresh_test.go` asserts that the poll re-issues itself — `PollTickMsg` always returns another `PollTick` command, so the poll can never silently stop. If you touch the poll loop, that test is the guard.