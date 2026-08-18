---
title: Installation
description: Prerequisites and the two ways to install farol — go install or build from source.
sidebar:
  order: 2
---

## Prerequisites

- **Go 1.26 or newer** — needed for `go install` and for building from source.
- **A terminal** — farol is a full-screen TUI. It works over SSH too; copy-id uses the terminal's own clipboard protocol, so no `xclip` or `pbcopy` is needed.
- **No CGO.** The SQLite driver is pure Go, so there is no C compiler in the toolchain and no platform-specific build step.

## Install with `go install`

```bash
go install github.com/filipemolina/farol@latest
```

This installs the `farol` binary into `$(go env GOPATH)/bin` (usually `~/go/bin`). Make sure that directory is on your `PATH`.

## Build from source

```bash
git clone https://github.com/filipemolina/farol.git
cd farol
make build     # installs to $(go env GOPATH)/bin, usually ~/go/bin
```

`make build` stamps the version into the binary from `git describe`, so `farol --version` reports the tag or commit you built.

## Verify

```bash
farol --version
```

An unstamped local build reports its commit hash instead of a version — that is expected, and it is exactly what a bug report wants.

## A note on binaries

Releases are built with GoReleaser: `CGO_ENABLED=0`, cross-compiled for **Linux and macOS, on amd64 and arm64**. Pushing a `v*` tag builds and drafts the release. `go install` or a clone is the way in for everyone else.
