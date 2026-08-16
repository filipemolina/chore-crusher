# FAROL-COPY-RESTRUCTURE-PLAN.md

Restructure of the Farol landing page: copy, section order, and demo imagery.

- **Written:** 2026-08-16
- **Scope:** `landingPages/farol/` (the page) and `~/Documents/projects/farol/demo/` (the assets)
- **Status:** approved for execution, not started. One decision was reopened
  and reversed on 2026-08-16 -- the split-screen shot is now a real two-process
  recording (sections 4.5b, 5.4, 8.1). Nothing has been dispatched yet.
- **Companion docs:** `FAROL-VISUAL-ASSETS-PLAN.md` (asset geometry, still authoritative),
  `CLAUDE.md` (dispatch loop), `AGENTS.md` (binding constraints)

This file is written to be executed by a small model with no prior context.
Every piece of copy is given verbatim. Do not paraphrase it, do not "improve"
it, do not translate it. Copy it exactly.

---

## 0. Ground rules -- read before touching anything

These exist because each one has already broken this project at least once.

### 0.1 ASCII only

**Every file you write or edit must be pure ASCII.** Verify before you finish:

```sh
LC_ALL=C grep -n '[^ -~\t]' <file>     # must print nothing
```

Non-ASCII punctuation sent through an agent prompt has produced 17 U+FFFD
replacement characters in this repo before. The rule that prevents it:

| Want | Write in markup | Do NOT write |
|---|---|---|
| em dash | `&mdash;` | a literal em dash |
| right arrow | `&rarr;` | a literal arrow glyph |
| middle dot | `&middot;` | a literal middot |
| ellipsis | `...` | a literal ellipsis glyph |

**Where entities work and where they do not:**

- Literal markup in an `.astro` template: entities render. Use them.
- A string passed to `set:html={...}`: entities render. Use them.
- A string rendered as `{expression}`: Astro escapes it, so `&mdash;` would
  print the literal seven characters. In those strings use plain ASCII: `--`
  for a dash, `->` for an arrow.

**Existing files already contain literal non-ASCII characters.** Edit *around*
them. Never retype a line that has one; change only the ASCII part of it.

### 0.2 The `{` trap in Astro templates

Astro treats `{` in markup as the start of a JavaScript expression. A JSON
sample typed directly into a `<pre>` block will break the build or render
wrong. **Any literal `{` or `}` in displayed text must come from a string
defined in the component frontmatter** and be rendered as `{myString}`. Every
code sample in this plan is already written that way -- keep it that way.

### 0.3 The `class` interpolation trap

Astro uses JSX attribute semantics. `class="foo {expr}"` emits the literal
text `{expr}` into the HTML. It reads correctly in source and is broken in the
browser. Four attributes shipped this bug on this exact page.

Use `class:list={[...]}` or a template literal. Never string-interpolate into
a `class` attribute.

### 0.4 Verify in the built output, not the source

Reading `.astro` source proves nothing. After building, grep `dist/index.html`
for the strings you wrote. Section 6 lists the exact checks.

### 0.5 pnpm, never npm

`npm` is broken in this environment. `pnpm run build`, `pnpm run check`.

### 0.6 Binding constraints that still apply

From `AGENTS.md`. Do not trade any of these for design:

1. Under 1s load, 100 on all four Lighthouse categories.
2. Zero CLS -- every image and video keeps an explicit `width`/`height` and a
   reserved aspect box.
3. Mobile-first from 360px up, no horizontal page scroll.
4. One dominant CTA.
5. No new runtime dependencies, no third-party scripts, no web fonts beyond
   the two already self-hosted.

### 0.7 Do not invent facts

Every capability claim in this plan was verified against the Farol Go source
on 2026-08-16. If you find that a claim does not match the code, **stop and
report it**. Do not silently reword it and do not silently drop it. The
README at `~/Documents/projects/farol/README.md` is stale in places (it says
`farol skill` is "planned"; the command exists at `src/cli/skill.go`). The
source is the truth, the README is not.

---

## 1. Why this restructure exists

Three findings drove it. Understanding them prevents drift during execution.

### 1.1 Two sections say the same thing

`Features.astro` and `ForAgents.astro` overlap almost completely:

| `Features` card | `ForAgents` card | Verdict |
|---|---|---|
| Agent-first CLI | 02 Real JSON contract | same claim |
| Instant sync, no server | 01 One store, two views | same claim |
| Live agent presence | 03 Agent identity and live presence | same claim |

Both section headers also restate the hero's promise. A visitor is told
"built for humans and agents" three times before learning anything new.

Both are pure feature lists, which is the weakest of the five feature-narrative
strategies identified in the Evil Martians study of 100 developer-tool landing
pages. Neither states a problem.

### 1.2 The page advertises the commodity half of the product

Absent from the page, present in the code:

| Capability | Where it lives |
|---|---|
| Assignment reserves the whole subtree, so two agents cannot take an ancestor or descendant of each other's work | `src/cli/work.go`, documented in `src/cli/skill.go` |
| `farol inbox` returns your list plus every foreign list in one read | `src/cli/inbox.go` |
| List ownership gate: structural writes refuse on another agent's list without `--force` | `src/cli/ownership.go` |
| Presence and assignment are two separate axes | `src/cli/presence.go` |
| Every write auto-claims presence, so the spinner is free | `autoClaimTask` |
| `--since` change feed on `farol tasks` and `farol diff` | `src/cli/tasks.go:253` |
| `farol comment`, `farol attach` | `src/cli/attachments.go` |

Present on the page, in prime grid slots: export/import, fuzzy search, tree
tasks. Every to-do application has had these for twenty years. They belong in
a one-line list, not on cards.

### 1.3 One screenshot, shown four times, of gardening

All four stills are the same seed, the same theme, the same viewport.
`landing-hero.webp` and `landing-agent.webp` differ only in which row the
cursor sits on; `landing-tree.webp` is the same view with different
percentages; `landing-search.webp` is that view with a modal over it. At card
scale the distinctions are invisible.

And the demo data is domestic chores: "Plan the garden", "Prep the beds",
"Refinish the deck", "Clean the kitchen", "Order seed compost". The page
claims a coding agent drives this tool, and the evidence is gardening. The
only developer signal on the whole page is the 12-character string
`45% claude` on one row.

---

## 2. Target page structure

`src/pages/index.astro` in final form renders, in this order:

| # | Component | Status | Anchor |
|---|---|---|---|
| 1 | `Navigation.astro` | EDIT (nav links only) | -- |
| 2 | `Hero.astro` | EDIT (copy + alt text) | -- |
| 3 | `Problem.astro` | **NEW** | `#problem` |
| 4 | `HowItWorks.astro` | **NEW** | `#how-it-works` |
| 5 | `DemoVideo.astro` | EDIT (copy only) | `#demo` |
| 6 | `Capabilities.astro` | **NEW** -- replaces `Features` + `ForAgents` | `#capabilities` |
| 7 | `AgentQuickstart.astro` | **NEW** | `#agents` |
| 8 | `WhyNot.astro` | **NEW** | `#why-not` |
| 9 | `Installation.astro` | EDIT (3 methods -> 2) | `#install` |
| 10 | `FinalCta.astro` | **NEW** | -- |
| 11 | `Footer.astro` | keep as is | -- |

**Deleted:** `src/components/Features.astro`, `src/components/ForAgents.astro`.

**Kept and reused, do not delete:** `FeatureCard.astro` (used by
`Capabilities`), `AgentFeature.astro` (used by `Capabilities`),
`InstallMethod.astro` (used by `Installation`).

Background alternation, so no two adjacent sections share a background:

