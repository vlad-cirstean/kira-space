// Package sqlitex is the repo-root generalization of apps/kira-space/internal/storage's own
// modernc.org/sqlite open/pragma/migrate mechanics (db.go/migrate.go): the same shape was needed
// for apps/kira-space's own, separate database (its own kira.db, opened against its own
// home — see internal/kirapaths' own doc comment for why the two apps never share a home, a
// database or a socket). Each app's own internal/storage keeps its own DB wrapper type, its own
// embedded *.sql migration files, and its own schema-specific query methods; this package owns
// only the driver-level mechanics both need identically: the DSN pragma string, opening+pinning+
// permissioning the file, and the forward-only schema_version migration runner.
package sqlitex

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

// BuildDSN sets the startup pragmas through the DSN query string rather than as Exec statements
// after Open — unlike mattn/go-sqlite3's Go-side options, modernc.org/sqlite takes pragmas this
// way (see adapters/sqlite/client.go's own buildDSN), and doing it here means every connection the
// pool ever opens carries them, not just the first.
//
// P23 D5(a)/D6: _auto_vacuum=INCREMENTAL and journal_size_limit are the two more deliberate ones.
// Both were measured (P23 F16/F17) against the pinned modernc.org/sqlite driver before being added
// here — carried verbatim into this package, not re-derived, since this is a move of existing,
// already-measured behavior:
//
//   - _auto_vacuum only takes on a brand-new database (PRAGMA auto_vacuum reads back 2) and is
//     inert on an existing one (reads back 0, unchanged) — so this needs no version gate and can
//     never half-convert a database that already exists. INCREMENTAL rather than FULL: FULL
//     reorganises pages on every commit, a cost paid constantly on a store that commits on every
//     keystroke-debounced save, for a benefit (reclaiming freed pages) wanted only occasionally.
//     INCREMENTAL puts freed pages on the freelist and leaves the reclaim decision to each app's
//     own maintenance sweep, run once at startup.
//   - journal_size_limit(4194304) truncates the -wal file back down after a commit, rather than
//     leaving it at its session high-water mark forever (SQLITE_DEFAULT_JOURNAL_SIZE_LIMIT is -1
//     in the pinned amalgamation). 4 MiB is not chosen, it is derived: 1,000 pages
//     (SQLITE_DEFAULT_WAL_AUTOCHECKPOINT) at SQLite's 4 KiB default page size is exactly the size
//     the WAL is expected to reach between two automatic checkpoints, so this never truncates a
//     WAL doing its ordinary job. modernc.org/sqlite has no _journal_size_limit shorthand — this
//     goes through the generic _pragma= list, which F17 measured to apply before _journal_mode and
//     to read back correctly.
func BuildDSN(path string) string {
	q := url.Values{}
	q.Set("_busy_timeout", "5000")
	q.Set("_foreign_keys", "1")
	q.Set("_auto_vacuum", "INCREMENTAL")
	q.Set("_pragma", "journal_size_limit(4194304)")
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", "NORMAL")
	return "file:" + escapeDSNPath(path) + "?" + q.Encode()
}

// escapeDSNPath percent-encodes the three bytes modernc's sqlite3_open_v2 (opened with
// SQLITE_OPEN_URI) treats as URI syntax rather than literal path bytes: "%" (percent-decoding),
// "?" (starts the query string BuildDSN itself appends right after path) and "#" (starts a
// fragment). Left unescaped, a KIRA_SPACE_HOME-derived path carrying any of the three
// would open a different file than path names, while Open's own os.Chmod(path) below still
// targets the real one. Deliberately not net/url's own URL{Scheme:"file",...}.String(): that adds
// a "//" authority marker unconditionally, which turns a relative path (this DSN's existing,
// unchanged shape for one) into "file://<first-segment>/...", a URI with a non-empty authority
// SQLite's own parser rejects — this stays a plain "file:<path>?<query>" the way it always was,
// with only the three syntax bytes touched.
func escapeDSNPath(path string) string {
	path = strings.ReplaceAll(path, "%", "%25")
	path = strings.ReplaceAll(path, "?", "%3F")
	path = strings.ReplaceAll(path, "#", "%23")
	return path
}

