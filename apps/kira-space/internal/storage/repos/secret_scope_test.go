package repos_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// P29 §4.2 against the app's own tables: a history entry only opens under its own cipher scope, so a
// ciphertext sealed for live variables, moved by hand into api_variable_history, is refused. Legacy
// history rows still carry sealed values, so the gate stays.
func TestAHistorySecretOpensOnlyUnderItsOwnScope(t *testing.T) {
	t.Setenv("KIRA_INSECURE_SECRETS", "1")
	db := newRepos(t).DB
	cipher := secrets.New()
	r := repos.NewVariables(db, cipher)

	const now = "2026-01-01T00:00:00Z"
	collID, varID := uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(`INSERT INTO api_collections (id, name, sort_order, created_at, updated_at) VALUES (?, 'Orders', 0, ?, ?)`, collID, now, now); err != nil {
		t.Fatalf("seed collection: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO api_variables (id, collection_id, name, value, is_secret, secret_value, sort_order, created_at, updated_at) VALUES (?, ?, 'k', '', 1, NULL, 0, ?, ?)`, varID, collID, now, now); err != nil {
		t.Fatalf("seed variable: %v", err)
	}
	insert := func(scope secrets.Scope, plain string) string {
		t.Helper()
		sealed, err := cipher.Encrypt(scope, plain)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		id := uuid.NewString()
		if _, err := db.Exec(
			`INSERT INTO api_variable_history (id, variable_id, value, is_secret, secret_value, recorded_at) VALUES (?, ?, '', 1, ?, ?)`,
			id, varID, sealed, kiratime.NowISO(),
		); err != nil {
			t.Fatalf("seed history row: %v", err)
		}
		return id
	}

	if got, err := r.RevealHistoryValue(insert(secrets.ScopeVariableHistory, "old-secret")); err != nil || got != "old-secret" {
		t.Fatalf("RevealHistoryValue(own scope) = %q, %v, want old-secret", got, err)
	}
	if _, err := r.RevealHistoryValue(insert(secrets.ScopeVariable, "live-secret")); err == nil {
		t.Fatal("RevealHistoryValue succeeded on a live-variable ciphertext planted in api_variable_history.secret_value")
	}
}
