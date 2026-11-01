package repos_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/postman"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// Four cases, each guarding arithmetic or an invariant rather than a CRUD round-trip —
// that CreateEnvironment then ListEnvironments returns it, that RenameEnvironment renames, that a
// missing name is refused, that the cipher round-trips a string: each is CLAUDE.md's "everything
// else gets nothing".

// newVariablesRepo mirrors newCollectionsRepo's shape: a real migrated database, plus the real
// secrets.Cipher under the Linux KIRA_INSECURE_SECRETS fallback (connections/service_test.go's own
// precedent) — a genuine encrypt/decrypt round trip, not a fake.
func newVariablesRepo(t *testing.T) (*repos.VariablesRepo, *sql.DB) {
	t.Helper()
	t.Setenv("KIRA_INSECURE_SECRETS", "1")
	db := newRepos(t).DB
	cipher := secrets.New()
	return repos.NewVariables(db, cipher), db
}

func newCollectionFor(t *testing.T, db *sql.DB) string {
	t.Helper()
	c, err := (&repos.CollectionsRepo{DB: db}).CreateCollection("Orders")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	return c.ID
}

// ---- 1. sort_order is dense and stable, per scope, independently ----

func TestVariablesSortOrderIsDenseAndScopeIndependent(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)
	env, err := r.CreateEnvironment("Staging", "", "none")
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}

	var collectionIDs, envIDs []string
	for _, name := range []string{"a", "b", "c"} {
		v, err := r.Upsert(model.VariableScopeCollection, collectionID, "", name, strPtr(name), false, "")
		if err != nil {
			t.Fatalf("Upsert(collection, %s): %v", name, err)
		}
		collectionIDs = append(collectionIDs, v.ID)
		w, err := r.Upsert(model.VariableScopeEnvironment, env.ID, "", name, strPtr(name), false, "")
		if err != nil {
			t.Fatalf("Upsert(environment, %s): %v", name, err)
		}
		envIDs = append(envIDs, w.ID)
	}

	// Delete the middle collection variable — its siblings must re-index dense, and the
	// environment's own list must be completely unaffected (a collection's reorder must not
	// renumber an environment's, D4's own comment).
	if _, _, err := r.Delete(collectionIDs[1]); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	remaining, err := r.List(model.VariableScopeCollection, collectionID)
	if err != nil {
		t.Fatalf("List(collection): %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("len(remaining) = %d, want 2", len(remaining))
	}
	for i, v := range remaining {
		if v.SortOrder != i {
			t.Errorf("remaining[%d].SortOrder = %d, want %d (dense)", i, v.SortOrder, i)
		}
	}

	envVars, err := r.List(model.VariableScopeEnvironment, env.ID)
	if err != nil {
		t.Fatalf("List(environment): %v", err)
	}
	if len(envVars) != 3 {
		t.Fatalf("len(envVars) = %d, want 3 (unaffected by the collection's delete)", len(envVars))
	}
	for i, v := range envVars {
		if v.SortOrder != i {
			t.Errorf("envVars[%d].SortOrder = %d, want %d", i, v.SortOrder, i)
		}
	}

	// Reorder the environment's own three (independent of the collection again).
	reordered := []string{envIDs[2], envIDs[0], envIDs[1]}
	if err := r.Reorder(model.VariableScopeEnvironment, env.ID, reordered); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	after, err := r.List(model.VariableScopeEnvironment, env.ID)
	if err != nil {
		t.Fatalf("List(environment) after reorder: %v", err)
	}
	for i, id := range reordered {
		if after[i].ID != id || after[i].SortOrder != i {
			t.Errorf("after[%d] = %+v, want id %s at sortOrder %d", i, after[i], id, i)
		}
	}
}

// ---- 2. history: recorded on change only, deduped, trimmed at 20 ----

