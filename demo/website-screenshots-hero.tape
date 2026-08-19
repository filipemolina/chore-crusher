# VHS tape for website/public/screenshot-hero.png: index.mdx's lead image
# ("a nested task tree on the right, the Lists panel on the left, one row
# lit with a live claim from an agent tagged claude").
#
# One screenshot, its own tape file - see
# demo/website-screenshots-split.tape's header for why.
#
# Needs a live claim first (WorkTTL 120s, so run this immediately before
# `vhs`, not minutes ahead) - mirrors demo/landing-presence.sh's approach
# for the landing page's equivalent shot. Records from the repo root:
#
#   ./demo/seed.sh
#   task=$(XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config /tmp/farol-demo/farol search 'Rewrite the ingest worker' | awk 'NR>1{print $1; exit}')
#   XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config FAROL_AGENT=claude /tmp/farol-demo/farol claim $task --kind working
#   vhs demo/website-screenshots-hero.tape

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
Screenshot website/public/screenshot-hero.png
Sleep 500ms

Ctrl+C
Sleep 600ms
