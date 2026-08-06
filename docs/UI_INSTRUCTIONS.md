# UI Coherence Instructions

This document hardens the visual coherence rules from `docs/DESIGN.md` §12 into a mechanical checklist. Every component must satisfy the applicable six rules below before it is complete. An inner control in the aggregate Tasks surface follows the documented frame exception in Rule 2; this is a **verification guide, not a prose explanation** — use it as a literal checklist, not a once-and-read document.

## The Chrome-Package Contract: Six Rules

Before a leaf component (`src/components/*`) is considered done, it satisfies **all** of the following. There is no partial credit — a component that passes 5 of 6 is broken, not mostly done.

### Rule 1: Every color is read from `appstyles.Active.*` at render time

**The rule:** Every color the component uses is `appstyles.Active.<ColorField>`, read at the moment of rendering. Never:
- A cached package-level `var` holding a color
- A literal hex string like `"#FF00FF"`
- A color assigned once and reused across renders

**Why:** Themes swap `appstyles.Active` on every render; a cached color makes a component ignore the active theme.

**How to verify:**

1. In the component's `.go` files, search for color assignments:
   ```bash
   grep -n "color.Color" <component>/Model.go
   grep -n "lipgloss.Color" <component>/*.go
   ```
   Any match must be a function parameter or a return from `appstyles.Active.*`, never a literal.

2. Search for literal hex or color names:
   ```bash
   grep -En "(0x[0-9A-Fa-f]{6}|#[0-9A-Fa-f]{6}|color\..*\{)" <component>/*.go
   ```
   Should return zero results (or only in comments).

3. Check the View/Render methods for direct `lipgloss.NewStyle()` chains:
   ```
   ✓ CORRECT:
   lipgloss.NewStyle().Foreground(appstyles.Active.TextPrimary)
   
   ✗ WRONG:
   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF00FF"))
   ```

**Example (from `tasktree/View.go` lines 57–58):**
```go
func muted() lipgloss.Style { return lipgloss.NewStyle().Foreground(appstyles.Active.TextMuted) }
func dim() lipgloss.Style   { return lipgloss.NewStyle().Foreground(appstyles.Active.TextDim) }
```
✓ Both read `appstyles.Active.*` at call time, not at package init.

---

### Rule 2: Outer surface is built with shared chrome; inner controls do not create one

**The rule:** A rendered body surface's outer boundary uses one of:
- `chrome.PanelFrame()` for the Lists, Tasks, and Details surfaces
- `chrome.ModalSurface()` for modals
- `chrome.EmptyStateCard()` for a recessed empty-state card

A leaf inside the aggregate Tasks surface (the task tree or add input) must
not create a second `PanelFrame`. Its parent frames and seals the aggregate;
the leaf receives and uses the parent-supplied inner dimensions and background.
No component sets its own `Padding()`, panel `Border()`, or `BorderStyle()`.

**Why:** Zones that pick different padding silently break the alignment of checkboxes, titles, and input fields across the whole layout.

**How to verify:**

1. Locate the component's View method:
   ```bash
   grep -A 20 "func (m Model) View()" <component>/View.go
   ```

2. Check that a surface owner's outermost return calls one of:
   - `chrome.PanelFrame()`, or
   - `chrome.ModalSurface()`, or
   - `chrome.EmptyStateCard()`.

   For an inner Tasks control, check its parent `taskspanel` calls
   `chrome.PanelFrame()` and the control accepts the supplied dimensions and
   background without creating another frame.

3. Search for banned methods in the component:
   ```bash
   grep -En "\.Padding\(|\.Border\(|\.BorderStyle\(" <component>/*.go
   ```
   Should return **zero results** (the chrome helper is the only place these live).

**Example (from `taskspanel/View.go`):**
```go
return tea.NewView(chrome.PanelFrame("Tasks", m.focused, width, height, body))
```
✓ The aggregate surface delegates all padding and framing to the shared helper.

**Counterexample (an error):**
```go
// ✗ WRONG: component setting its own padding
style := lipgloss.NewStyle().Padding(1, 2)
return tea.NewView(style.Render(body))
```

