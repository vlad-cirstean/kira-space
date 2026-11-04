// Package migrations embeds review.db's own schema steps — a sibling of internal/storage/
// migrations, deliberately duplicated rather than shared (gitreview/migrate.go's own doc comment
// explains why): a second SQLite file with its own lifecycle, not a table in kira.db.
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
	{Version: 1, Name: "g11_review", File: "0001_g11_review.sql"},
	{Version: 2, Name: "g13_comments", File: "0002_g13_comments.sql"},
	{Version: 3, Name: "p150_pin", File: "0003_p150_pin.sql"},
	{Version: 4, Name: "p286_comment_kind", File: "0004_p286_comment_kind.sql"},
	{Version: 5, Name: "p418_working", File: "0005_p418_working.sql"},
	{Version: 6, Name: "p499_removed", File: "0006_p499_removed.sql"},
}

// All returns every migration in ascending version order.
func All() ([]sqlitex.Migration, error) {
	return sqlitex.LoadMigrations(files, names)
}
