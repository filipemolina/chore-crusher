#!/usr/bin/env bash
# Seed the demo store for the agent-presence recording.
# Uses a dedicated temp dir (/tmp/farol-demo-presence) so it never collides
# with other concurrent recordings.
set -euo pipefail

DATA=/tmp/farol-demo-presence/data
CONFIG=/tmp/farol-demo-presence/config
BIN=${1:-/tmp/farol-demo-presence/farol}

VERSION=${FAROL_DEMO_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || true)}
LDFLAGS="-X github.com/filipemolina/farol/src/constants.version=$VERSION"

mkdir -p "$(dirname "$BIN")"
go build -ldflags "$LDFLAGS" -o "$BIN" .

rm -rf "$DATA" "$CONFIG"
mkdir -p "$DATA" "$CONFIG/farol"
printf 'theme: farol-dark\n' > "$CONFIG/farol/config.yaml"

export XDG_DATA_HOME="$DATA"
export XDG_CONFIG_HOME="$CONFIG"

run() { "$BIN" "$@"; }

add() { "$BIN" add --force "$@"; }

# Create a single list with a few tasks for the presence demo
LIST=$(run lists add "demo")

# Add a task that will be claimed by the agent
TASK=$(add "$LIST" "Implement the new API endpoint")

# Add a second task for context
add "$LIST" "Write tests for the endpoint"

# Add a third task
add "$LIST" "Update documentation"

# Release any auto-claims from seeding so the store starts quiet
run release --all >/dev/null

echo "seeded list $LIST with task $TASK"