// ReadOnlyURI is the URI of the database file at path opened read-only, for ATTACH on a connection
// opened through BuildDSN (which enables URIs).
func ReadOnlyURI(path string) string { return "file:" + escapeDSNPath(path) + "?mode=ro" }

// Open opens (or creates) the sqlite database file at path, applies the six startup pragmas via
// BuildDSN, forces the first real connection (database/sql's Open is lazy — Ping is what
// creates the file and applies the DSN pragmas), serializes every statement onto one connection
// (SetMaxOpenConns(1) — a small, single-writer configuration store has no SQLITE_BUSY class of bug
// to serialize away), and tightens the file to 0600 (unconditionally, not only on
// create, so an existing loose file is tightened too). The caller's own home/logs directory must
// already exist — Open does not create one, since "where the file lives" is each app's own
// kirapaths-based concern, not this package's.
func Open(path string) (*sql.DB, error) {
	return openDSN(path, BuildDSN(path))
}

// OpenImmediate is Open for a file two processes write: _txlock=immediate makes every
// transaction take the write lock at BEGIN, so a second writer waits on busy_timeout instead of
// failing mid-transaction on a read-to-write upgrade.
func OpenImmediate(path string) (*sql.DB, error) {
	return openDSN(path, BuildDSN(path)+"&_txlock=immediate")
}

func openDSN(path, dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlitex: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlitex: open %s: %w", path, err)
	}

	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlitex: chmod %s: %w", path, err)
	}

	return db, nil
}

// Migration is one forward-only schema step — the same shape both apps' own embedded migration
// packages (storage/migrations, gitreview/migrations) already return from their All() functions.
type Migration struct {
	Version int
	Name    string
	SQL     string
	// ForeignKeysOff runs the step with PRAGMA foreign_keys = OFF (a no-op inside a transaction,
	// so Migrate sets it on a pinned connection first), the SQLite 12-step table-rebuild shape:
	// DROP TABLE of a parent would otherwise fire ON DELETE CASCADE on every child. The step is
	// followed by PRAGMA foreign_key_check; any violation rolls the whole step back.
	ForeignKeysOff bool
}

// ForeignKeysOffMarker is the first line of a migration file that wants Migration.ForeignKeysOff.
const ForeignKeysOffMarker = "-- kira:foreign-keys off"

// SchemaTooNewError is Migrate's refusal when the database on disk was written by a build that
// understands a newer schema than this binary does — continuing would risk this older
// binary writing to a database shape it does not fully understand. Found is the schema_version
// row's value; Known is the highest version the migrations list passed to Migrate carries.
type SchemaTooNewError struct{ Found, Known int }

func (e *SchemaTooNewError) Error() string {
	return fmt.Sprintf(
		"sqlitex: database schema_version (%d) is newer than this build knows about (%d) — "+
			"refusing to run against a downgraded app", e.Found, e.Known,
	)
}

// SchemaTooNew lets a caller recognise this specific refusal structurally — via a small duck-typed
// interface it defines locally — without importing this package at all (G29 D1/D9's rationale:
// internal/startupfail stays a leaf with no modernc.org/sqlite in its dependency graph).
func (e *SchemaTooNewError) SchemaTooNew() (found, known int) { return e.Found, e.Known }

// SchemaTooOldError is Migrate's refusal when the database on disk predates the baseline: the
// first migration is a full-schema baseline, so a stored version between 0 and that baseline has
// no step that can bring it forward. Found is the schema_version row's value; Baseline is the
// first migration's version.
type SchemaTooOldError struct{ Found, Baseline int }

func (e *SchemaTooOldError) Error() string {
	return fmt.Sprintf(
		"sqlitex: database schema_version (%d) predates this build's baseline (%d) — "+
			"open it with the release that wrote the baseline first", e.Found, e.Baseline,
	)
}

