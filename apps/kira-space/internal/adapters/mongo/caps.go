package mongo

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// caps is caps.ts's mongoCaps, literally (§4.2, C18). Document-shaped, cursor pagination,
// estimate-only count, no FK navigation, a shell-style console. Definition is true (P19 D12) —
// a collection's creation options + validator, via definition.go.
var caps = adapters.Caps{
	Tabular:         false,
	Documents:       true,
	KeyValue:        false,
	Stream:          false,
	KeyBrowser:      false,
	KeyTypes:        false,
	DefaultPageKind: page.PageKindDocument,
	SQL:             true,
	Definition:      true,
	Describe:        true,
	SchemaColumns:   false,
	Projection:      true,
	ServerFilter:    true,
	ExactCount:      false,
	Pagination:      adapters.PaginationCursor,
	ForeignKeys:     false,
	// mutate.go implements insert (InsertOne), update (ReplaceOne) and delete (DeleteOne).
	CanInsert:    true,
	CanUpdate:    true,
	CanDelete:    true,
	Writable:     true,
	Transactions: false,
	Cancel:       true,
	FileTransfer: false,
}
