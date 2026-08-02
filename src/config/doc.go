// Package config reads and writes ~/.config/chore-crusher/config.yaml (or
// $XDG_CONFIG_HOME), following stack-stitcher's src/config exactly: a
// missing file or field falls back to the compiled default, a malformed
// file is reported. See docs/DESIGN.md §8.
package config
