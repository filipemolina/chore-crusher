---
title: The keybinding system
description: Every key declared once in src/keys; components match against those bindings, and the footer and help overlay render from them.
sidebar:
  order: 5
---

The keybinding system is built around one rule, stated in `docs/DESIGN.md` §5 and `CONTRIBUTING.md`:

> **Do not invent a keybinding.** Every key lives in `src/keys`, exactly once: components match against those bindings, and the footer and the help overlay render from them, so a key declared anywhere else is a key the help overlay can't advertise and the next contributor can't discover.

## The single source of truth

`src/keys/Keys.go` declares every binding in the app, grouped into keymap structs:

| Struct | Scope |
| --- | --- |
| `keys.Global` | Keys that work anywhere no overlay owns the keyboard: `tab`/`shift+tab`, `L`, `1`/`2` (page switch), `q`, `ctrl+c`, `esc`, `?`, `T`, `/`, `F`, `ctrl+y`, `s`, `a` |
| `keys.Tree` | The task tree: navigation, expand/collapse, `space` toggle, `enter` details, `n` new, `d` delete, `[`/`]` outdent/indent, `alt+↑/↓` move, `u`/`U` release assignment, `g`/`G`/`pgup`/`pgdown`, `v` cycle view |
| `keys.Create` | The inline create row: `enter` submit, `esc` cancel |
| `keys.Lists` | The lists panel: navigate, `enter` select, `n` new list, `R` rename, `d` delete, `alt+↑/↓` move, `e` export, `i` import |
| `keys.Details` | The Details modal: `ctrl+s` save, `tab` next field, `←`/`→` cycle mode/priority, `↑`/`↓` percent nudge, `c`/`enter`/`y`/`d` comment actions, `ctrl+y` copy task id |
| `keys.ArchivePage` | The Archived Lists page: navigate, `g`/`G`, `/` filter, `u` unarchive, `d` permanently delete |
| `keys.Overlay` | Every modal: `enter` confirm, `esc` cancel, `y`/`n` yes/no, `↑`/`↓` navigate |
| `keys.ExportModal`, `keys.ListNameModal` | Modal-specific extras (`tab` next field, `space` toggle collaborative) |

Each binding is a `key.Binding` built with `key.NewBinding(key.WithKeys(...), key.WithHelp(...))` — the keystrokes *and* the help text live in the same declaration, so the footer and overlay can never describe a key differently from how it's handled.

## How components match

A component matches a keypress against a binding with `key.Matches`:

```go
case tea.KeyPressMsg:
    if key.Matches(msg, keys.Tree.Toggle) {
        return m, cmds.ToggleTask(m.selectedID)
    }
```

The component never hardcodes a keystroke. The binding is the single declaration; the handler, the footer, and the help overlay all read from it.

## Which keys are live: `Active` and `GlobalsFor`

`keys.Active(ctx)` returns the bindings the user can press right now, in the order they should be shown, given a `keys.Context` describing the screen (focused zone, whether the lists panel is visible, whether Details is open, whether a modal owns the keyboard, whether the create input or filter is active). The rules:

- A modal or overlay owns the keyboard exclusively while open → `Active` returns nothing.
- Details open → only its own bindings plus `esc` are live.
- Archive page open → only its own bindings plus `esc` are live (the same shape as Details, `ctx.ArchivePageVisible`).
- Inline create input active → only create keys plus `[`/`]` are live.
- `/` filter being typed or applied → only `enter`/`esc` are live.
- Otherwise, the focused zone's bindings, plus `tab` only while a side panel is open (with the lists panel hidden the focus cycle is a single zone, so the hint would be dead).

`keys.GlobalsFor(ctx)` returns the always-available keys minus the ones that do nothing in `ctx` — `tab`/`shift+tab` are dropped while the create input is live or when no side panel is open, and `?` is dropped while the create input is live (it types a literal).

## The help overlay

`?` opens the help overlay, and it lists **every key in the app on every screen** — not the keys live on the screen it was opened from. `keys.Catalog(ctx)` builds it from the same binding structs the handlers match against, grouped into scopes (`Global`, `Task Tree`, `Creating a task`, `Filtering`, `Lists`, `Renaming a list`, `Details`, `Archived Lists`, `Overlays`).

Two design decisions make it a reference rather than a corner description:

- **Keys the user cannot press right now are dimmed, not omitted** — a dimmed row reads as "not here", an omitted one as "removed". Availability is carried per-entry (`Entry.Available`).
- **Every scope is present on every screen.** Scopes used to come and go with the context, which made the overlay useless for its actual job: a key you can only read about once you have already found the surface it belongs to is not documented at all. The worst form of that failure shipped once — `n` (new task) was bound, handled, advertised in the footer, and named by the empty state, but appeared nowhere in the overlay.

The guard that keeps this true is mechanical, not review discipline: **`src/components/helpoverlay/coverage_test.go` reflects over every keymap struct and fails if any binding is missing from the rendered overlay.** Adding a field to a keymap struct without documenting it fails the test on its own.

Where a key does something its help text cannot carry, the scope gets a one-line `Note` (`keys.Scope.Note`) — that is how `L` says it also moves focus into the panel it reveals, and how the Overlays scope says the Details discard prompt answers to `y`/`n` alone.

## How to add a new key

1. **Add the binding to `src/keys`** — a field on the right keymap struct, with `key.WithKeys(...)` and `key.WithHelp(...)`. The help text is mandatory: `coverage_test.go` fails on a binding with empty help.
2. **Update `docs/DESIGN.md` §5** in the same commit — the keybinding contract lives there, and a key that works but isn't declared in the spec is a key the next contributor can't discover.
3. **Match against it in the component** with `key.Matches(msg, keys.SomeGroup.NewBinding)`.
4. **Wire it into `keys.Active`/`GlobalsFor`/`Catalog`** if it should be advertised in the footer or the overlay — the overlay's coverage test will tell you if you forgot.

A key that needs a new glyph, a new visual detail, or a new scope note updates `docs/DESIGN.md` §12 and the relevant `Scope` in the same commit — never only the component that happens to handle it.