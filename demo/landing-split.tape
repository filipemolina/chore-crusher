# VHS tape for the FAROL LANDING PAGE split-screen still.
#
# This is a genuine two-process recording: an agent session on the left,
# the live Farol TUI on the right, both real processes in one frame.
#
# Record from the repo root:
#
#   FAROL_DEMO_VERSION=v0.3.0 ./demo/seed.sh
#   vhs demo/landing-split.tape           # -> demo/landing-split.png
#
# The agentshot script clears the screen first, prints real command output,
# then hands focus back to the right pane. HERDR_CONFIG_PATH below collapses
# herdr's sidebar to zero width so the rig stays out of frame.
#
# Set FontSize 16 (not 18) because the split pane is narrower; the transcript
# must be legible in the left half of the frame.

Set Shell "bash"
Set FontSize 16
Set Width 1440
Set Height 720
Set Padding 0
Set CursorBlink false
Set TypingSpeed 1ms
Output landing-split.mp4

Set Theme { "name": "farol-landing", "background": "#0E1B30", "foreground": "#F5EDE4", "cursor": "#F0B263", "selection": "#22385C", "black": "#0E1B30", "red": "#D9534F", "green": "#67C58A", "yellow": "#E8C547", "blue": "#22385C", "magenta": "#EB4268", "cyan": "#8A94A8", "white": "#F5EDE4", "brightBlack": "#3A4A63", "brightRed": "#EB4268", "brightGreen": "#67C58A", "brightYellow": "#F0B263", "brightBlue": "#3A4A63", "brightMagenta": "#EB4268", "brightCyan": "#C4BEB6", "brightWhite": "#F5EDE4" }

# --- Setup, entirely hidden -------------------------------------------------
Hide
Type "unset HERDR_ENV HERDR_PANE_ID HERDR_TAB_ID HERDR_SOCKET_PATH HERDR_WORKSPACE_ID"
Enter
Sleep 200ms
Type "export HERDR_CONFIG_PATH=$PWD/demo/herdr-split.toml"
Enter
Sleep 200ms
Type "export XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config"
Enter
Sleep 200ms
Type "herdr --session farol-split-v5"
Enter
Sleep 12s

# With the sidebar at zero width the full 1440 is available, so a plain 0.5
# gives a true 50/50. The old 0.33 was compensating for a 280px sidebar that
# should never have been in the frame at all.
Type "herdr pane split --current --direction right --ratio 0.5 --focus"
Enter
Sleep 4s

# focus back to the left pane -- the flag form, not a positional
Type "herdr pane focus --direction left"
Enter
Sleep 3s

# agentshot clears its own invocation line, prints the transcript, then
# hands focus back to the right pane
Type "./demo/agentshot"
Enter
Sleep 5s

Type "clear"
Enter
Sleep 500ms
Type "/tmp/farol-demo/farol"
Enter
Sleep 6s
Show
Sleep 1s
Screenshot demo/landing-split.png
Sleep 2s

Ctrl+C
Sleep 600ms