#!/usr/bin/env bash
# Light the live-agent spinner on a task for the agent-presence recording.
#
#   ./demo/agent-presence.sh [binary]
#
# Why this is a script and not tape lines: VHS `Type` sends a string to the
# shell as literal keystrokes, so any command with quoting or command
# substitution in it is one mistyped character away from a broken recording
# that still *looks* like it worked. A tape that types one bare path has
# nothing to get wrong.
#
# Presence is what `demo/agent-presence.tape` photographs -- the agent
# presence spinner + name appearing on a task row. It is a claim under the
# FAROL_AGENT identity, not an assignment: claiming does not move the task to
# in_progress (docs/DESIGN.md Sec3).
#
# Claims live for store.WorkTTL (120s) after the last ClaimWork, so this must
# run shortly before the shot, not minutes ahead of it. The tape calls it from
# its own hidden setup for exactly that reason.
set -euo pipefail

BIN=${1:-/tmp/farol-demo-presence/farol}

export XDG_DATA_HOME=/tmp/farol-demo-presence/data
export XDG_CONFIG_HOME=/tmp/farol-demo-presence/config

# Resolve by title rather than hardcoding an id: seed-presence.sh mints a
# fresh ULID every run, so a pinned id would silently claim nothing after a
# re-seed -- and a claim that lands on no row fails open, leaving a
# recording with no spinner that still exits 0.
claim_by_title() {
    local title="$1"
    local agent="$2"
    local task
    task=$("$BIN" search "$title" | awk 'NR>1 {print $1; exit}')

    if [ -z "${task:-}" ]; then
        echo "agent-presence: '$title' not found -- re-run demo/seed-presence.sh first" >&2
        exit 1
    fi

    FAROL_AGENT="$agent" "$BIN" claim "$task" --kind working
}

# Claim "Implement the new API endpoint" as demo-agent
claim_by_title "Implement the new API endpoint" "demo-agent"