# VHS tape for static README screenshots.
# Run the same seeded state the demo GIF uses:
#   ./demo/seed.sh ./complete
#   vhs demo/screenshots.tape
#
# VHS Screenshot takes relative paths only, so every PNG lands under demo/.

Set Shell "bash"
Set FontSize 15
Set Width 1200
Set Height 660
Set Theme "Dracula"
Set CursorBlink false
Set TypingSpeed 45ms

Hide
Type "export XDG_DATA_HOME=/tmp/complete-demo/data XDG_CONFIG_HOME=/tmp/complete-demo/config && clear"
Enter
Sleep 800ms
Show

Type "./complete"
Enter
Sleep 3s

# --- Main TUI: tree focused, pending/complete sections visible ------------
Screenshot demo/screenshot-main.png

# --- Help overlay: shows the ctrl+right / ctrl+left zone-focus cycle -------
Type "?"
Sleep 1.2s
Screenshot demo/screenshot-help.png
Escape
Sleep 600ms

# --- Add input: ctrl+right focuses it, tab makes the level a child ---------
Ctrl+Right
Sleep 500ms
Type "Grow a trellis"
Sleep 400ms
Tab
Sleep 500ms
Screenshot demo/screenshot-add.png
Escape
Sleep 500ms

# --- Theme picker: live preview --------------------------------------------
Type "T"
Sleep 900ms
Down
Sleep 900ms
Screenshot demo/screenshot-theme.png
Escape
Sleep 600ms

# --- Global search picker --------------------------------------------------
Type "F"
Sleep 800ms
Type "ferns"
Sleep 1.5s
Screenshot demo/screenshot-search.png
Escape
Sleep 500ms

Type "q"
Sleep 600ms
