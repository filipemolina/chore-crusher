#!/usr/bin/env bash
# Seed the demo store with deterministic data that the VHS tapes record in.
# The tapes (demo/*.tape) set the same XDG dirs, so a stamped seed is what
# they show -- the recording neither depends on nor clobbers the real store.
#
#   ./demo/seed.sh [binary]     launch path defaults to /tmp/farol-demo/farol
#
# Everything lives under /tmp/farol-demo so a run is reproducible from a
# clean checkout and touches nothing outside it.
set -euo pipefail

DATA=/tmp/farol-demo/data
CONFIG=/tmp/farol-demo/config
BIN=${1:-/tmp/farol-demo/farol}

# The version the recorded binary reports in its header bar. An unstamped
# build falls through to constants.Version's VCS-revision branch and renders a
# bare commit hash, which is what shipped in every landing-page screenshot up
# to 2026-08-15. FAROL_DEMO_VERSION overrides for a release recording; the
# default matches the Makefile so `make demo` and a manual run agree.
VERSION=${FAROL_DEMO_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || true)}
LDFLAGS="-X github.com/filipemolina/farol/src/constants.version=$VERSION"

# Build the binary into the demo dir, so the script is self-contained from a
# clean checkout. This rebuilds every run rather than skipping an existing
# binary: the version is baked in at link time, so a leftover binary from an
# earlier checkout silently records the wrong version -- a stale-binary trap
# that has already shipped wrong assets once. Go's build cache makes the
# repeat cost negligible.
mkdir -p "$(dirname "$BIN")"
go build -ldflags "$LDFLAGS" -o "$BIN" .

# Pin the theme so frames don't depend on whatever the recorder's own config
# holds. The demo records in farol-dark (the fresh-install default and the
# brand navy/amber palette); the pin keeps committed media reproducible if
# the default changes again.
rm -rf "$DATA" "$CONFIG"
mkdir -p "$DATA" "$CONFIG/farol"
printf 'theme: farol-dark\n' > "$CONFIG/farol/config.yaml"

export XDG_DATA_HOME="$DATA"
export XDG_CONFIG_HOME="$CONFIG"

run() { "$BIN" "$@"; }

# `lists add` leaves created_by empty, which marks the list human-managed. The
# ownership guard (docs/DESIGN.md Sec9) then refuses every structural write from
# an agent identity -- and agentIdentity() defaults to "agent" when FAROL_AGENT
# is unset, so a plain shell counts as one. That makes an unforced `add` fail
# here with "owned by nobody (human-managed)", which is exactly what this
# script is: a human populating their own store. --force is the documented
# escape hatch for that case, so every structural write below takes it.
#
# This guard postdates the last recording, which is why the committed demo
# media stopped matching the tapes' own comments.
add() { "$BIN" add --force "$@"; }

# The first list (adopted on launch). List ids are resolved by id-prefix,
# never by name, so every add below uses this id.
LIST=$(run lists add "api")

# Ship auth v2 -- a root with a 3-level subtree, so the tape can show more
# than breadcrumb depth when it inserts a nested task.
auth=$(add "$LIST" "Ship auth v2")
oauth=$(add "$LIST" "Wire the OAuth callback" --parent "$auth")
add "$LIST" "Add the state-param check" --parent "$oauth"
add "$LIST" "Migrate the sessions table" --parent "$auth"
add "$LIST" "Backfill refresh tokens" --parent "$auth"

# Cut the p95 on /search -- a side root for the focus to move to. Its id is kept
# because the search-ranking notes below hang off it.
p95=$(add "$LIST" "Cut the p95 on /search")

# Rewrite the ingest worker -- an in-progress percentage, so the (nn%) row suffix shows.
ingest=$(add "$LIST" "Rewrite the ingest worker")
run progress "$ingest" --mode percentage --percent 45

# Drop the legacy /v1 routes, completed, with descendants: the Complete section is
# populated, and the toggle-cascade demo has a real subtree to collapse.
legacy=$(add "$LIST" "Drop the legacy /v1 routes")
add "$LIST" "Delete the v1 handlers" --parent "$legacy"
add "$LIST" "Remove v1 from the router" --parent "$legacy"
run "$legacy"

# A second list so the lists panel has something to navigate between.
# "infra" gets a couple of tasks to show cross-list search.
INFRA=$(run lists add "infra")
add "$INFRA" "Raise the gateway timeout"
add "$INFRA" "Pin the runner image"

# Notes exist so a search screenshot can actually demonstrate the ranking rule
# the docs and the landing page both claim: title matches rank above
# notes-only hits. Without a single note in the store, a query can only ever
# return title matches, and the claim is untestable and unphotographable.
#
# The query these are built for is "timeout":
#   - "Raise the gateway timeout" (infra) matches on TITLE     -> ranks first
#   - "Cut the p95 on /search" (api) matches on NOTES only     -> ranks below
# which also spans two lists, so one shot shows cross-list search and the
# ranking rule at the same time.
run notes --force "$p95" "The N+1 on tags is most of it. Client timeout is 2s and p95 is 2.4s."

# Every write above auto-claimed presence under the default "agent" identity
# (autoClaimTask), and claims stay live for store.WorkTTL = 120s. Seeding
# therefore leaves ~12 spinners lit on a store that is supposed to be at rest,
# and any tape recorded within two minutes of seeding -- which is every tape,
# since seeding is the step before recording -- photographs them. Release them
# so the seeded store starts quiet and a spinner in a frame means something.
run release --all >/dev/null

echo "seeded list $LIST"
