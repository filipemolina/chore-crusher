#!/usr/bin/env bash
# Seed the demo store with deterministic data that the VHS tapes record in.
# The tapes (demo/*.tape) set the same XDG dirs, so a stamped seed is what
# they show — the recording neither depends on nor clobbers the real store.
#
#   ./demo/seed.sh [binary]     launch path defaults to /tmp/chore-crusher-demo/crush
#
# Everything lives under /tmp/chore-crusher-demo so a run is reproducible from a
# clean checkout and touches nothing outside it.
set -euo pipefail

DATA=/tmp/chore-crusher-demo/data
CONFIG=/tmp/chore-crusher-demo/config
BIN=${1:-/tmp/chore-crusher-demo/crush}

# Pin the theme so frames don't depend on whatever the recorder's own config
# holds. crush-dark is also the compiled default, so this is belt-and-
# suspenders against a later re-theme.
rm -rf "$DATA" "$CONFIG"
mkdir -p "$DATA" "$CONFIG/chore-crusher"
printf 'theme: crush-dark\n' > "$CONFIG/chore-crusher/config.yaml"

export XDG_DATA_HOME="$DATA"
export XDG_CONFIG_HOME="$CONFIG"

run() { "$BIN" "$@"; }

# The list the TUI adopts on launch (the only list, so it is first).
# List ids are resolved by id-prefix, never by name, so every add uses this id.
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

echo "seeded list $LIST"