package apiflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

const configFilesSecret = "sk-config-files-7c41"

// eventually polls cond until it holds; the folder watcher applies an external edit asynchronously.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ordersFile is the collection file as an editor would leave it. Contract api-files, read by
// tests/ui/api-config-files.spec.ts.
func ordersFile(base, itemID string) []byte {
	return []byte(`# Orders API
name: Orders API
variables:
  - name: base_url
    value: ` + base + `
  - name: token
    secret: ${secret:orders.token}
items:
  - id: ` + itemID + `
    name: List orders
    http:
      method: GET
      url: "{{base_url}}/echo?list=1"
      headers:
        - name: Authorization
          value: Bearer {{token}}
`)
}

func TestApiConfigFiles(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	const sc = "api-files"
	cs, vs, cfg := app.W.Collections, app.W.Variables, app.W.ApiConfig
	file := filepath.Join(app.W.ApiFiles.Dir(), "orders-api.collection.yaml")
	readFile := func() string {
		t.Helper()
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	items := func() []model.CollectionItem {
		t.Helper()
		tree, err := cs.List()
		if err != nil {
			t.Fatal(err)
		}
		return tree.Items
	}
	paths := flowharness.Replace(app.KiraHome, "<home>")

	// 1. Creating a collection writes its file.
	col, err := cs.CreateCollection(bridge.CollectionsCreateCollectionArgs{Name: "Orders API"})
	if err != nil {
		t.Fatal(err)
	}
	if col.File != "orders-api.collection.yaml" {
		t.Fatalf("file = %q, want orders-api.collection.yaml", col.File)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("collection file missing: %v", err)
	}

	// 2. An external atomic write (as an editor does) reaches the rows.
	if err := configfs.WriteAtomic(file, ordersFile(srv.URL, "list"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the external edit", func() bool { return len(items()) == 1 })
	item := items()[0]
	if item.Name != "List orders" || item.CollectionID != col.ID {
		t.Fatalf("item = %+v, want List orders in the collection", item)
	}
	req, err := cs.GetRequest(bridge.CollectionsItemArgs{ItemID: item.ID})
	if err != nil || req.URL != "{{base_url}}/echo?list=1" {
		t.Fatalf("GetRequest = %+v (%v)", req, err)
	}
	vars, err := vs.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeCollection, OwnerID: col.ID})
	if err != nil || len(vars) != 2 || vars[0].Name != "base_url" || vars[0].Value != srv.URL || !vars[1].IsSecret || vars[1].Value != "" {
		t.Fatalf("variables = %+v (%v), want base_url and an empty secret token", vars, err)
	}
	tree, err := cs.List()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "CollectionsService.List", tree, paths, flowharness.Replace(srv.URL, "<http>"))

	// 3. A bad edit keeps the last good rows and refuses app writes until fixed.
	bad := strings.Replace(string(ordersFile(srv.URL, "list")), "items:\n", "items:\n  - id: list\n    name: Twin\n    http:\n      method: GET\n", 1)
	if err := configfs.WriteAtomic(file, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	var view apifiles.View
	eventually(t, "the problem", func() bool {
		v, err := cfg.View()
		if err != nil {
			t.Fatal(err)
		}
		view = v
		return problemOf(v, "orders-api.collection.yaml") != nil
	})
	if p := problemOf(view, "orders-api.collection.yaml"); p.Line == 0 || p.Message == "" {
		t.Errorf("problem = %+v, want a line and a message", p)
	}
	app.Contract(t, sc, "ApiConfigService.View", view, paths)
	if got := items(); len(got) != 1 || got[0].Name != "List orders" {
		t.Errorf("rows after the bad edit = %+v, want the last good item", got)
	}
	_, err = cs.SaveRequest(bridge.CollectionsSaveRequestArgs{ItemID: item.ID, Name: "x", Request: req})
	if ie := ipcErr(t, err); ie.Code != "E_BAD_REQUEST" {
		t.Errorf("SaveRequest on a broken file = %v, want E_BAD_REQUEST", ie)
	}
	if err := configfs.WriteAtomic(file, ordersFile(srv.URL, "list"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the fixed file", func() bool {
		v, err := cfg.View()
		return err == nil && problemOf(v, "orders-api.collection.yaml") == nil
	})

	// 4. An unset secret fails the send by name; once set, only the wire sees it.
	args := get(srv.URL + "/status?code=200")
	args.CollectionID, args.ItemID = col.ID, item.ID
	args.Headers = []httpclient.Header{{Name: "Authorization", Value: "Bearer {{token}}"}}
	_, err = app.W.Http.Send(ctx, args)
	if ie := ipcErr(t, err); ie.Code != "E_BAD_REQUEST" || !strings.Contains(ie.Message, "orders.token") {
		t.Fatalf("send with an unset secret = %v, want E_BAD_REQUEST naming orders.token", err)
	}
	if _, err := cfg.SetSecret(bridge.ApiSecretArgs{Name: "orders.token", Value: configFilesSecret}); err != nil {
		t.Fatal(err)
	}
	args.OpID = newOp()
	send(t, app, args)
	if got := lastRequest(srv).Header.Get("Authorization"); got != "Bearer "+configFilesSecret {
		t.Errorf("server Authorization = %q, want the secret", got)
	}
	op := waitOp(t, app, args.OpID)
	if op.Command == nil || strings.Contains(*op.Command, configFilesSecret) {
		t.Errorf("op command = %v, want the masked form", op.Command)
	}
	rows, err := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{ItemID: item.ID})
	if err != nil || len(rows) != 1 {
		t.Fatalf("item history = %+v (%v), want one row", rows, err)
	}
	if contains([]byte(readFile()), configFilesSecret) {
		t.Error("the secret value reached the file")
	}
	if contains(dbBytes(t, app), configFilesSecret) {
		t.Error("the secret value reached the database in the clear")
	}

	// 5. An app save keeps the file's comments and the item's rows.
	req.URL = "{{base_url}}/echo?list=2"
	if _, err := cs.SaveRequest(bridge.CollectionsSaveRequestArgs{ItemID: item.ID, Name: "List orders", Request: req}); err != nil {
		t.Fatal(err)
	}
	if text := readFile(); !strings.Contains(text, "list=2") || !strings.Contains(text, "# Orders API") {
		t.Errorf("file after SaveRequest lost the edit or the comment:\n%s", text)
	}
	if got := items(); len(got) != 1 || got[0].ID != item.ID {
		t.Errorf("items after SaveRequest = %+v, want the same row", got)
	}
	if rows, _ := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{ItemID: item.ID}); len(rows) != 1 {
		t.Errorf("history after SaveRequest = %d rows, want 1 still attached", len(rows))
	}
	if contains([]byte(readFile()), configFilesSecret) {
		t.Error("the secret value reached the file after SaveRequest")
	}

	// 6. Deleting the file drops the collection and its history.
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the collection to go", func() bool {
		tree, err := cs.List()
		return err == nil && len(tree.Collections) == 0
	})
	if rows, _ := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{ItemID: item.ID}); len(rows) != 0 {
		t.Errorf("history after the file was deleted = %d rows, want 0", len(rows))
	}
}

func problemOf(v apifiles.View, file string) *apifiles.ProblemView {
	for _, f := range v.Files {
		if f.File == file {
			return f.Problem
		}
	}
	return nil
}

// AGENTS.md documents every top-level option of both file kinds.
func TestApiAgentsDocCoversSchemas(t *testing.T) {
	app := flowharness.New(t)
	doc, err := os.ReadFile(filepath.Join(app.W.ApiFiles.Dir(), "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"collection", "environment"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "apifiles", "schema", name+".schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		var sch struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &sch); err != nil || len(sch.Properties) == 0 {
			t.Fatalf("schema %s: %v (%d properties)", name, err, len(sch.Properties))
		}
		for prop := range sch.Properties {
			if !contains(doc, prop) {
				t.Errorf("AGENTS.md lacks the %s option %q", name, prop)
			}
		}
	}
}
