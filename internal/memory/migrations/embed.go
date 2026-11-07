// Package migrations embeds memory.db's schema steps. 0008 is the v0.3.1 baseline.
package migrations

import (
	"embed"

	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

//go:embed *.sql
var files embed.FS

var names = []sqlitex.MigrationSource{
	{Version: 8, Name: "p557_baseline", File: "0008_p557_baseline.sql"},
	{Version: 9, Name: "p557_drop_legacy_ids", File: "0009_p557_drop_legacy_ids.sql"},
}

// All returns every migration in ascending version order.
func All() ([]sqlitex.Migration, error) {
	return sqlitex.LoadMigrations(files, names)
}
