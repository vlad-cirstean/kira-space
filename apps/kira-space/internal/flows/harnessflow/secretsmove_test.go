package harnessflow_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// v0.3.1 kept agent credentials in <home>/agent-secrets.db with this schema.
const legacySecretsSchema = `CREATE TABLE secret_values (
  scope      TEXT NOT NULL,
  owner      TEXT NOT NULL,
  name       TEXT NOT NULL,
  value      TEXT NOT NULL,
  updated_ms  INTEGER NOT NULL,
  PRIMARY KEY (scope, owner, name)
) WITHOUT ROWID;
CREATE TABLE schema_version (version INTEGER NOT NULL);
INSERT INTO schema_version (version) VALUES (1);`

func writeLegacySecrets(t *testing.T, home string, values map[string]string) {
	t.Helper()
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sqlitex.Open(filepath.Join(home, "agent-secrets.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(legacySecretsSchema); err != nil {
		t.Fatal(err)
	}
	store := secrets.NewStore(db, secrets.New())
	for name, value := range values {
		if err := store.Put(db, secrets.ScopeAgentCredential, agentcfg.SecretOwner, name, value); err != nil {
			t.Fatal(err)
		}
	}
}

func agentSecretValues(t *testing.T, app *flowharness.App) map[string]string {
	t.Helper()
	vals, unreadable, err := app.W.AgentSecrets.Store.Open(secrets.ScopeAgentCredential, agentcfg.SecretOwner)
	if err != nil || len(unreadable) != 0 {
		t.Fatalf("open agent secrets = %v, %v", unreadable, err)
	}
	return vals
}

func backupDirs(t *testing.T, home string) []string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join(home, "backups", "p557-*", "agent-secrets.db"))
	if err != nil {
		t.Fatal(err)
	}
	return dirs
}

// The v0.3.1 agent-secrets.db moves into kira.db sealed, is parked under backups/, and a second boot
// leaves everything as is.
func TestAgentSecretsFileMovesIntoKiraDb(t *testing.T) {
	app := flowharness.New(t, flowharness.WithSeed(func(a *flowharness.App) {
		writeLegacySecrets(t, a.SpaceHome, map[string]string{"gh-token": "ghp-one", "jira": "jira-two"})
	}))
	want := map[string]string{"gh-token": "ghp-one", "jira": "jira-two"}
	check := func(step string) {
		t.Helper()
		got := agentSecretValues(t, app)
		if len(got) != len(want) || got["gh-token"] != want["gh-token"] || got["jira"] != want["jira"] {
			t.Fatalf("%s: agent secrets hold %d values, want %d", step, len(got), len(want))
		}
		if _, err := os.Stat(filepath.Join(app.SpaceHome, "agent-secrets.db")); !os.IsNotExist(err) {
			t.Fatalf("%s: agent-secrets.db still in the home (%v)", step, err)
		}
		if n := len(backupDirs(t, app.SpaceHome)); n != 1 {
			t.Fatalf("%s: %d parked copies, want 1", step, n)
		}
	}
	check("first boot")
	app.Restart()
	check("second boot")
}

// A kira.db that already holds a different value for a name wins nothing silently: the move refuses
// and leaves the file in place.
func TestAgentSecretsFileMoveRefusesAConflict(t *testing.T) {
	app := flowharness.New(t)
	if err := app.W.AgentSecrets.Put("gh-token", "kept"); err != nil {
		t.Fatal(err)
	}
	writeLegacySecrets(t, app.SpaceHome, map[string]string{"gh-token": "other", "jira": "jira-two"})
	app.Restart()

	if _, err := os.Stat(filepath.Join(app.SpaceHome, "agent-secrets.db")); err != nil {
		t.Fatalf("agent-secrets.db left the home on a conflict: %v", err)
	}
	if n := len(backupDirs(t, app.SpaceHome)); n != 0 {
		t.Fatalf("%d parked copies, want 0", n)
	}
	got := agentSecretValues(t, app)
	if got["gh-token"] != "kept" || len(got) != 1 {
		t.Fatalf("agent secrets hold %d values, gh-token changed = %v", len(got), got["gh-token"] != "kept")
	}
}