func TestVariableHistoryRecordsOnChangeDedupedAndTrimmed(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)

	v, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "baseUrl", strPtr("v1"), false, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Writing the same value twice records nothing: no prior value was actually replaced.
	if _, err := r.Upsert(model.VariableScopeCollection, collectionID, v.ID, "baseUrl", strPtr("v1"), false, ""); err != nil {
		t.Fatalf("no-op upsert: %v", err)
	}
	hist, err := r.History(v.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 0 {
		t.Fatalf("len(hist) after an unchanged write = %d, want 0", len(hist))
	}

	// A real change records the value it replaced.
	if _, err := r.Upsert(model.VariableScopeCollection, collectionID, v.ID, "baseUrl", strPtr("v2"), false, ""); err != nil {
		t.Fatalf("upsert v2: %v", err)
	}
	hist, err = r.History(v.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 1 || hist[0].Value != "v1" {
		t.Fatalf("hist = %+v, want one entry carrying v1", hist)
	}

	// Restoring writes through the ordinary path, so it is itself recorded — the value being
	// replaced (v2) becomes the newest history entry.
	if _, err := r.Upsert(model.VariableScopeCollection, collectionID, v.ID, "baseUrl", strPtr(hist[0].Value), false, ""); err != nil {
		t.Fatalf("restore: %v", err)
	}
	hist, err = r.History(v.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 2 || hist[0].Value != "v2" {
		t.Fatalf("hist after restore = %+v, want newest v2", hist)
	}

	// Trimmed at 20, oldest dropped first.
	for i := 0; i < 25; i++ {
		if _, err := r.Upsert(model.VariableScopeCollection, collectionID, v.ID, "baseUrl", strPtr("gen"+string(rune('a'+i))), false, ""); err != nil {
			t.Fatalf("upsert gen%d: %v", i, err)
		}
	}
	hist, err = r.History(v.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 20 {
		t.Fatalf("len(hist) after 25 more changes = %d, want 20 (trimmed)", len(hist))
	}
	// hist[0] is the value *replaced* by the very last write (i=24, "geny"), i.e. i=23's "genx" —
	// history records what a change overwrote, not the change itself.
	if want := "gen" + string(rune('a'+23)); hist[0].Value != want {
		t.Errorf("hist[0].Value = %q, want %q", hist[0].Value, want)
	}
}

// ---- 2b. flipping a variable from plain to secret purges its pre-secret plaintext history ----

// TestUpsertToSecretPurgesPlaintextHistory: a variable that was edited a few times while still
// plain, then flipped to Secret, must not leave its earlier plaintext values sitting in
// api_variable_history — that table has no reveal gate for a row whose own is_secret is 0, so
// those rows are exactly as exposed as a plain variable's current value, backups and all, once the
// variable itself is meant to be protected.
func TestUpsertToSecretPurgesPlaintextHistory(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)

	v, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "token", strPtr("sk-live-old"), false, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// A plain edit records "sk-live-old" in history.
	if _, err := r.Upsert(model.VariableScopeCollection, collectionID, v.ID, "token", strPtr("sk-live-mid"), false, ""); err != nil {
		t.Fatalf("plain edit: %v", err)
	}
	hist, err := r.History(v.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 1 || hist[0].Value != "sk-live-old" || hist[0].IsSecret {
		t.Fatalf("hist before flip = %+v, want one plain entry carrying sk-live-old", hist)
	}

	// Flip to secret in the same edit that also changes the value: recordHistory would otherwise
	// stamp *this* transition's row is_secret=0 too (the flag is read from the old row), leaving the
	// value it replaced ("sk-live-mid") in cleartext right alongside the earlier one.
	if _, err := r.Upsert(model.VariableScopeCollection, collectionID, v.ID, "token", strPtr("sk-live-new"), true, ""); err != nil {
		t.Fatalf("flip to secret: %v", err)
	}

	hist, err = r.History(v.ID)
	if err != nil {
		t.Fatalf("History after flip: %v", err)
	}
	if len(hist) != 0 {
		t.Fatalf("len(hist) after flipping to secret = %d, want 0 (every pre-secret plaintext row purged); got %+v", len(hist), hist)
	}

	// A subsequent secret-to-secret edit records history the normal, encrypted way and is
	// unaffected by the purge.
	if _, err := r.Upsert(model.VariableScopeCollection, collectionID, v.ID, "token", strPtr("sk-live-newer"), true, ""); err != nil {
		t.Fatalf("secret edit: %v", err)
	}
	hist, err = r.History(v.ID)
	if err != nil {
		t.Fatalf("History after secret edit: %v", err)
	}
	if len(hist) != 1 || !hist[0].IsSecret || hist[0].Value != "" {
		t.Fatalf("hist after secret edit = %+v, want one masked secret entry", hist)
	}
}

// TestUpsertWithNilValueLeavesASecretUntouched: List's own list projection
// always returns "" for a secret, so the frontend seeds an unrevealed secret's draft from
// that blank value — a nil value pointer must be Upsert's own "leave the stored value untouched"
// signal for exactly this case, or renaming/describing a secret without ever revealing it silently
// wipes it (the old value survives only in history; the next request sends an empty credential).
func TestUpsertWithNilValueLeavesASecretUntouched(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)

	created, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "apiKey", strPtr("s3cr3t"), true, "old desc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Rename and re-describe without ever revealing — exactly what an onBlur commit sends for an
	// unrevealed secret draft once the frontend only sends a real value when one was actually typed.
	updated, err := r.Upsert(model.VariableScopeCollection, collectionID, created.ID, "apiKeyRenamed", nil, true, "new desc")
	if err != nil {
		t.Fatalf("update with nil value: %v", err)
	}
	if updated.Value != "" {
		t.Fatalf("returned Value = %q, want '' for a secret", updated.Value)
	}
	if updated.Name != "apiKeyRenamed" || updated.Description != "new desc" {
		t.Fatalf("updated = %+v, want the name/description to have actually changed", updated)
	}

	plain, err := r.RevealValue(created.ID)
	if err != nil {
		t.Fatalf("RevealValue: %v", err)
	}
	if plain != "s3cr3t" {
		t.Fatalf("RevealValue after a nil-value update = %q, want the original s3cr3t (unwiped)", plain)
	}

	// No history recorded — nothing about the value changed.
	hist, err := r.History(created.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 0 {
		t.Fatalf("History = %+v, want none (a nil-value update touches no value)", hist)
	}

	// An explicit, real edit afterward still works normally.
	if _, err := r.Upsert(model.VariableScopeCollection, collectionID, created.ID, "apiKeyRenamed", strPtr("s3cr3t-2"), true, "new desc"); err != nil {
		t.Fatalf("real edit after a nil-value update: %v", err)
	}
	plain, err = r.RevealValue(created.ID)
	if err != nil {
		t.Fatalf("RevealValue after real edit: %v", err)
	}
	if plain != "s3cr3t-2" {
		t.Fatalf("RevealValue after real edit = %q, want s3cr3t-2", plain)
	}
}

// ---- 3. List never returns a secret's plaintext or ciphertext ----

func TestVariablesListNeverReturnsASecret(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)

	created, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "apiKey", strPtr("s3cr3t"), true, "")
	if err != nil {
		t.Fatalf("Upsert(secret): %v", err)
	}
	if created.Value != "" {
		t.Fatalf("Upsert's own returned Value = %q, want '' for a secret", created.Value)
	}

	rows, err := r.List(model.VariableScopeCollection, collectionID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].Value != "" || !rows[0].IsSecret {
		t.Fatalf("List row = %+v, want value '' and isSecret true", rows)
	}

	plain, err := r.RevealValue(created.ID)
	if err != nil {
		t.Fatalf("RevealValue: %v", err)
	}
	if plain != "s3cr3t" {
		t.Errorf("RevealValue = %q, want s3cr3t", plain)
	}
}

// ---- 4. PromoteImported is one-shot and idempotent ----

func TestPromoteImportedIsOneShotAndIdempotent(t *testing.T) {
	r, db := newVariablesRepo(t)

	// A pre-P5 collection row: created directly, carrying a top-level variable[] inside
	// origin_json exactly as a P4-era import would have left it.
	now := kiratime.NowISO()
	collectionID := "collection-pre-p5"
	if _, err := db.Exec(
		`INSERT INTO api_collections (id, name, sort_order, origin_json, created_at, updated_at)
		 VALUES (?, 'Legacy', 0, ?, ?, ?)`,
		collectionID,
		`{"variable":[{"key":"baseUrl","value":"https://api.example.com"},{"key":"apiKey","value":"","type":"secret"}]}`,
		now, now,
	); err != nil {
		t.Fatalf("seed pre-P5 collection: %v", err)
	}

	rows, err := r.List(model.VariableScopeCollection, collectionID)
	if err != nil {
		t.Fatalf("List (triggers promotion): %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) after promotion = %d, want 2", len(rows))
	}

	var promoted int
	if err := db.QueryRow(`SELECT variables_promoted FROM api_collections WHERE id = ?`, collectionID).Scan(&promoted); err != nil {
		t.Fatalf("read variables_promoted: %v", err)
	}
	if promoted != 1 {
		t.Fatalf("variables_promoted = %d, want 1", promoted)
	}
	var originJSON string
	if err := db.QueryRow(`SELECT origin_json FROM api_collections WHERE id = ?`, collectionID).Scan(&originJSON); err != nil {
		t.Fatalf("read origin_json: %v", err)
	}
	if originJSON != `{}` {
		t.Errorf("origin_json = %q, want the variable member shed to '{}'", originJSON)
	}

	// A second call — via PromoteImported directly this time — changes nothing.
	if err := r.PromoteImported(collectionID); err != nil {
		t.Fatalf("PromoteImported (second call): %v", err)
	}
	rowsAgain, err := r.List(model.VariableScopeCollection, collectionID)
	if err != nil {
		t.Fatalf("List (second call): %v", err)
	}
	if len(rowsAgain) != 2 {
		t.Fatalf("len(rowsAgain) = %d, want still 2 (idempotent, no duplicate rows)", len(rowsAgain))
	}
}

// ---- 5. import → ImportVariables → LoadTree → export, end to end, secrets valueless (D15/D16) ----

func TestImportVariablesThenExportRoundTripsWithSecretsValueless(t *testing.T) {
	r, db := newVariablesRepo(t)
	cr := &repos.CollectionsRepo{DB: db}

	c, err := cr.CreateCollection("With variables")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	vars := []postman.Variable{
		{Name: "baseUrl", Value: "https://api.example.com"},
		{Name: "apiKey", Value: "s3cr3t", Secret: true, Type: "secret"},
	}
	if err := r.ImportVariables(c.ID, vars); err != nil {
		t.Fatalf("ImportVariables: %v", err)
	}

	// The rows exist with the right names, order and secret flags, and the list projection never
	// carries the secret's plaintext (D5).
	rows, err := r.List(model.VariableScopeCollection, c.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].Name != "baseUrl" || rows[0].Value != "https://api.example.com" || rows[0].IsSecret {
		t.Errorf("rows[0] = %+v", rows[0])
	}
	if rows[1].Name != "apiKey" || rows[1].Value != "" || !rows[1].IsSecret {
		t.Errorf("rows[1] = %+v", rows[1])
	}
	plain, err := r.RevealValue(rows[1].ID)
	if err != nil {
		t.Fatalf("RevealValue: %v", err)
	}
	if plain != "s3cr3t" {
		t.Errorf("RevealValue = %q, want s3cr3t", plain)
	}

	// The collection is stamped promoted — PromoteImported (List's own lazy trigger) must not
	// duplicate these rows.
	rows2, err := r.List(model.VariableScopeCollection, c.ID)
	if err != nil {
		t.Fatalf("List (again): %v", err)
	}
	if len(rows2) != 2 {
		t.Fatalf("len(rows2) = %d, want still 2", len(rows2))
	}

	// Export re-emits `variable` from the rows, in order, with the secret's value empty and its
	// type "secret" — D16, exercised through the real LoadTree -> postman.Write path.
	tree, err := cr.LoadTree(c.ID)
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}
	var buf bytes.Buffer
	if err := postman.Write(&buf, tree); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("decode written file: %v", err)
	}
	written, ok := doc["variable"].([]any)
	if !ok || len(written) != 2 {
		t.Fatalf("doc[\"variable\"] = %#v, want 2 entries", doc["variable"])
	}
	want := []map[string]any{
		{"key": "baseUrl", "value": "https://api.example.com"},
		{"key": "apiKey", "value": "", "type": "secret"},
	}
	for i, row := range written {
		m, ok := row.(map[string]any)
		if !ok || len(m) != len(want[i]) {
			t.Fatalf("written[%d] = %#v, want %#v", i, row, want[i])
		}
		for k, v := range want[i] {
			if m[k] != v {
				t.Errorf("written[%d][%q] = %#v, want %#v", i, k, m[k], v)
			}
		}
	}
}

