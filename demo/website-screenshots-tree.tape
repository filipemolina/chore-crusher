# VHS tape for website/public/screenshot-tree.png: nested tasks with
# expand/collapse markers, full width (tui.md's "task tree" section).
#
# One screenshot, its own tape file - see
# demo/website-screenshots-split.tape's header for why.
#
# Records from the repo root after seeding:
#
#   ./demo/seed.sh
#   vhs demo/website-screenshots-tree.tape

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

# The Lists panel is closed by default at launch, so the tree already fills
# the width - nothing to toggle.
Sleep 1s
Screenshot website/public/screenshot-tree.png
Sleep 500ms

Ctrl+C
Sleep 600ms
