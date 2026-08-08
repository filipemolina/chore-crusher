.PHONY: dev build test demo

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

# Run the test suite
test:
	go test -count=1 ./...
	go test -race ./src/mcpserver/ ./src/store/ ./src/cli/

# Build and seed the demo, then record a new demo GIF.
# Requires VHS (https://github.com/charmbracelet/vhs) and ffmpeg.
demo:
	go build -o /tmp/chore-crusher-demo/crush .
	./demo/seed.sh /tmp/chore-crusher-demo/crush
	vhs demo/demo.tape
	./demo/compress.sh
	vhs demo/screenshots.tape