---

### Rule 3: Any user-supplied text goes through `chrome.Truncate()`

**The rule:** Any text that comes from user input (a task title, a list name, a note preview, a CLI argument) **must** pass through `chrome.Truncate(text, availableWidth)` before rendering. Never:
- Render a task title directly
- Truncate manually with `text[:n]`
- Use `lipgloss.Width()` to slice mid-string

**Why:** User text can be arbitrarily long; truncation can split multi-byte runes or ANSI escape sequences, corrupting the rest of the line. `chrome.Truncate()` handles both. A component that doesn't truncate overflows the layout.

**How to verify:**

1. Find all the user-supplied data the component renders:
   ```bash
   # Look for Task.Title, Task.Notes, List.Name, or similar fields
   grep -En "\.Title|\.Name|\.Notes" <component>/View.go
   ```

2. For each match, verify it's wrapped in `chrome.Truncate()`:
   ```bash
   grep -B 2 "\.Title\|\.Name\|\.Notes" <component>/View.go | grep -i "truncate"
   ```
   Should find a `chrome.Truncate()` on the same line or the previous line.

3. Search for manual slicing of rendered text (these are hidden bugs):
   ```bash
   grep -En "text\[:.*\]|title\[:.*\]|name\[:.*\]" <component>/*.go
   ```
   Should return **zero results**.

**Example (when components render titles):**
```go
// ✓ CORRECT
title := chrome.Truncate(task.Title, availableWidth)
return lipgloss.NewStyle().Render(title)

// ✗ WRONG: no truncation, will overflow
return lipgloss.NewStyle().Render(task.Title)

// ✗ WRONG: manual truncation loses ANSI/multi-byte handling
return lipgloss.NewStyle().Render(task.Title[:min(len(task.Title), width)])
```

---

### Rule 4: Background is sealed before returning content

**The rule:** Before a component returns its rendered view, it seals its own background tier against gaps. Unsealed tiers let the terminal's own background show through.

**The tiers:**
| Component Type | Tier | Field | How to seal |
|---|---|---|---|
| Lists or Tasks surface | 3 or 4 | `BackgroundPanel` / `BackgroundElevated` | `chrome.PanelFrame()` seals this automatically |
| Task-tree or add-input inner control | its supplied Tasks tier | parent-supplied Tasks background | `appstyles.FillBackground()` before returning its raw content |
| Modal | modal | `ModalBg` | `chrome.ModalSurface()` seals this automatically |
| Empty state | recessed | `BackgroundRecessed` | `chrome.EmptyStateCard()` seals this automatically |

**How to verify:**

1. If a surface uses `chrome.PanelFrame()`, `chrome.ModalSurface()`, or `chrome.EmptyStateCard()`, **the seal is automatic** — the helper handles it.

2. If an inner Tasks control builds raw content, verify it calls `appstyles.FillBackground(bg, content)` with its supplied background before returning it:
   ```bash
   grep "FillBackground" <component>/*.go
   ```
   Should find at least one call in any component that doesn't delegate to a chrome helper.

3. Check test expectations: if the component renders a multi-line body and one line is shorter than the others, that short line **must** still have a background color, not be blank.

**Example (from `chrome/Styles.go` lines 18–23):**
```go
func PanelFrame(isFocused bool, width, height int, body string) string {
	bg := PanelBg(isFocused)
	content := appstyles.FillBackground(bg, body)  // ← seals gaps
	return FitBox(WrapperStyle.Background(bg), width, height).Render(content)
}
```
✓ Seals the background before fitting into the frame box.

---

### Rule 5: Every glyph and symbol is in the vocabulary, or was added in the same change

**The rule:** Any visual symbol (checkbox, expand arrow, progress indicator, etc.) must be one of the glyphs listed in `docs/DESIGN.md` §12 "The glyph vocabulary." If a new glyph is needed:
1. Add it to the table in `docs/DESIGN.md` first
2. Implement it in the component in the same commit
3. Update `docs/DESIGN.md` and the component together — no undocumented glyphs

