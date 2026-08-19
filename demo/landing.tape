# VHS tape for the FAROL LANDING PAGE stills.
#
# This is NOT the README set. demo/screenshots.tape stays as it is: the README
# is read on GitHub, where 1200x660 in Dracula chrome is correct. The landing
# page is a different surface with different requirements, so it gets its own
# tape rather than a reinterpretation of that one.
#
# Record from the repo root:
#
#   ./demo/seed.sh                 # MANDATORY -- see "the polluted store" below
#   vhs demo/landing.tape          # -> demo/landing-{agent,search,tree}.png
#
# THE HERO IS NO LONGER RECORDED HERE. It moved to demo/landing-hero.tape on
# 2026-08-16 because it needed a taller frame (1440x720, 2:1) than the three
# feature-card stills can have: those sit in a grid beside icon-only cards that
# are also aspect-[8/3], so re-shaping them breaks the grid. This tape keeps
# 1440x540 for exactly that reason. Recording the hero now takes two runs --
# reseed between them, since this tape completes a task.
#
# Three things this tape does differently from screenshots.tape, each fixing a
# defect that shipped on the landing page (FAROL-VISUAL-ASSETS-PLAN.md Sec1):
#
# 1. 1440x540 -- exactly 8:3, and the page containers are set to aspect-[8/3]
#    to match. The old 640x352 (1.818) sources were object-cover'd into a 4:3
#    box, which threw away 26.7% of every image's width -- the lists panel on
#    the right and the selection gutter on the left, i.e. the two things that
#    make it read as a navigable TUI. Source ratio == container ratio => zero
#    crop. Do not change these numbers without changing the container to match.
#
#    The height is 540 rather than a "standard" 810 because it was chosen from
#    the CONTENT: at 810 this seed filled about 60% of the frame and the rest
#    was empty navy, which is dead bytes in the LCP image and makes the text
#    tiny once a card scales it to ~516px. 540 is the height at which the
#    seeded Home list plus its Complete section plus the keybinding bar fill
#    the frame. Re-tune it if the seed grows.
#
# 2. The terminal chrome is the PAGE's navy, not Dracula's grey. Every shipped
#    screenshot had #1D1D29 in its corners, which is neither Dracula's #282A36
#    nor the page's #0E1B30 nor the card's #212D41 -- three near-but-unequal
#    darks stacked inside 20px. The theme below pins the chrome to the page's
#    own background, and Padding 0 means there is almost none of it to see.
#
#    Verify in the PIXELS, never by reading this file:
#      ffmpeg -v error -i demo/landing-hero.png -vf "crop=4:4:0:0" \
#        -f rawvideo -pix_fmt rgb24 - | od -An -tu1 -N3
#    The corner lands on the app's own header tier rather than on VHS's
#    padding (Padding is 0), so the exact triple depends on which tier the
#    first pixel falls in -- 13 25 47 and 24 36 58 have both been observed and
#    both are correct. What matters is that blue >> red, i.e. it is brand
#    navy. The bug this guards against is 29 29 41 (#1D1D29), the desaturated
#    grey-purple every shipped screenshot had, which fails that test.
#
# 3. The store is seeded immediately before recording. Every screenshot on the
#    landing page today shows a list called "Weekend chores" holding two tasks
#    -- leftovers from demo.tape, which shares /tmp/farol-demo. screenshots.tape
#    does not seed itself and was run after the GIF, so it photographed the
#    GIF's debris. seed.sh now also releases its own auto-claims, so a spinner
#    in a frame means something.
#
# The app theme stays farol-dark. Unlike cais, farol's landing page palette IS
# the app's default theme -- tailwind.config.mjs's terminal.* tokens are a
# literal copy of farol-dark's Go source. There is nothing to re-theme.

Set Shell "bash"
Set FontSize 18
Set Width 1440
Set Height 540
Set Padding 0
Set CursorBlink false
Set TypingSpeed 45ms

# The page's own background, so any chrome that does show through is invisible
# against the card it sits in. The ANSI slots are farol-dark's status
# vocabulary (Complete / InProgress / Danger / Accent) so anything the app
# renders through a bare ANSI index lands on-brand instead of on Dracula.
Set Theme { "name": "farol-landing", "background": "#0E1B30", "foreground": "#F5EDE4", "cursor": "#F0B263", "selection": "#22385C", "black": "#0E1B30", "red": "#D9534F", "green": "#67C58A", "yellow": "#E8C547", "blue": "#22385C", "magenta": "#EB4268", "cyan": "#8A94A8", "white": "#F5EDE4", "brightBlack": "#3A4A63", "brightRed": "#EB4268", "brightGreen": "#67C58A", "brightYellow": "#F0B263", "brightBlue": "#3A4A63", "brightMagenta": "#EB4268", "brightCyan": "#C4BEB6", "brightWhite": "#F5EDE4" }

# --- Setup, entirely hidden -------------------------------------------------
# Light the live-agent spinner BEFORE the TUI starts. The TUI polls SQLite
# every second, so a claim made here is on screen by the time anything is
# photographed. The helper is a script precisely so this line has no quoting
# in it to get wrong.
Hide
Type "export XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config"
Enter
Sleep 500ms
Type "./demo/landing-presence.sh"
Enter
Sleep 1500ms
Type "clear"
Enter
Sleep 300ms