// TestSecretsForDuplicateNameResolvesFirstWinsBySortOrder: mergeSecrets used to have no ORDER BY
// at all, leaving a duplicate name's winner undefined (whatever order SQLite happened to return
// rows in), where the documented rule is first-wins by sort_order — the same rule
// mergedValuesAndSecrets (frontend/src/api/state/variables.ts) and findSecretVariableId (curl.ts)
// already implement.
func TestSecretsForDuplicateNameResolvesFirstWinsBySortOrder(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)

	if _, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "token", strPtr("first-value"), true, ""); err != nil {
		t.Fatalf("Upsert(first): %v", err)
	}
	if _, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "token", strPtr("second-value"), true, ""); err != nil {
		t.Fatalf("Upsert(second): %v", err)
	}

	secrets, err := r.SecretsFor(collectionID, "")
	if err != nil {
		t.Fatalf("SecretsFor: %v", err)
	}
	if got := secrets["token"]; got != "first-value" {
		t.Fatalf("SecretsFor()[\"token\"] = %q, want %q (first-wins by sort_order)", got, "first-value")
	}
}

// TestSecretsForEnvironmentStillOverridesCollectionDespiteFirstWins guards the interaction:
// first-wins applies *within* a scope, never across scopes — an environment-scope variable must
// still override a same-named collection-scope one, even though mergeSecrets now skips a later
// same-named row within either individual scope's own query.
func TestSecretsForEnvironmentStillOverridesCollectionDespiteFirstWins(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)
	env, err := r.CreateEnvironment("Staging", "", "none")
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}

	if _, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "token", strPtr("collection-value"), true, ""); err != nil {
		t.Fatalf("Upsert(collection): %v", err)
	}
	if _, err := r.Upsert(model.VariableScopeEnvironment, env.ID, "", "token", strPtr("env-value"), true, ""); err != nil {
		t.Fatalf("Upsert(environment): %v", err)
	}

	secrets, err := r.SecretsFor(collectionID, env.ID)
	if err != nil {
		t.Fatalf("SecretsFor: %v", err)
	}
	if got := secrets["token"]; got != "env-value" {
		t.Fatalf("SecretsFor()[\"token\"] = %q, want %q (environment overrides collection)", got, "env-value")
	}
}