```
Hero            bg-terminal-bg (hero wash)
Problem         bg-terminal-bg-elevated/30
HowItWorks      bg-terminal-bg
DemoVideo       bg-terminal-bg-elevated/30   <-- CHANGED from bg-terminal-bg
Capabilities    bg-terminal-bg
AgentQuickstart bg-terminal-bg-elevated/30
WhyNot          bg-terminal-bg
Installation    bg-terminal-bg-elevated/30   <-- CHANGED from bg-terminal-bg
FinalCta        bg-terminal-bg
Footer          bg-terminal-bg-elevated
```

---

## 3. Asset contract

This section is the interface between Lane A and Lane B. **Both lanes must
agree on it exactly.** Filenames do not change except for one addition, so
the two lanes can run in parallel.

| File in `public/assets/images/` | Source PNG | Geometry | Must show |
|---|---|---|---|
| `landing-hero.webp` | `demo/landing-hero.png` | 1440x720 (2:1) | resting state, both panels, 3-level tree, Complete section, live `claude` claim |
| `landing-agent.webp` | `demo/landing-agent.png` | 1440x540 (8:3) | **two different agent tags on two different rows** |
| `landing-tree.webp` | `demo/landing-tree.png` | 1440x540 (8:3) | a parent deriving COMPLETE from its child, another deriving a percentage |
| `landing-search.webp` | `demo/landing-search.png` | 1440x540 (8:3) | query `timeout`: title match ranked above a notes-only match, across two lists |
| `landing-split.webp` | `demo/landing-split.png` | 1440x720 (2:1) | **NEW.** One frame, two live panes: the agent's shell session on the left, the Farol TUI on the right. See 4.5b -- this is a real two-process recording, not a composite |
| `landing-demo.mp4` | `demo/landing-demo-web.mp4` | 1152x576 | the walkthrough, reseeded |
| `landing-demo-poster.webp` | first frame of the master mp4 | 1152x576 | matches the video's first frame |

Note the mp4 rename on copy: `demo/landing-demo-web.mp4` (the compressed one)
is what lands as `public/assets/images/landing-demo.mp4`. Copying the
uncompressed master instead would ship 407KB instead of 119KB and break the
1s budget.

**Every one of these files changes bytes in this restructure.** Therefore the
service worker cache key must be bumped -- see section 5.10. Skipping that
step means returning visitors keep the gardening screenshots forever, and
nothing you can check on disk will reveal it.

---

## 4. Lane A -- reseed and re-record (repo: `~/Documents/projects/farol/`)

### 4.1 The governing principle: rename only, never reshape

The tapes navigate the task tree by counting keystrokes. `demo/landing.tape`
sends six `Down` presses to land on a specific row; `demo/landing-demo.tape`
sends three `Down`, then `Space`, then two `Up`. **If the tree shape, the
task count, or the ordering changes, every one of those counts silently lands
on the wrong row and the recording is wrong in a way that still exits 0.**

So: change the *strings*, keep the *structure* byte-for-byte identical. Same
number of lists, same number of tasks, same parent-child relationships, same
order, same one task with notes, same one task at 45 percent, same one
completed root with two children.

### 4.2 Edit `demo/seed.sh`

Replace only the task titles, list names, notes text, and the shell variable
names. Change nothing else in the file -- not the `--force` flags, not the
`release --all` at the end, not the comments' structure.

Rename map, in the order the tasks appear in the file:

| Old title | New title | Structure (unchanged) |
|---|---|---|
| `Home` | `api` | list 1 |
| `Plan the garden` | `Ship auth v2` | root, 3 children |
| `Prep the beds` | `Wire the OAuth callback` | child of above, 1 child |
| `Order seed compost` | `Add the state-param check` | grandchild |
| `Build the beds` | `Migrate the sessions table` | child |
| `Source the plants` | `Backfill refresh tokens` | child |
| `Refinish the deck` | `Cut the p95 on /search` | root, carries the notes |
| `Reach the ferns` | `Rewrite the ingest worker` | root, 45 percent |
| `Clean the kitchen` | `Drop the legacy /v1 routes` | root, completed, 2 children |
| `Clear the counters` | `Delete the v1 handlers` | child |
| `Dust the shelves` | `Remove v1 from the router` | child |
| `Deck project` | `infra` | list 2 |
| `Buy lumber` | `Raise the gateway timeout` | root in list 2, search title match |
| `Measure the site` | `Pin the runner image` | root in list 2 |

Shell variable renames (cosmetic, but keep the file readable):
`garden` -> `auth`, `soil` -> `oauth`, `deck` -> `p95`, `ferns` -> `ingest`,
`kitchen` -> `legacy`, `DECK` -> `INFRA`.

The notes line becomes exactly:

```sh
run notes --force "$p95" "The N+1 on tags is most of it. Client timeout is 2s and p95 is 2.4s."
```

That string is load-bearing: it is the only place the word `timeout` appears
in a notes field, and the search screenshot depends on it ranking below the
title match in `infra`.

Also update the explanatory comment above that line so it names the new query
and the new tasks. The comment currently says:

```
#   - "Buy lumber" (Deck project) matches on TITLE      -> ranks first
#   - "Refinish the deck" (Home) matches on NOTES only  -> ranks below
```

It must become:

```
#   - "Raise the gateway timeout" (infra) matches on TITLE     -> ranks first
#   - "Cut the p95 on /search" (api) matches on NOTES only     -> ranks below
```

### 4.3 Verify the seed before recording anything

Recording against an unverified seed wastes the whole lane. Run:

```sh
cd ~/Documents/projects/farol
FAROL_DEMO_VERSION=v0.3.0 ./demo/seed.sh
export XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config
/tmp/farol-demo/farol search "timeout"
```

**Required outcome: exactly two rows, `Raise the gateway timeout` first and
`Cut the p95 on /search` second.**

Farol's search is fuzzy, so a subsequence match on some other title is
possible. If you get more than two rows, or the wrong order, adjust the
*offending* title (not the two above) until the search returns exactly two,
then record which title you changed and report it. Do not proceed with a
search that returns the wrong thing and do not fake the screenshot.

Also confirm no title overflows the pane:

```sh
/tmp/farol-demo/farol tasks api
```

No row may wrap. The longest new title is `Migrate the sessions table` at 26
characters plus indentation; the pane is roughly 80 columns, so this should
pass, but check rather than assume.

### 4.4 Edit `demo/landing-presence.sh` -- add a second agent

This is the change that makes `landing-agent.webp` a genuinely different
image instead of a near-duplicate of the hero.

Today the script claims one task as `claude`. It must claim two tasks under
two identities:

- `Rewrite the ingest worker` as `claude` (unchanged behaviour, new title)
- `Cut the p95 on /search` as `codex` (new)

Implement it by generalising the existing lookup into a function that takes a
title and an agent tag, then calling it twice. Keep the existing safety
behaviour: resolve by title (never a hardcoded ULID, because `seed.sh` mints
fresh ULIDs every run), and exit non-zero with a message if a title is not
found. Keep the existing comment block explaining why this is a script rather
than lines inside the tape, and why the 120s `WorkTTL` means it must run
shortly before the shot.

**Then verify the TUI actually renders both tags** before recording:

```sh
export XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config
./demo/landing-presence.sh
/tmp/farol-demo/farol work --json
```

`farol work --json` must list two live claims with two distinct agent tags.
Then launch `/tmp/farol-demo/farol` and look at the actual rows.

**If the second tag does not render on its row, stop.** Revert
`landing-presence.sh` to a single `claude` claim, record the assets that way,
and report the limitation. Do not ship a caption that describes two agents
over an image that shows one.