# Launch the TUI and let it paint before anything is shown.
Type "/tmp/farol-demo/farol"
Enter
Sleep 4s

# The app launches with the Lists panel ALREADY OPEN and focus on the task
# tree. That is worth stating because two earlier attempts at this tape
# assumed the opposite and pressed L to "open" it -- which closed it, put every
# later toggle off by one, and ended up completing a task in the wrong list
# and photographing a two-task list as the "tree-structured tasks" shot.
# Nothing below opens the panel; the hero simply uses the state the app
# starts in.
Escape
Sleep 800ms
Show

# ---------------------------------------------------------------------------
# A NOTE ON SEQUENCING, learned by getting it wrong twice.
#
# Do not count toggles. The first two attempts at this tape both drifted,
# because "L" is a toggle whose parity depends on state this tape cannot see,
# and one wrong parity silently moves every later keystroke into the wrong
# list -- the second attempt completed a task in "Deck project" and
# photographed a two-task list as the "tree-structured tasks" shot, which is
# the exact defect this tape exists to fix.
#
# So: every shot below is taken from a state that is ENTERED EXPLICITLY, in
# one forward pass, and the list is never changed. The Lists panel is a
# transient picker (docs/DESIGN.md Sec5) -- it closes as soon as focus leaves it
# or a list is selected, which is why "both panels open with the tree
# focused", the state TerminalDemo.astro draws, is not a state the app has.
# ---------------------------------------------------------------------------

# --- HERO: moved out, deliberately -------------------------------------------
# `Screenshot demo/landing-hero.png` used to be here. It was NOT merely moved
# to demo/landing-hero.tape -- it was DELETED from this tape, because leaving it
# would mean any routine re-record of the feature cards silently overwrites the
# 1440x720 hero with a 1440x540 one, and nothing on the page would fail loudly:
# Hero.astro's aspect-[2/1] would just object-cover a third off the top and
# bottom. Do not add it back.
#
# The keystrokes below are unchanged, so the state each remaining shot is
# entered from is exactly what it was.

# --- AGENT PRESENCE ---------------------------------------------------------
# The page's most distinctive claim -- set FAROL_AGENT and watch a spinner
# appear on the row -- had NO screenshot at all; the card used a picture of the
# add-task input instead. Park the cursor ON "Rewrite the ingest worker" so the claimed
# row is the highlighted subject.
#
# From "Ship auth v2" (row 0) the visible order is: Wire the OAuth callback, Add
# the state-param check, Migrate the sessions table, Backfill refresh tokens,
# Cut the p95 on /search, Rewrite the ingest worker -- six Downs to row 6.
Down
Down
Down
Down
Down
Down
Sleep 800ms
Screenshot demo/landing-agent.png

# The details panel was recorded here as an alternative framing for the agent
# card and REJECTED after looking at it: for this task it is a near-empty
# modal (no notes, no comments) that does not show the agent at all. Kept as a
# comment so the next person does not re-derive it.
#   Enter / Screenshot demo/landing-details.png / Escape

# --- TREE: nesting and DERIVED progress -------------------------------------
# The "Tree-structured tasks" card promises "complete every child and the
# parent completes itself". Its shipped image is a flat two-task list with no
# tree and no Complete section, which demonstrates the opposite.
#
# The cursor is on row 6 ("Rewrite the ingest worker"). Four Ups reach row 2,
# "Add the state-param check" -- the only child of "Wire the OAuth callback",
# which is itself a child of "Ship auth v2". space completes it, so "Wire the
# OAuth callback" has every child complete and derives completion, and "Ship
# auth v2" derives a partial percentage from its four children. The card's
# sentence becomes something visible in the frame instead of a claim sitting
# next to a static 0%.
#
# (Updated 2026-08-19 for the current seed.sh, which replaced the old
# "Plan the garden" gardening-themed seed with the "Ship auth v2" api seed -
# the row-index math above still lands in the same structural shape, a
# grandchild completing and cascading derived progress up two levels, so
# this needed a rename, not a redesign.)
Up
Up
Up
Up
Sleep 600ms
Space
Sleep 1500ms
Screenshot demo/landing-tree.png

# --- Close the Lists panel for the search shot ------------------------------
# L toggles the panel that has been open since launch, so the search modal
# reads against a full-width tree instead of competing with a sidebar. A
# separate "lists panel" still was recorded here and dropped: with the panel
# open in the hero already, it was the same frame minus the panel.
Type "L"
Sleep 1000ms

# --- SEARCH: real matches, and the ranking rule ------------------------------
# The shipped search screenshot reads "No matches" -- screenshots.tape types
# the task it later searches for, then presses Escape, which DISCARDS the
# input, so the task never existed and the search correctly found nothing.
#
# "timeout" is chosen to prove the card's exact sentence, "Title matches rank
# above notes-only hits", in one frame:
#   Raise the gateway timeout (infra) -- TITLE match      -> first
#   Cut the p95 on /search (api)         -- NOTES-only match -> below
# which also spans two lists, so it shows cross-list search at the same time.
# seed.sh writes that note for this shot.
Type "F"
Sleep 800ms
Type "timeout"
Sleep 1500ms
Screenshot demo/landing-search.png
Escape
Sleep 500ms

Ctrl+C
Sleep 600ms
