package repos_test

// P29 §4.2: the cross-kind property proved against the app's own tables, not just against the
// cipher in isolation. Both tests use newVariablesRepo's harness shape (variables_test.go:22-28) —
// a genuine round trip under KIRA_INSECURE_SECRETS=1 and a real secrets.New().

import (
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"testing"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// TestAHistorySecretRoundTripsUnderItsOwnScope is the regression guard: it fails outright the
// moment recordHistory goes back to copying the live api_variables ciphertext verbatim instead of
// re-sealing it under ScopeVariableHistory.
func TestAHistorySecretRoundTripsUnderItsOwnScope(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)

	created, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "apiKey", strPtr("old-secret"), true, "")
	if err != nil {
		t.Fatalf("Upsert(create): %v", err)
	}
	if _, err := r.Upsert(model.VariableScopeCollection, collectionID, created.ID, "apiKey", strPtr("new-secret"), true, ""); err != nil {
		t.Fatalf("Upsert(change): %v", err)
	}

	hist, err := r.History(created.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("History = %+v, want exactly one entry", hist)
	}

	revealed, err := r.RevealHistoryValue(hist[0].ID)
	if err != nil {
		t.Fatalf("RevealHistoryValue: %v", err)
	}
	if revealed != "old-secret" {
		t.Fatalf("RevealHistoryValue = %q, want %q (the value the edit replaced)", revealed, "old-secret")
	}
}

// TestACiphertextMovedBetweenColumnsIsRefused is the attack, executed against the
// app's real tables in both directions that matter: a connection password can no longer be read
// through the lower-friction variables gate, and a live variable secret can no longer be laundered
// into the history table and revealed there.
func TestACiphertextMovedBetweenColumnsIsRefused(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)

	t.Run("connection password into a variable row", func(t *testing.T) {
		connID := uuid.NewString()
		seedConnection(t, db, connID)

		secretsRepo := repos.NewSecrets(db, secrets.New())
		password := "db-password"
		if err := secretsRepo.Set(connID, &password); err != nil {
			t.Fatalf("SecretsRepo.Set: %v", err)
		}

		var connCiphertext string
		if err := db.QueryRow(`SELECT password FROM connections WHERE id = ?`, connID).Scan(&connCiphertext); err != nil {
			t.Fatalf("read connection ciphertext: %v", err)
		}

		v, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "stolen", strPtr("placeholder"), true, "")
		if err != nil {
			t.Fatalf("Upsert(placeholder secret): %v", err)
		}
		if _, err := db.Exec(`UPDATE api_variables SET secret_value = ? WHERE id = ?`, connCiphertext, v.ID); err != nil {
			t.Fatalf("plant connection ciphertext into api_variables: %v", err)
		}

		if _, err := r.RevealValue(v.ID); err == nil {
			t.Fatal("RevealValue succeeded on a connection ciphertext planted in api_variables.secret_value — a connection credential must not be revealable through the variables gate")
		}
	})

	t.Run("live variable secret into a history row", func(t *testing.T) {
		v, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "liveSecret", strPtr("current-value"), true, "")
		if err != nil {
			t.Fatalf("Upsert(live secret): %v", err)
		}
		var liveCiphertext string
		if err := db.QueryRow(`SELECT secret_value FROM api_variables WHERE id = ?`, v.ID).Scan(&liveCiphertext); err != nil {
			t.Fatalf("read live ciphertext: %v", err)
		}

		// Seed a history row directly and plant the live ciphertext into it — recordHistory would
		// never produce this row (it re-encrypts under ScopeVariableHistory), so the raw INSERT is
		// what stands in for "an attacker who can write kira.db but not read the
		// Keychain moves a live variable's ciphertext into the history table by hand."
		historyID := uuid.NewString()
		if _, err := db.Exec(
			`INSERT INTO api_variable_history (id, variable_id, value, is_secret, secret_value, recorded_at)
			 VALUES (?, ?, '', 1, ?, ?)`,
			historyID, v.ID, liveCiphertext, kiratime.NowISO(),
		); err != nil {
			t.Fatalf("seed history row with planted live ciphertext: %v", err)
		}

		if _, err := r.RevealHistoryValue(historyID); err == nil {
			t.Fatal("RevealHistoryValue succeeded on a live-variable ciphertext planted in api_variable_history.secret_value — a live secret must not be revealable through the history gate")
		}
	})
}