### 4.5 Edit `demo/landing.tape` -- one string change

`Type "lumber"` becomes `Type "timeout"`. Nothing else in this file changes.

**Do not add a `Screenshot demo/landing-split.png` line here.** An earlier
draft of this plan produced the split still from this tape by hiding the Lists
panel. That is superseded by 4.5b, which records a real two-process frame
instead. If you add it here as well, the 4.5b recording will be silently
overwritten by whichever tape runs last.

### 4.5b NEW FILE `demo/landing-split.tape` -- the two-process split

This is the most important new asset on the page, and the only one that
*proves* rather than asserts the central claim: your agent and your TUI are
two live processes on one store.

**This is a genuine recording.** Do not fabricate the agent side. Every line
in the left pane must be real output from the reseeded store, produced by
running the commands against it. Inventing the JSON is a section 0.7
violation, and it would be the one invented thing on a page whose entire
argument is "look, it is really doing this".

#### How the split is made

`tmux` is not installed. `herdr` is, and it splits panes, and a nested herdr
session runs correctly inside VHS. A working recipe with a proof frame is
kept in `scripts/farol-split-probe/` of the landingPages repo -- `split.tape`,
`agentshot`, `proof-split.png`. **Start from those files; do not re-derive
this.** Two traps already cost a probe cycle each:

1. **`herdr pane focus left` silently does nothing.** The syntax is
   `herdr pane focus --direction left`. The positional form is not rejected --
   focus just stays where it was, so every keystroke afterwards lands in the
   wrong pane. The first probe launched the TUI into the pane meant for the
   agent and still exited 0. Same failure mode as 4.1: wrong output, clean
   exit, nothing to see in the log.
2. **VHS rejects absolute paths in `Output` and `Screenshot`.** Use relative
   names and `cd` to the destination directory before invoking `vhs`.

Keeping herdr out of the frame is what `agentshot` is for: it **clears the
screen itself** before printing, so its own invocation line, the split
command's JSON reply, and the focus commands are all gone by the time the
screenshot is taken. Verify this held -- `proof-split.png` shows no trace of
herdr, and neither must the shipped frame.

#### Required geometry and content

- `Set Width 1440`, `Set Height 720` -- 2:1, matching `landing-hero.webp`.
  The probe was 1600x800; re-record at 1440x720 so it matches the contract in
  section 3.
- `Set FontSize 16`, `Set Padding 0`, and the same `farol-landing` theme block
  every other landing tape uses. Copy the theme verbatim from
  `demo/landing.tape` -- do not retype it, and do not "fix" its hex values.
- Split ratio `0.45`, so the TUI gets the wider right pane. The TUI needs the
  width; the transcript does not.
- Left pane: a real agent session against the reseeded store. Use the software
  tasks from 4.2, never the gardening ones.
- Right pane: the Farol TUI at rest, showing the same task the agent just
  touched, with the claim lit so the two panes visibly refer to one store.

That last point is the whole shot. If the left pane updates a task the right
pane does not show, the image proves nothing and there is no reason to ship
it.

Output `demo/landing-split.png`.

### 4.6 Edit `demo/landing-demo.tape` -- two string changes

1. `Type "lumber"` becomes `Type "timeout"`.
2. `Type "Order the trellis kit"` becomes `Type "Add rate-limit tests"`.

Change nothing else. Every `Down`, `Up`, `Space`, `Left`, `Right`, and
`Sleep` stays exactly as it is.

### 4.7 Edit `demo/landing-hero.tape`

No directive changes. The recording geometry, the theme, the sleeps, and the
hidden setup are all still correct.

Do update the prose comments that name the old tasks so the file does not
describe a store that no longer exists -- specifically the line mentioning
the live claim on "Reach the ferns".

### 4.8 Record

Order matters. `landing.tape` completes a task (it sends `Space`), which
mutates the store, so the seed must be re-run between tapes.

```sh
cd ~/Documents/projects/farol

# 1. Hero
FAROL_DEMO_VERSION=v0.3.0 ./demo/seed.sh
vhs demo/landing-hero.tape          # -> demo/landing-hero.png

# 2. The three card stills
FAROL_DEMO_VERSION=v0.3.0 ./demo/seed.sh
vhs demo/landing.tape               # -> landing-agent, landing-tree,
                                    #    landing-search

# 3. The two-process split (section 4.5b)
FAROL_DEMO_VERSION=v0.3.0 ./demo/seed.sh
vhs demo/landing-split.tape         # -> demo/landing-split.png

# 4. The video master
FAROL_DEMO_VERSION=v0.3.0 ./demo/seed.sh
vhs demo/landing-demo.tape          # -> demo/landing-demo.mp4
```

Step 3 mutates the store -- the agent side really runs `farol progress` and
`farol comment` -- so the reseed before step 4 is load-bearing, not a
formality.

`FAROL_DEMO_VERSION=v0.3.0` is mandatory on every call. Without it `seed.sh`
bakes in `git describe`, which on a dirty working tree renders something like
`v0.3.0-18-g4c2d40f-dirty` into the header bar -- a version string no user
will ever see.

### 4.9 Post-process

Compress the video master, then extract the poster:

```sh
cd ~/Documents/projects/farol

ffmpeg -y -i demo/landing-demo.mp4 \
  -vf "fps=12,scale=1152:-2:flags=lanczos" \
  -c:v libx264 -crf 33 -preset slow -tune stillimage \
  -pix_fmt yuv420p -an -movflags +faststart \
  demo/landing-demo-web.mp4

ffmpeg -y -i demo/landing-demo.mp4 -vf "select=eq(n\,0)" -frames:v 1 \
  -c:v libwebp -quality 82 demo/landing-demo-poster.webp
```

Convert the five stills to webp. **`cwebp` is not installed in this
environment** -- use ffmpeg's libwebp encoder:

```sh
cd ~/Documents/projects/farol
for f in hero agent tree search split; do
  ffmpeg -y -i demo/landing-$f.png -c:v libwebp -quality 82 demo/landing-$f.webp
done
```

### 4.10 Copy into the landing page

```sh
DEST=/home/filipe/Documents/projects/landingPages/farol/public/assets/images
cp ~/Documents/projects/farol/demo/landing-hero.webp          $DEST/
cp ~/Documents/projects/farol/demo/landing-agent.webp         $DEST/
cp ~/Documents/projects/farol/demo/landing-tree.webp          $DEST/
cp ~/Documents/projects/farol/demo/landing-search.webp        $DEST/
cp ~/Documents/projects/farol/demo/landing-split.webp         $DEST/
cp ~/Documents/projects/farol/demo/landing-demo-poster.webp   $DEST/
cp ~/Documents/projects/farol/demo/landing-demo-web.mp4       $DEST/landing-demo.mp4
```

Note the last line's rename. Copying `landing-demo.mp4` (the master) instead
of `landing-demo-web.mp4` ships 407KB in place of 119KB.

### 4.11 Lane A acceptance

- [ ] `farol search "timeout"` returns exactly two rows in the documented order
- [ ] `farol work --json` shows two live claims with two distinct agent tags
- [ ] All five `.webp` stills exist, each under 60KB
- [ ] `landing-agent.webp` is visibly distinguishable from `landing-hero.webp`
      at thumbnail size (different agent tags on different rows)
- [ ] `landing-split.webp` is 1440x720 and shows **two panes in one frame**
- [ ] Nothing in `landing-split.webp` reveals herdr -- no split-command JSON,
      no `herdr pane focus` line, no stray `clear`
- [ ] Every line in its left pane is real output from the reseeded store, not
      typed prose. Re-run the same commands by hand and diff the text
