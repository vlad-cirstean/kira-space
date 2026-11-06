package repos_test

import (
	"database/sql"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

// newRepos opens a real (tmpfile-backed) SQLite database through the real migrations — going
// through storage.Open() rather than hand-restating the schema is what makes every migration
// covered on every test run (P52 §13).
func newRepos(t *testing.T) *storage.DB {
	t.Helper()
	t.Setenv("KIRA_SPACE_HOME", t.TempDir())
	db, err := storage.Open()
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newFilterHistoryRepo(t *testing.T) *repos.FilterHistoryRepo {
	return &repos.FilterHistoryRepo{DB: newRepos(t).DB}
}

func newMetadataCacheRepo(t *testing.T) *repos.MetadataCacheRepo {
	return &repos.MetadataCacheRepo{DB: newRepos(t).DB}
}

func newOpsRepo(t *testing.T) *repos.DbOpsRepo {
	return &repos.DbOpsRepo{DB: newRepos(t).DB}
}

// seedConnection inserts a minimal connections row so a test can reference connID from a table
// with a foreign key into connections(id) — foreign_keys=ON (P52 §4.3) enforces this for real.
func seedConnection(t *testing.T, db *sql.DB, connID string) {
	t.Helper()
	now := kiratime.NowISO()
	if _, err := db.Exec(
		`INSERT INTO connections (id, name, kind, color, mode, read_only, created_at, updated_at, sort_order)
		 VALUES (?, ?, 'postgres', 'blue', 'fields', 0, ?, ?, 0)`,
		connID, connID, now, now,
	); err != nil {
		t.Fatalf("seedConnection(%s): %v", connID, err)
	}
}