// TestSecretsForUndecryptableEnvSecretLeavesReferenceUnresolvedNotCollectionValue: mergeSecrets
// used to set seen[name] only after a successful decrypt, so a NULL or
// undecryptable row `continue`d without ever marking the name seen. Since out already held the
// collection-scope value from SecretsFor's earlier, lower-precedence mergeSecrets call, a broken
// higher-precedence (environment) row silently left that stale collection value in place — Send
// would use the collection's secret while findSecretVariableId (curl.ts) already treats the
// broken env row as unresolved, so Copy-as-curl and Send disagreed. A corrupted env-scope secret
// shadowing a same-named collection secret must now come back absent from SecretsFor (unresolved)
// rather than falling through to the collection's value.
func TestSecretsForUndecryptableEnvSecretLeavesReferenceUnresolvedNotCollectionValue(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)
	env, err := r.CreateEnvironment("Staging", "", "none")
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}

	if _, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "token", strPtr("collection-value"), true, ""); err != nil {
		t.Fatalf("Upsert(collection): %v", err)
	}
	if _, err := r.Upsert(model.VariableScopeEnvironment, env.ID, "", "token", strPtr("env-value"), true, ""); err != nil {
		t.Fatalf("Upsert(environment): %v", err)
	}

	// Simulate a corrupted ciphertext (e.g. key rotation gone wrong) on the environment-scope row.
	if _, err := db.Exec(`UPDATE api_variables SET secret_value = 'not-valid-ciphertext' WHERE environment_id = ? AND name = 'token'`, env.ID); err != nil {
		t.Fatalf("corrupt env secret: %v", err)
	}

	secrets, err := r.SecretsFor(collectionID, env.ID)
	if err != nil {
		t.Fatalf("SecretsFor: %v", err)
	}
	if got, ok := secrets["token"]; ok {
		t.Fatalf("SecretsFor()[\"token\"] = %q, want absent (unresolved) — must not fall through to the collection's value", got)
	}
}

