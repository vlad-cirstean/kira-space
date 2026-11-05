package gitreview

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview/migrations"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// Guards 0004: a comment stored before the kind/state columns reads back as an open change request.
func TestMigration0004DefaultsOldComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.db")
	db, err := sqlitex.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.Migrate(db, all[:3]); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if _, err := db.Exec(`INSERT INTO review_session (repo_id, branch, created_at, last_used_at) VALUES ('r', 'b', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO review_comment (session_id, path, start_line, end_line, body, anchor_sha, anchor_blob_oid, created_at)
		VALUES (1, 'a.txt', 1, 1, 'old', 'sha', 'oid', 5)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s := NewStore(path)
	defer s.Close()
	got, err := s.Comments(context.Background(), "r", "b")
	if err != nil || len(got) != 1 {
		t.Fatalf("Comments = %v, %v", got, err)
	}
	if c := got[0]; c.Kind != CommentKindChange || c.State != CommentStateOpen || c.Reply != "" || c.RepliedAt != nil || c.SentAt != nil {
		t.Fatalf("old comment = %+v, want open change with no reply", c)
	}
}

// Guards 0007: integer comment ids become UUIDv7 strings that still sort in creation order.
func TestMigration0007ConvertsCommentIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.db")
	db, err := sqlitex.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.Migrate(db, all[:6]); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if _, err := db.Exec(`INSERT INTO review_session (repo_id, branch, created_at, last_used_at) VALUES ('r', 'b', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	for _, at := range []int64{1_700_000_000_000, 1_700_000_000_000, 1_700_000_005_000} {
		if _, err := db.Exec(`INSERT INTO review_comment (session_id, path, start_line, end_line, body, anchor_sha, anchor_blob_oid, created_at)
			VALUES (1, 'a.txt', 1, 1, 'old', 'sha', 'oid', ?)`, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s := NewStore(path)
	defer s.Close()
	got, err := s.Comments(context.Background(), "r", "b")
	if err != nil || len(got) != 3 {
		t.Fatalf("Comments = %v, %v", got, err)
	}
	seen := map[string]bool{}
	for _, c := range got {
		if !ids.IsUUID(c.ID) || c.ID[14] != '7' || seen[c.ID] {
			t.Fatalf("id %q is not a distinct UUIDv7", c.ID)
		}
		seen[c.ID] = true
	}
	if got[2].ID[:13] <= got[0].ID[:13] {
		t.Fatalf("ids %q, %q do not follow created_at", got[0].ID, got[2].ID)
	}
}
