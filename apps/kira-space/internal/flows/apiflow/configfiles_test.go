package apiflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/configfs/configsvc"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
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
	paths := flowharness.Replace(app.SpaceHome, "<home>")

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
	openArgs := configsvc.FileArgs{Folder: "api", File: col.File}
	if err := app.W.ConfigFiles.Open(openArgs); err != nil {
		t.Fatal(err)
	}
	if err := app.W.ConfigFiles.Reveal(openArgs); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(app.ConfigDir("api"), col.File); app.FileOpener.OpenedFiles()[0] != want || app.FileOpener.RevealedFiles()[0] != want {
		t.Fatalf("opened %v, revealed %v, want %s", app.FileOpener.OpenedFiles(), app.FileOpener.RevealedFiles(), want)
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
	bad := strings.Replace(string(ordersFile(srv.URL, "list")), "items:\n", "bogus: true\nitems:\n", 1)
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
	for _, f := range view.Files {
		if f.File == "orders-api.collection.yaml" && len(f.Skipped) != 0 {
			t.Errorf("skipped = %+v, want none for a whole-file problem", f.Skipped)
		}
	}
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

func skippedOf(v apifiles.View, file string) []apifiles.SkippedView {
	for _, f := range v.Files {
		if f.File == file {
			return f.Skipped
		}
	}
	return nil
}

// J-COLL-003. A person hand-edits a collection and leaves one request with no protocol node between
// two good ones. The good items load, the bad one is reported with its line, an item that loaded
// before keeps its last good row and history, app edits are refused, and the file is never rewritten.
func TestHandEditedCollectionWithBadEntry(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	const name = "hand.collection.yaml"
	path := filepath.Join(app.W.ApiFiles.Dir(), name)
	item := func(id, title string) string {
		return "  - id: " + id + "\n    name: " + title + "\n    http:\n      method: GET\n      url: https://x.test/" + id + "\n"
	}
	noProto := "  - id: bad\n    name: No protocol\n"
	head := "# hand written\nname: Hand\nitems:\n"
	text := head + item("ok1", "Good one") + noProto + item("ok2", "Good two")
	rows := func() []model.CollectionItem {
		t.Helper()
		tree, err := app.W.Collections.List()
		if err != nil {
			t.Fatal(err)
		}
		return tree.Items
	}
	names := func() string {
		var out []string
		for _, it := range rows() {
			out = append(out, it.Name)
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	orders := func() string {
		var out []string
		for _, it := range rows() {
			out = append(out, it.Name+"="+strconv.Itoa(it.SortOrder))
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	view := func() apifiles.View {
		t.Helper()
		v, err := app.W.ApiConfig.View()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	lineOf := func(src, marker string) int {
		i := strings.Index(src, marker)
		if i < 0 {
			t.Fatalf("marker %q not in source", marker)
		}
		return 1 + strings.Count(src[:i], "\n")
	}
	kindMsg := i18n.T("backend.studio.apiFiles.itemKind.body", nil)
	wantBad := []apifiles.SkippedView{{Line: 9, Index: 1, ID: "bad", Name: "No protocol", Message: kindMsg}}

	// a. First write is bad: the valid items load.
	app.WriteAPIFile(t, name, []byte(text))
	if got := names(); got != "Good one,Good two" {
		t.Fatalf("rows = %q, want the two good items", got)
	}
	if got := orders(); got != "Good one=0,Good two=2" {
		t.Errorf("orders = %q, want the file indices", got)
	}
	v := view()
	if p := problemOf(v, name); p != nil {
		t.Errorf("problem = %+v, want none", p)
	}
	for _, f := range v.Files {
		if f.File == name && f.LastGood {
			t.Error("lastGood = true, want false on a partial load")
		}
	}
	if got := skippedOf(v, name); !reflect.DeepEqual(got, wantBad) {
		t.Fatalf("skipped = %+v, want %+v", got, wantBad)
	}
	paths := flowharness.Replace(app.SpaceHome, "<home>")
	tree, err := app.W.Collections.List()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, "api-collection-skipped", "CollectionsService.List", tree, paths)
	app.Contract(t, "api-collection-skipped", "ApiConfigService.View", v, paths)

	// b. App edits are refused while an item is skipped; the file stays as written.
	ok1 := rows()[0]
	for _, it := range rows() {
		if it.Name == "Good one" {
			ok1 = it
		}
	}
	req, err := app.W.Collections.GetRequest(bridge.CollectionsItemArgs{ItemID: ok1.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.W.Collections.SaveRequest(bridge.CollectionsSaveRequestArgs{ItemID: ok1.ID, Name: "x", Request: req})
	if ie := ipcErr(t, err); ie.Code != "E_BAD_REQUEST" || !strings.Contains(ie.Message, "Line 9") {
		t.Errorf("SaveRequest = %v, want E_BAD_REQUEST naming line 9", ie)
	}
	if got, _ := os.ReadFile(path); string(got) != text {
		t.Errorf("file changed by a refused edit:\n%s", got)
	}

	// c. A restart leaves the file and the partial load as they were.
	app.Restart()
	if got, _ := os.ReadFile(path); string(got) != text {
		t.Errorf("the app rewrote the hand-edited file:\n%s", got)
	}
	if got := names(); got != "Good one,Good two" {
		t.Errorf("rows after restart = %q", got)
	}
	if got := skippedOf(view(), name); !reflect.DeepEqual(got, wantBad) {
		t.Errorf("skipped after restart = %+v", got)
	}

	// d. Fixing the item loads it in file order.
	fixedBad := noProto + "    http:\n      method: GET\n      url: https://x.test/bad\n"
	fixed := head + item("ok1", "Good one") + fixedBad + item("ok2", "Good two")
	app.WriteAPIFile(t, name, []byte(fixed))
	if got := orders(); got != "Good one=0,Good two=2,No protocol=1" {
		t.Errorf("orders after the fix = %q, want 0,1,2 in file order", got)
	}
	if got := names(); got != "Good one,Good two,No protocol" {
		t.Errorf("rows after the fix = %q", got)
	}
	if got := skippedOf(view(), name); len(got) != 0 {
		t.Errorf("skipped after the fix = %+v, want none", got)
	}

	// e. An item that turns bad after a good load keeps its row and history.
	var ok2 model.CollectionItem
	var colID string
	for _, it := range rows() {
		if it.Name == "Good two" {
			ok2, colID = it, it.CollectionID
		}
	}
	args := get(srv.URL + "/status?code=200")
	args.CollectionID, args.ItemID = colID, ok2.ID
	send(t, app, args)
	history := func() int {
		r, err := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{ItemID: ok2.ID})
		if err != nil {
			t.Fatal(err)
		}
		return len(r)
	}
	if history() != 1 {
		t.Fatalf("history before = %d, want 1", history())
	}
	broken := head + item("ok1", "Good one renamed") + fixedBad + "  - id: ok2\n    name: Good two\n"
	app.WriteAPIFile(t, name, []byte(broken))
	var renamed, kept bool
	for _, it := range rows() {
		renamed = renamed || it.Name == "Good one renamed"
		if it.ID == ok2.ID && it.Name == "Good two" {
			kept = true
		}
	}
	if !renamed || !kept {
		t.Errorf("rows = %q, want ok1 renamed and the ok2 row kept with its old name", names())
	}
	if history() != 1 {
		t.Errorf("history after = %d, want 1 still attached", history())
	}
	wantOK2 := []apifiles.SkippedView{{Line: lineOf(broken, "  - id: ok2"), Index: 2, ID: "ok2", Name: "Good two", Message: kindMsg}}
	if got := skippedOf(view(), name); !reflect.DeepEqual(got, wantOK2) {
		t.Errorf("skipped = %+v, want %+v", got, wantOK2)
	}

	// f. Nested and duplicate skips.
	const nest = "nest.collection.yaml"
	nestText := `name: Nest
items:
  - id: dir
    name: Folder
    items:
      - id: child-bad
        name: Bad child
        http:
          method: GET
          bogus: 1
      - id: child-ok
        name: Good child
        http:
          method: GET
          url: https://x.test/c
  - id: dup
    name: First
    http:
      method: GET
  - id: dup
    name: Second
    http:
      method: GET
`
	app.WriteAPIFile(t, nest, []byte(nestText))
	var folderID string
	byName := map[string]model.CollectionItem{}
	for _, it := range rows() {
		if slices.Contains([]string{"Folder", "Good child", "First", "Second"}, it.Name) {
			byName[it.Name] = it
		}
	}
	folderID = byName["Folder"].ID
	if _, ok := byName["Second"]; ok || byName["First"].ID == "" || folderID == "" {
		t.Errorf("rows = %+v, want Folder, Good child and First only", byName)
	}
	if p := byName["Good child"].ParentID; p == nil || *p != folderID {
		t.Errorf("Good child parent = %v, want the Folder row", p)
	}
	additional := i18n.T("backend.core.config.additional.body", map[string]string{"field": "items.0.items.0.http.bogus"})
	wantNest := []apifiles.SkippedView{
		{Line: 10, Index: 0, ID: "child-bad", Name: "Bad child", Message: additional},
		{Line: 20, Index: 2, ID: "dup", Name: "Second", Message: i18n.T("backend.studio.apiFiles.itemDuplicate.body", map[string]string{"id": "dup"})},
	}
	if got := skippedOf(view(), nest); !reflect.DeepEqual(got, wantNest) {
		t.Errorf("skipped = %+v, want %+v", got, wantNest)
	}
}
