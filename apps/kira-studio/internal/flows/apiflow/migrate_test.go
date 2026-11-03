package apiflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// Rows written before the api files existed become files at the next start: ids, secrets and history
// carry over, and a later start never exports again.
func TestApiConfigMigration(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	const (
		legacyKey = "legacy-secret-9"
		legacyEnv = "legacy-env-secret-3"
		now       = "2026-01-01T00:00:00Z"
	)
	cipher := secrets.New()
	seal := func(plain string) string {
		t.Helper()
		sealed, err := cipher.Encrypt(secrets.ScopeVariable, plain)
		if err != nil {
			t.Fatal(err)
		}
		return sealed
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := app.DB().Exec(q, args...); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}

	exec(`DELETE FROM settings WHERE key = 'config.migrated.api'`)
	exec(`INSERT INTO api_collections (id, name, sort_order, created_at, updated_at) VALUES ('c1', 'Legacy Shop', 0, ?, ?)`, now, now)
	exec(`INSERT INTO api_items (id, collection_id, parent_id, kind, name, sort_order, created_at, updated_at) VALUES ('f1', 'c1', NULL, 'folder', 'Orders', 0, ?, ?)`, now, now)
	httpReq := `{"method":"GET","url":"{{base}}/status?code=200","headers":[{"name":"X-Key","value":"{{key}}","enabled":true}],"bodyMode":"none","body":"","code":"","codeLanguage":"json","urlEncoded":[],"formData":[],"binaryFile":null}`
	exec(`INSERT INTO api_items (id, collection_id, parent_id, kind, protocol, name, sort_order, method, url, request_json, created_at, updated_at)
		VALUES ('r1', 'c1', 'f1', 'request', 'http', 'List', 0, 'GET', '{{base}}/status?code=200', ?, ?, ?)`, httpReq, now, now)
	grpcReq := `{"target":"localhost:50051","tlsMode":"plaintext","caFile":"","serverName":"","descriptorMode":"reflection","protoPath":"","importPaths":[],"service":"kira.flow.v1.Flow","method":"Unary","message":"{}","metadata":[]}`
	exec(`INSERT INTO api_items (id, collection_id, parent_id, kind, protocol, name, sort_order, request_json, created_at, updated_at)
		VALUES ('g1', 'c1', NULL, 'request', 'grpc', 'Unary', 1, ?, ?, ?)`, grpcReq, now, now)
	exec(`INSERT INTO api_variables (id, collection_id, name, value, is_secret, sort_order, created_at, updated_at) VALUES ('v1', 'c1', 'base', ?, 0, 0, ?, ?)`, srv.URL, now, now)
	exec(`INSERT INTO api_variables (id, collection_id, name, value, is_secret, sort_order, created_at, updated_at) VALUES ('v2', 'c1', 'base', 'dead', 0, 1, ?, ?)`, now, now)
	exec(`INSERT INTO api_variables (id, collection_id, name, value, is_secret, secret_value, sort_order, created_at, updated_at) VALUES ('v3', 'c1', 'key', '', 1, ?, 2, ?, ?)`, seal(legacyKey), now, now)
	exec(`INSERT INTO api_environments (id, name, sort_order, is_active, created_at, updated_at) VALUES ('e1', 'Prod', 0, 1, ?, ?)`, now, now)
	exec(`INSERT INTO api_variables (id, environment_id, name, value, is_secret, secret_value, sort_order, created_at, updated_at) VALUES ('v4', 'e1', 'pass', '', 1, ?, 0, ?, ?)`, seal(legacyEnv), now, now)
	if err := (&repos.ResponseHistoryRepo{DB: app.DB()}).Record(model.ResponseHistoryRecord{
		ItemID: "r1", TabID: "tab-1", Method: "GET", URL: "{{base}}/status", Body: httpclient.Body{Mode: "none"},
		Response: httpclient.Response{Status: 200, StatusText: "OK", Proto: "HTTP/1.1", BodyEncoding: "utf8"},
	}); err != nil {
		t.Fatal(err)
	}

	app.Restart(t)

	dir := app.W.ApiFiles.Dir()
	collectionFile, envFile := filepath.Join(dir, "legacy-shop.collection.yaml"), filepath.Join(dir, "prod.environment.yaml")
	text, err := os.ReadFile(collectionFile)
	if err != nil {
		t.Fatalf("collection file: %v", err)
	}
	if _, err := os.Stat(envFile); err != nil {
		t.Fatalf("environment file: %v", err)
	}
	if !strings.Contains(string(text), "Duplicate variable, not applied: base = dead") {
		t.Errorf("duplicate variable not kept as a comment:\n%s", text)
	}
	if contains(text, legacyKey) {
		t.Errorf("the secret value is in the file:\n%s", text)
	}

	tree, err := app.W.Collections.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Collections) != 1 || tree.Collections[0].ID != "c1" || tree.Collections[0].File != "legacy-shop.collection.yaml" || len(tree.Items) != 3 {
		t.Fatalf("tree = %+v, want collection c1 with 3 items", tree)
	}
	byID := map[string]model.CollectionItem{}
	for _, it := range tree.Items {
		byID[it.ID] = it
	}
	if r := byID["r1"]; r.ParentID == nil || *r.ParentID != "f1" || r.Name != "List" {
		t.Errorf("r1 = %+v, want List under f1", r)
	}
	if g := byID["g1"]; g.Protocol != model.ItemProtocolGrpc {
		t.Errorf("g1 = %+v, want a gRPC item", g)
	}
	if req, err := app.W.Collections.GetRequest(bridge.CollectionsItemArgs{ItemID: "r1"}); err != nil || req.URL != "{{base}}/status?code=200" || len(req.Headers) != 1 {
		t.Errorf("GetRequest(r1) = %+v (%v)", req, err)
	}
	if g, err := app.W.Collections.GetGrpcRequest(bridge.CollectionsItemArgs{ItemID: "g1"}); err != nil || g.Target != "localhost:50051" {
		t.Errorf("GetGrpcRequest(g1) = %+v (%v)", g, err)
	}
	vars, err := app.W.Variables.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeCollection, OwnerID: "c1"})
	if err != nil || len(vars) != 2 || vars[0].Name != "base" || vars[0].Value != srv.URL || !vars[1].IsSecret || vars[1].Value != "" {
		t.Errorf("variables = %+v (%v), want base and a secret key", vars, err)
	}
	envs, err := app.W.Variables.ListEnvironments()
	if err != nil || len(envs) != 1 || envs[0].ID != "e1" || !envs[0].IsActive {
		t.Errorf("environments = %+v (%v), want Prod active", envs, err)
	}
	if rows, err := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{ItemID: "r1"}); err != nil || len(rows) != 1 {
		t.Errorf("history = %+v (%v), want one row still on r1", rows, err)
	}

	// The sealed value moved as it was: a send resolves it with no new secret saved.
	args := get(srv.URL + "/status?code=200")
	args.CollectionID = "c1"
	args.Headers = []httpclient.Header{{Name: "X-Key", Value: "{{key}}"}}
	send(t, app, args)
	if got := lastRequest(srv).Header.Get("X-Key"); got != legacyKey {
		t.Errorf("server X-Key = %q, want the migrated secret", got)
	}
	var left int
	if err := app.DB().QueryRow(`SELECT COUNT(*) FROM api_variables WHERE secret_value IS NOT NULL`).Scan(&left); err != nil || left != 0 {
		t.Errorf("rows still holding a sealed value = %d (%v), want 0", left, err)
	}
	var marker string
	if err := app.DB().QueryRow(`SELECT value FROM settings WHERE key = 'config.migrated.api'`).Scan(&marker); err != nil || marker != "true" {
		t.Errorf("marker = %q (%v), want true", marker, err)
	}

	// A later start leaves an edited file alone.
	if err := os.WriteFile(collectionFile, append([]byte("# edited by hand\n"), text...), 0o644); err != nil {
		t.Fatal(err)
	}
	app.Restart(t)
	after, err := os.ReadFile(collectionFile)
	if err != nil || !strings.HasPrefix(string(after), "# edited by hand\n") {
		t.Errorf("file after restart = %q (%v), want the hand edit kept", after, err)
	}
	if tree, err := app.W.Collections.List(); err != nil || len(tree.Items) != 3 || tree.Collections[0].ID != "c1" {
		t.Errorf("tree after restart = %+v (%v)", tree, err)
	}
}
