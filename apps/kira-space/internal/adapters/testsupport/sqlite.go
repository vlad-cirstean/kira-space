package testsupport

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const bigRowsSqlite = 1_000_000

// SqliteFixture is sqlite.ts's SqliteFixture — D32: a temp-file fixture, not a container. No image
// pin, no healthcheck, no wait strategy, no root-versus-app-user split, no resolveDockerHost() — a
// file-based engine makes this the simplest fixture of the four.
type SqliteFixture struct {
	Path   string
	Dir    string
	Config model.ResolvedConnectionConfig
}

var sqliteMemo fixture[SqliteFixture]

const sqliteDatabaseFile = "kira_test.sqlite"

// StartSqlite is sqlite.ts's startSqlite. No Docker gate — nothing it needs depends on the daemon.
// Memoized the same way every other fixture in this package is (B15): termination happens once,
// from the package's own TestMain, never from an individual test's t.Cleanup.
func StartSqlite(t *testing.T) *SqliteFixture {
	t.Helper()
	f, err := sqliteMemo.get(startSqlite)
	if err != nil {
		t.Fatalf("sqlite fixture: %v", err)
	}
	return f
}

// StopSqlite removes the memoized temp directory, if one was ever created. Call once, from the
// test binary's own TestMain, after m.Run() returns.
func StopSqlite() {
	sqliteMemo.stop(func(f *SqliteFixture) { _ = os.RemoveAll(f.Dir) })
}

func startSqlite() (*SqliteFixture, error) {
	dir, err := os.MkdirTemp("", "kira-sqlite-")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, sqliteDatabaseFile)

	seedPath := filepath.Join(repoRoot(), "packages", "db-fixtures", "fixtures", "0009_sqlite_seed.sql")
	seedSQL, err := os.ReadFile(seedPath)
	if err != nil {
		return nil, err
	}

	// Plain create-if-missing DSN, deliberately not the adapter's own mode=ro/mode=rw — this is the
	// fixture creating the file for the first time, the one case in the whole codebase where SQLite
	// creating a database on open is exactly what is wanted (the "Kira never creates a database"
	// rule is about the *adapter*, not this harness).
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if _, err := db.Exec(string(seedSQL)); err != nil {
		return nil, err
	}
	// One plain WITH RECURSIVE CTE inserts all 1,000,000 rows directly — no chunking, no PRAGMA
	// adjustment needed, unlike MariaDB's SEQUENCE-engine seq_1_to_N or MySQL's chunked digits cross
	// join (sqlite.ts's own comment, ported verbatim).
	bigRowsSQL := `WITH RECURSIVE seq(n) AS (
		SELECT 1
		UNION ALL
		SELECT n + 1 FROM seq WHERE n < ` + strconv.Itoa(bigRowsSqlite) + `
	)
	INSERT INTO big_rows (id, payload)
	SELECT n, hex(randomblob(16)) FROM seq`
	if _, err := db.Exec(bigRowsSQL); err != nil {
		return nil, err
	}
	// ANALYZE on big_rows only, mirroring 0002_mariadb_seed.sql's own note — every other table is
	// left with no sqlite_stat1 row, which is what scenario 6 needs to assert a null estimate.
	if _, err := db.Exec("ANALYZE big_rows"); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	cfg := model.ResolvedConnectionConfig{
		ID: "test-sqlite", SortOrder: 0, CreatedAt: now, UpdatedAt: now,
		Name: "Test SQLite", Kind: "sqlite", Color: "violet", Mode: "fields", ReadOnly: false,
		Database: Strp(path), Options: map[string]any{},
	}
	return &SqliteFixture{Path: path, Dir: dir, Config: cfg}, nil
}
