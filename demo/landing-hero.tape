# VHS tape for the FAROL LANDING PAGE hero still.
#
# Split out of demo/landing.tape on 2026-08-16. That tape records at 1440x540
# (8:3) and still does, because the three FEATURE-CARD stills sit in a grid
# beside icon-only cards that are also aspect-[8/3]; change their ratio and the
# grid rows stop lining up. The HERO is a different surface: it is the largest
# image on the page, shown alone at max-w-4xl, and at 8:3 it read as a
# letterbox strip rather than as a terminal window. This tape gives it 2:1.
#
# Record from the repo root:
#
#   FAROL_DEMO_VERSION=v0.3.0 ./demo/seed.sh   # MANDATORY - see landing.tape
#   vhs demo/landing-hero.tape                 # -> demo/landing-hero.png
#
# Pass FAROL_DEMO_VERSION explicitly. seed.sh otherwise bakes in `git describe`,
# which on a working tree renders "v0.3.0-18-g4c2d40f-dirty" in the header bar
# and photographs a version string no user will ever see.
#
# WHY 1440x720, AND WHY THE FONT DID NOT CHANGE
#
#   The obvious way to make a terminal shot taller is to scale the font with
#   the frame. That does not work here, and the reason is a hard constraint in
#   the app: constants.AUTO_SHOW_LISTS_MIN_WIDTH = 120. Below 120 COLUMNS the
#   app does not auto-open the Lists panel at launch, and the panel is half of
#   what this image exists to show ("one store, two views").
#
#   A cell at FontSize F is about 0.645*F wide, so 1440px holds 120 columns
#   only while F <= 18. FontSize 22 was tried first: it gives a genuine 16:9
#   frame and a nicely spacious tree, but only ~106 columns, so the panel never
#   appeared AND the tape's own Escape - a harmless no-op when the panel is up -
#   became "quit", and the recording captured a shell prompt.
#
#   So the font stays at 18 (124 columns, matching landing.tape) and the only
#   remaining lever is ROWS. A cell is ~23.5px tall, so:
#
#       height 540 -> 23 rows -> 2.67:1   the seed fills the frame exactly
#       height 660 -> 28 rows -> 2.18:1
#       height 720 -> 30 rows -> 2.00:1   <- chosen
#       height 810 -> 34 rows -> 1.78:1   11 empty rows; the "60% fill" defect
#                                         landing.tape's header warns about
#
#   720 buys seven rows of slack over the 23 the seed fills. That slack is not
#   dead space: the keybinding bar is anchored to the bottom by the app, so it
#   lands inside the task pane and reads as a list with room left in it, which
#   is what a real terminal looks like. 810 is where it stops reading that way
#   and starts reading as an unfinished screen.
#
#   Re-tune the height if the seed grows; do NOT raise the font past 18 without
#   first checking that the Lists panel is still in the frame.
#
# The container on the page is aspect-[2/1] on the IMAGE TRACK, not on
# .terminal-window - the header is a fixed ~37px, so ratio-ing the whole window
# leaves the image track short by that much (see FeatureCard.astro's note on
# the same bug). Source ratio == container ratio => object-cover crops nothing.
# Changing either number here means changing Hero.astro to match.
#
# Everything else - the navy chrome, Padding 0, the seeded store, the live
# agent claim - is identical to demo/landing.tape. Read that file's header for
# the reasoning behind each; it is not repeated here.

Set Shell "bash"
Set FontSize 18
Set Width 1440
Set Height 720
Set Padding 0
Set CursorBlink false
Set TypingSpeed 45ms

Set Theme { "name": "farol-landing", "background": "#0E1B30", "foreground": "#F5EDE4", "cursor": "#F0B263", "selection": "#22385C", "black": "#0E1B30", "red": "#D9534F", "green": "#67C58A", "yellow": "#E8C547", "blue": "#22385C", "magenta": "#EB4268", "cyan": "#8A94A8", "white": "#F5EDE4", "brightBlack": "#3A4A63", "brightRed": "#EB4268", "brightGreen": "#67C58A", "brightYellow": "#F0B263", "brightBlue": "#3A4A63", "brightMagenta": "#EB4268", "brightCyan": "#C4BEB6", "brightWhite": "#F5EDE4" }

# --- Setup, entirely hidden -------------------------------------------------
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

# The app launches with the Lists panel ALREADY OPEN and focus on the task
# tree; nothing here opens it. Escape is a no-op against that state and is kept
# only so this tape's entry sequence matches landing.tape's exactly.
Escape
Sleep 800ms
Show

# --- HERO: the resting state ------------------------------------------------
# Tree focused, Lists panel open: nesting three levels deep, derived progress
# on the parents, the Complete section, the live "claude" claim on "Rewrite the ingest worker", and the keybinding bar.
#
# The two Sleeps are load-bearing, not padding. Unlike landing.tape - where
# every Screenshot is followed by more keystrokes, so the app is still up when
# the frame is written - this tape ends immediately after its only shot. With
# Screenshot butted straight against Ctrl+C, the written PNG was the SHELL
# PROMPT after the app had already exited. Three separate recordings were lost
# to this before the cause was found. Give the frame room on both sides.
Sleep 2s
Screenshot demo/landing-hero.png
Sleep 2s

Ctrl+C
Sleep 600ms
