package apiflow

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/postman"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

func TestSavedRequestHistory(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)

	mark := app.Events.Mark()
	col := newCollection(t, app, "orders")
	if ch := apiChanges(t, app, mark); len(ch) != 1 || len(ch[0]) != 1 || ch[0][0].Kind != bridge.ApiDataTree {
		t.Errorf("CreateCollection events = %+v, want one tree change", ch)
	}

	req := savedGet()
	req.URL = srv.URL + "/echo?saved=1"
	item, err := app.W.Collections.CreateItem(bridge.CollectionsCreateItemArgs{
		CollectionID: col.ID, Kind: "request", Name: "list", Request: &req,
	})
	if err != nil {
		t.Fatal(err)
	}
	req.URL = srv.URL + "/echo?saved=2"
	mark = app.Events.Mark()
	if _, err := app.W.Collections.SaveRequest(bridge.CollectionsSaveRequestArgs{ItemID: item.ID, Name: "list", Request: req}); err != nil {
		t.Fatal(err)
	}
	ch := apiChanges(t, app, mark)
	if len(ch) != 1 || !hasChange(ch[0], bridge.ApiDataChange{Kind: bridge.ApiDataTree}) ||
		!hasChange(ch[0], bridge.ApiDataChange{Kind: bridge.ApiDataSavedRequest, ItemID: item.ID}) {
		t.Errorf("SaveRequest events = %+v, want tree and savedRequest(%s)", ch, item.ID)
	}

	bound := get(req.URL)
	bound.ItemID, bound.CollectionID = item.ID, col.ID
	send(t, app, bound)
	scratch := get(srv.URL + "/echo?scratch=1")
	scratch.TabID = "scratch-tab"
	send(t, app, scratch)

	itemRows, err := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{ItemID: item.ID})
	if err != nil || len(itemRows) != 1 || itemRows[0].ItemID == nil || *itemRows[0].ItemID != item.ID {
		t.Fatalf("item history = %+v (%v), want one row under the item", itemRows, err)
	}
	if tabRows, _ := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{TabID: "tab-1"}); len(tabRows) != 0 {
		t.Errorf("the saved request's send also landed under its tab: %+v", tabRows)
	}

	ad, err := app.W.ResponseHistory.Adopt(bridge.ResponseHistoryAdoptArgs{TabID: "scratch-tab", ItemID: item.ID})
	if err != nil || ad.Adopted != 1 {
		t.Fatalf("Adopt = %+v (%v), want 1", ad, err)
	}
	if rows, _ := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{ItemID: item.ID}); len(rows) != 2 {
		t.Errorf("item history after adopt = %d rows, want 2", len(rows))
	}
	if rows, _ := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{TabID: "scratch-tab"}); len(rows) != 0 {
		t.Errorf("scratch tab still holds %d rows after adopt", len(rows))
	}

	mark = app.Events.Mark()
	app.RemoveAPIFile(t, col.File)
	ch = apiChanges(t, app, mark)
	if len(ch) == 0 || !hasChange(ch[0], bridge.ApiDataChange{Kind: bridge.ApiDataTree}) {
		t.Errorf("collection file removal events = %+v, want a tree change", ch)
	}
	if rows, _ := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{ItemID: item.ID}); len(rows) != 0 {
		t.Errorf("deleting the collection left %d history rows for its item", len(rows))
	}
	if _, err := app.W.Collections.GetRequest(bridge.CollectionsItemArgs{ItemID: item.ID}); ipcErr(t, err).Code != "E_NOT_FOUND" {
		t.Errorf("GetRequest after delete = %v, want E_NOT_FOUND", err)
	}
}