**The vocabulary (from DESIGN.md §12):**

| Meaning | Glyph | Notes |
|---|---|---|
| Task: pending | `◻` | |
| Task: in progress | `◻` | Same as pending — no dedicated glyph. For all three progress kinds alike — the `(NN%)` suffix, not the checkbox, distinguishes them. |
| Task: complete | `◼` | Title renders in `TextMuted`, not `TextPrimary` — completion is when a title becomes secondary. |
| Task has detail text | `🗎` | U+1F5CE DOCUMENT, right-aligned in the fixed trailing icon column, immediately right of the (fixed-width) status column; the column is reserved on every row and blank when there are no notes. One cell in go-runewidth; emoji fonts may render wider — see DESIGN.md §12. |
| Node has children, expanded | `▾` | Appended to the *end* of the title — never a leading column, so a parent's title starts at its own depth. |
| Node has children, collapsed | `▸` | Same position, end of title. |
| Node is a leaf | *(no glyph)* | Nothing appended; titles of leaves simply carry no marker. |
| Add-input level: sibling (default) | `-` | from `docs/DESIGN.md` §4. |
| Add-input level: child | `+` | from `docs/DESIGN.md` §4. |
| Add-input level: parent-of-selection | `^` | from `docs/DESIGN.md` §4. |
| Trailing progress | ` (NN%)` | In `TextMuted`, after the title with one leading space; omitted when `DerivedProgress` says `displayAsSimple`. |

**How to verify:**

1. Search the component for any string literals that look like symbols:
   ```bash
   grep -En '[◻◼▾▸\+\-^]|\(.*%\)' <component>/*.go | grep -v "//"
   ```

2. For each match, verify it's in the vocabulary above.

3. If a new glyph is needed, verify:
   - It's added to `docs/DESIGN.md` §12's table
   - The same commit has both the `.md` change and the code

**Example (correct, from existing code):**
```go
// ✓ Using vocabulary glyphs
checkbox := "◻"  // pending / in-progress task
arrow := "▾"       // expanded node
progress := " (42%)"  // progress suffix
```

**Counterexample:**
```go
// ✗ WRONG: using unapproved symbol
emoji := "✓"  // not in vocabulary, and emoji width is unreliable in terminals
icon := "→"   // not in vocabulary; use ▾/▸ for expand/collapse
```

---

### Rule 6: Focus is shown by lifting a tier (background color), never by changing size or border

**The rule:** When a body surface gains or loses focus, **only the background color changes**:
- Unfocused: `appstyles.Active.BackgroundPanel` (tier 3)
- Focused: `appstyles.Active.BackgroundElevated` (tier 4)

Lists is focused only for Lists keyboard focus. Tasks is focused for either
task-tree or add-input keyboard focus; that child-control transition changes
neither surface dimensions nor chrome. A surface's width, height, border
weight, and corner radius **do not change**. Never:
- Add a border when focused
- Make the border thicker
- Resize the box
- Change corner radius on focus

**Why:** Any size shift pushes every element below it one row down, breaking the entire layout.

**How to verify:**

1. Locate the component's Model and View:
   ```bash
   grep -n "type Model struct" <component>/Model.go
   grep -n "func (m Model) View()" <component>/View.go
   ```

2. Check that a surface owner derives its focus through `chrome.PanelBg()`:
   ```bash
   grep "PanelBg\|BackgroundElevated\|BackgroundPanel" <component>/*.go
   ```
   An inner Tasks control may retain its own focus for cursor/key handling, but
   it must not derive a second surface tier or direct focus-dependent geometry.

3. Verify no focus-dependent sizing:
   ```bash
   grep -B 2 -A 2 "isFocused" <component>/*.go | grep -En "Width|Height|Border"
   ```
   Should return **zero results** related to sizing or borders.

4. Check the surface owner passes its focus to `chrome.PanelFrame()`:
   ```bash
   grep "chrome.PanelFrame" <component>/*.go
   ```
   `PanelFrame` applies the tier; an inner Tasks control never calls it.

