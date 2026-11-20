package apiflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/fileservice"
	"github.com/kirathecat/kira-studio/internal/testx"
	"go.yaml.in/yaml/v3"
)

// Decision 3, API request in two windows (OQ-API-6). Both windows read the request, the first
// saves, the second then saves from what it read: the backend refuses with a conflict that carries
// `changed`, the file keeps the first save, and only a resend without the guard (the user's
// confirm) overwrites. An edit made in the file by hand counts as another window too.
func TestSavedRequestTwoWindows(t *testing.T) {
	app := flowharness.New(t)
	cs := app.W.Collections
	col := newCollection(t, app, "Two windows")
	read := savedGet()
	read.URL = "https://x.test/read"
	item, err := cs.CreateItem(bridge.CollectionsCreateItemArgs{CollectionID: col.ID, Kind: "request", Name: "shared", Request: &read})
	if err != nil {
		t.Fatal(err)
	}
	stored := func() string {
		t.Helper()
		r, err := cs.GetRequest(bridge.CollectionsItemArgs{ItemID: item.ID})
		if err != nil {
			t.Fatal(err)
		}
		return r.URL
	}
	const sc = "api-request-two-windows"
	mask := flowharness.Mask("itemId", "id", "collectionId", "createdAt", "updatedAt")
	save := func(url string, expect *model.SavedRequest, tag string) error {
		req := read
		req.URL = url
		args := bridge.CollectionsSaveRequestArgs{ItemID: item.ID, Name: "shared", Request: req, Expect: expect}
		if expect != nil {
			args.ExpectName = ptr("shared")
		}
		if tag != "" {
			app.Contract(t, sc, "args:SaveRequest#"+tag, args, mask)
		}
		got, err := cs.SaveRequest(args)
		if tag == "confirmed" {
			app.Contract(t, sc, "SaveRequest#confirmed", got, mask)
		}
		return err
	}

	// Window A saves from what it read; window B is still on the same read.
	if err := save("https://x.test/a", &read, ""); err != nil {
		t.Fatalf("the first save: %v", err)
	}
	err = save("https://x.test/b", &read, "guarded")
	ie := testx.AsIpcErr(t, err)
	var details map[string]any
	if ie.Code != "E_CONFLICT" || json.Unmarshal(ie.Details, &details) != nil || details["changed"] != true {
		t.Fatalf("the second save = %v (details %s), want E_CONFLICT with changed", err, ie.Details)
	}
	app.Contract(t, sc, "SaveRequest#changed", map[string]any{"code": ie.Code, "message": ie.Message, "details": details})
	if got := stored(); got != "https://x.test/a" {
		t.Fatalf("stored after the refused save = %q, want window A's", got)
	}

	// Confirm: the resend carries no guard and wins.
	if err := save("https://x.test/b", nil, "confirmed"); err != nil {
		t.Fatal(err)
	}
	if got := stored(); got != "https://x.test/b" {
		t.Fatalf("stored after the confirmed overwrite = %q, want window B's", got)
	}

	// A save from the current read goes through, and so does one that changed nothing the file holds.
	current := read
	current.URL = "https://x.test/b"
	if err := save("https://x.test/c", &current, ""); err != nil {
		t.Fatalf("save from the current read: %v", err)
	}

	// Another window renames the request. A save from a body read before the rename refuses: it
	// would write the old name back, a rename lost without a prompt.
	if err := cs.Rename(bridge.CollectionsTargetArgs{ID: item.ID, Target: "item", Name: "renamed"}); err != nil {
		t.Fatal(err)
	}
	current.URL = "https://x.test/c"
	if ie := testx.AsIpcErr(t, save("https://x.test/d", &current, "")); ie.Code != "E_CONFLICT" {
		t.Fatalf("save after a rename = %v, want E_CONFLICT", ie)
	}
	if got := itemName(t, cs, item.ID); got != "renamed" {
		t.Fatalf("name after the refused save = %q, want the other window's rename", got)
	}
	if err := cs.Rename(bridge.CollectionsTargetArgs{ID: item.ID, Target: "item", Name: "shared"}); err != nil {
		t.Fatal(err)
	}

	// A hand edit of the file is another window.
	file := filepath.Join(app.W.ApiFiles.Dir(), col.File)
	raw, err := fileservice.Host().ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	edited := []byte(strings.Replace(string(raw), "https://x.test/c", "https://x.test/by-hand", 1))
	if err := fileservice.Host().WriteFile(file, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	app.W.ApiFiles.Sync()
	seen := current
	seen.URL = "https://x.test/c"
	if ie := testx.AsIpcErr(t, save("https://x.test/d", &seen, "")); ie.Code != "E_CONFLICT" {
		t.Errorf("save over a hand edit = %v, want E_CONFLICT", ie)
	}
	if got, _ := fileservice.Host().ReadFile(file); !bytes.Equal(got, edited) {
		t.Error("a refused save changed the file")
	}
}

// J-ENV-011. Two windows edit a collection's variables and an environment's at the same time.
// There is no global scope in the app, so the scopes are the collection, the environment and the
// app-wide active selection. Saves go through the file hash: a window on a stale hash is refused
// and writes nothing, re-reads and applies its edit again, and both edits end up in valid files.
// Every change reaches both windows as a kira:api:dataChanged event.
func TestVariablesTwoWindows(t *testing.T) {
	app := flowharness.New(t)
	cfg := app.W.ApiConfig
	col := app.SeedCollection(t, "Shared col", flowharness.APIVar{Name: "base", Value: "one"})
	env := app.SeedEnvironment(t, "Shared env", flowharness.APIVar{Name: "base", Value: "one"})
	other := app.SeedEnvironment(t, "Other env", flowharness.APIVar{Name: "base", Value: "other"})

	// edit adds one variable on top of whatever the file holds when the window read it.
	type window struct {
		text, hash string
	}
	read := func(file string) window {
		t.Helper()
		v, err := cfg.File(bridge.ApiFileArgs{File: file})
		if err != nil || v.Problem != nil {
			t.Fatalf("File %s = %+v (%v)", file, v, err)
		}
		return window{text: v.Text, hash: v.Hash}
	}
	edit := func(file string, w window, name string) error {
		res, err := cfg.Form(bridge.ApiFormArgs{File: file, Text: w.text, Edits: []configfs.Edit{
			{Path: []string{"variables"}, Value: append(variablesOf(t, w.text), map[string]any{"name": name, "value": "from-" + name})},
		}})
		if err != nil || res.Problem != nil {
			return fmt.Errorf("Form %s = %+v (%v)", file, res, err)
		}
		_, err = cfg.SaveFile(bridge.ApiSaveFileArgs{File: file, Text: res.Text, BaseHash: w.hash})
		return err
	}

	mark := app.Events.Mark()
	for _, file := range []string{col.File, env.File} {
		a, b := read(file), read(file)
		if err := edit(file, a, "from_a"); err != nil {
			t.Fatalf("window A on %s: %v", file, err)
		}
		if ie := testx.AsIpcErr(t, edit(file, b, "from_b")); ie.Code != "E_CONFLICT" {
			t.Fatalf("window B on %s with a stale hash = %v, want E_CONFLICT", file, ie)
		}
		if v := read(file); contains([]byte(v.text), "from_b") {
			t.Fatalf("%s holds window B's refused edit", file)
		}
		// B re-reads and applies its edit again.
		if err := edit(file, read(file), "from_b"); err != nil {
			t.Fatalf("window B retry on %s: %v", file, err)
		}
	}
	if err := app.W.Variables.SetActiveEnvironment(bridge.VariablesEnvironmentIDArgs{ID: other.ID}); err != nil {
		t.Fatal(err)
	}

	for scope, owner := range map[model.VariableScope]string{model.VariableScopeCollection: col.ID, model.VariableScopeEnvironment: env.ID} {
		vars, err := app.W.Variables.List(bridge.VariablesScopeArgs{Scope: scope, OwnerID: owner})
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, v := range vars {
			got[v.Name] = v.Value
		}
		if len(got) != 3 || got["from_a"] != "from-from_a" || got["from_b"] != "from-from_b" || got["base"] != "one" {
			t.Errorf("%s variables = %v, want base, from_a and from_b", scope, got)
		}
	}
	var kinds = map[string]int{}
	for _, changes := range apiChanges(t, app, mark) {
		for _, c := range changes {
			kinds[fmt.Sprintf("%s/%s", c.Kind, c.OwnerID)]++
		}
	}
	for _, want := range []string{"variables/" + col.ID, "variables/" + env.ID, "environments/"} {
		if kinds[want] == 0 {
			t.Errorf("no %s change event (got %v)", want, kinds)
		}
	}
	envs, _ := app.W.Variables.ListEnvironments()
	for _, e := range envs {
		if e.IsActive != (e.ID == other.ID) {
			t.Errorf("environment %q active = %v after the selection changed", e.Name, e.IsActive)
		}
	}

	// The two writers racing on different files never leave a partial write behind.
	var wg sync.WaitGroup
	for _, file := range []string{col.File, env.File} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 15; i++ {
				w := read(file)
				if err := edit(file, w, fmt.Sprintf("race_%d", i)); err != nil {
					t.Errorf("race edit on %s: %v", file, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	for _, file := range []string{col.File, env.File} {
		if v, err := cfg.File(bridge.ApiFileArgs{File: file}); err != nil || v.Problem != nil {
			t.Errorf("%s after the race = %+v (%v), want a valid file", file, v.Problem, err)
		}
	}
}

// variablesOf is the file's variables as generic maps, ready to be sent back as a list replace.
func variablesOf(t *testing.T, text string) []any {
	t.Helper()
	var doc struct {
		Variables []map[string]any `yaml:"variables"`
	}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatal(err)
	}
	out := make([]any, len(doc.Variables))
	for i, v := range doc.Variables {
		out[i] = v
	}
	return out
}

func itemName(t *testing.T, cs *bridge.CollectionsService, id string) string {
	t.Helper()
	tree, err := cs.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range tree.Items {
		if it.ID == id {
			return it.Name
		}
	}
	t.Fatalf("item %s not listed", id)
	return ""
}

// Decision 3 for a gRPC saved request (contract api-grpc-two-windows, UI half
// tests/ui/studio/grpc-request-conflict.spec.ts): the same two-window rules as an HTTP request. A
// save from what the window read is refused after another window saved or renamed the item, and
// only a resend without the guard, after the user confirms, overwrites.
func TestSavedGrpcRequestTwoWindows(t *testing.T) {
	app := flowharness.New(t)
	cs := app.W.Collections
	col := newCollection(t, app, "Two windows")
	read := model.SavedGrpcRequest{
		Target: "localhost:50051", TLSMode: "plaintext", DescriptorMode: "reflection",
		Service: "kira.flow.v1.Flow", Method: "Unary", Message: `{"text":"read"}`,
	}
	item, err := cs.CreateGrpcItem(bridge.CollectionsCreateGrpcItemArgs{CollectionID: col.ID, Name: "shared", Request: &read})
	if err != nil {
		t.Fatal(err)
	}
	stored := func() string {
		t.Helper()
		r, err := cs.GetGrpcRequest(bridge.CollectionsItemArgs{ItemID: item.ID})
		if err != nil {
			t.Fatal(err)
		}
		return r.Message
	}
	const sc = "api-grpc-two-windows"
	mask := flowharness.Mask("itemId", "id", "collectionId", "createdAt", "updatedAt")
	save := func(message string, expect *model.SavedGrpcRequest, tag string) error {
		req := read
		req.Message = message
		args := bridge.CollectionsSaveGrpcRequestArgs{ItemID: item.ID, Name: "shared", Request: req, Expect: expect}
		if expect != nil {
			args.ExpectName = ptr("shared")
		}
		if tag != "" {
			app.Contract(t, sc, "args:SaveGrpcRequest#"+tag, args, mask)
		}
		got, err := cs.SaveGrpcRequest(args)
		if tag == "confirmed" {
			app.Contract(t, sc, "SaveGrpcRequest#confirmed", got, mask)
		}
		return err
	}

	if err := save(`{"text":"a"}`, &read, ""); err != nil {
		t.Fatalf("the first save: %v", err)
	}
	err = save(`{"text":"b"}`, &read, "guarded")
	ie := testx.AsIpcErr(t, err)
	var details map[string]any
	if ie.Code != "E_CONFLICT" || json.Unmarshal(ie.Details, &details) != nil || details["changed"] != true {
		t.Fatalf("the second save = %v (details %s), want E_CONFLICT with changed", err, ie.Details)
	}
	app.Contract(t, sc, "SaveGrpcRequest#changed", map[string]any{"code": ie.Code, "message": ie.Message, "details": details})
	if got := stored(); got != `{"text":"a"}` {
		t.Fatalf("stored after the refused save = %q, want window A's", got)
	}

	if err := save(`{"text":"b"}`, nil, "confirmed"); err != nil {
		t.Fatal(err)
	}
	if got := stored(); got != `{"text":"b"}` {
		t.Fatalf("stored after the confirmed overwrite = %q, want window B's", got)
	}

	// A rename in another window refuses a save from a body read before it.
	current := read
	current.Message = `{"text":"b"}`
	if err := cs.Rename(bridge.CollectionsTargetArgs{ID: item.ID, Target: "item", Name: "renamed"}); err != nil {
		t.Fatal(err)
	}
	if ie := testx.AsIpcErr(t, save(`{"text":"c"}`, &current, "")); ie.Code != "E_CONFLICT" {
		t.Fatalf("save after a rename = %v, want E_CONFLICT", ie)
	}
	if got := itemName(t, cs, item.ID); got != "renamed" {
		t.Fatalf("name after the refused save = %q, want the other window's rename", got)
	}
}
