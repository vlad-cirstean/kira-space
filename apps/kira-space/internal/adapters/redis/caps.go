package redis

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// caps is caps.ts's redisCaps, literally (§4.3, C18). Key/value-shaped, cursor (SCAN) pagination,
// no FK navigation, a shell-style console. Cancel is true and honest despite Cancel() returning
// false permanently (C9) — CheckCancelled between bounded SCAN-family rounds is fully effective.
var caps = adapters.Caps{
	Tabular:    false,
	Documents:  false,
	KeyValue:   true,
	Stream:     false,
	KeyBrowser: true, // P41: a db index's key namespace is unbounded — browsed in a Browse tab
	// P63 §4.3: catalog.go's own KeyTypes pipelines TYPE per path, windowed by the renderer to the
	// visible rows rather than the whole (up to 200 000-key) level.
	KeyTypes:        true,
	DefaultPageKind: page.PageKindKeyValue,
	SQL:             true,
	// P23 D10: stays false permanently — a key's type/TTL/memory usage are already on every
	// KeyValuePage.
	Definition:    false,
	Describe:      false,
	SchemaColumns: false,
	Projection:    false,
	ServerFilter:  false,
	// Per-key counts use O(1) exact type-length commands (HLEN/SCARD/ZCARD/LLEN/XLEN, or 1 for a
	// string).
	ExactCount:  true,
	Pagination:  adapters.PaginationCursor,
	ForeignKeys: false,
	// mutate.go backs all three: insert (SET ... NX, string-typed only), update (a plain SET,
	// also string-typed only), delete (DEL, type-agnostic).
	CanInsert:    true,
	CanUpdate:    true,
	CanDelete:    true,
	Writable:     true,
	Transactions: false,
	Cancel:       true,
	FileTransfer: false,
}
