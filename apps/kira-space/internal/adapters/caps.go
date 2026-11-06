package adapters

import "github.com/kirathecat/kira-studio/apps/kira-space/internal/page"

// PaginationStrategy mirrors shared/caps.ts's PaginationStrategy.
type PaginationStrategy string

const (
	PaginationKeyset       PaginationStrategy = "keyset"
	PaginationOffset       PaginationStrategy = "offset"
	PaginationCursor       PaginationStrategy = "cursor"
	PaginationToken        PaginationStrategy = "token"
	PaginationOffsetWindow PaginationStrategy = "offsetWindow"
	PaginationBatch        PaginationStrategy = "batch"
)

// Caps mirrors shared/caps.ts's Caps exactly — field order follows caps.ts's own declaration
// order so the two files diff against each other. MaxPageSize is a pointer with
// omitempty: a plain int without omitempty would emit "maxPageSize": 0 for the ten adapters that
// have none, which capsSchema.parse (positive-int) rejects on the TypeScript side while the two
// coexist.
type Caps struct {
	Tabular         bool          `json:"tabular"`
	Documents       bool          `json:"documents"`
	KeyValue        bool          `json:"keyValue"`
	Stream          bool          `json:"stream"`
	KeyBrowser      bool          `json:"keyBrowser"`
	KeyTypes        bool          `json:"keyTypes"`
	DefaultPageKind page.PageKind `json:"defaultPageKind"`
	SQL             bool          `json:"sql"`
	Definition      bool          `json:"definition"`
	Describe        bool          `json:"describe"`
	// SchemaColumns: the adapter implements SchemaColumns() — P22c D1. true for the five SQL
	// kinds (postgres/mariadb/mysql/sqlite/clickhouse); false for mongo (no field-level schema at
	// all, F11) and every non-SQL kind.
	SchemaColumns bool               `json:"schemaColumns"`
	Projection    bool               `json:"projection"`
	ServerFilter  bool               `json:"serverFilter"`
	ExactCount    bool               `json:"exactCount"`
	Pagination    PaginationStrategy `json:"pagination"`
	ForeignKeys   bool               `json:"foreignKeys"`
	CanInsert     bool               `json:"canInsert"`
	CanUpdate     bool               `json:"canUpdate"`
	CanDelete     bool               `json:"canDelete"`
	Writable      bool               `json:"writable"`
	Transactions  bool               `json:"transactions"`
	Cancel        bool               `json:"cancel"`
	FileTransfer  bool               `json:"fileTransfer"`
	MaxPageSize   *int               `json:"maxPageSize,omitempty"`
}

// RelationalCaps is the four relational engines' own caps value, byte-identical across
// mariadb/mysql/postgres/sqlite (it stays declared per engine rather than shared, so a
// future real divergence has somewhere honest to be said; the four are still equal,
// so each package's own caps var is this value, unmodified).
var RelationalCaps = Caps{
	Tabular: true, Documents: false, KeyValue: false, Stream: false,
	KeyBrowser: false, KeyTypes: false, DefaultPageKind: page.PageKindTabular,
	SQL: true, Definition: true, Describe: true, SchemaColumns: true,
	Projection: true, ServerFilter: true, ExactCount: true, Pagination: PaginationKeyset,
	ForeignKeys: true, CanInsert: true, CanUpdate: true, CanDelete: true,
	Writable: true, Transactions: true, Cancel: true, FileTransfer: false,
}
