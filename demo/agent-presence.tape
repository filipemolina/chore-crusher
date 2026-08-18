# VHS tape for the agent presence demo GIF.
# Shows: agent claims a task -> presence spinner + agent name appear on row ->
# agent completes task -> spinner disappears.
#
# Record from the repo root:
#
#   ./demo/seed-presence.sh          # MANDATORY -- seeds dedicated store
#   vhs demo/agent-presence.tape     # -> demo/agent-presence.gif
#   ./demo/compress.sh agent-presence.gif assets/demo/agent-presence.gif
#
# The recording starts the instant the TUI window appears -- all shell setup
# (build, seed, env vars) happens with Show suppressed, so the GIF is 100% TUI.

Output demo/agent-presence.gif

Set Shell "bash"
Set FontSize 15
Set Width 1200
Set Height 660
Set Theme "Dracula"
Set CursorBlink false
Set TypingSpeed 45ms

# Build the binary and seed the throwaway store in a dedicated temp dir.
# Hide every frame of setup -- the GIF starts at the TUI on the seeded list.
Hide
Type "cd /home/filipe/Documents/projects/farol/.slim/worktrees/demo-presence && ./demo/seed-presence.sh && clear"
Enter
Sleep 3s
# Export the same throwaway XDG dirs inside the visible shell so the TUI reads
# them (also hidden -- we only want to show the app).
Type "export XDG_DATA_HOME=/tmp/farol-demo-presence/data XDG_CONFIG_HOME=/tmp/farol-demo-presence/config && clear"
Enter
Sleep 800ms

# Launch the TUI. Show only after it has had time to render.
Type "/tmp/farol-demo-presence/farol"
Enter
Sleep 3s
Show

# --- Initial state: task list at rest, no spinners ---------------------------
# Hold so the first frame shows the clean list.
Sleep 1500ms

# --- Light the agent presence spinner ----------------------------------------
# The helper script claims the task as "demo-agent" so the spinner + name
# appear on the row. This runs in the hidden shell so the TUI picks it up
# via its 1s poll loop.
Hide
Type "./demo/agent-presence.sh"
Enter
Sleep 2000ms
Show

# --- Show the spinner + agent name on the task row ---------------------------
# Hold to capture the presence indicator.
Sleep 2500ms

# --- Complete the task -------------------------------------------------------
# Space completes the task (simple progress mode). The spinner should
# disappear when the task is marked complete.
Space
Sleep 2000ms

# --- Final state: task completed, no spinner ---------------------------------
Sleep 1000ms

Ctrl+C
Sleep 600ms