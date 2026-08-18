# VHS tape for website screenshots with the farol-dark theme.
# Records from the repo root after seeding:
#
#   ./demo/seed.sh
#   vhs demo/website-screenshots.tape
#
# Every screenshot lands in website/public/.

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

# --- Lists panel: open it so both panels are visible ---
Type "L"
Sleep 600ms

# --- Main TUI: Lists + Tasks panels, pending/complete sections visible ------
Screenshot website/public/screenshot-main-dark.png

# --- Help overlay: shows the full keybinding catalog -----------------------
Type "?"
Sleep 1.2s
Screenshot website/public/screenshot-help-dark.png
Escape
Sleep 600ms

# --- Complete section: toggle a task to show the cascade --------------------
Space
Sleep 1500ms
Screenshot website/public/screenshot-complete-dark.png
Space
Sleep 1200ms

# --- Add input: n starts creating, ] makes the level a child ---------------
Type "n"
Sleep 500ms
Type "Set up monitoring"
Sleep 400ms
Type "]"
Sleep 500ms
Screenshot website/public/screenshot-add-dark.png
Escape
Sleep 500ms
Escape
Sleep 500ms

# Close the lists panel so the task tree fills the width for theme/search shots
Type "L"
Sleep 500ms

# --- Theme picker: live preview --------------------------------------------
Type "T"
Sleep 1500ms
Screenshot website/public/screenshot-theme-dark.png
Escape
Sleep 600ms

# --- Global search picker --------------------------------------------------
Type "F"
Sleep 800ms
Type "timeout"
Sleep 1.5s
Screenshot website/public/screenshot-search-dark.png
Escape
Sleep 500ms

# --- Details modal: open a task's details -----------------------------------
# Navigate to "Cut the p95 on /search" which has notes
Down
Down
Down
Down
Sleep 400ms
Enter
Sleep 1.5s
Screenshot website/public/screenshot-details-dark.png
Escape
Sleep 500ms

Ctrl+C
Sleep 600ms