func TestVariablePrecedence(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)
	col := app.SeedCollection(t, "scoped", studioflowharness.APIVar{Name: "who", Value: "from-collection", Secret: true})
	dev := app.SeedEnvironment(t, "dev", studioflowharness.APIVar{Name: "who", Value: "from-dev", Secret: true})
	prod := app.SeedEnvironment(t, "prod", studioflowharness.APIVar{Name: "who", Value: "from-prod", Secret: true})

	sendWho := func(envID string) string {
		t.Helper()
		args := get(srv.URL + "/echo?who={{who}}")
		args.CollectionID, args.EnvironmentID = col.ID, envID
		send(t, app, args)
		return lastRequest(srv).Query
	}
	if got := sendWho(dev.ID); got != "who=from-dev" {
		t.Errorf("with dev = %q, want the environment value over the collection's", got)
	}
	if got := sendWho(prod.ID); got != "who=from-prod" {
		t.Errorf("with prod = %q, want who=from-prod", got)
	}
	if got := sendWho(""); got != "who=from-collection" {
		t.Errorf("with no environment = %q, want the collection value", got)
	}

	if err := app.W.Variables.SetActiveEnvironment(bridge.VariablesEnvironmentIDArgs{ID: prod.ID}); err != nil {
		t.Fatal(err)
	}
	envs, err := app.W.Variables.ListEnvironments()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range envs {
		if e.IsActive != (e.ID == prod.ID) {
			t.Errorf("environment %s active = %v", e.Name, e.IsActive)
		}
	}

	t.Run("reveal", func(t *testing.T) {
		vars, err := app.W.Variables.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeEnvironment, OwnerID: dev.ID})
		if err != nil || len(vars) != 1 || !vars[0].IsSecret || vars[0].Value != "" {
			t.Fatalf("variables = %+v (%v), want one secret without a value", vars, err)
		}
		denied := app.W.Variables.Reveal(bridge.VariablesRevealArgs{VariableID: vars[0].ID})
		if denied.Outcome != "confirmation-required" || denied.Value != nil {
			t.Errorf("unconfirmed reveal = %+v, want confirmation-required without a value", denied)
		}
		ok := app.W.Variables.Reveal(bridge.VariablesRevealArgs{VariableID: vars[0].ID, Confirmed: true})
		if ok.Outcome != "revealed" || ok.Value == nil || *ok.Value != "from-dev" {
			t.Errorf("confirmed reveal = %+v, want from-dev", ok)
		}
	})

	t.Run("a changed secret is used by the next send", func(t *testing.T) {
		if _, err := app.W.ApiConfig.SetSecret(bridge.ApiSecretArgs{Name: "dev.who", Value: "from-dev-2"}); err != nil {
			t.Fatal(err)
		}
		if got := sendWho(dev.ID); got != "who=from-dev-2" {
			t.Errorf("after the change = %q, want the new value", got)
		}
		vars, err := app.W.Variables.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeEnvironment, OwnerID: dev.ID})
		if err != nil || len(vars) != 1 {
			t.Fatalf("variables = %+v (%v), want one", vars, err)
		}
		// A history row from before secrets moved to the secret store still carries its sealed value.
		sealed, err := secrets.New().Encrypt(secrets.ScopeVariableHistory, "legacy-old")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := app.DB().Exec(`INSERT INTO api_variable_history (id, variable_id, value, is_secret, secret_value, recorded_at)
			VALUES ('legacy-h1', ?, '', 1, ?, '2026-01-01T00:00:00Z')`, vars[0].ID, sealed); err != nil {
			t.Fatal(err)
		}
		old := app.W.Variables.RevealHistory(bridge.VariablesRevealHistoryArgs{HistoryID: "legacy-h1", Confirmed: true})
		if old.Outcome != "revealed" || old.Value == nil || *old.Value != "legacy-old" {
			t.Errorf("history reveal = %+v, want legacy-old", old)
		}
	})

	t.Run("a plain value edited in the file keeps its history", func(t *testing.T) {
		plain := app.SeedEnvironment(t, "plain", studioflowharness.APIVar{Name: "mode", Value: "one"})
		app.SeedEnvironment(t, "plain", studioflowharness.APIVar{Name: "mode", Value: "two"})
		vars, err := app.W.Variables.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeEnvironment, OwnerID: plain.ID})
		if err != nil || len(vars) != 1 || vars[0].Value != "two" {
			t.Fatalf("variables = %+v (%v), want mode=two", vars, err)
		}
		hist, err := app.W.Variables.History(bridge.VariablesHistoryArgs{VariableID: vars[0].ID})
		if err != nil || len(hist) != 1 || hist[0].Value != "one" {
			t.Fatalf("history = %+v (%v), want the replaced value one", hist, err)
		}
	})
}

