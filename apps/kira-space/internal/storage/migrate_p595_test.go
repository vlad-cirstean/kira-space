package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/migrations"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// 0096 pins "deny" for installs that had rules, so requests no rule matches stay refused. An install
// without rules, or with an unreadable stored value, must not silently become ask.
func TestMigration0096APIDefault(t *testing.T) {
	cases := []struct {
		name  string
		seed  map[string]string
		wants string
	}{
		{"no rules leaf", nil, "ask"},
		{"empty list", map[string]string{"dbMcp.apiRules": `[]`}, "ask"},
		{"null", map[string]string{"dbMcp.apiRules": `null`}, "ask"},
		{"one rule", map[string]string{"dbMcp.apiRules": `[{"effect":"allow","hosts":["a.test"],"methods":["GET"]}]`}, "deny"},
		{"unparsable rules", map[string]string{"dbMcp.apiRules": `{"x":1}`}, "deny"},
		{"existing default kept", map[string]string{
			"dbMcp.apiRules":   `[{"effect":"deny","hosts":["*"],"methods":["DELETE"]}]`,
			"dbMcp.apiDefault": `"ask"`,
		}, "ask"},
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
			if err := sqlitex.Migrate(db, all[:len(all)-1]); err != nil {
				t.Fatal(err)
			}
			for k, v := range c.seed {
				if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`, k, v); err != nil {
					t.Fatal(err)
				}
			}
			if err := sqlitex.Migrate(db, all); err != nil {
				t.Fatal(err)
			}
			got, err := (&repos.SettingsRepo{DB: db}).GetAll()
			if err != nil {
				t.Fatal(err)
			}
			if got.DbMcp.APIDefault != c.wants {
				t.Errorf("apiDefault = %q, want %q", got.DbMcp.APIDefault, c.wants)
			}
		})
	}
}
