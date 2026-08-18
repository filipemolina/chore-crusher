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

# --- Complete section -------------------------------------------------------
Space
Sleep 1500ms
Screenshot website/public/screenshot-complete-day.png
Space
Sleep 1200ms

# --- Theme picker -----------------------------------------------------------
Type "L"
Sleep 500ms
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
Down
Down
Down
Down
Sleep 400ms
Enter
Sleep 1.5s
Screenshot website/public/screenshot-details-day.png
Escape
Sleep 500ms

Ctrl+C
Sleep 600ms