**Example (from `chrome/Styles.go` lines 36–47):**
```go
func PanelBg(isFocused bool) color.Color {
	if isFocused {
		return appstyles.Active.BackgroundElevated  // tier 4
	}
	return appstyles.Active.BackgroundPanel        // tier 3
}
```
✓ Only the color changes; the size doesn't.

**Counterexample (an error):**
```go
// ✗ WRONG: focus changes the border
if isFocused {
	return style.Border(lipgloss.RoundedBorder())
}
return style.Border(lipgloss.NoBorder())

// ✗ WRONG: focus changes the width
w := m.width
if isFocused {
	w -= 2  // to show a border
}
```

---

## Applying the Checklist

Before marking a component as complete:

### Step 1: Run automated checks

In the component's directory:

```bash
# Rule 1: No literal hex or cached colors
grep -rEn "(0x[0-9A-Fa-f]{6}|#[0-9A-Fa-f]{6}|color\..*\{)" .

# Rule 2: No banned padding/border methods
grep -rEn "\.Padding\(|\.Border\(|\.BorderStyle\(" . | grep -v "chrome\|PanelFrame"

# Rule 3: User-supplied text is truncated
# (Verify by reading the View method and checking for chrome.Truncate on Task/List fields)

# Rule 4: FillBackground is called (or chrome helper is used)
grep -rn "FillBackground\|chrome\.PanelFrame\|chrome\.ModalSurface\|chrome\.EmptyStateCard" .

# Rule 5: Glyphs are in the vocabulary
grep -rEn '[◻◼▾▸🗎\+\-^]|\(.*%\)' . | grep -v "docs/DESIGN\|//"

# Rule 6: Focus only changes background
grep -B 2 -A 2 "isFocused" . | grep -rEn "Width|Height|Border"
```

### Step 2: Visual inspection

1. **Render the component** in the running app (use `make dev`)
2. At startup, verify Lists is hidden, Tasks is titled, and the footer advertises `L lists`
3. Focus Lists, then focus Tasks through both the task tree and add input — verify only the appropriate outer surface lifts, with no border or size change
4. **Check alignment** — the title chips share their inset, and task rows and the add input align inside Tasks
5. **Test with long text** — paste a 100-character task title and verify it truncates with `…`, no overflow
6. **Switch themes** — verify all colors change; a static color wouldn't

### Step 3: Tests

If the component has rendering tests (e.g., `Model_test.go`), verify they check for:
- The presence of expected colors from `appstyles.Active.*`
- No hardcoded hex values
- Correct glyph usage (checkboxes, expand arrows)
- Truncation of long text

**Example pattern (from `src/appstyles/Contrast_test.go` and similar):**
```go
// Verify no literal colors slip through
func TestComponentHasNoLiteralColors(t *testing.T) {
    m := componentPackage.New(...)
    v := m.View()
    
    // Should contain theme colors, not hex
    if strings.Contains(v.Content, "#") {
        t.Errorf("Component contains literal hex color: %s", v.Content)
    }
}
```

---

## When to Extend the Rules

If a rule seems to require an exception:

1. **Do not create a local exception.** First, check whether re-reading `docs/DESIGN.md` §12 clarifies the rule.

2. **If the rule genuinely doesn't fit,** propose the change as its own commit:
   - Update `docs/DESIGN.md` §12 with the new rule or exception
   - Explain why the existing rule doesn't apply
   - Update this file to match
   - Then implement the component using the updated rule

3. **Never ship undocumented exceptions.** A component that visually deviates from the rest of the app because a local developer felt the general rule didn't apply is broken, not innovative.

---

## Related Files

- `docs/DESIGN.md` §12 — The original prose; read it for the reasoning behind each rule
- `src/components/chrome/` — The shared helpers (`PanelFrame`, `ModalSurface`, `EmptyStateCard`, `Truncate`)
- `src/appstyles/` — The theme system and color definitions (`appstyles.Active.*`)
- `CONTRIBUTING.md` — The glossary and how to verify instead of guessing (§ "How to verify instead of guessing")
