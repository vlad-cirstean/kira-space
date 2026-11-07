package appwire

import (
	"database/sql"
	"log/slog"

	"github.com/kirathecat/kira-studio/internal/agentcfg"
)

// RunOneTimeMoves runs the v0.3.1 to v0.3.2 data moves, in order, after the database opens and before
// Build. A failed move logs and leaves its source in place; the next boot retries. The next chapter
// deletes this file and every move it calls.
func RunOneTimeMoves(db *sql.DB, home string) {
	if err := agentcfg.MoveSecretsFile(db, home); err != nil {
		slog.Warn("move agent secrets into kira.db", "scope", "startup", "err", err)
	}
}
