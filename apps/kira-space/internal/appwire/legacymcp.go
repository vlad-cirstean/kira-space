package appwire

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/renameio/v2"
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

// legacyMCPMarker is the file in the Space home that records a clean removal, so later boots skip the
// two CLI spawns.
const legacyMCPMarker = ".legacy-mcp-removed"

// removeLegacyMCP deletes the retired user-scope registrations. Best effort: a failure logs the name and
// the error and the next name still runs. Absent names and a missing CLI are silent. A pass with no
// failure writes the marker; a failed pass retries next boot. The next chapter deletes this file and its
// call in RunOneTimeMoves.
func removeLegacyMCP(r harness.MCPRemover, home string) {
	marker := filepath.Join(home, legacyMCPMarker)
	if _, err := os.Lstat(marker); err == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	failed := false
	for _, name := range legacyMCPNames {
		if err := r.Remove(ctx, name); err != nil {
			failed = true
			slog.Warn("remove retired MCP registration", "scope", "startup", "name", name, "err", err)
		}
	}
	if failed {
		return
	}
	if err := renameio.WriteFile(marker, nil, 0o600); err != nil {
		slog.Warn("record retired MCP removal", "scope", "startup", "err", err)
	}
}