// tree flattens a collection into comparable lines: folder path, kind, method, url and body.
func tree(t *testing.T, app *studioflowharness.App, collectionID string) []string {
	t.Helper()
	all, err := app.W.Collections.List()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]model.CollectionItem{}
	for _, it := range all.Items {
		if it.CollectionID == collectionID {
			byID[it.ID] = it
		}
	}
	path := func(it model.CollectionItem) string {
		parts := []string{it.Name}
		for it.ParentID != nil {
			it = byID[*it.ParentID]
			parts = append([]string{it.Name}, parts...)
		}
		return strings.Join(parts, "/")
	}
	var lines []string
	for _, it := range byID {
		line := path(it) + "|" + it.Kind + "|" + it.Method + "|" + it.URL
		if it.Kind == model.CollectionItemRequest && it.Protocol == model.ItemProtocolHTTP {
			r, err := app.W.Collections.GetRequest(bridge.CollectionsItemArgs{ItemID: it.ID})
			if err != nil {
				t.Fatal(err)
			}
			line += "|" + r.BodyMode + "|" + r.Body + "|" + r.Code + "|" + r.CodeLanguage + "|" + mustHeaders(r)
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	return lines
}

func mustHeaders(r model.SavedRequest) string {
	var parts []string
	for _, h := range r.Headers {
		parts = append(parts, h.Name+"="+h.Value)
	}
	return strings.Join(parts, ";")
}

func TestPostmanImportSendExport(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)

	fixture, err := os.ReadFile(filepath.Join("testdata", "collection.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	imported := filepath.Join(dir, "collection.json")
	if err := os.WriteFile(imported, []byte(strings.ReplaceAll(string(fixture), "__BASE_URL__", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}

	// Contract api-postman, read by tests/ui/collections.spec.ts "Postman import builds the tree
	// and export asks for a save path".
	const sc = "api-postman"
	pathOpts := []studioflowharness.ContractOption{studioflowharness.Replace(dir, "<tmp>"), studioflowharness.Replace(srv.URL, "<http>")}
	app.Dialogs.QueueOpenFiles(imported)
	chosen, err := app.W.Files.ChooseOpenMany(bridge.FilesChooseOpenArgs{})
	if err != nil || len(chosen.Files) != 1 {
		t.Fatalf("ChooseOpenMany = %+v (%v)", chosen, err)
	}
	app.Contract(t, sc, "FilesService.ChooseOpenMany", chosen, append(pathOpts, studioflowharness.Mask("size"))...)
	importArgs := bridge.CollectionsImportArgs{Paths: []string{chosen.Files[0].Path}}
	app.Contract(t, sc, "args:CollectionsService.Import", importArgs, pathOpts...)
	res, err := app.W.Collections.Import(importArgs)
	if err != nil || len(res.Files) != 1 || res.Files[0].Collection == nil {
		t.Fatalf("Import = %+v (%v)", res, err)
	}
	app.Contract(t, sc, "CollectionsService.Import", res, pathOpts...)
	rep := *res.Files[0].Collection
	if rep.Name != "Flow Orders" || rep.Folders != 1 || rep.Requests != 4 {
		t.Errorf("report = %+v, want Flow Orders with 1 folder and 4 requests", rep)
	}
	warnings := map[string]int{}
	for _, w := range rep.Warnings {
		warnings[w.Kind] = w.Count
	}
	for kind, count := range map[string]int{
		postman.WarnScriptsInert: 3, postman.WarnAuthInert: 2, postman.WarnVariablesImported: 2,
		postman.WarnGraphQLBody: 1, postman.WarnUnresolvedFile: 1,
	} {
		if warnings[kind] != count {
			t.Errorf("warning %s = %d, want %d (all: %v)", kind, warnings[kind], count, warnings)
		}
	}

	// A collection secret arrives unset; the user sets it, and the send then resolves it on the wire.
	view, err := app.W.ApiConfig.View()
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Secrets) != 1 || view.Secrets[0].Set || !view.Secrets[0].Referenced {
		t.Fatalf("secrets after import = %+v, want one referenced and unset", view.Secrets)
	}
	if _, err := app.W.ApiConfig.SetSecret(bridge.ApiSecretArgs{Name: view.Secrets[0].Name, Value: srv.URL}); err != nil {
		t.Fatal(err)
	}
	items, err := app.W.Collections.List()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "CollectionsService.List", items, pathOpts...)
	var create model.CollectionItem
	for _, it := range items.Items {
		if it.Name == "Create order" && it.CollectionID == rep.CollectionID {
			create = it
		}
	}
	saved, err := app.W.Collections.GetRequest(bridge.CollectionsItemArgs{ItemID: create.ID})
	if err != nil {
		t.Fatal(err)
	}
	args := get(saved.URL)
	args.Method, args.CollectionID, args.ItemID = saved.Method, rep.CollectionID, create.ID
	args.Body.Mode, args.Body.Code, args.Body.CodeLanguage = "code", saved.Code, saved.CodeLanguage
	for _, h := range saved.Headers {
		args.Headers = append(args.Headers, headerOf(h))
	}
	send(t, app, args)
	got := lastRequest(srv)
	if got.Method != "POST" || got.Path != "/echo" || got.Header.Get("X-Flow") != "orders" || string(got.Body) != `{"sku":"x"}` {
		t.Errorf("server saw %s %s X-Flow=%q body %q, want the imported request", got.Method, got.Path, got.Header.Get("X-Flow"), got.Body)
	}
	if got.Query != "v={{apiVersion}}" {
		t.Errorf("query = %q; a plain collection variable is left for the renderer", got.Query)
	}

	if _, err := app.W.Collections.CreateGrpcItem(bridge.CollectionsCreateGrpcItemArgs{
		CollectionID: rep.CollectionID, Name: "Stream orders",
		Request: &model.SavedGrpcRequest{TLSMode: "plaintext", DescriptorMode: "reflection", Target: "localhost:50051"},
	}); err != nil {
		t.Fatal(err)
	}

	exported := filepath.Join(dir, "export.json")
	app.Dialogs.QueueSaveFile(exported)
	dest, err := app.W.Files.ChooseSave(bridge.FilesChooseSaveArgs{DefaultName: "Flow Orders.postman_collection.json"})
	if err != nil || dest.FilePath == nil {
		t.Fatalf("ChooseSave = %+v (%v)", dest, err)
	}
	app.Contract(t, sc, "FilesService.ChooseSave", dest, pathOpts...)
	exportArgs := bridge.CollectionsExportArgs{CollectionID: rep.CollectionID, Path: *dest.FilePath}
	app.Contract(t, sc, "args:CollectionsService.Export", exportArgs, pathOpts...)
	exp, err := app.W.Collections.Export(exportArgs)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "CollectionsService.Export", exp, pathOpts...)
	if exp.SecretCount != 1 || exp.SkippedGrpc != 1 {
		t.Errorf("export = %+v, want 1 secret and 1 skipped gRPC item", exp)
	}
	raw, err := os.ReadFile(exported)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), srv.URL) {
		t.Error("the export carries the secret variable's value")
	}

	app.Dialogs.QueueOpenFile(exported)
	again, err := app.W.Files.ChooseOpen(bridge.FilesChooseOpenArgs{})
	if err != nil || again.File == nil {
		t.Fatal(err)
	}
	res2, err := app.W.Collections.Import(bridge.CollectionsImportArgs{Paths: []string{again.File.Path}})
	if err != nil || len(res2.Files) != 1 || res2.Files[0].Collection == nil {
		t.Fatalf("re-import = %+v (%v)", res2, err)
	}
	rep2 := *res2.Files[0].Collection
	if rep2.CollectionID == rep.CollectionID {
		t.Fatal("re-import reused the collection")
	}
	want := tree(t, app, rep.CollectionID)
	var wantHTTP []string
	for _, l := range want {
		if !strings.Contains(l, "Stream orders") {
			wantHTTP = append(wantHTTP, l)
		}
	}
	if gotTree := tree(t, app, rep2.CollectionID); strings.Join(gotTree, "\n") != strings.Join(wantHTTP, "\n") {
		t.Errorf("re-imported tree differs\n got: %q\nwant: %q", gotTree, wantHTTP)
	}
}

