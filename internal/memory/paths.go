// Package memory is the long-term memory store shared by Kira Space's Memory module and the
// kira-memory stdio MCP server: SQLite + FTS5 storage, recall-first search, and the Claude Code
// gate/reconcile pipeline.
package memory

import (
	"path/filepath"

	"github.com/kirathecat/kira-studio/internal/kirapaths"
)

// Home resolves the memory data directory: <Space home>/memory, default ~/.kira-space/memory.
func Home() string {
	return kirapaths.MemoryHome()
}

// DefaultPath is memory.db under Home.
func DefaultPath() string {
	return filepath.Join(Home(), "memory.db")
}