// SchemaTooOld is the duck-typed counterpart of SchemaTooNew, for internal/startupfail.
func (e *SchemaTooOldError) SchemaTooOld() (found, baseline int) { return e.Found, e.Baseline }

// Migrate runs the forward-only schema_version runner against db: create schema_version if
// missing, seed it at 0, refuse (SchemaTooNewError) if the stored version is newer than the
// highest version in migrations, refuse (SchemaTooOldError) if it is above 0 and below the lowest
// migration version (the baseline), then apply every pending step in ascending order, one
// transaction per step, recording the new version in the same transaction as the step's own SQL.
// A fresh database (version 0) applies the baseline first.
func Migrate(db *sql.DB, migrations []Migration) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("sqlitex: create schema_version: %w", err)
	}

	var current int
	row := db.QueryRow(`SELECT version FROM schema_version LIMIT 1`)
	switch err := row.Scan(&current); err {
	case sql.ErrNoRows:
		if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (0)`); err != nil {
			return fmt.Errorf("sqlitex: seed schema_version: %w", err)
		}
		current = 0
	case nil:
		// current already populated by Scan.
	default:
		return fmt.Errorf("sqlitex: read schema_version: %w", err)
	}

	var maxVersion int
	baseline := 0
	for i, m := range migrations {
		if m.Version > maxVersion {
			maxVersion = m.Version
		}
		if i == 0 || m.Version < baseline {
			baseline = m.Version
		}
	}
	if current > maxVersion {
		return &SchemaTooNewError{Found: current, Known: maxVersion}
	}
	if current > 0 && current < baseline {
		return &SchemaTooOldError{Found: current, Baseline: baseline}
	}

	for _, m := range migrations {
		if m.Version <= current {
			continue
		}
		if m.ForeignKeysOff {
			if err := applyForeignKeysOff(db, m); err != nil {
				return err
			}
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("sqlitex: begin migration %s: %w", m.Name, err)
		}
		if _, err := tx.Exec(m.SQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlitex: apply migration %s: %w", m.Name, err)
		}
		if _, err := tx.Exec(`UPDATE schema_version SET version = ?`, m.Version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlitex: record migration %s: %w", m.Name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("sqlitex: commit migration %s: %w", m.Name, err)
		}
	}
	return nil
}

// applyForeignKeysOff applies one ForeignKeysOff step on a pinned connection and re-enables
// foreign keys on every exit path.
func applyForeignKeysOff(db *sql.DB, m Migration) (err error) {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("sqlitex: pin connection for migration %s: %w", m.Name, err)
	}
	defer func() {
		if _, ferr := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); ferr != nil && err == nil {
			err = fmt.Errorf("sqlitex: re-enable foreign keys after migration %s: %w", m.Name, ferr)
		}
		_ = conn.Close()
	}()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("sqlitex: disable foreign keys for migration %s: %w", m.Name, err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlitex: begin migration %s: %w", m.Name, err)
	}
	if _, err := tx.Exec(m.SQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("sqlitex: apply migration %s: %w", m.Name, err)
	}
	if err := checkForeignKeys(tx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("sqlitex: migration %s: %w", m.Name, err)
	}
	if _, err := tx.Exec(`UPDATE schema_version SET version = ?`, m.Version); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("sqlitex: record migration %s: %w", m.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlitex: commit migration %s: %w", m.Name, err)
	}
	return nil
}

// checkForeignKeys fails on the first PRAGMA foreign_key_check violation, naming table, rowid and parent.
func checkForeignKeys(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var table, parent string
		var rowid sql.NullInt64
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return fmt.Errorf("foreign_key_check scan: %w", err)
		}
		return fmt.Errorf("foreign key violation: table %s rowid %d references missing %s", table, rowid.Int64, parent)
	}
	return rows.Err()
}

// RequireOneRow turns an UPDATE/DELETE's RowsAffected() == 0 into a "no such <what>" error — both
// apps' repos packages' own requireOneRow, genericized to one shared copy (P107 T1-7).
func RequireOneRow(res sql.Result, what string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlitex: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("sqlitex: no %s", what)
	}
	return nil
}