// ---- 5. DuplicateEnvironment (P17 D17): a raw-ciphertext copy, no history, never active ----

func TestDuplicateEnvironmentCopiesCiphertextVerbatimNoHistoryNeverActive(t *testing.T) {
	r, db := newVariablesRepo(t)

	env, err := r.CreateEnvironment("Staging", "a description", "none")
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if err := r.SetActiveEnvironment(env.ID); err != nil {
		t.Fatalf("SetActiveEnvironment: %v", err)
	}
	plain, err := r.Upsert(model.VariableScopeEnvironment, env.ID, "", "apiKey", strPtr("s3cr3t-value"), true, "")
	if err != nil {
		t.Fatalf("Upsert(secret): %v", err)
	}
	// A second, real edit so the source variable actually has history — proving the clone
	// carries none of it is meaningless against a variable that never had any.
	if _, err := r.Upsert(model.VariableScopeEnvironment, env.ID, plain.ID, "apiKey", strPtr("s3cr3t-value-2"), true, ""); err != nil {
		t.Fatalf("Upsert(secret, changed): %v", err)
	}
	srcHistory, err := r.History(plain.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(srcHistory) == 0 {
		t.Fatal("source variable has no history — this fixture doesn't exercise the guard")
	}

	dup, err := r.DuplicateEnvironment(env.ID)
	if err != nil {
		t.Fatalf("DuplicateEnvironment: %v", err)
	}
	if dup.Name != "Staging copy" {
		t.Errorf("dup.Name = %q, want %q", dup.Name, "Staging copy")
	}
	if dup.Description != "a description" {
		t.Errorf("dup.Description = %q, want copied from source", dup.Description)
	}
	if dup.IsActive {
		t.Error("dup.IsActive = true, want false even though the source is active (D3)")
	}

	// The source environment must remain the active one — duplicating must not steal the
	// selection or create a second active row.
	envs, err := r.ListEnvironments()
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	activeCount := 0
	for _, e := range envs {
		if e.IsActive {
			activeCount++
			if e.ID != env.ID {
				t.Errorf("active environment = %s, want the original %s", e.ID, env.ID)
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("active environment count = %d, want exactly 1", activeCount)
	}

	dupVars, err := r.List(model.VariableScopeEnvironment, dup.ID)
	if err != nil {
		t.Fatalf("List(dup): %v", err)
	}
	if len(dupVars) != 1 || dupVars[0].Name != "apiKey" || !dupVars[0].IsSecret {
		t.Fatalf("dupVars = %+v, want one secret row named apiKey", dupVars)
	}
	if dupVars[0].ID == plain.ID {
		t.Fatal("the duplicated variable shares the source's own id")
	}

	// The property that matters: the copy's secret_value column is byte-identical to the
	// source's — a raw column copy, never a decrypt/re-encrypt (which would produce a different
	// ciphertext even for the same plaintext, since AES-GCM uses a fresh nonce per encryption).
	var srcCipher, dupCipher string
	if err := db.QueryRow(`SELECT secret_value FROM api_variables WHERE name = 'apiKey' AND environment_id = ?`, env.ID).Scan(&srcCipher); err != nil {
		t.Fatalf("read source ciphertext: %v", err)
	}
	if err := db.QueryRow(`SELECT secret_value FROM api_variables WHERE name = 'apiKey' AND environment_id = ?`, dup.ID).Scan(&dupCipher); err != nil {
		t.Fatalf("read duplicate ciphertext: %v", err)
	}
	if srcCipher != dupCipher {
		t.Fatalf("ciphertext changed across the duplicate — source %q, copy %q (expected byte-identical)", srcCipher, dupCipher)
	}
	// Both still decrypt to the same plaintext (sanity: the byte-identical ciphertext is not
	// simply empty or garbage).
	revealed, err := r.RevealValue(dupVars[0].ID)
	if err != nil {
		t.Fatalf("RevealValue(dup): %v", err)
	}
	if revealed != "s3cr3t-value-2" {
		t.Fatalf("RevealValue(dup) = %q, want %q", revealed, "s3cr3t-value-2")
	}

	// No history rows copied — the clone's row has none, even though the source (the same name,
	// a distinct id) does.
	dupHistory, err := r.History(dupVars[0].ID)
	if err != nil {
		t.Fatalf("History(dup): %v", err)
	}
	if len(dupHistory) != 0 {
		t.Fatalf("History(dup) = %+v, want none copied", dupHistory)
	}
}

// ---- 6. ApplyBulk (P17 D22/D23): the two properties that matter most ----

func TestApplyBulkLeavesAnUntouchedSecretByteIdentical(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)

	created, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "apiKey", strPtr("s3cr3t"), true, "d")
	if err != nil {
		t.Fatalf("Upsert(secret): %v", err)
	}
	var before string
	if err := db.QueryRow(`SELECT secret_value FROM api_variables WHERE id = ?`, created.ID).Scan(&before); err != nil {
		t.Fatalf("read secret_value before: %v", err)
	}

	// The bulk editor's own "untouched" shape: a bare `KEY=` line, HasValue: false, same
	// description — the property that makes it safe to open the editor and press Apply without
	// re-typing every secret.
	result, err := r.ApplyBulk(model.VariableScopeCollection, collectionID, []model.VariableBulkEntry{
		{Name: "apiKey", Value: "", HasValue: false, Description: "d"},
	})
	if err != nil {
		t.Fatalf("ApplyBulk: %v", err)
	}
	if result.Added != 0 || result.Updated != 0 || result.Removed != 0 || result.Reordered {
		t.Fatalf("result = %+v, want an untouched no-op", result)
	}

	var after string
	if err := db.QueryRow(`SELECT secret_value FROM api_variables WHERE id = ?`, created.ID).Scan(&after); err != nil {
		t.Fatalf("read secret_value after: %v", err)
	}
	if before != after {
		t.Fatalf("secret_value changed across an untouched ApplyBulk: before %q, after %q", before, after)
	}

	rows, err := r.List(model.VariableScopeCollection, collectionID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || !rows[0].IsSecret {
		t.Fatalf("rows = %+v, want the one secret row intact", rows)
	}
}

func TestApplyBulkNewSecretValueRecordsExactlyOneHistoryRow(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)

	created, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "apiKey", strPtr("old-secret"), true, "")
	if err != nil {
		t.Fatalf("Upsert(secret): %v", err)
	}

	result, err := r.ApplyBulk(model.VariableScopeCollection, collectionID, []model.VariableBulkEntry{
		{Name: "apiKey", Value: "new-secret", HasValue: true, Description: ""},
	})
	if err != nil {
		t.Fatalf("ApplyBulk: %v", err)
	}
	if result.Updated != 1 || result.Added != 0 || result.Removed != 0 {
		t.Fatalf("result = %+v, want exactly one update", result)
	}

	hist, err := r.History(created.ID)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 1 || !hist[0].IsSecret {
		t.Fatalf("hist = %+v, want exactly one secret history row (the value it replaced)", hist)
	}
	revealed, err := r.RevealValue(created.ID)
	if err != nil {
		t.Fatalf("RevealValue: %v", err)
	}
	if revealed != "new-secret" {
		t.Fatalf("RevealValue = %q, want %q", revealed, "new-secret")
	}
}

