# VHS tape for website screenshots with the farol-day (light) theme.
# Records from the repo root after seeding with the light theme:
#
#   vhs demo/website-screenshots-day.tape

Set Shell "bash"
Set FontSize 15
Set Width 1200
Set Height 660
Set Theme "Dracula"
Set CursorBlink false
Set TypingSpeed 45ms

# Set up the throwaway XDG dirs with the light theme.
Hide
Type "export XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config && clear"
Enter
Sleep 800ms

# Switch config to farol-day before launching
Type "sed -i 's/farol-dark/farol-day/' /tmp/farol-demo/config/farol/config.yaml && clear"
Enter
Sleep 500ms
Show

# Launch the TUI.
Type "/tmp/farol-demo/farol"
Enter
Sleep 3s

# --- Lists panel: open it so both panels are visible ---
Type "L"
Sleep 600ms

# --- Main TUI: Lists + Tasks panels, pending/complete sections visible ------
Screenshot website/public/screenshot-main-day.png

# --- Help overlay -----------------------------------------------------------
Type "?"
Sleep 1.2s
Screenshot website/public/screenshot-help-day.png
Escape
Sleep 600ms

# Close the lists panel before any task-level action. Opening it at the top
# of the tape (for the main/help shots) moved focus onto Lists and it never
# moved back - Space below landed on the Lists panel instead of Tasks
# (toggled nothing) until this second L closes it and returns focus to
# Tasks. Confirmed while regenerating for v0.4.0: this was silently broken
# since the tape's creation.
Type "L"
Sleep 600ms

# --- Complete section -------------------------------------------------------
Space
Sleep 1500ms
Screenshot website/public/screenshot-complete-day.png
Space
Sleep 1200ms

# --- Theme picker -----------------------------------------------------------
Type "T"
Sleep 1500ms
Screenshot website/public/screenshot-theme-day.png
Escape
Sleep 600ms

# --- Search -----------------------------------------------------------------
Type "F"
Sleep 800ms
Type "timeout"
Sleep 1.5s
Screenshot website/public/screenshot-search-day.png
Escape
Sleep 500ms

# --- Details modal ----------------------------------------------------------
# Navigate to "Cut the p95 on /search" (a root-level sibling of "Ship auth
# v2", which has notes). Collapsing Ship auth v2 first makes the walk to its
# sibling deterministic regardless of its subtree's visible size (see the
# same fix in website-screenshots.tape for why a fixed Down-count is
# fragile here).
Left
Sleep 500ms
Down
Sleep 400ms
Enter
Sleep 1.5s
Screenshot website/public/screenshot-details-day.png
Escape
Sleep 500ms

# --- Archive page: the seeded archived list + its task preview -------------
Type "2"
Sleep 1200ms
Screenshot website/public/screenshot-archive-day.png
Escape
Sleep 500ms

Ctrl+C
Sleep 600ms
