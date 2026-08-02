.PHONY: dev build

# The version the binary reports. `git describe` gives the tag on a tagged
# commit and tag-commits-hash between tags, so a build always says which
# commit it came from. With no tag in history it comes out empty, and the
# binary falls back to the commit in its own build info — see
# constants.Version, which is why this can be missing without breaking.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null)
LDFLAGS := -X github.com/filipemolina/chore-crusher/src/constants.version=$(VERSION)

dev:
	go run main.go

# Build and install to $(go env GOPATH)/bin (~/go/bin by default).
# ~/go/bin is on PATH, so `crush` is runnable immediately
# after `make build` — no sudo, no extra setup.
build:
	go build -ldflags "$(LDFLAGS)" -o "$(shell go env GOPATH)/bin/crush" .
