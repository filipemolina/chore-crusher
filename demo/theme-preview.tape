# VHS tape for the theme picker live preview demo.
# Records: open theme picker (T) → navigate themes with ↑/↓ showing live preview
# → select a theme (enter) → modal closes.
# Shows 4 themes including farol-day (light) and farol-dark (dark).
#
# Uses a dedicated temp dir /tmp/farol-demo-themes so it never collides with
# other concurrent recordings.
#
# Record from the repo root:
#   vhs demo/theme-preview.tape
#   ./demo/compress-theme-preview.sh   (or adapt the recipe)

Output demo/theme-preview.gif

Set Shell "bash"
Set FontSize 15
Set Width 1200
Set Height 660
Set Theme "Dracula"
Set CursorBlink false
Set TypingSpeed 45ms

# Build the binary and seed the throwaway store in a DEDICATED temp dir.
# Hide every frame of setup — the GIF starts at the TUI on the seeded list.
Hide
Type "mkdir -p /tmp/farol-demo-themes"
Enter
Sleep 200ms
Type "go build -o /tmp/farol-demo-themes/farol ."
Enter
Sleep 5s
Type "./demo/seed.sh /tmp/farol-demo-themes/farol"
Enter
Sleep 3s
Type "export XDG_DATA_HOME=/tmp/farol-demo-themes/data XDG_CONFIG_HOME=/tmp/farol-demo-themes/config && clear"
Enter
Sleep 800ms

# Launch the TUI. Show only after it has had time to render.
Type "/tmp/farol-demo-themes/farol"
Enter
Sleep 3s
Show

# --- Theme picker: live preview ----------------------------------------------
# T opens the picker. The picker starts on the active theme (farol-dark after
# seed.sh). Navigate with Up/Down to show live preview updating in real time.
# Press Enter to select a theme and close the modal.

Type@1ms "T"
Sleep 1500ms

# Start on farol-dark (default). Hold to show it.
Sleep 1000ms

# Down to farol-ember (dark)
Down
Sleep 1200ms

# Down to farol-slate (dark)
Down
Sleep 1200ms

# Down to farol-day (light) — the light theme
Down
Sleep 1500ms

# Up back to farol-slate
Up
Sleep 1000ms

# Up to farol-ember
Up
Sleep 1000ms

# Up to farol-dark (back to default)
Up
Sleep 1000ms

# Select farol-dark with Enter — modal closes
Enter
Sleep 1500ms

# Show the TUI with the selected theme applied
Sleep 1000ms

Ctrl+C
Sleep 600ms