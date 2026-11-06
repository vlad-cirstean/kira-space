package repos_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/internal/kiratime"
)

// TestMaskRulesCascadeOnConnectionDelete confirms connection_mask_rules(connection_id) really does
// carry ON DELETE CASCADE, so a connection whose file is gone leaves no orphaned rule rows.
func TestMaskRulesCascadeOnConnectionDelete(t *testing.T) {
	db := newRepos(t).DB
	r := &repos.MaskRulesRepo{DB: db}
	connID := uuid.NewString()
	seedConnection(t, db, connID)

	now := kiratime.NowISO()
	if _, err := db.Exec(`INSERT INTO connection_mask_rules (id, connection_id, table_name, column_name, mask_kind, keep_hint, correlate, created_at, updated_at)
		VALUES (?, ?, '*', 'ssn', 'redact', 0, 0, ?, ?)`, uuid.NewString(), connID, now, now); err != nil {
		t.Fatalf("insert rule: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM connections WHERE id = ?`, connID); err != nil {
		t.Fatalf("delete connection: %v", err)
	}

	list, err := r.ListForConnection(connID)
	if err != nil {
		t.Fatalf("ListForConnection after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("ListForConnection after connection delete = %d rows, want 0 (ON DELETE CASCADE did not fire)", len(list))
	}
}
