package appwire

import (
	"context"
	"log/slog"
	"time"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/claude"
)

// legacyMCPNames are the MCP server names P560 retired. Only kira-db was ever registered at user scope
// (the Settings Install button); kira-ade lived in per-run configs, and the remove covers it anyway.
var legacyMCPNames = []string{"kira-db", "kira-ade"}

// NewLegacyMCPRemover is the real remover: Claude Code's own `mcp remove`. Kira never edits the
// agent's config files itself.
func NewLegacyMCPRemover() harness.MCPRemover {
	r, _ := claude.New().MCPInstaller().(harness.MCPRemover)
	return r
}

// removeLegacyMCP deletes the retired user-scope registrations. Best effort: a failure logs the name and
// the error and the next name still runs. Absent names and a missing CLI are silent. The next chapter
// deletes this file and its call in RunOneTimeMoves.
func removeLegacyMCP(r harness.MCPRemover) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, name := range legacyMCPNames {
		if err := r.Remove(ctx, name); err != nil {
			slog.Warn("remove retired MCP registration", "scope", "startup", "name", name, "err", err)
		}
	}
}
