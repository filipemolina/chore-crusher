// Package cli holds the Cobra command definitions — one file per subcommand
// group — each a thin adapter from flags to a src/store call and a
// --json-aware printer. See docs/DESIGN.md §9 for the full contract. This
// package never imports src/model, and src/model never imports this one:
// they are siblings over src/store, not layered on each other.
package cli
