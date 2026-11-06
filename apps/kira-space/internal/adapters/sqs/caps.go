package sqs

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// caps is caps.ts's sqsCaps, literally (P58d D18). Stream-shaped, batch pagination (poll-on-
// demand, no addressable position), approximate count only, no console (P10's D13).
var caps = adapters.Caps{
	Tabular:         false,
	Documents:       false,
	KeyValue:        false,
	Stream:          true,
	KeyBrowser:      false,
	KeyTypes:        false,
	DefaultPageKind: page.PageKindStream,
	SQL:             false,
	// P23 D9: a queue's attributes — visibility timeout, retention, redrive policy, FIFO/dedup,
	// KMS key, ARN — reverses P10's original "no definition" call. One GetQueueAttributes call,
	// no automatic message read.
	Definition: true,
	// Describe throws E_UNSUPPORTED (adapter.go) — a queue has no column/PK/FK metadata.
	Describe:      false,
	SchemaColumns: false,
	Projection:    false,
	ServerFilter:  false,
	ExactCount:    false, // ApproximateNumberOfMessages only
	Pagination:    adapters.PaginationBatch,
	ForeignKeys:   false,
	// mutate.go's SendMessage/DeleteMessage land both CanInsert and CanDelete here. Unlike Kafka,
	// SQS's DeleteMessage is a real per-item operation (removes it from the queue via its receipt
	// handle, kept adapter-local — see mutate.go's own comment). There is still no CanUpdate — a
	// delivered message can't be edited in place, only replaced by delete+resend.
	CanInsert:    true,
	CanUpdate:    false,
	CanDelete:    true,
	Writable:     true,
	Transactions: false,
	Cancel:       true, // the op's own ctx aborting the in-flight SDK call is fully effective (P58d D3)
	FileTransfer: false,
}
