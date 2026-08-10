# Component Checklist: Before Marking Complete

Use this as a literal checklist for any UI component (`src/components/<name>/`). All six items must be satisfied before the component is done.

## The Six Rules (from `docs/UI_INSTRUCTIONS.md`)

- [ ] **Rule 1: Colors**  
  Every color is `appstyles.Active.*` at render time, never literal hex or cached colors.  
  Run: `grep -rEn "(0x[0-9A-Fa-f]{6}|#[0-9A-Fa-f]{6})" src/components/<component>/`

- [ ] **Rule 2: Frame**
  Each body surface uses `chrome.PanelFrame()`; modals and empty cards use their shared chrome helpers.
  A leaf inside the aggregate Tasks surface (the task tree, the inline create row) is an inner control: its parent frames and seals the aggregate, while it uses the supplied dimensions and background without a second frame.
  No hand-set `.Padding()`, panel `.Border()`, or `.BorderStyle()`.
  Run: `grep -rEn "\.Padding\(|\.Border\(|\.BorderStyle\(" src/components/<component>/ | grep -v chrome`

- [ ] **Rule 3: Truncation**  
  Any user-supplied text (Task.Title, List.Name, etc.) passes through `chrome.Truncate()`.  
  Check: Read `View()` method and verify user fields use truncation.

- [ ] **Rule 4: Background Seal**
  Background tier is sealed. A surface uses a chrome helper (automatically sealed); an inner Tasks control calls `appstyles.FillBackground()` with the supplied Tasks background.
  Run: `grep -rn "FillBackground\|chrome.PanelFrame\|chrome.ModalSurface\|chrome.EmptyStateCard" src/components/<component>/`

- [ ] **Rule 5: Glyphs**  
  Only vocabulary symbols: `[ ] [~] [x] ▾ ▸ - + ^ (NN%)`  
  Any new glyph must be added to `docs/DESIGN.md` §12 in the same commit.

- [ ] **Rule 6: Focus**  
  Focus only changes background color (via `chrome.PanelBg()`), never size, border, or corners.  
  Run: `grep -B 2 -A 2 "isFocused" src/components/<component>/*.go | grep -En "Width|Height|Border"`  
  Result should be empty.

## Automated Verification

Run the verification script:
```bash
scripts/verify-ui-component.sh src/components/<component-name>
```

This checks all six rules and reports which ones pass.

## Manual Verification (in the running app)

With the app running (`make dev`):

1. **Check startup** — Lists is hidden, Tasks is titled, and the footer advertises `L lists`
2. **Focus Lists and Tasks** — Focus Tasks through both tree and input; only the correct outer surface lifts, with no new border or size change
3. **Check alignment** — Title chips share their inset; task rows and the input align inside Tasks
4. **Test long text** — Paste a 100+ character task title; should truncate with `…`
5. **Switch themes** — Colors should change; verify no static colors remain

## If a Rule Doesn't Apply

1. Check whether re-reading `docs/DESIGN.md` §12 clarifies the rule
2. If the rule genuinely doesn't apply, propose a change to `docs/UI_INSTRUCTIONS.md` first
3. Document the exception in a comment and update the rule — never have undocumented exceptions

## Before Committing

1. Run `scripts/verify-ui-component.sh` — must pass
2. Run `go test ./...` — must pass
3. Run `gofmt -l .` — must print nothing
4. Verify manually in the running app
5. Update `docs/DESIGN.md` if you added a new glyph, color tier, or other visual detail

## Links

- Full instructions: [`docs/UI_INSTRUCTIONS.md`](UI_INSTRUCTIONS.md)
- Design specification: [`docs/DESIGN.md`](DESIGN.md)
- Contributing guide: [`CONTRIBUTING.md`](../CONTRIBUTING.md)
