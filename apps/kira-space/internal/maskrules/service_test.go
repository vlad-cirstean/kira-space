package maskrules

import (
	"testing"

	"github.com/kirathecat/kira-studio/internal/kiratime"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

// TestMaskSetForDoesNotCacheSetStaleAfterConcurrentWrite guards P168 Part 6 F10: a rule written
// (and its cache invalidated) between MaskSetFor's query and its store must not leave the
// pre-write set cached.
func TestMaskSetForDoesNotCacheSetStaleAfterConcurrentWrite(t *testing.T) {
	t.Setenv("KIRA_SPACE_HOME", t.TempDir())
	db, err := storage.Open()
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r, err := repos.New(db.DB)
	if err != nil {
		t.Fatalf("repos.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	now := kiratime.NowISO()
	if _, err := db.DB.Exec(
		`INSERT INTO connections (id, name, kind, color, mode, read_only, created_at, updated_at, sort_order)
		 VALUES ('c1', 'c1', 'postgres', 'blue', 'fields', 0, ?, ?, 0)`, now, now,
	); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	svc := New(r.MaskRules, nil, nil)
	svc.afterRead = func() {
		svc.afterRead = nil
		// A file load projects the rule row, then invalidates the cache.
		if _, err := db.DB.Exec(
			`INSERT INTO connection_mask_rules (id, connection_id, table_name, column_name, mask_kind, keep_hint, correlate, created_at, updated_at)
			 VALUES ('r1', 'c1', '*', 'email', 'redact', 0, 0, ?, ?)`, now, now,
		); err != nil {
			t.Errorf("insert rule: %v", err)
		}
		svc.Invalidate("c1")
	}
	if _, err := svc.MaskSetFor("c1"); err != nil {
		t.Fatalf("MaskSetFor: %v", err)
	}
	set, err := svc.MaskSetFor("c1")
	if err != nil {
		t.Fatalf("MaskSetFor: %v", err)
	}
	if _, ok := set.Rules["email"]; !ok {
		t.Fatal("second MaskSetFor served the stale pre-write set")
	}
}