func headerOf(h model.SavedHeader) httpclient.Header {
	return httpclient.Header{Name: h.Name, Value: h.Value}
}

func TestGrpcItemSaveAndRestart(t *testing.T) {
	app := studioflowharness.New(t)
	cs := app.W.Collections
	from := newCollection(t, app, "from")
	folder, err := cs.CreateItem(bridge.CollectionsCreateItemArgs{CollectionID: from.ID, Kind: model.CollectionItemFolder, Name: "dir"})
	if err != nil {
		t.Fatal(err)
	}
	req := model.SavedGrpcRequest{
		Target: "localhost:50051", TLSMode: "plaintext", DescriptorMode: "reflection",
		Service: "kira.flow.v1.Flow", Method: "Unary", Message: `{"text":"hi"}`,
		Metadata: []model.SavedGrpcMetaRow{{Name: "x-id", Value: "1", Enabled: true}},
	}
	item, err := cs.CreateGrpcItem(bridge.CollectionsCreateGrpcItemArgs{CollectionID: from.ID, ParentID: &folder.ID, Name: "unary", Request: &req})
	if err != nil {
		t.Fatal(err)
	}
	if item.Protocol != model.ItemProtocolGrpc {
		t.Fatalf("created item protocol %q, want grpc", item.Protocol)
	}

	// SaveGrpcRequest replaces the body and the name; GetGrpcRequest reads it back.
	req.Message = `{"text":"edited"}`
	req.Metadata = append(req.Metadata, model.SavedGrpcMetaRow{Name: "x-trace", Value: "t", Enabled: true})
	if _, err := cs.SaveGrpcRequest(bridge.CollectionsSaveGrpcRequestArgs{ItemID: item.ID, Name: "unary v2", Request: req}); err != nil {
		t.Fatal(err)
	}
	got, err := cs.GetGrpcRequest(bridge.CollectionsItemArgs{ItemID: item.ID})
	if err != nil || got.Message != req.Message || len(got.Metadata) != 2 || got.Method != "Unary" {
		t.Fatalf("GetGrpcRequest = %+v (%v), want the edited request", got, err)
	}
	if _, err := cs.GetRequest(bridge.CollectionsItemArgs{ItemID: item.ID}); ipcErr(t, err).Code != "E_BAD_REQUEST" {
		t.Fatalf("GetRequest on a gRPC item = %v, want E_BAD_REQUEST", err)
	}
	if _, err := cs.GetGrpcRequest(bridge.CollectionsItemArgs{ItemID: "missing"}); ipcErr(t, err).Code != "E_NOT_FOUND" {
		t.Fatalf("GetGrpcRequest of an unknown item = %v, want E_NOT_FOUND", err)
	}

	check := func(label string) {
		t.Helper()
		all, err := cs.List()
		if err != nil {
			t.Fatal(err)
		}
		byID := map[string]model.CollectionItem{}
		for _, it := range all.Items {
			byID[it.ID] = it
		}
		if it := byID[item.ID]; it.Name != "unary v2" || it.ParentID == nil || *it.ParentID != folder.ID || it.Protocol != model.ItemProtocolGrpc {
			t.Fatalf("%s: item = %+v, want the saved name, still under the folder, gRPC", label, it)
		}
		if g, err := cs.GetGrpcRequest(bridge.CollectionsItemArgs{ItemID: item.ID}); err != nil || g.Message != req.Message {
			t.Fatalf("%s: request body = %+v (%v)", label, g, err)
		}
	}
	check("after save")
	app.Restart(t)
	cs = app.W.Collections
	check("after relaunch")

	// Postman has no gRPC item, so an export skips it; the rest of the collection still exports.
	dest := filepath.Join(t.TempDir(), "out.json")
	exp, err := cs.Export(bridge.CollectionsExportArgs{CollectionID: from.ID, Path: dest})
	if err != nil || exp.SkippedGrpc != 1 {
		t.Fatalf("Export = %+v (%v), want the gRPC item skipped", exp, err)
	}
}