- [ ] Its two panes refer to the same task -- the row the agent touches on the
      left is visible, and claimed, on the right
- [ ] No frame contains a version string other than `v0.3.0`
- [ ] No frame contains any gardening or housework task
- [ ] `public/assets/images/landing-demo.mp4` is ~119KB, not ~407KB
- [ ] The README at `~/Documents/projects/farol/README.md` still references
      `demo/screenshot-*.png`, which this lane does NOT touch. Leave them.

---

## 5. Lane B -- rebuild the page (repo: `landingPages/farol/`)

All paths below are relative to `/home/filipe/Documents/projects/landingPages/farol/`.

### 5.1 `src/components/Navigation.astro` -- EDIT

Replace the `navLinks` array in the frontmatter with exactly:

```js
const navLinks = [
  { href: '#how-it-works', label: 'How it works' },
  { href: '#demo', label: 'Demo' },
  { href: '#capabilities', label: 'Capabilities' },
  { href: '#install', label: 'Install' },
];
```

Change nothing else in the file.

### 5.2 `src/components/Hero.astro` -- EDIT

Four changes. **Do not touch the beacon markup, the beacon CSS classes, or
anything above the `<h1>`.**

**(a) Keep the `<h1>` exactly as it is.** Three lines, three colours. It is
the strongest copy on the page.

**(b) Replace the subheading paragraph.** Find:

```html
      <p class="text-lg sm:text-xl text-terminal-text-muted max-w-2xl mx-auto mb-10 animate-slide-up animate-delay-200">
        One store. Two views.
        <br class="hidden sm:block" />
        Your agent works; you keep watch.
      </p>
```

Replace its inner content (keep the `<p>` and its classes) with:

```html
        Your coding agent claims tasks and moves them.
        <br class="hidden sm:block" />
        You watch it happen live, in a terminal pane, without switching windows.
```

**(c) Change the secondary CTA label.** Find the `<a href="#install" class="btn-secondary w-full sm:w-auto">` and change its text from `Get Started` to:

```
Install in 10 seconds
```

Leave the primary GitHub button exactly as it is. It stays the dominant CTA.

**(d) Fix the hero image alt text and remove the duplicated image role.**

The wrapper `<div class="terminal-window ...">` currently carries
`role="img"` and an `aria-label` that duplicates the inner `<img>`'s `alt`.
That nests one image role inside another and reads the same description
twice. **Delete `role="img"` and the entire `aria-label` attribute from that
wrapper div.** Keep every class on it.

Then replace the inner `<img>`'s `alt` attribute value with exactly:

```
Farol TUI showing the api list: a three-level task tree with derived progress on parents, a Complete section, the Lists side panel, and a live claim reading 45% claude IN PROGRESS on Rewrite the ingest worker
```

Leave the `src`, `loading`, `fetchpriority`, `decoding`, `width`, `height`,
and `class` attributes untouched. Leave the long explanatory comment above
the `aspect-[2/1]` div untouched -- it documents geometry that has not
changed.

### 5.3 `src/components/Problem.astro` -- NEW

Create the file with exactly this content:

```astro
---
const steps = [
  'You hand the agent a task, then wait for it to finish.',
  'It finishes. You switch to your to-do app, find the row, and tick it off yourself.',
  'An idea lands mid-run. Switch back, type it in, switch away again.',
  'Repeat. The more work in flight, the more of your attention the loop eats.',
];
---

<section id="problem" class="py-20 lg:py-28 bg-terminal-bg-elevated/30" aria-labelledby="problem-title">
  <div class="section-container">
    <div class="max-w-3xl mx-auto text-center mb-12">
      <h2 id="problem-title" class="text-3xl sm:text-4xl font-bold tracking-tight mb-4">
        The <span class="text-terminal-accent">window-switching</span> tax
      </h2>
      <p class="text-lg text-terminal-text-muted">
        Working with a coding agent means running two systems of record at once.
        Keeping them in sync is manual, and it is on you.
      </p>
    </div>

    <div class="max-w-2xl mx-auto">
      <ol class="feature-card p-6 sm:p-8 space-y-5">
        {steps.map((step, i) => (
          <li class="flex gap-4">
            <span class="font-mono text-sm text-terminal-accent shrink-0 pt-1">
              {String(i + 1).padStart(2, '0')}
            </span>
            <p class="text-terminal-text-muted leading-relaxed">{step}</p>
          </li>
        ))}
      </ol>

      <p class="mt-10 text-center text-lg text-terminal-text text-balance">
        Farol removes the second window. Your agent writes to the same store
        your terminal is already showing you.
      </p>
    </div>
  </div>
</section>
```

### 5.4 `src/components/HowItWorks.astro` -- NEW

This is the section that carries the single most important new image: the
human's pane next to the agent's pane.

Note how `agentLines` is built. The JSON line contains `{`, which cannot
appear in Astro markup -- so it lives in a frontmatter array and is rendered
as an expression. Do not move it into the template.

```astro
---
const agentLines = [
  '$ export FAROL_AGENT=claude',
  '$ farol next api --json',
  '{"id":"01JB7Q2KX4","title":"Wire the OAuth callback",',
  ' "status":"in_progress","assignee":"claude"}',
  '$ farol progress 01JB7Q2KX4 --mode percentage --percent 45',
  '$ farol comment 01JB7Q2KX4 "callback verified against staging"',
  '$ farol 01JB7Q2KX4',
];
---

<section id="how-it-works" class="py-20 lg:py-28 bg-terminal-bg" aria-labelledby="how-title">
  <div class="section-container">
    <div class="max-w-3xl mx-auto text-center mb-16">
      <h2 id="how-title" class="text-3xl sm:text-4xl font-bold tracking-tight mb-4">
        Two views. <span class="text-terminal-accent">One store.</span>
      </h2>
      <p class="text-lg text-terminal-text-muted">
        The TUI is your dashboard. The CLI is your agent's API. Neither wraps the
        other &mdash; they read and write the same SQLite file, and a change on one
        side is visible on the other within a second.
      </p>
    </div>

    <div class="max-w-6xl mx-auto">
      <div class="terminal-window flex flex-col">
        <div class="terminal-header">
          <div class="flex items-center gap-1.5">
            <span class="terminal-dot bg-terminal-red" aria-hidden="true"></span>
            <span class="terminal-dot bg-terminal-yellow" aria-hidden="true"></span>
            <span class="terminal-dot bg-terminal-green" aria-hidden="true"></span>
          </div>
          <div class="flex-1 truncate text-center font-mono text-xs text-terminal-text-muted">agent + farol</div>
          <div class="w-14" aria-hidden="true"></div>
        </div>
        <div class="aspect-[2/1] overflow-hidden">
          <img
            src="/assets/images/landing-split.webp"
            alt="One terminal split in two. On the left an agent session runs farol next api --json, farol progress and farol comment against the api list. On the right the Farol TUI shows that same task claimed and moving, updated live."
            loading="lazy"
            decoding="async"
            width="1440"
            height="720"
            class="h-full w-full object-cover"
          />
        </div>
      </div>

      <div class="grid gap-8 sm:grid-cols-2 mt-6">
        <p class="text-sm text-terminal-text-muted">
          <span class="font-semibold text-terminal-text">Left: your agent's pane.</span>
          Plain shell calls. One JSON value per command, on stdout, every time.
          No server, no socket, no handshake.
        </p>
        <p class="text-sm text-terminal-text-muted">
          <span class="font-semibold text-terminal-text">Right: your pane.</span>
          The spinner names the agent holding the row, so you see which task is
          moving &mdash; not just that something changed.
        </p>
      </div>

      <div class="mt-8 lg:hidden">
        <div class="terminal-window flex flex-col">
          <div class="terminal-header">
            <div class="flex items-center gap-1.5">
              <span class="terminal-dot bg-terminal-red" aria-hidden="true"></span>
              <span class="terminal-dot bg-terminal-yellow" aria-hidden="true"></span>
              <span class="terminal-dot bg-terminal-green" aria-hidden="true"></span>
            </div>
            <div class="flex-1 truncate text-center font-mono text-xs text-terminal-text-muted">agent</div>
            <div class="w-14" aria-hidden="true"></div>
          </div>
          <div class="overflow-x-auto">
            <pre class="p-4"><code class="font-mono text-xs leading-relaxed">{agentLines.map((line) => (
              <span class:list={["block whitespace-pre", line.startsWith('$') ? "text-terminal-text" : "text-terminal-green"]}>{line}</span>
            ))}</code></pre>
          </div>
        </div>
        <p class="mt-3 text-sm text-terminal-text-muted">
          The agent side of that frame, as text.
        </p>
      </div>
    </div>
  </div>
</section>
```

