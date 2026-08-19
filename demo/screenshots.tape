# VHS tape for static README screenshots.
# Uses the same seeded throwaway store the demo GIF uses. Record from the
# repo root after seeding:
#
#   ./demo/seed.sh
#   vhs demo/screenshots.tape
#
# VHS Screenshot takes relative paths only, so every PNG lands under demo/.
# The recording starts only after the TUI window appears — all shell setup
# (env vars) happens with Show suppressed, so the shots are 100% app.

Set Shell "bash"
Set FontSize 15
Set Width 1200
Set Height 660
Set Theme "Dracula"
Set CursorBlink false
Set TypingSpeed 45ms

# Set up the throwaway XDG dirs so the TUI reads the seeded store.
# Hide the shell setup — we only want to capture the app.
Hide
Type "export XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config && clear"
Enter
Sleep 800ms
Show

# Launch the TUI.
Type "/tmp/farol-demo/farol"
Enter
Sleep 3s

# --- Lists panel: open it so both panels are visible ------------------------
# L (shift+l) toggles the lists panel; the cursor stays on the task tree.
Type "L"
Sleep 600ms

# --- Main TUI: Lists + Tasks panels, pending/complete sections visible ------
Screenshot demo/screenshot-main.png

# --- Help overlay: shows the full keybinding catalog -----------------------
Type "?"
Sleep 1.2s
Screenshot demo/screenshot-help.png
Escape
Sleep 600ms

# --- Complete section: watch the cascade move items to the Complete section --
# The cursor is on "Plan the garden" (first root). space completes it and
# every descendant at once; they move to the Complete section below.
Type "L"
Sleep 500ms
Sleep 300ms
Space
Sleep 1500ms
Screenshot demo/screenshot-complete.png
Escape
Sleep 500ms
# Toggle it back so the remaining shots start from a clean tree.
Space
Sleep 1200ms

# --- Add input: n starts creating, ] makes the level a child ---------------
Type "n"
Sleep 500ms
Type "Grow a trellis"
Sleep 400ms
Type "]"
Sleep 500ms
Screenshot demo/screenshot-add.png
Escape
Sleep 500ms
Escape
Sleep 500ms

# Close the lists panel so the task tree fills the width for the remaining
# shots (theme picker, search).
Type "L"
Sleep 500ms

# --- Theme picker: live preview --------------------------------------------
# T opens the picker. The picker starts on the active theme (farol-dark after
# seed.sh), so the screenshot here is the live preview of farol-dark itself —
# a closing shot of the demo's default theme. Pressing Down moves the
# preview to a different theme, which would contradict the rest of the README.
Type "T"
Sleep 1500ms
Screenshot demo/screenshot-theme.png
Escape
Sleep 600ms

# --- Global search picker --------------------------------------------------
Type "F"
Sleep 800ms
Type "trellis"
Sleep 1.5s
Screenshot demo/screenshot-search.png
Escape
Sleep 500ms

# --- Archive page: the seeded archived list + its task preview -------------
# 2 opens the Archived Lists page, the top nav's second tab. seed.sh archives
# "Q2 roadmap" ahead of time, so this captures a real list + preview, not an
# empty state.
Type "2"
Sleep 1200ms
Screenshot demo/screenshot-archive.png
Escape
Sleep 500ms

Ctrl+C
Sleep 600ms
