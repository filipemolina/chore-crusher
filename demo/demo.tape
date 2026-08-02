# VHS tape for the chore-crusher README demo.
# Install VHS first: https://github.com/charmbracelet/vhs (needs ttyd + ffmpeg too).
#
# The binary is the repo-root `./crush` (built with `make build`; the
# `crush` on PATH is bash's completion builtin, so the tape uses the
# relative path on purpose). Seed the store first into the same throwaway
# XDG dirs the recording uses, so the frames are deterministic:
#
#   ./demo/seed.sh           # builds state into /tmp/chore-crusher-demo/{data,config}
#   vhs demo/demo.tape       # record from the repo root
#
# VHS writes every frame at 25fps and the raw GIF comes out large; shrink it
# before a README pins it, the way stack-stitcher's demo does:
#
#   FILT="mpdecimate,fps=10,scale=900:-1:flags=lanczos"
#   ffmpeg -i demo/demo.gif -vf "$FILT,palettegen=max_colors=48" /tmp/pal.png
#   ffmpeg -i demo/demo.gif -i /tmp/pal.png \
#     -lavfi "$FILT[x];[x][1:v]paletteuse=dither=none" \
#     -fps_mode vfr /tmp/demo.gif && mv /tmp/demo.gif demo/demo.gif

Output demo/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1200
Set Height 660
Set Theme "Dracula"
# A blinking cursor changes pixels twice a second; every tick is a frame the
# GIF must carry. Holding it still saves megabytes on a recording this long.
Set CursorBlink false
Set TypingSpeed 45ms

# Same throwaway dirs the seed used: recording neither depends on nor
# overwrites the recorder's real store or config.
Hide
Type "export XDG_DATA_HOME=/tmp/chore-crusher-demo/data XDG_CONFIG_HOME=/tmp/chore-crusher-demo/config && clear"
Enter
Sleep 800ms
Show

Type "./crush"
Enter
Sleep 3s

# --- Toggle complete, watch the cascade -----------------------------------
# The cursor lands on "Plan the garden". space completes it and every
# descendant at once; the second space walks the whole subtree back to
# pending, so both directions of the cascade are on screen.
Sleep 1500ms
Space
Sleep 1800ms
Space
Sleep 1800ms

# --- Add a nested task; ctrl+right reaches the input, tab/shift-tab set level ---
# The zone-focus cycle is ctrl+right/ctrl+left (docs/DESIGN.md §5), so
# Ctrl+Right moves focus tree to inline add input — tab is left for the
# input's own level selector. Type a title; Tab makes it a child of the
# selected task ('+'), Shift+Tab steps it back to a sibling ('-'), Tab once
# more for a child again, then Enter to land it as a child of "Plan the
# garden" (the task left selected by the cascade above).
Ctrl+Right
Sleep 700ms
Type "Grow a trellis"
Sleep 500ms
Tab
Sleep 1200ms
Shift+Tab
Sleep 1200ms
Tab
Sleep 800ms
Enter
Sleep 2000ms

# --- Theme picker: live preview ------------------------------------------
# T opens the picker; moving the cursor repaints the whole app live behind
# it. Two moves, then Esc closes without persisting. T is a global key, so
# it opens while the add input (left focused from the last add) still owns
# the text cursor.
Type "T"
Sleep 1000ms
Down
Sleep 1500ms
Down
Sleep 1500ms
Escape
Sleep 1200ms

# --- Global search picker -------------------------------------------------
# F searches every list at once; typing ranks title matches first. Enter on
# a result jumps the list and selects the task.
Type "F"
Sleep 1000ms
Type "trellis"
Sleep 2500ms
Down
Sleep 800ms
Enter
Sleep 2000ms

Type "q"
Sleep 1000ms