package repos_test

import (
	"bytes"
	"database/sql"
	"errors"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// M5 §8's own narrow scope for this repo: the case-insensitive unique index, ON DELETE CASCADE, and
// that InsertDuplicateWithSecret does not copy mask_correlation_key — three real correctness
// properties with a silent failure mode, not CRUD round-trips (which get nothing, per CLAUDE.md).

func newMaskRulesRepo(t *testing.T) (*repos.MaskRulesRepo, *sql.DB) {
	t.Helper()
	db := newRepos(t).DB
	return &repos.MaskRulesRepo{DB: db}, db
}

// TestMaskRulesUniqueIndexIsCaseInsensitive is plan §3.1's own stated reason the index is built on
// lower(table_name), lower(column_name): a second Upsert differing only in case must update the
// same row, never create a second one that "which wins is arbitrary" at render time.
func TestMaskRulesUniqueIndexIsCaseInsensitive(t *testing.T) {
	r, db := newMaskRulesRepo(t)
	connID := uuid.NewString()
	seedConnection(t, db, connID)
	now := kiratime.NowISO()

	first, err := r.Upsert(uuid.NewString(), connID, model.MaskRuleFields{
		TableName: "customers", ColumnName: "email", Kind: model.MaskKindEmail, KeepHint: true, Correlate: true,
	}, now)
	if err != nil {
		t.Fatalf("Upsert(email): %v", err)
	}

	second, err := r.Upsert(uuid.NewString(), connID, model.MaskRuleFields{
		TableName: "Customers", ColumnName: "Email", Kind: model.MaskKindRedact, KeepHint: false, Correlate: false,
	}, kiratime.NowISO())
	if err != nil {
		t.Fatalf("Upsert(Email, different case): %v", err)
	}

	if second.ID != first.ID {
		t.Fatalf("a case-differing Upsert created a second row (id %s), want it to update the existing row %s", second.ID, first.ID)
	}
	if second.Kind != model.MaskKindRedact {
		t.Fatalf("second.Kind = %s, want redact (the update should have taken)", second.Kind)
	}

	list, err := r.ListForConnection(connID)
	if err != nil {
		t.Fatalf("ListForConnection: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListForConnection = %d rows, want exactly 1 (case-insensitive collision must collapse to one row)", len(list))
	}
}

// TestMaskRulesCascadeOnConnectionDelete confirms connection_mask_rules(connection_id) really does
// carry ON DELETE CASCADE (the migration's own stated intent) rather than silently orphaning rows
// once a connection is removed.
func TestMaskRulesCascadeOnConnectionDelete(t *testing.T) {
	r, db := newMaskRulesRepo(t)
	connID := uuid.NewString()
	seedConnection(t, db, connID)

	if _, err := r.Upsert(uuid.NewString(), connID, model.MaskRuleFields{
		TableName: "*", ColumnName: "ssn", Kind: model.MaskKindRedact, Correlate: false,
	}, kiratime.NowISO()); err != nil {
		t.Fatalf("Upsert: %v", err)
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

// TestInsertDuplicateWithSecretDoesNotCopyMaskCorrelationKey: a copied key would silently break
// the "key is scoped to one connection" invariant, making the duplicate's masked tags linkable
// back to the original's. Verified directly against the real ConnectionsRepo/MaskKeysRepo pair,
// not just read from the INSERT statement's column list.
func TestInsertDuplicateWithSecretDoesNotCopyMaskCorrelationKey(t *testing.T) {
	t.Setenv("KIRA_INSECURE_SECRETS", "1")
	db := newRepos(t).DB
	cipher := secrets.New()
	conns := &repos.ConnectionsRepo{DB: db}
	keys := repos.NewMaskKeys(db, cipher)

	fromID := uuid.NewString()
	fields := model.ConnectionFields{Name: "src", Kind: "postgres", Color: "blue", Mode: "fields"}
	if _, err := conns.InsertWithSecret(fromID, fields, kiratime.NowISO(), nil, nil); err != nil {
		t.Fatalf("InsertWithSecret: %v", err)
	}

	originalKey, err := keys.EnsureKey(fromID)
	if err != nil {
		t.Fatalf("EnsureKey: %v", err)
	}
	if len(originalKey) == 0 {
		t.Fatal("EnsureKey returned an empty key")
	}

	toID := uuid.NewString()
	if _, err := conns.InsertDuplicateWithSecret(fromID, toID, fields, kiratime.NowISO()); err != nil {
		t.Fatalf("InsertDuplicateWithSecret: %v", err)
	}

	duplicateKey, err := keys.Get(toID)
	if err != nil {
		t.Fatalf("Get(duplicate): %v", err)
	}
	if duplicateKey != nil {
		t.Fatalf("duplicate connection already has a mask_correlation_key (%d bytes) — InsertDuplicateWithSecret must leave it unminted, not copy the original's", len(duplicateKey))
	}

	// A fresh EnsureKey on the duplicate must mint its OWN key, distinct from the original's — the
	// property that actually matters (linkability), not merely "the column was empty at one instant".
	duplicateMinted, err := keys.EnsureKey(toID)
	if err != nil {
		t.Fatalf("EnsureKey(duplicate): %v", err)
	}
	if bytes.Equal(duplicateMinted, originalKey) {
		t.Fatal("the duplicate's freshly-minted key equals the original's — keys are linkable across connections")
	}
}

// TestInsertDuplicateWithSecretAlwaysStartsMcpDisabled: the duplicate's row must
// never be committed already MCP-live, whatever the source's own McpEnabled was — mask rules live
// in a separate table this single INSERT cannot also write, so connections.Service.Duplicate flips
// mcp_enabled on (via SetMcpEnabled) only after copying them succeeds. A duplicate inserted
// already-enabled here would be exposed with zero mask rules for the entire window before that
// second write, and would stay that way forever if it errored.
func TestInsertDuplicateWithSecretAlwaysStartsMcpDisabled(t *testing.T) {
	db := newRepos(t).DB
	conns := &repos.ConnectionsRepo{DB: db}

	fromID := uuid.NewString()
	fields := model.ConnectionFields{
		Name: "src", Kind: "postgres", Color: "blue", Mode: "fields",
		McpEnabled: true, McpReadMode: "allow", McpWriteMode: "deny", McpDdlMode: "deny",
	}
	if _, err := conns.InsertWithSecret(fromID, fields, kiratime.NowISO(), nil, nil); err != nil {
		t.Fatalf("InsertWithSecret: %v", err)
	}

	toID := uuid.NewString()
	created, err := conns.InsertDuplicateWithSecret(fromID, toID, fields, kiratime.NowISO())
	if err != nil {
		t.Fatalf("InsertDuplicateWithSecret: %v", err)
	}
	if created.McpEnabled {
		t.Fatal("InsertDuplicateWithSecret result has McpEnabled = true, want it always inserted off")
	}

	got, err := conns.Get(toID)
	if err != nil {
		t.Fatalf("Get(duplicate): %v", err)
	}
	if got == nil || got.McpEnabled {
		t.Fatalf("duplicate row McpEnabled = %v, want false regardless of the source's own McpEnabled", got)
	}
	// The permission modes themselves are still copied verbatim — only the live/off switch is held
	// back, not the connection's configured MCP shape.
	if got.McpReadMode != "allow" || got.McpWriteMode != "deny" || got.McpDdlMode != "deny" {
		t.Fatalf("duplicate permission modes = %+v, want copied verbatim from the source", got.ConnectionFields)
	}
}

// TestInsertDuplicateWithSecretCopiesEveryColumn guards the INSERT's placeholder/column alignment:
// a miscount once wrote the throttle into mcp_enabled, leaving a duplicate MCP-live with no mask
// rules. Fractional throttle and MCP off make any shift visible.
func TestInsertDuplicateWithSecretCopiesEveryColumn(t *testing.T) {
	db := newRepos(t).DB
	conns := &repos.ConnectionsRepo{DB: db}

	fromID := uuid.NewString()
	host, port, dbName, user, pre := "db.local", 5432, "app", "u", "echo hi"
	fields := model.ConnectionFields{
		Name: "src", Kind: "postgres", Color: "blue", Mode: "fields", ReadOnly: true,
		Host: &host, Port: &port, Database: &dbName, Username: &user,
		Options:    map[string]any{"k": "v"},
		Preconnect: &pre, PreconnectSidecar: true, PreconnectDelaySec: 7,
		AutoExplain: true, ThrottlePerSec: 0.5,
		McpEnabled: false, McpDescription: "desc", McpReadMode: "allow", McpWriteMode: "deny",
		McpDdlMode: "deny", McpAutoExplain: true, McpReviewResults: "send",
	}
	if _, err := conns.InsertWithSecret(fromID, fields, kiratime.NowISO(), nil, nil); err != nil {
		t.Fatalf("InsertWithSecret: %v", err)
	}
	toID := uuid.NewString()
	if _, err := conns.InsertDuplicateWithSecret(fromID, toID, fields, kiratime.NowISO()); err != nil {
		t.Fatalf("InsertDuplicateWithSecret: %v", err)
	}
	got, err := conns.Get(toID)
	if err != nil || got == nil {
		t.Fatalf("Get(duplicate) = %v, %v", got, err)
	}
	if got.McpEnabled {
		t.Fatal("duplicate McpEnabled = true, want false")
	}
	if !reflect.DeepEqual(got.ConnectionFields, fields) {
		t.Fatalf("duplicate fields = %+v, want %+v", got.ConnectionFields, fields)
	}
}

// TestSetMcpEnabledFlipsFlagAndRejectsMissingID pins connections.SetMcpEnabled's own two
// behaviours (finding #4, M7): it flips mcp_enabled alone (leaving every other column untouched)
// and reports sql.ErrNoRows for an id that matches no row, rather than a silent no-op success.
func TestSetMcpEnabledFlipsFlagAndRejectsMissingID(t *testing.T) {
	db := newRepos(t).DB
	conns := &repos.ConnectionsRepo{DB: db}

	connID := uuid.NewString()
	fields := model.ConnectionFields{Name: "conn", Kind: "postgres", Color: "blue", Mode: "fields"}
	if _, err := conns.InsertWithSecret(connID, fields, kiratime.NowISO(), nil, nil); err != nil {
		t.Fatalf("InsertWithSecret: %v", err)
	}

	if err := conns.SetMcpEnabled(connID, true, kiratime.NowISO()); err != nil {
		t.Fatalf("SetMcpEnabled(true): %v", err)
	}
	got, err := conns.Get(connID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || !got.McpEnabled {
		t.Fatalf("McpEnabled after SetMcpEnabled(true) = %v, want true", got)
	}
	if got.Name != "conn" {
		t.Fatalf("Name = %q, want unchanged %q — SetMcpEnabled must touch mcp_enabled alone", got.Name, "conn")
	}

	if err := conns.SetMcpEnabled(uuid.NewString(), true, kiratime.NowISO()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("SetMcpEnabled(missing id) = %v, want a wrapped sql.ErrNoRows", err)
	}
}
