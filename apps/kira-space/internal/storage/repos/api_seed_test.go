package repos_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
)

// seedApiItem inserts a collection and one request item straight into the tables; the repos are
// read-only projections of the api config files, so a repo test seeds rows the way the projection does.
func seedApiItem(t *testing.T, db *sql.DB, protocol string) string {
	t.Helper()
	const now = int64(1767225600000)
	collID, itemID := uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(`INSERT INTO api_collections (id, name, sort_order, created_at, updated_at) VALUES (?, 'Orders', 0, ?, ?)`, collID, now, now); err != nil {
		t.Fatalf("seed collection: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO api_items (id, collection_id, kind, protocol, name, sort_order, created_at, updated_at) VALUES (?, ?, 'request', ?, 'Get order', 0, ?, ?)`, itemID, collID, protocol, now, now); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	return itemID
}
