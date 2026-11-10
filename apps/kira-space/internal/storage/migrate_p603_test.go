package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/migrations"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// 0097 folds four per-operation modes into a switch plus one approval mode. It must never widen
// access: on only if some mode was prompt or allow, allow only if every mode was allow, any other
// word counts as deny.
func TestMigration0097KubeMcpAccess(t *testing.T) {
	cases := []struct {
		name         string
		modes        [4]string // discover, list, describe, logs
		wantEnabled  bool
		wantApproval string
	}{
		{"all prompt", [4]string{"prompt", "prompt", "prompt", "prompt"}, true, "prompt"},
		{"all deny", [4]string{"deny", "deny", "deny", "deny"}, false, "prompt"},
		{"all allow", [4]string{"allow", "allow", "allow", "allow"}, true, "allow"},
		{"allow plus deny", [4]string{"allow", "allow", "allow", "deny"}, true, "prompt"},
		{"allow plus prompt", [4]string{"allow", "prompt", "allow", "allow"}, true, "prompt"},
		{"prompt plus deny", [4]string{"deny", "prompt", "deny", "deny"}, true, "prompt"},
		{"unknown plus deny", [4]string{"bogus", "deny", "deny", "deny"}, false, "prompt"},
		{"unknown plus allow", [4]string{"allow", "allow", "allow", "bogus"}, true, "prompt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, err := sqlitex.Open(filepath.Join(t.TempDir(), "kira.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			all, err := migrations.All()
			if err != nil {
				t.Fatal(err)
			}
			if err := sqlitex.Migrate(db, migrationsBefore(all, 97)); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO kube_clusters
				(id, sort_order, created_at, last_seen_at, context, server, mcp_description,
				 mcp_discover_mode, mcp_list_mode, mcp_describe_mode, mcp_logs_mode)
				VALUES ('k1', 3, 1, 1, 'ctx', 'https://k.test', 'my cluster', ?, ?, ?, ?)`,
				c.modes[0], c.modes[1], c.modes[2], c.modes[3]); err != nil {
				t.Fatal(err)
			}
			if err := sqlitex.Migrate(db, all); err != nil {
				t.Fatal(err)
			}
			rec, ok, err := (&repos.KubeClustersRepo{DB: db}).Get("k1")
			if err != nil || !ok {
				t.Fatalf("Get = %v, %v", ok, err)
			}
			if rec.Mcp.Enabled != c.wantEnabled || rec.Mcp.Approval != c.wantApproval {
				t.Errorf("mcp = %+v, want enabled %v approval %q", rec.Mcp, c.wantEnabled, c.wantApproval)
			}
			if rec.Mcp.Description != "my cluster" || rec.SortOrder == nil || *rec.SortOrder != 3 {
				t.Errorf("description %q, sort order %v: want them kept", rec.Mcp.Description, rec.SortOrder)
			}
		})
	}
}

// migrationsBefore keeps the schema at the state just before a migration, whatever lands after it.
func migrationsBefore(all []sqlitex.Migration, version int) []sqlitex.Migration {
	for i, m := range all {
		if m.Version >= version {
			return all[:i]
		}
	}
	return all
}