**Why the transcript still exists below `lg`.** The image carries the proof,
but at 360px a 1440px-wide two-pane frame is roughly 180px per pane -- the
text in it is decoration, not content. The `lg:hidden` block gives small
viewports the agent side as real selectable text. It costs no image bytes and
it is what keeps the accessibility score honest rather than merely green.

Keep `agentLines` in sync with what the recording actually shows. It is a
caption for the image now, not an independent illustration -- and per section
4.5b those lines are real command output, so they must match the frame.

The `aspect-[2/1]` box is what reserves space and keeps CLS at zero. Do not
remove it. If the `lg:hidden` transcript overflows, it scrolls inside its own
`overflow-x-auto` container, which is the behaviour section 6.5 checks for.

### 5.5 `src/components/DemoVideo.astro` -- EDIT

Two changes. **Do not touch the `<video>` element, the toggle button, the
inline `<script>`, the `aspect-[1152/576]` box, or any of the comment
blocks.** They encode fixes that took several attempts to land.

**(a)** Change the section's background class from `bg-terminal-bg` to
`bg-terminal-bg-elevated/30`. The `id="demo"` and `aria-labelledby` stay.

**(b)** Replace the heading and subheading. Find:

```html
      <h2 id="demo-video-title" class="text-3xl sm:text-4xl font-bold tracking-tight mb-4">
        <span class="text-terminal-accent">Watch it</span> in action
      </h2>
      <p class="text-lg text-terminal-text-muted">
        A 23-second walkthrough of the TUI and CLI working together.
      </p>
```

Replace with:

```html
      <h2 id="demo-video-title" class="text-3xl sm:text-4xl font-bold tracking-tight mb-4">
        <span class="text-terminal-accent">23 seconds,</span> start to finish
      </h2>
      <p class="text-lg text-terminal-text-muted">
        Completing a parent and cascading it to its children, collapsing the tree,
        searching across lists, and adding a nested task &mdash; with an agent's
        claim staying live through all of it.
      </p>
```

### 5.6 `src/components/Capabilities.astro` -- NEW

This replaces both `Features.astro` and `ForAgents.astro`. It reuses the two
existing card components unchanged.

Every claim below was verified against the Go source. Do not alter the
technical wording.

```astro
---
import FeatureCard from '@/components/FeatureCard.astro';
import AgentFeature from '@/components/AgentFeature.astro';

const agentFeatures = [
  {
    number: "01",
    title: "Assignment reserves the subtree",
    description: 'Taking a task with <code>farol next</code> or <code>farol assign</code> locks its whole subtree: no other agent can claim an ancestor or a descendant of it. That is how two agents working the same list never end up doing the same work twice.',
  },
  {
    number: "02",
    title: "One JSON value, always",
    description: 'Every subcommand accepts <code>--json</code> and prints exactly one JSON value on stdout &mdash; the payload, or <code>{"error": "..."}</code>. Nothing else. Exit codes are stable: 0 success, 1 domain failure, 2 usage.',
  },
  {
    number: "03",
    title: "Presence costs the agent nothing",
    description: 'Every write auto-claims presence under <code>FAROL_AGENT</code>, so the spinner lights in your TUI without the agent doing anything for it. For a read-only look, <code>farol claim --kind inspecting</code> says so explicitly.',
  },
  {
    number: "04",
    title: "<code>farol inbox</code>",
    description: 'One call returns the agent\'s own list plus every other list in the store, with pending and complete counts and the top pending tasks in each. The whole board in a single read, at the start of a session.',
  },
  {
    number: "05",
    title: "An ownership gate, not a free-for-all",
    description: 'Structural writes &mdash; <code>add</code>, <code>mv</code>, <code>rm</code>, <code>rename</code> &mdash; refuse to run on a list another agent owns. Status, progress, and comments stay open to everyone, so agents cooperate on work without reshaping each other\'s boards.',
  },
  {
    number: "06",
    title: "<code>farol skill</code>",
    description: 'Prints the complete agent reference as markdown: the identity contract, the working loop, the presence-versus-assignment distinction, and the gotchas. Pipe it into your agent\'s context and it knows the whole API.',
  },
];
---

<section id="capabilities" class="py-20 lg:py-28 bg-terminal-bg" aria-labelledby="capabilities-title">
  <div class="section-container">
    <div class="max-w-3xl mx-auto text-center mb-16">
      <h2 id="capabilities-title" class="text-3xl sm:text-4xl font-bold tracking-tight mb-4">
        What each side <span class="text-terminal-accent">gets</span>
      </h2>
      <p class="text-lg text-terminal-text-muted">
        One store, two audiences. Neither surface is a wrapper around the other.
      </p>
    </div>

    <h3 class="mb-8 text-center font-mono text-sm uppercase tracking-widest text-terminal-accent">
      For you, at the keyboard
    </h3>
    <div class="grid gap-8 md:grid-cols-3 mb-20">
      <FeatureCard
        title="See which task, not just that something changed"
        description="When an agent claims a row, its tag appears on that row. Two agents working at once are two visible spinners, not one vague notification."
        screenshot="/assets/images/landing-agent.webp"
        alt="Farol task list with two live agent claims: claude on Rewrite the ingest worker and codex on Cut the p95 on /search"
      />
      <FeatureCard
        title="Nested tasks with derived progress"
        description="Nest to any depth. A parent's percentage computes from its children, and completing the last child completes the parent."
        screenshot="/assets/images/landing-tree.webp"
        alt="Task tree where Wire the OAuth callback shows COMPLETE derived from its child and Ship auth v2 shows a derived percentage"
      />
      <FeatureCard
        title="Search that spans every list"
        description="Filter the current list with <kbd>/</kbd>, or search the whole store with <kbd>F</kbd>. Title matches rank above notes-only hits."
        screenshot="/assets/images/landing-search.webp"
        alt="Global search for timeout showing a title match from the infra list ranked above a notes-only match from the api list"
      />
    </div>

    <h3 class="mb-8 text-center font-mono text-sm uppercase tracking-widest text-terminal-green">
      For your agent, at the CLI
    </h3>
    <div class="grid gap-6 md:grid-cols-2 lg:grid-cols-3 max-w-6xl mx-auto">
      {agentFeatures.map(feature => (
        <AgentFeature
          number={feature.number}
          title={feature.title}
          description={feature.description}
        />
      ))}
    </div>

    <p class="mt-12 mx-auto max-w-3xl text-center text-sm leading-relaxed text-terminal-text-muted">
      <span class="font-semibold text-terminal-text">Also, without ceremony:</span>
      14 themes &middot; vim and arrow navigation &middot; four-level priority
      &middot; notes, comments and file attachments &middot; a
      <code class="rounded border border-terminal-border bg-terminal-bg px-1.5 py-0.5 font-mono text-xs text-terminal-accent">--since</code>
      change feed &middot; versioned export and import &middot; one static binary,
      no daemon, no config required.
    </p>
  </div>
</section>
```

