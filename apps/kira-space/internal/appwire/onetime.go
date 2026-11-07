package appwire

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/wtmove"
)

// RunWorktreeMove moves task worktrees from <userHome>/wt to <userHome>/kira-space-wts (P563). It runs
// after the settings read and before Build, so no board, terminal or run holds a worktree yet. It
// needs a working git, looked up only when a move is pending: otherwise it logs and the next boot retries.
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