func TestPostmanImportCollectionAndEnvironment(t *testing.T) {
	app := studioflowharness.New(t)
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	collection := write("collection.json", `{"info":{"name":"Multi","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},"variable":[{"key":"base","value":"https://example.test"},{"key":"blank","value":""},{"key":"cfg","value":{"k":[1]}},{"key":"off","value":"x","disabled":true},{"value":"k"}],"item":[{"name":"Ping","request":{"method":"GET","url":"https://example.test/ping"}},{"name":"Bare"}]}`)
	env, err := os.ReadFile(filepath.Join("testdata", "environment.json"))
	if err != nil {
		t.Fatal(err)
	}
	environment := write("environment.json", string(env))
	bad := write("bad.json", `{"hello":"world"}`)
	wrapped := write("wrapped.json", `{"environment":{"name":"Wrapped","values":[{"key":"a","value":"1"},{"key":"b","value":""},{"value":"z"}]}}`)

	// Contract api-postman-multi, read by tests/ui/collections.spec.ts "Postman import of a
	// collection, an environment and a bad file".
	const sc = "api-postman-multi"
	pathOpts := []studioflowharness.ContractOption{studioflowharness.Replace(dir, "<tmp>")}
	app.Dialogs.QueueOpenFiles(collection, environment, bad, wrapped)
	chosen, err := app.W.Files.ChooseOpenMany(bridge.FilesChooseOpenArgs{})
	if err != nil || len(chosen.Files) != 4 {
		t.Fatalf("ChooseOpenMany = %+v (%v)", chosen, err)
	}
	app.Contract(t, sc, "FilesService.ChooseOpenMany", chosen, append(pathOpts, studioflowharness.Mask("size"))...)
	paths := make([]string, len(chosen.Files))
	for i, f := range chosen.Files {
		paths[i] = f.Path
	}
	args := bridge.CollectionsImportArgs{Paths: paths}
	app.Contract(t, sc, "args:CollectionsService.Import", args, pathOpts...)
	mark := app.Events.Mark()
	res, err := app.W.Collections.Import(args)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "CollectionsService.Import", res, pathOpts...)

	if len(res.Files) != 4 {
		t.Fatalf("files = %+v, want one entry per path", res.Files)
	}
	warningCounts := func(ws []bridge.ImportWarning) map[string]int {
		m := map[string]int{}
		for _, w := range ws {
			m[w.Kind] = w.Count
		}
		return m
	}
	colEntry := res.Files[0]
	if colEntry.Kind != "collection" || colEntry.Collection == nil || colEntry.Collection.Requests != 2 {
		t.Fatalf("collection entry = %+v", colEntry)
	}
	if diff := cmp.Diff(map[string]int{
		postman.WarnVariablesImported: 4, postman.WarnEmptyValues: 1, postman.WarnDisabledVariables: 1,
		postman.WarnUnnamedVariables: 1, postman.WarnMissingRequest: 1,
	}, warningCounts(colEntry.Collection.Warnings)); diff != "" {
		t.Errorf("collection warnings (-want +got):\n%s", diff)
	}
	envEntry := res.Files[1]
	if envEntry.Kind != "environment" || envEntry.Environment == nil || envEntry.Environment.Variables != 6 {
		t.Fatalf("environment entry = %+v, want 6 variables", envEntry)
	}
	if diff := cmp.Diff(map[string]int{postman.WarnDisabledVariables: 1, postman.WarnEmptyValues: 1},
		warningCounts(envEntry.Environment.Warnings)); diff != "" {
		t.Errorf("environment warnings (-want +got):\n%s", diff)
	}
	if res.Files[2].Kind != "" || res.Files[2].Error == nil {
		t.Errorf("bad file entry = %+v, want a failure", res.Files[2])
	}
	wrapEntry := res.Files[3]
	if wrapEntry.Kind != "environment" || wrapEntry.Environment == nil || wrapEntry.Environment.Name != "Wrapped" ||
		wrapEntry.Environment.Variables != 3 ||
		!cmp.Equal(map[string]int{postman.WarnEmptyValues: 1, postman.WarnUnnamedVariables: 1}, warningCounts(wrapEntry.Environment.Warnings)) {
		t.Errorf("wrapped entry = %+v, want Wrapped with 3 variables, one empty and one unnamed", wrapEntry)
	}

	if got := tree(t, app, colEntry.Collection.CollectionID); len(got) != 2 {
		t.Errorf("collection tree = %q, want Ping and Bare", got)
	}
	colVars, err := app.W.Variables.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeCollection, OwnerID: colEntry.Collection.CollectionID})
	if err != nil {
		t.Fatal(err)
	}
	gotCol := map[string]string{}
	for _, v := range colVars {
		gotCol[v.Name] = v.Value
	}
	if diff := cmp.Diff(map[string]string{"base": "https://example.test", "blank": "", "cfg": `{"k":[1]}`, "unnamed_1": "k"}, gotCol); diff != "" {
		t.Errorf("collection variables (-want +got):\n%s", diff)
	}
	envs, err := app.W.Variables.ListEnvironments()
	if err != nil || len(envs) != 2 {
		t.Fatalf("environments = %+v (%v), want Staging and Wrapped", envs, err)
	}
	envIDs := map[string]string{}
	for _, e := range envs {
		envIDs[e.Name] = e.ID
	}
	vars, err := app.W.Variables.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeEnvironment, OwnerID: envIDs["Staging"]})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "VariablesService.List", vars, studioflowharness.Mask("ownerId", "sortOrder"))
	got := map[string]model.Variable{}
	for _, v := range vars {
		got[v.Name] = v
	}
	if len(got) != 6 || got["token"].Value != "" || !got["token"].IsSecret || got["host"].Value != "https://staging.example.test" ||
		got["config"].Value != `{"retry":[1,2]}` || got["blank"].Value != "" || got["legacy"].Name != "" {
		t.Errorf("variables = %+v, want 6 with a blank-listed secret, a compact object value and the disabled one skipped", got)
	}
	var all []bridge.ApiDataChange
	for _, c := range apiChanges(t, app, mark) {
		all = append(all, c...)
	}
	if !hasChange(all, bridge.ApiDataChange{Kind: bridge.ApiDataTree}) || !hasChange(all, bridge.ApiDataChange{Kind: bridge.ApiDataEnvironments}) {
		t.Errorf("change events = %+v, want a tree and an environments change", all)
	}
}