**Note on `AgentFeature.astro`:** its `title` is currently rendered as
`{title}`, which escapes HTML. Two of the titles above contain `<code>` tags.
So `AgentFeature.astro` needs one small edit -- change:

```astro
  <h3 class="text-lg font-semibold text-terminal-text">{title}</h3>
```

to:

```astro
  <h3 class="text-lg font-semibold text-terminal-text" set:html={title}></h3>
```

and add the same `:global(code)` style block that `FeatureCard.astro` uses, so
inline code in a title is styled. Append to `AgentFeature.astro`:

```astro
<style>
  .agent-feature :global(code) {
    @apply rounded border border-terminal-border bg-terminal-bg px-1.5 py-0.5 font-mono text-xs text-terminal-accent;
  }
</style>
```

and add `agent-feature` to the wrapper div's class list. This is the same bug
class that shipped `<kbd>/</kbd>` as literal text on this page before -- the
existing `description` already uses `set:html` for exactly this reason; the
title was simply never given markup until now.

### 5.7 `src/components/AgentQuickstart.astro` -- NEW

```astro
---
const teachLines = [
  '$ farol skill > .agent/skills/farol.md',
];

const loopLines = [
  '$ export FAROL_AGENT=claude',
  '$ farol inbox --json',
  '$ farol next api --json',
  '$ farol progress <id> --mode percentage --percent 60',
  '$ farol comment <id> "tests green"',
  '$ farol <id>',
  '$ farol release --all',
];
---

<section id="agents" class="py-20 lg:py-28 bg-terminal-bg-elevated/30" aria-labelledby="agents-title">
  <div class="section-container">
    <div class="max-w-3xl mx-auto text-center mb-16">
      <h2 id="agents-title" class="text-3xl sm:text-4xl font-bold tracking-tight mb-4">
        Give your agent the API in <span class="text-terminal-accent">one command</span>
      </h2>
      <p class="text-lg text-terminal-text-muted">
        No MCP server, no daemon, no config file. Farol shipped an MCP server and
        removed it &mdash; a subprocess and a JSON-RPC handshake to run commands
        your agent can already run is a moving part with nothing to show for it.
      </p>
    </div>

    <div class="grid gap-8 lg:grid-cols-2 max-w-5xl mx-auto">
      <div>
        <h3 class="mb-4 text-lg font-semibold text-terminal-text">1. Teach it</h3>
        <div class="terminal-window">
          <div class="terminal-header">
            <div class="flex items-center gap-1.5">
              <span class="terminal-dot bg-terminal-red" aria-hidden="true"></span>
              <span class="terminal-dot bg-terminal-yellow" aria-hidden="true"></span>
              <span class="terminal-dot bg-terminal-green" aria-hidden="true"></span>
            </div>
            <div class="flex-1 text-center font-mono text-xs text-terminal-text-muted">$</div>
          </div>
          <pre class="overflow-x-auto p-4"><code class="font-mono text-sm leading-relaxed text-terminal-text">{teachLines.join('\n')}</code></pre>
        </div>
        <p class="mt-4 text-sm text-terminal-text-muted leading-relaxed">
          <code class="rounded border border-terminal-border bg-terminal-bg px-1.5 py-0.5 font-mono text-xs text-terminal-accent">farol skill</code>
          emits the whole reference: the identity contract, the working loop, the
          ownership gate, and the traps an agent hits on its first run.
        </p>
      </div>

      <div>
        <h3 class="mb-4 text-lg font-semibold text-terminal-text">2. That is the whole loop</h3>
        <div class="terminal-window">
          <div class="terminal-header">
            <div class="flex items-center gap-1.5">
              <span class="terminal-dot bg-terminal-red" aria-hidden="true"></span>
              <span class="terminal-dot bg-terminal-yellow" aria-hidden="true"></span>
              <span class="terminal-dot bg-terminal-green" aria-hidden="true"></span>
            </div>
            <div class="flex-1 text-center font-mono text-xs text-terminal-text-muted">$</div>
          </div>
          <pre class="overflow-x-auto p-4"><code class="font-mono text-sm leading-relaxed text-terminal-text">{loopLines.join('\n')}</code></pre>
        </div>
        <p class="mt-4 text-sm text-terminal-text-muted leading-relaxed">
          Every id argument takes an unambiguous ULID prefix, so eight characters
          copied out of any JSON response is enough.
        </p>
      </div>
    </div>
  </div>
</section>
```

**Note:** `<id>` inside those strings is fine -- they are JavaScript strings
rendered as text nodes, so Astro escapes the angle brackets for you. Do not
"fix" them into entities; that would print the entities literally.

### 5.8 `src/components/WhyNot.astro` -- NEW

Deliberately generic. No competing product is named, by decision.

```astro
---
const faqs = [
  {
    q: 'Why not a markdown checklist in the repo?',
    a: 'It works until two agents edit it at the same time. Farol stores tasks transactionally, and assignment reserves a whole subtree, so a second agent cannot take work that is already claimed. A markdown file has no way to say "someone is on this right now".',
  },
  {
    q: 'Why not the to-do list built into my agent?',
    a: 'That list lives inside one session. It disappears when the session ends, it is not shared with a second agent, and you cannot watch it while the agent is mid-run. Farol\'s store outlives the session, and you are looking at it the whole time.',
  },
  {
    q: 'Why not a classic terminal task manager?',
    a: 'Those are built for one person typing. Farol is built for a human and several agents writing at once: every write carries an identity, live presence shows up in the UI, and structural writes are gated by list ownership.',
  },
  {
    q: 'Why not an MCP server?',
    a: 'Farol had one and retired it. Running a subprocess and a JSON-RPC handshake so an agent can invoke a command it could already invoke adds a moving part and buys nothing. The CLI is the API, and it works from any agent, any script, any shell.',
  },
];

const faqJsonLd = JSON.stringify({
  "@context": "https://schema.org",
  "@type": "FAQPage",
  "mainEntity": faqs.map((f) => ({
    "@type": "Question",
    "name": f.q,
    "acceptedAnswer": { "@type": "Answer", "text": f.a },
  })),
});
---

<section id="why-not" class="py-20 lg:py-28 bg-terminal-bg" aria-labelledby="why-not-title">
  <div class="section-container">
    <div class="max-w-3xl mx-auto text-center mb-16">
      <h2 id="why-not-title" class="text-3xl sm:text-4xl font-bold tracking-tight mb-4">
        Why not <span class="text-terminal-accent">just...</span>
      </h2>
      <p class="text-lg text-terminal-text-muted">
        The honest answers. Farol is worth installing for one specific reason,
        and it is not that it stores tasks.
      </p>
    </div>

    <div class="grid gap-6 md:grid-cols-2 max-w-5xl mx-auto">
      {faqs.map(faq => (
        <div class="feature-card p-6">
          <h3 class="mb-3 text-lg font-semibold text-terminal-text">{faq.q}</h3>
          <p class="text-sm leading-relaxed text-terminal-text-muted">{faq.a}</p>
        </div>
      ))}
    </div>
  </div>

  <script type="application/ld+json" set:html={faqJsonLd} />
</section>
```

### 5.9 `src/components/Installation.astro` -- EDIT

**(a)** Change the section background from `bg-terminal-bg` to
`bg-terminal-bg-elevated/30`.

