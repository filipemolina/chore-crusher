Set Shell "bash"
Set FontSize 16
Set Width 1600
Set Height 800
Set Padding 0
Set CursorBlink false
Set TypingSpeed 1ms
Output split.mp4

Set Theme { "name": "farol-landing", "background": "#0E1B30", "foreground": "#F5EDE4", "cursor": "#F0B263", "selection": "#22385C", "black": "#0E1B30", "red": "#D9534F", "green": "#67C58A", "yellow": "#E8C547", "blue": "#22385C", "magenta": "#EB4268", "cyan": "#8A94A8", "white": "#F5EDE4", "brightBlack": "#3A4A63", "brightRed": "#EB4268", "brightGreen": "#67C58A", "brightYellow": "#F0B263", "brightBlue": "#3A4A63", "brightMagenta": "#EB4268", "brightCyan": "#C4BEB6", "brightWhite": "#F5EDE4" }

Hide
Type "unset HERDR_ENV HERDR_PANE_ID HERDR_TAB_ID HERDR_SOCKET_PATH HERDR_WORKSPACE_ID"
Enter
Type "export XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config"
Enter
Type "clear"
Enter
Sleep 500ms
Type "herdr --session farolsplit2"
Enter
Sleep 12s

# left pane keeps 45 percent, farol gets the wider right pane
Type "herdr pane split --current --direction right --ratio 0.45 --focus"
Enter
Sleep 4s

# focus back to the left pane -- the flag form, not a positional
Type "herdr pane focus --direction left"
Enter
Sleep 3s

# agentshot clears its own invocation line, prints the transcript, then
# hands focus back to the right pane
Type "/home/filipe/Documents/projects/landingPages/scripts/farol-split-probe/agentshot"
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
Screenshot proof-split.png
Sleep 2s
