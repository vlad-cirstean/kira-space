package sqlite

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// caps is caps.ts's sqliteCaps. Cancel is true here, not false like the TypeScript original:
// node:sqlite exposed no sqlite3_interrupt and its whole API was synchronous, so a running
// statement blocked the event loop and an abort could never even be delivered while one ran —
// the app's first honest `false`. modernc.org/sqlite has a real sqlite3_interrupt, reached by
// cancelling the adapter-owned per-op driver context (adapter.go's runOnConn), so this is the Go
// port's first honest `true` instead. That value now matches the other three relational engines'
// own Cancel (adapters.RelationalCaps) — the divergence this comment describes is against the old
// TypeScript build, not against them.
// ExactCount is unchanged: count(*) over a million rows measured at 9ms in the sandbox — cheaper
// than any other engine in the app. FileTransfer stays false: SQLite being itself a file does not
// make its items (rows) files.
var caps = adapters.RelationalCaps
