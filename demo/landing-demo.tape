# VHS tape for the FAROL LANDING PAGE walkthrough video.
#
# This is NOT demo/demo.tape. That one produces the README's GIF at 1200x660
# in Dracula chrome and stays exactly as it is. This one produces an H.264
# master for the landing page's <video>, in the page's own navy chrome at the
# same 1440x720 the hero still uses.
#
# The height moved from 540 to 720 on 2026-08-16, alongside demo/landing-hero.
# tape. At 540 (8:3) the player read as a letterbox slot rather than as a
# terminal; 720 is 2:1, which is 30 rows at FontSize 18 against the ~23 the
# seed fills. The FONT MUST STAY AT 18: a cell is ~0.645*FontSize wide, so 1440
# holds the 120 columns that constants.AUTO_SHOW_LISTS_MIN_WIDTH needs to
# auto-open the Lists panel only while FontSize <= 18. Raise it and the panel
# silently vanishes from the recording. The full reasoning, including why the
# frame is not 16:9, is in demo/landing-hero.tape's header.
#
# Record from the repo root:
#
#   ./demo/seed.sh                 # MANDATORY, and AGAIN after landing.tape
#   vhs demo/landing-demo.tape     # -> demo/landing-demo.mp4  (a MASTER)
#
# The VHS output is a master, not a shippable asset -- it is several MB. It
# must go through the compression pass documented at the bottom of this file
# before it reaches the page. Do not wire the raw file into the site.
#
# GIF is deliberately not produced here. The landing page shipped a 163 KB
# demo.gif that nothing referenced; the same content as H.264 is both smaller
# per second and far higher quality. VP9/webm was measured on the sibling
# cais project and came out LARGER than H.264 for terminal content
# (793 KB vs 569 KB), so H.264 is the only codec here.
#
# This tape MUTATES the store (it completes a task and creates one). Re-run
# ./demo/seed.sh before recording anything else.

Set Shell "bash"
Set FontSize 18
Set Width 1440
Set Height 720
Set Padding 0
Set CursorBlink false
Set TypingSpeed 55ms

Output demo/landing-demo.mp4

# Same chrome as demo/landing.tape -- see that file's header for why the
# terminal background is the landing page's navy rather than Dracula's grey.
Set Theme { "name": "farol-landing", "background": "#0E1B30", "foreground": "#F5EDE4", "cursor": "#F0B263", "selection": "#22385C", "black": "#0E1B30", "red": "#D9534F", "green": "#67C58A", "yellow": "#E8C547", "blue": "#22385C", "magenta": "#EB4268", "cyan": "#8A94A8", "white": "#F5EDE4", "brightBlack": "#3A4A63", "brightRed": "#EB4268", "brightGreen": "#67C58A", "brightYellow": "#F0B263", "brightBlue": "#3A4A63", "brightMagenta": "#EB4268", "brightCyan": "#C4BEB6", "brightWhite": "#F5EDE4" }

# --- Setup, entirely hidden -------------------------------------------------
# The agent claim is lit before launch so the spinner is on screen for the
# whole recording -- the video's job is to show a human and an agent working
# the same list at the same time, which is the page's core claim.
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
Type "/tmp/farol-demo/farol"
Enter
Sleep 4s
Show

# --- Open on the resting state ----------------------------------------------
# The Lists panel is already open at launch with focus on the tree (see
# landing.tape). Hold here so the first frame -- which becomes the poster --
# is the full product: nested tree, derived progress, Complete section, and
# the live "claude" claim on "Rewrite the ingest worker".
Sleep 2500ms

# --- Walk down the tree ------------------------------------------------------
# Slow, deliberate movement. This is a keyboard-driven TUI; the video should
# read as someone using it, not as a cut sequence.
Down
Sleep 700ms
Down
Sleep 700ms
Down
Sleep 900ms

# --- Complete a leaf and let the parents derive -------------------------------
# The cursor is on "Build the beds". space completes it, and "Plan the garden"
# recomputes its percentage from its children -- the single most specific thing
# farol does, happening on screen rather than being asserted in copy.
Space
Sleep 2000ms

# --- Collapse and expand the subtree ------------------------------------------
# Shows the tree is a tree, not an indented list.
Up
Up
Sleep 600ms
Left
Sleep 1200ms
Right
Sleep 1500ms

# --- Global search across lists ----------------------------------------------
# "timeout" returns a title match in one list above a notes-only match in
# another, which is the ranking rule the page claims. seed.sh writes the note.
Type "F"
Sleep 900ms
Type "timeout"
Sleep 2000ms
Escape
Sleep 1000ms

# --- Add a nested task --------------------------------------------------------
# n opens the inline create input; ] makes the new task a child of the row it
# is anchored on. Enter SAVES it -- note that demo/screenshots.tape presses
# Escape here instead, which discards the input, which is why the README's
# search screenshot searches for a task that was never created.
Type "n"
Sleep 800ms
Type "Add rate-limit tests"
Sleep 700ms
Type "]"
Sleep 900ms
Enter
Sleep 2000ms

# --- Rest on the finished state ----------------------------------------------
# The tape used to end with Ctrl+C + Sleep 600ms to quit the app cleanly, but
# that flashed the exited shell prompt into the final ~0.6s of the recording,
# which read as "the demo is over" instead of "look at the finished tree".
# Ending inside the app leaves the last frame on the tree state; VHS tears
# down the pty when the tape ends, and the app never gets a chance to redraw.
Escape
Sleep 3000ms

# ---------------------------------------------------------------------------
# COMPRESSION -- mandatory. The VHS output above is a master.
#
#   ffmpeg -y -i demo/landing-demo.mp4 \
#     -vf "fps=12,scale=1152:-2:flags=lanczos" \
#     -c:v libx264 -crf 33 -preset slow -tune stillimage \
#     -pix_fmt yuv420p -an -movflags +faststart \
#     demo/landing-demo-web.mp4
#
# fps=12 is the big saving: a terminal is mostly duplicate frames, and 12fps
# is indistinguishable from 60 for this content. 1152 is the widest the video
# container ever renders on the page, so anything above it is thrown away by
# the browser. -tune stillimage suits large flat areas of solid colour.
# +faststart puts the moov atom first so playback can begin before the file
# has finished downloading.
#
# The poster frame is extracted from the first frame, which is why the tape
# holds still for 2.5s before touching anything:
#
#   ffmpeg -y -i demo/landing-demo.mp4 -vf "select=eq(n\,0)" -frames:v 1 \
#     -c:v libwebp -quality 82 demo/landing-demo-poster.webp
#
# A poster is not optional here: without one the <video> box is empty until
# enough of the file has buffered, which is a visible layout-stability and
# perceived-performance problem on a page held to a 100 Lighthouse bar.
# ---------------------------------------------------------------------------