**(b)** Delete the third `<InstallMethod ... title="Build from source" ... />`
block entirely.

**(c)** Change the grid from `lg:grid-cols-3` to `md:grid-cols-2`, and change
`max-w-5xl` to `max-w-4xl` on that same div.

**(d)** In the remaining "Pre-built binary" block, the `command` string
contains a literal em dash. Replace that one character with `--`:

```
# Linux (amd64) -- more platforms in releases
```

That string is rendered as `{command}`, a text node, so it must be ASCII per
rule 0.1.

**(e) Leave the "First run" terminal block exactly as it is.** It states the
data path, the config path, the default list `"Inbox"`, and the default theme
`farol-dark`. All four were verified against the Go source on 2026-08-16 and
are correct. Do not touch them.

Leave the headline, the subheadline, and everything else in this file as is.

### 5.10 `src/components/FinalCta.astro` -- NEW

```astro
---
---

<section class="py-20 lg:py-28 bg-terminal-bg border-t border-terminal-border" aria-labelledby="final-cta-title">
  <div class="section-container">
    <div class="max-w-2xl mx-auto text-center">
      <h2 id="final-cta-title" class="text-3xl sm:text-4xl font-bold tracking-tight text-balance mb-4">
        Stop checking off your agent's <span class="text-terminal-accent">homework</span>
      </h2>
      <p class="text-lg text-terminal-text-muted mb-10">
        One binary. No server. MIT licensed. Runs on macOS, Linux, and Windows.
      </p>
      <a href="https://github.com/filipemolina/farol" target="_blank" rel="noopener noreferrer" class="btn-primary group">
        <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576C20.566 21.797 24 17.3 24 12c0-6.627-5.373-12-12-12z"/>
        </svg>
        Get Farol on GitHub
      </a>
    </div>
  </div>
</section>
```

Copy the `<path d="...">` string verbatim from the existing `Hero.astro`
GitHub icon rather than retyping it.

### 5.11 `src/pages/index.astro` -- EDIT

Replace the whole file with:

```astro
---
import Layout from '@/layouts/Layout.astro';
import Navigation from '@/components/Navigation.astro';
import Hero from '@/components/Hero.astro';
import Problem from '@/components/Problem.astro';
import HowItWorks from '@/components/HowItWorks.astro';
import DemoVideo from '@/components/DemoVideo.astro';
import Capabilities from '@/components/Capabilities.astro';
import AgentQuickstart from '@/components/AgentQuickstart.astro';
import WhyNot from '@/components/WhyNot.astro';
import Installation from '@/components/Installation.astro';
import FinalCta from '@/components/FinalCta.astro';
import Footer from '@/components/Footer.astro';
---

<Layout title="Farol - a terminal to-do list you can watch, and your agent can drive" description="Farol is a terminal to-do list built for working alongside coding agents. A TUI for you, a CLI for them, one SQLite store, live presence on every row. No server required.">
  <Navigation />
  <main id="main-content">
    <Hero />
    <Problem />
    <HowItWorks />
    <DemoVideo />
    <Capabilities />
    <AgentQuickstart />
    <WhyNot />
    <Installation />
    <FinalCta />
  </main>
  <Footer />
</Layout>
```

Note the title uses a plain hyphen, not an em dash, per rule 0.1.

### 5.12 Delete the replaced components

```sh
rm src/components/Features.astro
rm src/components/ForAgents.astro
```

**Do not delete** `FeatureCard.astro`, `AgentFeature.astro`, or
`InstallMethod.astro`.

### 5.13 `src/layouts/Layout.astro` -- EDIT

Update the JSON-LD `description` field to match the new page description:

```js
  "description": "Farol is a terminal to-do list built for working alongside coding agents. A TUI for you, a CLI for them, one SQLite store, live presence on every row. No server required.",
```

Change nothing else in this file. In particular, leave the font preloads, the
service worker registration, and the meta tags alone.

### 5.14 `public/sw.js` -- EDIT (do not skip this)

Every image on the page changed bytes. The service worker is cache-first for
static assets, so a returning visitor keeps the old gardening screenshots
indefinitely unless the cache key changes. This has already caused a shipped
visual bug on this page once.

Change:

```js
const CACHE_NAME = 'farol-landing-v4';
```

to:

```js
const CACHE_NAME = 'farol-landing-v5';
```

Add a comment above it in the same style as the existing v3 -> v4 note,
saying: v4 -> v5 (2026-08-16): the demo store was reseeded from housework to a
software project, so every still and the video changed bytes under unchanged
filenames -- the one case a cache-first worker cannot detect on its own.

`PRECACHE_ASSETS` stays as it is. `landing-hero.webp` is still the LCP image
and still the only still worth precaching.

### 5.15 Lane B acceptance

- [ ] `pnpm run check` passes with no errors
- [ ] `pnpm run build` succeeds
- [ ] `LC_ALL=C grep -rn '[^ -~\t]' src/components/Problem.astro src/components/HowItWorks.astro src/components/Capabilities.astro src/components/AgentQuickstart.astro src/components/WhyNot.astro src/components/FinalCta.astro` prints nothing
- [ ] `grep -c 'Built for' dist/index.html` returns 0 (old heading gone)
- [ ] `grep -c 'First-class' dist/index.html` returns 0 (old heading gone)
- [ ] `grep -c 'window-switching' dist/index.html` returns 1
- [ ] `grep -c 'reserves the subtree' dist/index.html` returns 1
- [ ] `grep -o '{expr}\|{feature\|{line}' dist/index.html` prints nothing (the class-interpolation trap)
- [ ] `grep -c 'landing-split.webp' dist/index.html` returns 1
- [ ] `grep -c 'FAQPage' dist/index.html` returns 1
- [ ] `grep -c 'farol-landing-v5' dist/sw.js` returns 1

---

## 6. Verification -- after both lanes land

**Never measure a moving target.** Do not run any of this while an agent may
still be writing. Wait for both lanes to report done, then:

```sh
cd /home/filipe/Documents/projects/landingPages/farol
rm -rf dist
pnpm run check
pnpm run build
```

### 6.1 Rendered-output checks

Reading source proves nothing here -- four `class` attributes on this exact
page read correctly in source and had never worked in the browser. Run every
`dist/` grep in section 5.15.

Additionally, confirm no raw markup leaked into text:

```sh
grep -o '&lt;code&gt;\|&lt;kbd&gt;' dist/index.html    # must print nothing
```

If that prints anything, a `set:html` was missed somewhere -- most likely the
`AgentFeature.astro` title change in section 5.6.

### 6.2 Geometry

Serve the built site and measure the two new image boxes. The recipe is in
`FAROL-VISUAL-ASSETS-PLAN.md` section 6, "Measuring rendered geometry". The
`landing-split.webp` container must render at ratio 2.0 (+/- 0.02); anything
else means `object-cover` is cropping the frame. Cropping this particular
image is worse than cosmetic -- it eats a pane edge, and the two-pane
composition is the entire point of the shot.

### 6.3 Budget

```sh
du -sh dist/
ls -la dist/assets/images/
```

Total page weight must not regress. The new section adds one image
(`landing-split.webp`) and removes no image. It is 1440x720 rather than the
1440x540 of the card stills, and it is denser -- two panes of text -- so
budget **up to 55KB** for it rather than the ~35KB a card still costs. If it
lands materially above that, re-encode at a lower `-quality` before accepting
it; do not shrink the pixel dimensions, because the text legibility at 2:1 is
what the image is for.

### 6.4 Lighthouse

100 on all four categories, mobile and desktop. This is non-negotiable per
`AGENTS.md`. Pay particular attention to:

