package appwire

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/leftovers"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/wtmove"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// RunOneTimeMoves runs the v0.3.1 to v0.3.2 data moves (the worktree move is RunWorktreeMove) after the database opens and before Build. The
// Keychain item moves when the cipher is created, so the caller creates it first. A failed move logs and
// leaves its source in place; the next boot retries. The leftovers cleanup runs only when every move
// succeeded in this boot. The next chapter deletes this file and everything it calls.
func RunOneTimeMoves(db *sql.DB, home string, cipher *secrets.Cipher) {
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

// RunWorktreeMove moves task worktrees from <userHome>/wt to <userHome>/kira-space-wts (P563). It runs
// after the settings read and before Build, so no board, terminal or run holds a worktree yet. It
// needs a working git, looked up only when a move is pending: otherwise it logs and the next boot retries. Independent of RunOneTimeMoves.
func RunWorktreeMove(db *sql.DB, userHome, configuredGitPath string, locator gitclient.Locator) {
	if !wtmove.Needed(db, userHome) {
		return
	}
	status := gitclient.NewDiscovery(locator, gitclient.NewExecRunner(), gitclient.NewRealClock()).Status(context.Background(), configuredGitPath)
	if status.Kind != "ok" {
		slog.Warn("worktree move skipped: git unavailable", "scope", "startup", "kind", status.Kind)
		return
	}
	wtmove.Run(wtmove.Options{DB: db, Home: userHome, GitPath: status.Path, NewBase: ade.WorktreesBase(userHome), Segment: ade.PathSegment})
}
