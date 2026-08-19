# VHS tape for website/public/screenshot-agent.png: "A task row with a live
# claim from an agent, the tag rendered inline with the spinner" (the "For
# you, at the keyboard" section on index.mdx).
#
# One screenshot, its own tape file - see
# demo/website-screenshots-split.tape's header for why.
#
# Needs a live claim first (WorkTTL 120s, so run this immediately before
# `vhs`, not minutes ahead). Records from the repo root:
#
#   ./demo/seed.sh
#   task=$(XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config /tmp/farol-demo/farol search 'Rewrite the ingest worker' | awk 'NR>1{print $1; exit}')
#   XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config FAROL_AGENT=claude /tmp/farol-demo/farol claim $task --kind working
#   vhs demo/website-screenshots-agent.tape

Set Shell "bash"
Set FontSize 15
Set Width 1200
Set Height 660
Set Theme "Dracula"
Set CursorBlink false
Set TypingSpeed 45ms

Hide
Type "export XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config && clear"
Enter
Sleep 800ms
Show

Type "/tmp/farol-demo/farol"
Enter
Sleep 3s

# The Lists panel is closed by default at launch, so the claimed row already
# gets the full width - nothing to toggle.
Sleep 1s
Screenshot website/public/screenshot-agent.png
Sleep 500ms

Ctrl+C
Sleep 600ms
