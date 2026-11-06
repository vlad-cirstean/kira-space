package kafka

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// caps is caps.ts's kafkaCaps, field for field (P58e E17). Stream-shaped, offsetWindow
// pagination, estimated count (high - low watermark subtraction, summed across partitions), no FK
// navigation, no console (P10's D13 — neither engine has an ad-hoc command surface named in
// scope).
var caps = adapters.Caps{
	Tabular:         false,
	Documents:       false,
	KeyValue:        false,
	Stream:          true,
	KeyBrowser:      false,
	KeyTypes:        false,
	DefaultPageKind: page.PageKindStream,
	SQL:             false,
	// P23 D5: a topic's partitions/config and a consumer group's members/offsets moved here once
	// the tree stopped showing them — this reverses P10's original "no definition" call.
	Definition: true,
	// Describe throws E_UNSUPPORTED (adapter.go) — a stream has no column/PK/FK metadata. Gates
	// the definition view's separate describe() load so it's never issued (P31 D2).
	Describe:      false,
	SchemaColumns: false,
	Projection:    false,
	ServerFilter:  false,
	ExactCount:    false, // high - low summed across partitions over-counts compacted and transactional topics
	Pagination:    adapters.PaginationOffsetWindow,
	ForeignKeys:   false,
	// produce.go's ProduceSync lands canInsert here. A topic's log is immutable, so Kafka never
	// gets canUpdate or canDelete — there is no per-message update or delete in the Kafka API,
	// only retention/compaction at the topic level — these two stay false permanently, not "not
	// yet implemented".
	CanInsert:    true,
	CanUpdate:    false,
	CanDelete:    false,
	Writable:     true,
	Transactions: false,
	// The op's own ctx aborting an in-flight PollRecords/kadm call is fully effective (P58e E3),
	// confirmed promptly by KF-2 — honest despite Cancel() itself always returning false (see
	// adapter.go's comment for why the mechanism and the RPC are different things).
	Cancel:       true,
	FileTransfer: false,
}
