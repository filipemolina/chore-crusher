package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver; see CONTRIBUTING.md's trap notes

	"github.com/filipemolina/chore-crusher/src/store/migrations"
)

// Status is a task's lifecycle state. The values are the literal strings
// stored in the database; docs/DESIGN.md §3 is the authority on which
// transitions are allowed.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusComplete   Status = "complete"
)

// ProgressKind describes how an in-progress task reports progress. It only
// has meaning while Status is StatusInProgress; the store keeps it 'none'
// for pending and complete tasks (docs/DESIGN.md §3).
type ProgressKind string

const (
	ProgressNone       ProgressKind = "none"
	ProgressSimple     ProgressKind = "simple"
	ProgressSubtasks   ProgressKind = "subtasks"
	ProgressPercentage ProgressKind = "percentage"
)

// Task mirrors one row of the Task table. ParentID is nil for a root-level
// task; ProgressPct is set only when ProgressKind is ProgressPercentage;
// CompletedAt is set only when Status is StatusComplete.
type Task struct {
	ID           string
	ListID       string
	ParentID     *string
	Title        string
	Notes        string
	Status       Status
	ProgressKind ProgressKind
	ProgressPct  *int
	Position     int
	CreatedAt    int64
	UpdatedAt    int64
	CompletedAt  *int64
}

// List mirrors one row of the List table.
type List struct {
	ID        string
	Name      string
	CreatedAt int64
	Position  int
	CreatedBy string
}

// ListSummary is a List plus its task counts, as returned by ListLists.
// PendingCount counts every task whose status is not complete (pending and
// in-progress alike); CompleteCount counts status = complete.
type ListSummary struct {
	List
	PendingCount  int
	CompleteCount int
}

// Store is a handle to one SQLite database file. Open is the only way to
// construct one and the only function in the codebase that opens the
// database (docs/DESIGN.md §8) — nothing else may call sql.Open.
type Store struct {
	db *sql.DB
}

// Open opens (creating the file if needed) the SQLite database at path,
// applies any pending migrations, and returns a Store for it. The parent
// directory is created when missing — the CLI's default path lives under
// $XDG_DATA_HOME/complete (docs/DESIGN.md §8), which exists on no fresh
// machine. Calling Open twice against the same path is a no-op on the
// schema: migrations that are already recorded are skipped.
//
// The DSN carries the connection-level settings the driver parses directly
// off the query string rather than as separate PRAGMA statements — WAL
// journal mode so the TUI's read connection and a CLI write never block each
// other, foreign-key enforcement so the ON DELETE CASCADE foreign keys in
// 0001_init.sql actually cascade, and a busy timeout so a writer waiting on
// the TUI's read connection errors after 5s instead of immediately. The
// parameter names are verified against modernc.org/sqlite v1.55.0's DSN
// parsing in driver.go.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data directory for %s: %w", path, err)
	}
	dsn := "file:" + path + "?_journal_mode=WAL&_foreign_keys=1&_busy_timeout=5000"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	// Serialize all access within this process on a single connection. The
	// WAL writer lock is process-wide: with an unlimited pool, two agent
	// writes dispatched in one batch each grab their own connection and the
	// second blocks on the first's write; under the harness's parallel
	// dispatch that contention surfaced as SQLITE_BUSY even with the 5s
	// busy_timeout. One connection removes intra-process lock contention
	// entirely. Cross-process (TUI <-> agent) safety still relies on the
	// _busy_timeout=5000 above.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.applyMigrations(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	// Adopt A's "<tag>:" name convention into created_by for any list still
	// untagged. Runs once at Open (see backfillOwners for the rationale).
	if err := s.backfillOwners(); err != nil {
		db.Close()
		return nil, fmt.Errorf("backfill list owners %s: %w", path, err)
	}
	return s, nil
}

// ownerTagRE matches the leading "<tag>:" prefix of a list name — A's
// ownership convention (docs/plan/list-ownership-enforcement.md §3.7). The
// capture is the owner tag. modernc.org/sqlite exposes no REGEXP, so the scan
// runs in Go over the rows whose created_by is still empty.
var ownerTagRE = regexp.MustCompile(`^([A-Za-z0-9_-]+):`)

