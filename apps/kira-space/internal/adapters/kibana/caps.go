package kibana

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// caps: a read-only document kind. SQL is true because the Discover editor is the console, which
// also gives Re-run and DB MCP run_query; the frontend gates SQL-only UI by kind.
var caps = adapters.Caps{
	Documents:       true,
	DefaultPageKind: page.PageKindDocument,
	SQL:             true,
	ServerFilter:    true,
	ExactCount:      true,
	Pagination:      adapters.PaginationOffset,
	Cancel:          true,
	MaxPageSize:     intp(maxResultWindow),
}

func intp(n int) *int { return &n }
