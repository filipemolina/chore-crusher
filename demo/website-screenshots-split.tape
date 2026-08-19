# VHS tape for website/public/screenshot-split.png: the Lists panel open
# beside the task tree (tui.md's "Lists panel open beside the task tree"
# section) - a plain split framing, no live-claim requirement.
#
# One screenshot, its own tape file, on purpose - see
# demo/website-screenshots.tape's header for why: appending scenes like this
# one to a longer tape made an early Screenshot command unreliable (VHS
# silently drops it about half the time, no error). A short, single-purpose
# tape avoided it every time while regenerating for v0.4.0.
#
# Records from the repo root after seeding:
#
#   ./demo/seed.sh
#   vhs demo/website-screenshots-split.tape

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

Type "L"
Sleep 1s
Screenshot website/public/screenshot-split.png
Sleep 500ms

Ctrl+C
Sleep 600ms
