package clickhouse

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// caps is caps.ts's clickhouseCaps, literally — the app's first tabular SQL adapter with
// CanUpdate/CanDelete both false: a MergeTree PRIMARY KEY is a sparse index, not a uniqueness
// constraint, so a row cannot be addressed unambiguously. Pagination is "offset" for the
// identical structural reason — no unique total order to build a keyset cursor on. ForeignKeys is
// false because ClickHouse has no such concept at all, not merely no catalog for one. Cancel is
// true: KILL QUERY on a second HTTP request the client's own connection pool already has free.
var caps = adapters.Caps{
	Tabular:         true,
	Documents:       false,
	KeyValue:        false,
	Stream:          false,
	KeyBrowser:      false,
	KeyTypes:        false,
	DefaultPageKind: page.PageKindTabular,
	SQL:             true,
	Definition:      true,
	Describe:        true,
	SchemaColumns:   true,
	Projection:      true,
	ServerFilter:    true,
	ExactCount:      true,
	Pagination:      adapters.PaginationOffset,
	ForeignKeys:     false,
	CanInsert:       true,
	CanUpdate:       false,
	CanDelete:       false,
	Writable:        true,
	Transactions:    false,
	Cancel:          true,
	FileTransfer:    false,
}
