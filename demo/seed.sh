#!/usr/bin/env bash
# Seed the demo store with deterministic data that the VHS tapes record in.
# The tapes (demo/*.tape) set the same XDG dirs, so a stamped seed is what
# they show — the recording neither depends on nor clobbers the real store.
#
#   ./demo/seed.sh [binary]     launch path defaults to /tmp/farol-demo/farol
#
# Everything lives under /tmp/farol-demo so a run is reproducible from a
# clean checkout and touches nothing outside it.
set -euo pipefail

DATA=/tmp/farol-demo/data
CONFIG=/tmp/farol-demo/config
BIN=${1:-/tmp/farol-demo/farol}

# Build the binary into the demo dir when it is missing, so the script is
# self-contained from a clean checkout. The caller may pass an explicit path
# (or rely on `farol` already being built/on PATH); otherwise we build in place.
if [ ! -x "$BIN" ]; then
    mkdir -p "$(dirname "$BIN")"
    go build -o "$BIN" .
fi

# Pin the theme so frames don't depend on whatever the recorder's own config
# holds. The demo deliberately records in farol-ember (the warm amber reads
# well at GIF scale); the compiled default is farol-dark, so the pin is what
# keeps committed media reproducible if the default changes again.
rm -rf "$DATA" "$CONFIG"
mkdir -p "$DATA" "$CONFIG/farol"
printf 'theme: farol-ember\n' > "$CONFIG/farol/config.yaml"

export XDG_DATA_HOME="$DATA"
export XDG_CONFIG_HOME="$CONFIG"

run() { "$BIN" "$@"; }

# The first list (adopted on launch). List ids are resolved by id-prefix,
# never by name, so every add below uses this id.
LIST=$(run lists add "Home")

# Plan the garden — a root with a 3-level subtree, so the tape can show more
# than breadcrumb depth when it inserts a nested task.
garden=$(run add "$LIST" "Plan the garden")
soil=$(run add "$LIST" "Prep the beds" --parent "$garden")
run add "$LIST" "Order seed compost" --parent "$soil"
run add "$LIST" "Build the beds" --parent "$garden"
run add "$LIST" "Source the plants" --parent "$garden"

# Refinish the deck — a side root for the focus to move to.
run add "$LIST" "Refinish the deck"

# Reach the ferns — an in-progress percentage, so the (nn%) row suffix shows.
ferns=$(run add "$LIST" "Reach the ferns")
run progress "$ferns" --mode percentage --percent 45

# Clean the kitchen, completed, with descendants: the Complete section is
# populated, and the toggle-cascade demo has a real subtree to collapse.
kitchen=$(run add "$LIST" "Clean the kitchen")
run add "$LIST" "Clear the counters" --parent "$kitchen"
run add "$LIST" "Dust the shelves" --parent "$kitchen"
run "$kitchen"

# A second list so the lists panel has something to navigate between.
# "Deck project" gets a couple of tasks to show cross-list search.
DECK=$(run lists add "Deck project")
run add "$DECK" "Buy lumber"
run add "$DECK" "Measure the site"

echo "seeded list $LIST"
