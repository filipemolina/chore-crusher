# VHS tape for the farol README demo.
# Install VHS first: https://github.com/charmbracelet/vhs (needs ttyd + ffmpeg too).
#
# seed.sh compiles ./farol into /tmp/farol-demo/farol and seeds the
# throwaway store there, so the recording never touches the recorder's real
# data. Re-record from the repo root with:
#
#   make demo
#
# That runs seed.sh, records, and compresses via demo/compress.sh (mpdecimate
# for duplicate frames, 10fps, 900px wide, 48-colour palette — same recipe
# stack-stitcher uses).
#
# The recording starts the instant the TUI window appears — all shell setup
# (build, seed, env vars) happens with Show suppressed, so the GIF is 100%
# TUI. The demo walks the 1/2/3 view-mode filter on the seeded list, opens
# the lists panel, creates a list, adds a root task and a nested subtask (one
# level deep via ]), previews the theme picker, then opens the Archived
# Lists page (A) to show the seeded archived list and its task preview.

Output demo/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1200
Set Height 660
Set Theme "Dracula"
# A blinking cursor changes pixels twice a second; every tick is a frame the
# GIF has to carry. Holding it still saves megabytes on a recording this long.
Set CursorBlink false
Set TypingSpeed 45ms

# Build the binary and seed the throwaway store. seed.sh compiles ./farol into
# /tmp/farol-demo/farol itself, so this is self-contained from a clean
# checkout. Hide every frame of setup — the GIF starts at the TUI on the
# seeded "Home" list.
Hide
Type "cd /home/filipe/Documents/projects/farol && ./demo/seed.sh && clear"
Enter
Sleep 3s
# Export the same throwaway XDG dirs inside the visible shell so the TUI reads
# them (also hidden — we only want to show the app).
Type "export XDG_DATA_HOME=/tmp/farol-demo/data XDG_CONFIG_HOME=/tmp/farol-demo/config && clear"
Enter
Sleep 800ms

# Launch the TUI. Show only after it has had time to render.
Type "/tmp/farol-demo/farol"
Enter
Sleep 3s
Show

# --- View-mode filter: 1 = Pending only, 2 = Complete only, 3 = All --------
# The seeded "api" list already has both pending and complete tasks (Drop
# the legacy /v1 routes cascaded to Complete in seed.sh), so filtering here
# is visibly meaningful before the walkthrough moves on to lists/creation.
Type@1ms "1"
Sleep 1200ms
Type@1ms "2"
Sleep 1200ms
Type@1ms "3"
Sleep 1200ms

# --- Open the lists side panel ----------------------------------------------
# L (shift+l) toggles the lists panel. Opening it moves focus onto it, so the
# next keystrokes act on the lists, not the task tree.
Type@1ms "L"
Sleep 800ms

# --- Navigate up and down once in the lists panel ---------------------------
Up
Sleep 600ms
Down
Sleep 800ms

# --- Create a new list -------------------------------------------------------
# n opens the new-list name modal; type the name, Enter submits. The app then
# selects the new list AND auto-closes the lists panel (ListCreatedMsg calls
# closeListsPanel), so no separate "L to close" is needed — pressing L again
# would re-open it. Focus lands on the now-empty task tree.
Type@1ms "n"
Sleep 700ms
Type@1ms "Weekend chores"
Sleep 600ms
Enter
Sleep 1500ms

# --- Add a root task ---------------------------------------------------------
# The new list is empty, so its inline create input auto-opened and is live —
# no n is needed for the first task (n would just type the letter into the
# title). Type the name, Enter saves it. Rapid entry keeps a fresh, focused
# input anchored on the just-created task, so the next task can be indented
# straight away.
Type@1ms "Grocery run"
Sleep 500ms
Enter
Sleep 1200ms

# --- Add a nested subtask (one level deep) -----------------------------------
# The input is still live and anchored on "Grocery run". ] (Tree.Indent) sets
# the create level to +1, so the new task becomes a child of "Grocery run".
Type@1ms "]"
Sleep 800ms
Type@1ms "Pick up milk"
Sleep 500ms
Enter
Sleep 1500ms

# --- Close the create input --------------------------------------------------
# Esc discards the still-live rapid-entry input and returns the keyboard to
# the tree so the theme picker can open.
Escape
Sleep 800ms

# --- Theme picker: live preview ----------------------------------------------
# T opens the picker. The picker starts on the active theme (farol-dark after
# seed.sh), so opening it and holding here is the live preview of the app's
# default theme — the README's screenshots are also captured in this view.
# Walking the cursor would re-paint the app in unrelated themes, breaking the
# demo's claim that the rest of the recording is farol-dark. Esc discards the
# preview without persisting.
Type@1ms "T"
Sleep 1500ms
Escape
Sleep 2000ms

# --- Archive page: browse the archived list + its task preview, then back --
# A (capital) opens the Archived Lists page (src/keys/Keys.go). seed.sh
# archives "Q2 roadmap" ahead of time, so this shows a real list + preview,
# not an empty state. Esc returns to Home.
Type@1ms "A"
Sleep 1500ms
Escape
Sleep 1200ms
