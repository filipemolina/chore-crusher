# VHS tape for website screenshots with the farol-dark theme.
# Records from the repo root after seeding:
#
#   ./demo/seed.sh
#   vhs demo/website-screenshots.tape
#
# Every screenshot lands in website/public/.
#
# screenshot-split.png, screenshot-tree.png, screenshot-hero.png, and
# screenshot-agent.png are NOT recorded here even though they're the same
# farol-dark theme - see demo/website-screenshots-live.tape. They were split
# into their own short tape because appending their scenes to this one made
# this tape long enough to trigger a VHS quirk where one of the EARLIEST
# Screenshot commands gets silently dropped (no error, the file just isn't
# written) - confirmed repeatedly while regenerating for v0.4.0, and it went
# away once this tape went back to its original, shorter length. Keeping
# tapes short seems to be the reliable fix, not any particular ordering.

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

# Close the lists panel before any task-level action. Opening it at the top
# of the tape (for the main/help shots) moved focus onto Lists and it never
# moved back — Space/n below landed on the Lists panel instead of Tasks
# (toggled nothing, opened "New list") until this second L closes it and
# returns focus to Tasks. Confirmed while regenerating for v0.4.0: this was
# silently broken since the tape's creation (demo/screenshots.tape, the
# README variant, already does this same second-L close before its own
# Space step).
Type "L"
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
# Navigate to "Cut the p95 on /search" (a root-level sibling of "Ship auth
# v2", which has notes). A fixed Down-count from here is fragile - toggling
# Ship auth v2 complete-then-pending above does not necessarily restore
# every descendant's own status (reopen does not cascade), so its visible
# subtree size can vary. Collapsing it first makes the walk to its sibling
# deterministic regardless of that.
Left
Sleep 500ms
Down
Sleep 400ms
Enter
Sleep 1.5s
Screenshot website/public/screenshot-details-dark.png
Escape
Sleep 500ms

# --- Archive page: the seeded archived list + its task preview -------------
# A (capital) opens the Archived Lists page. seed.sh archives "Q2 roadmap"
# ahead of time, so this captures a real list + preview, not an empty state.
Type "A"
Sleep 1200ms
Screenshot website/public/screenshot-archive-dark.png
Escape
Sleep 500ms

Ctrl+C
Sleep 600ms
