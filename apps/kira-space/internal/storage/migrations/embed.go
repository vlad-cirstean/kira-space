// Package migrations embeds kira.db's schema steps. 0090 is the v0.3.1 baseline; later steps
// number from 0091.
package migrations

import (
	"embed"

	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

//go:embed *.sql
var files embed.FS

// names lists the embedded files in the exact order they must apply, rather than trusting
// directory listing order.
var names = []sqlitex.MigrationSource{
	{Version: 90, Name: "p557_baseline", File: "0090_p557_baseline.sql"},
	{Version: 91, Name: "p557_drop_legacy", File: "0091_p557_drop_legacy.sql"},
}

// All returns every migration in ascending version order.
func All() ([]sqlitex.Migration, error) {
	return sqlitex.LoadMigrations(files, names)
}