// backfillOwners adopts A's "<tag>:" name convention into the created_by
// column for every list still untagged. It runs once, at Open, after
// migrations. The selection + update are split into two phases on purpose:
// SetMaxOpenConns(1) leaves a single pooled connection, so UPDATEs cannot run
// while the SELECT's rows still hold that connection — we drain the rows into
// a slice, close them, then issue the UPDATEs.
//
// The pass is idempotent: it only touches rows where created_by = "", and a
// row once tagged keeps its tag, so re-opening never re-adopts it. Lists
// created after Open with a "<tag>:" name stay untagged until the next Open —
// a documented limitation (§3.7).
func (s *Store) backfillOwners() error {
	rows, err := s.db.Query(`SELECT id, name FROM List WHERE created_by = ''`)
	if err != nil {
		return err
	}
	type taggedList struct{ id, owner string }
	var tagged []taggedList
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return err
		}
		if m := ownerTagRE.FindStringSubmatch(name); m != nil {
			tagged = append(tagged, taggedList{id, m[1]})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, tl := range tagged {
		if _, err := s.db.Exec(
			`UPDATE List SET created_by = ? WHERE id = ?`, tl.owner, tl.id,
		); err != nil {
			return err
		}
	}
	return nil
}

// Close releases the underlying database handle. The TUI holds a Store for
// the process's lifetime; CLI subcommands close theirs before exiting.
func (s *Store) Close() error {
	return s.db.Close()
}

// applyMigrations runs every embedded migration whose version is not yet
// recorded in schema_migrations, in filename order, each inside its own
// transaction.
func (s *Store) applyMigrations() error {
	applied := map[int]bool{}

	// On a brand-new database schema_migrations does not exist yet — the
	// tracking table is created by 0001_init.sql — so the applied-versions
	// read is conditional on the table already being there.
	var tracked bool
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations')`).Scan(&tracked); err != nil {
		return err
	}
	if tracked {
		rows, err := s.db.Query(`SELECT version FROM schema_migrations`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v int
			if err := rows.Scan(&v); err != nil {
				return err
			}
			applied[v] = true
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}

	names, err := migrations.FS.ReadDir(".")
	if err != nil {
		return err
	}
	sort.Slice(names, func(i, j int) bool {
		return versionOf(names[i].Name()) < versionOf(names[j].Name())
	})

	for _, entry := range names {
		name := entry.Name()
		v := versionOf(name)
		if applied[v] {
			continue
		}
		contents, err := migrations.FS.ReadFile(name)
		if err != nil {
			return err
		}
		if err := s.applyOneMigration(v, string(contents)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		applied[v] = true
	}
	return nil
}

// applyOneMigration runs one migration file in a transaction and records its
// version. The whole file goes to a single Exec — the driver executes
// multiple statements per Exec (verified against modernc.org/sqlite
// v1.55.0) — rather than splitting on ";", which would mangle a semicolon
// inside a SQL comment. A failure that happened because a concurrent Open
// (first run of the TUI next to a CLI invocation) already applied this same
// migration is not an error — the schema_migrations entry it left behind is
// the same success.
func (s *Store) applyOneMigration(version int, contents string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(contents); err != nil {
		if appliedByOther(tx, version) {
			return nil
		}
		return err
	}

	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, version, time.Now().Unix()); err != nil {
		if appliedByOther(tx, version) {
			return nil
		}
		return err
	}
	return tx.Commit()
}

// appliedByOther reports whether version is already recorded in
// schema_migrations, which — because the recording insert shares the
// migration transaction — can only be true if another process's Open applied
// it concurrently.
func appliedByOther(tx *sql.Tx, version int) bool {
	var one int
	return tx.QueryRow(`SELECT 1 FROM schema_migrations WHERE version = ?`, version).Scan(&one) == nil
}

// versionOf extracts the numeric version prefix of a migration filename
// ("0001_init.sql" → 1). The filename convention is enforced by the runner,
// not by the embed.
func versionOf(name string) int {
	prefix, _, _ := strings.Cut(name, "_")
	v, err := strconv.Atoi(prefix)
	if err != nil {
		return -1 // sorts before any valid version; a malformed name is loud
	}
	return v
}

// querier is satisfied by both *sql.DB and *sql.Tx, so the row helpers in
// this package run inside a transaction or directly without the caller
// choosing which.
type querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// rowScanner is the common Scan surface of *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// requireAffected turns a mutating statement that touched zero rows into the
// same "not found" error the read path returns.
func requireAffected(res sql.Result, what, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s %q not found", what, id)
	}
	return nil
}

// isNoRows reports whether err is database/sql's sentinel for an empty result.
func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
