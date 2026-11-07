package appwire

import (
	"database/sql"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/leftovers"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// RunOneTimeMoves runs the v0.3.1 to v0.3.2 data moves after the database opens and before Build. The
// Keychain item moves when the cipher is created, so the caller creates it first. A failed move logs and
// leaves its source in place; the next boot retries. The leftovers cleanup runs only when every move
// succeeded in this boot. The retired MCP registrations are removed in the background, since each remove
// spawns the agent CLI; a nil legacyMCP skips it. The next chapter deletes this file and everything it calls.
func RunOneTimeMoves(db *sql.DB, home string, cipher *secrets.Cipher, legacyMCP harness.MCPRemover) {
	if legacyMCP != nil {
		go removeLegacyMCP(legacyMCP)
	}
	if err := agentcfg.MoveSecretsFile(db, home); err != nil {
		slog.Warn("move agent secrets into kira.db", "scope", "startup", "err", err)
		return
	}
	if !cipher.Status().Available {
		slog.Warn("leftovers cleanup skipped: secret storage is unavailable", "scope", "startup")
		return
	}
	leftovers.Run(leftovers.Options{SpaceHome: home, StudioHome: leftovers.StudioHomeDefault(), MemoryHome: leftovers.MemoryHomeDefault()})
}
