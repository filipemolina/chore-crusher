---
title: Development workflow
description: The build/test loop, CI, code style, and releases.
sidebar:
  order: 9
---

## Setup

Prerequisites: **Go 1.26 or newer** (the module declares `go 1.26.4`). No CGO required — the SQLite driver is pure Go, so `CGO_ENABLED=0` works everywhere.

```bash
go build ./...
make dev             # launches the TUI against your real store
go run main.go --help   # the CLI reference
```

For throwaway data, point `XDG_DATA_HOME` and `XDG_CONFIG_HOME` at a temp directory, the way `demo/seed.sh` does.

## The loop

Run before every commit:

```bash
make dev            # go run main.go (launches the TUI against your real store)
go build ./...
go vet ./...
go test ./...
gofmt -l .          # must print nothing
```

CI runs exactly these on every pull request, `go test -race` included: `src/store` opens real SQLite connections and the TUI's poll loop runs on its own goroutine, both worth checking under the race detector.

## Keep every commit green

**Keep every commit green, not just the branch tip.** The phases were built as feature branches of small commits with `go build ./... && go vet ./... && go test ./...` green at every commit, merged with `--no-ff` so a phase can be reverted as a unit (`docs/ROADMAP.md`). A commit that breaks the build makes `git bisect` useless and forces the next contributor to guess which of several broken commits is the one to fix.

## Work in the smallest increment you can verify

Write one function (or one tight group of related ones), then run `go build ./... && go vet ./...` immediately, and `go test ./...` too if it has a test yet. A model's error rate per line written is roughly constant regardless of how careful the instructions are; the thing that actually works is shrinking how many unverified lines can pile up before the first check catches a mistake.

Before calling a dependency function you didn't write this session, confirm its signature with `go doc <package>.<Symbol>` — it reads the actual installed version, not whatever a training corpus happened to describe. Before assuming a helper doesn't exist in this repo and writing your own, `grep -rn` for it first: `docs/DESIGN.md` repeatedly names behavior that one shared function already provides for every surface (the tree flattening in `src/apptypes/flatten.go`, the three `store` mutators in §3), and a second implementation is the most common way the surfaces drift apart.

## Code style

- **Comments say why, not what.** The code already says what happens; a comment earns its place by recording a decision, a constraint, or a trap — the kind of thing a future edit would otherwise "fix" back into a bug. Design reasoning belongs in `docs/DESIGN.md`, at length; a code comment is for the reasoning that's only legible standing next to the specific line it explains.
- **Constructors are always `New`; the exported type is always `Model`** for a Bubble Tea component, so callers read as `listnamemodal.New(...)` and assert on `listnamemodal.Model`.
- **Commit messages:** a short summary line, then prose explaining why, not what. Match the register of the existing `git log`. The convention is `area: description` (e.g. `keys: add the panel-cycle binding`).
- **Use the glossary's terms everywhere** (`CONTRIBUTING.md`): Task, Subtask, List, Status, Progress kind, Cascade, Zone, Level offset, Tier. A term that drifts between "task" and "item" and "todo" across files is indistinguishable, to the next reader, from three different things.

## CI

`.github/workflows/ci.yml` runs on every push to `main` and every pull request:

1. **Build** — `go build ./...`
2. **Vet** — `go vet ./...`
3. **Check formatting** — `gofmt -l .`, with the listing itself turned into the failure (gofmt exits 0 whatever it finds)
4. **Test** — `go test -race ./...`

The Go toolchain is pinned by `go-version-file: go.mod`, so bumping the module bumps CI with it. A second push to a branch cancels the first run's in-progress job.

## Releases

Maintainer-only. Pushing a `v*` tag triggers `.github/workflows/release.yml`, which runs GoReleaser (`.goreleaser.yaml`):

- `CGO_ENABLED=0`, cross-compiled **linux/darwin × amd64/arm64** — the pure-Go SQLite driver is what keeps that matrix possible.
- Archives as `tar.gz` (`farol_<version>_<os>_<arch>`), a `checksums.txt`, and a changelog built from commits since the last tag (docs/test/merge commits excluded).
- Releases are **drafted**, not published — a human reads the notes before they go out.

## Version stamping

The version the binary reports comes from `src/constants.Version()`:

- A **stamped value wins** — that is a release, and it says so. The stamp is applied at link time: `-X github.com/filipemolina/farol/src/constants.version=<version>`. The Makefile stamps `git describe --tags --always --dirty`; GoReleaser stamps the tag with the leading `v` put back (`v{{ .Version }}`), so a released binary and a local `make build` answer `--version` the same way.
- Otherwise the build info answers: a checkout's short VCS revision (with `-dirty` when uncommitted changes exist), or the module version for a `go install ...@v0.1.0` install.

`--version` comes from the cobra root command's `Version` field, which reads `constants.Version()`.