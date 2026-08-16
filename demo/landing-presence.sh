#!/usr/bin/env bash
# Light the live-agent spinner on "Rewrite the ingest worker" and "Cut the p95 on /search"
# for the landing-page recording.
#
#   ./demo/landing-presence.sh [binary]
#
# Why this is a script and not two lines inside the tape: VHS `Type` sends a
# string to the shell as literal keystrokes, so any command with quoting or
# command substitution in it is one mistyped character away from a broken
# recording that still *looks* like it worked. A tape that types one bare
# path has nothing to get wrong.
#
# Presence is what `demo/landing.tape` photographs for the landing page's
# "Live agent presence" card -- the page's most distinctive claim, and the one
# screen the shipped screenshots never showed. It is a claim under the
# FAROL_AGENT identity, not an assignment: claiming does not move the task to
# in_progress (docs/DESIGN.md Sec3).
#
# Claims live for store.WorkTTL (120s) after the last ClaimWork, so this must
# run shortly before the shot, not minutes ahead of it. The tape calls it from
# its own hidden setup for exactly that reason.
set -euo pipefail

BIN=${1:-/tmp/farol-demo/farol}

export XDG_DATA_HOME=/tmp/farol-demo/data
export XDG_CONFIG_HOME=/tmp/farol-demo/config

# Resolve by title rather than hardcoding an id: seed.sh mints a fresh ULID
# every run, so a pinned id would silently claim nothing after a re-seed --
# and a claim that lands on no row fails open, leaving a screenshot with no
# spinner that still exits 0.
claim_by_title() {
    local title="$1"
    local agent="$2"
    local task
    task=$("$BIN" search "$title" | awk 'NR>1 {print $1; exit}')

    if [ -z "${task:-}" ]; then
        echo "landing-presence: '$title' not found -- re-run demo/seed.sh first" >&2
        exit 1
    fi

    FAROL_AGENT="$agent" "$BIN" claim "$task" --kind working
}

# Claim "Rewrite the ingest worker" as claude
claim_by_title "Rewrite the ingest worker" "claude"

# Claim "Cut the p95 on /search" as codex
claim_by_title "Cut the p95 on /search" "codex"