// TestApplyBulkFullReconcile exercises every D22 rule in one transaction: an update, a create
// (new, non-secret), a delete, and a description-only change on a secret whose value line was
// left untouched.
func TestApplyBulkFullReconcile(t *testing.T) {
	r, db := newVariablesRepo(t)
	collectionID := newCollectionFor(t, db)

	kept, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "kept", strPtr("old-value"), false, "")
	if err != nil {
		t.Fatalf("Upsert(kept): %v", err)
	}
	gone, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "gone", strPtr("x"), false, "")
	if err != nil {
		t.Fatalf("Upsert(gone): %v", err)
	}
	secret, err := r.Upsert(model.VariableScopeCollection, collectionID, "", "secretVar", strPtr("s3cr3t"), true, "old desc")
	if err != nil {
		t.Fatalf("Upsert(secretVar): %v", err)
	}

	result, err := r.ApplyBulk(model.VariableScopeCollection, collectionID, []model.VariableBulkEntry{
		{Name: "kept", Value: "new-value", HasValue: true, Description: ""},
		{Name: "secretVar", Value: "", HasValue: false, Description: "new desc"},
		{Name: "brandNew", Value: "v", HasValue: true, Description: ""},
	})
	if err != nil {
		t.Fatalf("ApplyBulk: %v", err)
	}
	if result.Added != 1 || result.Updated != 2 || result.Removed != 1 {
		t.Fatalf("result = %+v, want {Added:1 Updated:2 Removed:1}", result)
	}

	rows, err := r.List(model.VariableScopeCollection, collectionID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byName := map[string]model.Variable{}
	for _, row := range rows {
		byName[row.Name] = row
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %+v, want exactly 3 (kept, secretVar, brandNew — gone removed)", rows)
	}
	if v, ok := byName["kept"]; !ok || v.Value != "new-value" || v.ID != kept.ID {
		t.Errorf("kept = %+v, want value updated with the same id", v)
	}
	if _, ok := byName["gone"]; ok {
		t.Error("gone still present, want removed")
	}
	if v, ok := byName["secretVar"]; !ok || v.ID != secret.ID || v.Description != "new desc" {
		t.Errorf("secretVar = %+v, want description updated, same id, value still masked", v)
	}
	if v, ok := byName["brandNew"]; !ok || v.IsSecret {
		t.Errorf("brandNew = %+v, want a new non-secret row", v)
	}

	// The removed row's history is gone too (cascade, same as Delete).
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM api_variable_history WHERE variable_id = ?`, gone.ID).Scan(&count); err != nil {
		t.Fatalf("count history: %v", err)
	}
	if count != 0 {
		t.Errorf("history rows for the removed variable = %d, want 0 (cascaded)", count)
	}
}

// ---- 6. Environment colour (P18 D18/D19): round-trips through create/update/list, a duplicate
// inherits it, and an unrecognised stored value reads back as 'none' without dropping the row ----

func TestEnvironmentColorRoundTripsThroughCreateUpdateList(t *testing.T) {
	r, _ := newVariablesRepo(t)

	env, err := r.CreateEnvironment("Staging", "", "blue")
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if env.Color != "blue" {
		t.Fatalf("CreateEnvironment's own returned Color = %q, want blue", env.Color)
	}

	envs, err := r.ListEnvironments()
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	found := false
	for _, e := range envs {
		if e.ID == env.ID {
			found = true
			if e.Color != "blue" {
				t.Errorf("ListEnvironments' Color = %q, want blue", e.Color)
			}
		}
	}
	if !found {
		t.Fatal("created environment not found in ListEnvironments")
	}

	if err := r.UpdateEnvironment(env.ID, "Staging", "", "amber"); err != nil {
		t.Fatalf("UpdateEnvironment: %v", err)
	}
	envs, err = r.ListEnvironments()
	if err != nil {
		t.Fatalf("ListEnvironments after update: %v", err)
	}
	for _, e := range envs {
		if e.ID == env.ID && e.Color != "amber" {
			t.Errorf("Color after update = %q, want amber", e.Color)
		}
	}
}

func TestDuplicateEnvironmentInheritsColor(t *testing.T) {
	r, _ := newVariablesRepo(t)

	env, err := r.CreateEnvironment("Staging", "", "magenta")
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	dup, err := r.DuplicateEnvironment(env.ID)
	if err != nil {
		t.Fatalf("DuplicateEnvironment: %v", err)
	}
	if dup.Color != "magenta" {
		t.Errorf("dup.Color = %q, want inherited magenta", dup.Color)
	}
}

// TestListEnvironmentsCoercesAnUnrecognisedColorWithoutDroppingTheRow is the deliberate
// divergence from repos/connections.go:53 (which *drops* a connection row whose colour it does
// not recognise): an environment owns variables, so losing one over a cosmetic column would be a
// data-loss bug. A hand-edited or legacy row with a bogus colour must still list, with its colour
// coerced to 'none'.
func TestListEnvironmentsCoercesAnUnrecognisedColorWithoutDroppingTheRow(t *testing.T) {
	r, db := newVariablesRepo(t)

	env, err := r.CreateEnvironment("Staging", "", "blue")
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if _, err := db.Exec(`UPDATE api_environments SET color = ? WHERE id = ?`, "not-a-real-color", env.ID); err != nil {
		t.Fatalf("corrupt color column: %v", err)
	}

	envs, err := r.ListEnvironments()
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	found := false
	for _, e := range envs {
		if e.ID == env.ID {
			found = true
			if e.Color != "none" {
				t.Errorf("Color = %q, want coerced to none", e.Color)
			}
		}
	}
	if !found {
		t.Fatal("the row with an unrecognised color must still be listed, not dropped")
	}
}
