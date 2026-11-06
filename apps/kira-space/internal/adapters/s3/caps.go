package s3

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// caps is caps.ts's s3Caps, literally (P58d D18). Reuses the keyvalue shape — a single object's
// metadata+body is exactly a flat field/value listing, same as a redis hash.
var caps = adapters.Caps{
	Tabular:    false,
	Documents:  false,
	KeyValue:   true,
	Stream:     false,
	KeyBrowser: true, // a bucket's prefix/object space is unbounded — browsed in a Browse tab
	// An S3 object has no per-item "type" the way a redis key's TYPE command reports one — every
	// object in the list is just "a file". P63 §4.1's own out-of-scope note: object size in the
	// list would be free (ListObjectsV2 already returns it) but SPEC scopes the type-badge work to
	// Redis.
	KeyTypes:        false,
	DefaultPageKind: page.PageKindKeyValue,
	SQL:             false,
	// Stays false for now, as a named follow-up rather than a permanent no — an *object* already
	// shows its full metadata in the keyvalue view it opens into, so only a *bucket* has anything
	// new, and a bucket's properties are five separate SDK calls each of which a single-bucket IAM
	// policy routinely denies.
	Definition: false,
	// Describe throws E_UNSUPPORTED (adapter.go). Definition is already false above, so this is a
	// coincidence of two unrelated flags, not something Describe:false relies on.
	Describe:      false,
	SchemaColumns: false,
	Projection:    false,
	ServerFilter:  false,
	// countObject (read.go) answers a single object's own field count via HeadObject, which is
	// always exact — the same per-item-exact resolution redis/caps.go makes for its own per-key
	// counts, not the bucket-wide "how many keys total" question ListObjectsV2 would need to
	// answer approximately.
	ExactCount:  true,
	Pagination:  adapters.PaginationToken, // ListObjectsV2's own ContinuationToken
	ForeignKeys: false,
	// An object's body is replaced wholesale via PutObject (CanUpdate), a new object is created
	// via a local-file upload (CanInsert), and DeleteObject removes one (CanDelete) — see
	// mutate.go. No per-object update in place; PutObject is always a full overwrite.
	CanInsert:    true,
	CanUpdate:    true,
	CanDelete:    true,
	Writable:     true,
	Transactions: false,
	Cancel:       true, // the op's own ctx aborting the in-flight SDK call is fully effective (P58d D3)
	// Gates Download outright and Upload together with CanInsert — the only engine whose items are
	// files with an OS dialog to move them through.
	FileTransfer: true,
}
