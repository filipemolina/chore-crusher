// Package store is the only package in this module that imports
// database/sql or modernc.org/sqlite. It owns the schema, the embedded
// migrations, and every read/write function — including the full
// status/progress state machine in docs/DESIGN.md §3. Both src/model (the
// TUI) and src/cli depend on this package and nothing deeper; neither
// builds a SQL string of its own.
package store