- **CLS**: the split image in `HowItWorks` is the only lazily-loaded thing in
  that section, and its `aspect-[2/1]` box reserves the space. The `lg:hidden`
  transcript below it is static markup with no async load, so it cannot shift
  anything -- but confirm zero shift rather than reasoning about it.
- **Accessibility**: the new `<h3>` elements in `Capabilities` sit between an
  `<h2>` and the cards' own `<h3>`s. Verify the heading order does not skip a
  level and that the card headings are still `<h3>`, not promoted.

### 6.5 Manual read at 360px

The page must have no horizontal scroll at 360px wide. The two most likely
offenders are the `HowItWorks` agent transcript (long lines in a `<pre>`) and
the `AgentQuickstart` loop block. Both have `overflow-x-auto` or
`overflow-auto` on their own container, so the scroll should stay inside the
box. Confirm it does.

Also specific to the split image, at 360px:

- The `lg:hidden` transcript below it must be **visible**, and the image must
  still be present above it. If the transcript is missing, the mobile reader
  has an illegible 180px-per-pane picture and no text at all.
- Confirm the image is not the LCP element on mobile. It is `loading="lazy"`
  and sits well below the fold, so it should not be -- but the section moved,
  and this is exactly the kind of thing that silently costs the performance
  100.

---

## 7. Explicitly out of scope

Do not do any of these as part of this work.

- **The hero beacon.** The animation, its CSS, and every comment in
  `global.css` stay untouched. It is finished work.
- **The A/B variants** required by `AGENTS.md` constraint 9. Farol has no
  variant infrastructure today, and building it is a separate piece of work
  tracked in `AB-TESTING-PLAN.md`. Note the gap, do not close it here.
- **The README screenshots** at `~/Documents/projects/farol/demo/screenshot-*.png`.
  They are recorded by `demo/screenshots.tape` at a different size and in
  Dracula chrome for GitHub. They will still show gardening after this work.
  That is a known, accepted inconsistency; fixing it is a separate task.
- **The stale README claim** that `farol skill` is planned. Worth fixing in the
  farol repo, but not part of this restructure. Report it, do not fix it here.
- **Any change to the Farol Go source.** This work is copy, structure, and
  demo data only.
- **`Footer.astro`.** It is fine as it stands.

---

## 8. Decisions already made -- do not reopen

| Question | Decision | Why |
|---|---|---|
| Reseed the demo store to a software project? | **Yes** | The page claims a coding agent drives the tool and shows gardening. Highest-impact single change on the page. |
| Add a social-proof or metrics block? | **No** | There is no user base yet. Inventing numbers is forbidden by `AGENTS.md`. The existing hero badge row (single binary / MIT / works offline) is the trust block. |
| Name competing tools in the "Why not" section? | **No, stay generic** | Named comparisons invite comparison shopping and date badly. |
| Split-screen image inside VHS? | **Yes, via herdr** (reversed 2026-08-16) | Originally no, because tmux is not installed. herdr splits panes and runs inside VHS, so the blocker was never real. Recording it proved feasible end to end -- see 8.1. The page's central claim is that the agent and the TUI are two live processes on one store, and a real frame is the only thing that shows it rather than asserting it. |
| Keep `FeatureCard.astro` and `AgentFeature.astro`? | **Yes** | Both are reused by `Capabilities.astro`. Only the two section files are deleted. |
| Change the hero `<h1>`? | **No** | It is the strongest copy on the page. |

### 8.1 A real two-process split IS recordable -- herdr replaces tmux

**Investigated 2026-08-16. Feasibility proven. ADOPTED** -- the procedure is
section 4.5b and the consuming markup is section 5.4. What follows is the
evidence and the traps; execute from 4.5b.

The "no tmux" premise above is true but no longer decisive. `herdr` splits
panes, and a nested herdr session runs happily inside VHS. A genuine
split-screen still -- an agent session on the left, the live Farol TUI on the
right, both real processes in one frame -- was recorded end to end. The recipe
and its proof frame are kept in `scripts/farol-split-probe/`:
`split.tape`, `agentshot`, and `proof-split.png` (1600x800).

To reproduce: `cd` to a scratch dir and run `vhs .../split.tape` from there
(see trap 2 below). It needs `/tmp/farol-demo/` populated -- the seeded store
and the `farol` binary -- exactly as the section 4 recording procedure sets up.

Two traps cost a probe cycle each; both are recorded here so they are not
rediscovered:

1. **`herdr pane focus left` silently does nothing.** The syntax is
   `herdr pane focus --direction left`. The positional form is not rejected
   loudly -- focus simply stays put, so every keystroke after it lands in the
   *wrong pane*. The first probe launched Farol into the pane meant for the
   agent and still exited 0. This is the section 4.1 failure mode in a new
   costume: wrong output, clean exit.
2. **VHS `Output` and `Screenshot` reject absolute paths.** Use relative
   names and `cd` to the destination before invoking `vhs`.

The composition trick that keeps the plumbing out of frame: the left pane runs
a small script (`/tmp/farol-demo/agentshot`) that **clears the screen itself**,
prints the transcript, then calls `herdr pane focus --direction right`. Because
the script clears first, its own invocation line, the split's JSON reply, and
the focus commands are all gone from the final frame. Nothing in
`proof-split.png` reveals that herdr is involved.

**The proof frame is a feasibility probe, not a draft asset.** Two things
about it must not be copied forward, and 4.5b already requires both:

- Its store is **still the gardening seed** -- section 4.2 had not run when it
  was taken. The shipped frame comes after the reseed.
- Its left-hand transcript was **hand-written and printed by `cat`**. Shipping
  that would invent command output, which section 0.7 forbids. The shipped
  frame runs the commands for real -- the binary is right there, and real
  output is strictly better evidence anyway.

**What this cost, honestly.** The HTML two-column it replaces was cheaper on
every axis `AGENTS.md` measures: real selectable text instead of a raster, and
it reflowed at 360px where an image cannot. Two of those three are bought back
-- bytes come out roughly even because the image *replaces* the old
`landing-split.webp` rather than adding to it, and section 5.4 keeps the
transcript as text below `lg` so small viewports lose nothing. What is
genuinely spent is desktop text selectability, in exchange for the page
showing its central claim instead of asserting it.

---

## 9. Suggested dispatch

Per `CLAUDE.md`: one phase, one dispatch. Two lanes, and they touch disjoint
repositories, so they parallelize safely.

| Lane | Repo | Brief covers | Tag |
|---|---|---|---|
| A | `~/Documents/projects/farol/` | Sections 3, 4 | `@fixer` (mechanical string edits plus a documented recording procedure) |
| B | `landingPages/farol/` | Sections 3, 5 | `@designer` (new sections, layout, copy) |

**Lane A's brief must carry section 4.5b in full, plus the two herdr traps
from 8.1**, and must state that `scripts/farol-split-probe/` is the starting
point. The split recording is the one part of Lane A that is not a mechanical
string edit, and both of its traps fail *silently* -- an agent that hits them
produces a wrong frame and reports success. Copy the probe files into the
agent's cwd, or the cross-directory read will stall the run on a permission
prompt (see `CLAUDE.md`, "Keep briefs inside the agent's cwd").

Both briefs must carry section 0 verbatim.

Section 3 is the shared contract and must appear in **both** briefs. If the
lanes disagree on a filename, Lane B ships `<img>` tags pointing at files that
do not exist, and the build will not catch it -- Astro does not validate
`public/` paths.

Verification (section 6) is the orchestrator's job and must not be delegated
to either lane. `RESULT=done` means the turn ended, not that the work is
correct.
