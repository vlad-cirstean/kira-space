package apiflow

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// Contract api-lifecycle, read by tests/ui/api-lifecycle.spec.ts: the tree menu renames, moves and
// deletes as file edits (comments outside the edited node stay), a move into another collection file
// makes new rows and answers an id map, history follows a moved request, and an environment is
// duplicated and deleted as a file.
func TestApiLifecycle(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	const sc = "api-lifecycle"
	cs, cfg := app.W.Collections, app.W.ApiConfig
	paths := flowharness.Replace(app.KiraHome, "<home>")
	httpURL := flowharness.Replace(srv.URL, "<http>")

	app.WriteAPIFile(t, "orders.collection.yaml", []byte(`# Orders API, owned by the web team.
name: Orders
items:
  - id: list
    name: List orders
    http:
      method: GET
      url: `+srv.URL+`/echo?list=1
  - id: admin
    name: Admin
    items:
      - id: purge
        name: Purge
        http:
          method: GET
          url: `+srv.URL+`/echo?purge=1
  - id: get
    name: Get order
    http:
      method: GET
      url: `+srv.URL+`/echo?get=1
`))
	app.WriteAPIFile(t, "billing.collection.yaml", []byte(`name: Billing
items:
  - id: list
    name: List invoices
    http:
      method: GET
      url: `+srv.URL+`/echo?invoices=1
  - id: archive
    name: Archive
    items: []
`))

	var orders, billing model.Collection
	var items map[string]model.CollectionItem // by collection file, then item name
	load := func() {
		t.Helper()
		tree, err := cs.List()
		if err != nil {
			t.Fatal(err)
		}
		items = map[string]model.CollectionItem{}
		for _, c := range tree.Collections {
			switch c.File {
			case "orders.collection.yaml":
				orders = c
			case "billing.collection.yaml":
				billing = c
			}
		}
		for _, it := range tree.Items {
			items[it.CollectionID+"/"+it.Name] = it
		}
	}
	item := func(c model.Collection, name string) model.CollectionItem {
		t.Helper()
		it, ok := items[c.ID+"/"+name]
		if !ok {
			t.Fatalf("no item %q in %s", name, c.Name)
		}
		return it
	}
	text := func(file string) string {
		t.Helper()
		v, err := cfg.File(bridge.ApiFileArgs{File: file})
		if err != nil {
			t.Fatal(err)
		}
		return v.Text
	}
	order := func(file string) string {
		t.Helper()
		var ids []string
		for _, line := range strings.Split(text(file), "\n") {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "- id: "); ok {
				ids = append(ids, rest)
			}
		}
		return strings.Join(ids, ",")
	}
	// The list order of items follows random row ids; sorting keeps the fixture and its id numbering stable.
	record := func(key string) {
		t.Helper()
		tree, err := cs.List()
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]string{}
		for _, c := range tree.Collections {
			names[c.ID] = c.Name
		}
		for _, it := range tree.Items {
			names[it.ID] = it.Name
		}
		sortKey := func(it model.CollectionItem) string {
			parent := ""
			if it.ParentID != nil {
				parent = names[*it.ParentID]
			}
			return fmt.Sprintf("%s/%s/%03d/%s", names[it.CollectionID], parent, it.SortOrder, it.Name)
		}
		slices.SortFunc(tree.Items, func(a, b model.CollectionItem) int { return strings.Compare(sortKey(a), sortKey(b)) })
		app.StudioContract(t, sc, key, tree, paths, httpURL)
	}
	load()
	record("tree#seed")

	// Rename a request, a folder and a collection; stems and ids in the file stay.
	rename := func(key, id, target, name string) {
		t.Helper()
		args := bridge.CollectionsTargetArgs{ID: id, Target: target, Name: name}
		app.StudioContract(t, sc, "args:CollectionsService.Rename#"+key, args, paths)
		if err := cs.Rename(args); err != nil {
			t.Fatalf("Rename %s: %v", target, err)
		}
	}
	rename("request", item(orders, "List orders").ID, "item", "List all orders")
	rename("folder", item(orders, "Admin").ID, "item", "Operations")
	rename("collection", orders.ID, "collection", "Orders v2")
	load()
	if got := text("orders.collection.yaml"); !strings.HasPrefix(got, "# Orders API, owned by the web team.\nname: Orders v2\n") ||
		!strings.Contains(got, "name: List all orders") || !strings.Contains(got, "id: list\n") || !strings.Contains(got, "name: Operations") {
		t.Errorf("renamed file:\n%s", got)
	}
	if orders.Name != "Orders v2" || orders.File != "orders.collection.yaml" {
		t.Errorf("renamed collection row = %+v, want the new name on the same file", orders)
	}
	if err := cs.Rename(bridge.CollectionsTargetArgs{ID: orders.ID, Target: "collection"}); ipcErr(t, err).Code != "E_BAD_REQUEST" {
		t.Errorf("Rename without a name = %v, want E_BAD_REQUEST", err)
	}

	// Move within a file: Get order before List orders; a request into the folder.
	getItem, list, admin := item(orders, "Get order"), item(orders, "List all orders"), item(orders, "Operations")
	moveArgs := bridge.CollectionsMoveItemsArgs{ItemIDs: []string{getItem.ID}, CollectionID: orders.ID, BeforeID: &list.ID}
	app.StudioContract(t, sc, "args:CollectionsService.MoveItems#before", moveArgs, paths)
	moved, err := cs.MoveItems(moveArgs)
	if err != nil || moved[getItem.ID] != getItem.ID {
		t.Fatalf("MoveItems before = %v, %v, want the same row id", moved, err)
	}
	if got := order("orders.collection.yaml"); got != "get,list,admin,purge" {
		t.Errorf("order after the move = %s, want get,list,admin,purge", got)
	}
	app.StudioContract(t, sc, "result#before", moved, paths)
	into := bridge.CollectionsMoveItemsArgs{ItemIDs: []string{list.ID}, CollectionID: orders.ID, ParentID: &admin.ID}
	app.StudioContract(t, sc, "args:CollectionsService.MoveItems#into", into, paths)
	if _, err := cs.MoveItems(into); err != nil {
		t.Fatalf("MoveItems into a folder: %v", err)
	}
	if got := order("orders.collection.yaml"); got != "get,admin,purge,list" {
		t.Errorf("order after moving into the folder = %s, want get,admin,purge,list", got)
	}
	load()
	if p := item(orders, "List all orders").ParentID; p == nil || *p != admin.ID {
		t.Errorf("List all orders parent = %v, want the folder row", p)
	}
	record("tree#nested")
	if _, err := cs.MoveItems(bridge.CollectionsMoveItemsArgs{ItemIDs: []string{admin.ID}, CollectionID: orders.ID, ParentID: &list.ID}); ipcErr(t, err).Code != "E_BAD_REQUEST" {
		t.Errorf("a folder into its own child = %v, want E_BAD_REQUEST", err)
	}

	// Move across files: ids collide on "list", so the copy is re-slugged; history follows the request.
	bound := get(srv.URL + "/echo?list=1")
	bound.ItemID, bound.CollectionID = list.ID, orders.ID
	send(t, app, bound)
	if rows, _ := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{ItemID: list.ID}); len(rows) != 1 {
		t.Fatalf("history before the move = %d rows, want 1", len(rows))
	}
	across := bridge.CollectionsMoveItemsArgs{ItemIDs: []string{admin.ID}, CollectionID: billing.ID}
	app.StudioContract(t, sc, "args:CollectionsService.MoveItems#across", across, paths)
	moved, err = cs.MoveItems(across)
	if err != nil {
		t.Fatalf("MoveItems across: %v", err)
	}
	load()
	record("tree#moved")
	app.StudioContract(t, sc, "result#across", moved, paths)
	if len(moved) != 3 || moved[admin.ID] == "" || moved[admin.ID] == admin.ID {
		t.Fatalf("id map = %v, want the folder and its two children on new rows", moved)
	}
	if got := order("billing.collection.yaml"); got != "list,archive,admin,purge,list-2" {
		t.Errorf("billing order = %s, want list,archive,admin,purge,list-2", got)
	}
	if got := text("orders.collection.yaml"); strings.Contains(got, "admin") || strings.Contains(got, "purge") || !strings.Contains(got, "# Orders API, owned by the web team.") {
		t.Errorf("orders file after the move:\n%s", got)
	}
	newList := moved[list.ID]
	if rows, _ := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{ItemID: newList}); len(rows) != 1 {
		t.Errorf("history under the new row = %d rows, want 1", len(rows))
	}
	if rows, _ := app.W.ResponseHistory.List(bridge.ResponseHistoryScopeArgs{ItemID: list.ID}); len(rows) != 0 {
		t.Errorf("history under the old row = %d rows, want 0", len(rows))
	}

	// Delete: a folder and its child are one delete; a collection removes its file.
	archive, purge := item(billing, "Archive"), item(billing, "Purge")
	del := bridge.CollectionsDeleteManyArgs{ItemIDs: []string{purge.ID, archive.ID, moved[admin.ID]}}
	app.StudioContract(t, sc, "args:CollectionsService.DeleteMany#items", del, paths)
	if err := cs.DeleteMany(del); err != nil {
		t.Fatalf("DeleteMany items: %v", err)
	}
	if got := order("billing.collection.yaml"); got != "list" {
		t.Errorf("billing order after delete = %s, want list", got)
	}
	colDel := bridge.CollectionsDeleteManyArgs{CollectionIDs: []string{orders.ID}, ItemIDs: []string{getItem.ID}}
	app.StudioContract(t, sc, "args:CollectionsService.DeleteMany#collection", colDel, paths)
	if err := cs.DeleteMany(colDel); err != nil {
		t.Fatalf("DeleteMany collection: %v", err)
	}
	if _, err := cfg.File(bridge.ApiFileArgs{File: "orders.collection.yaml"}); ipcErr(t, err).Code != "E_NOT_FOUND" {
		t.Errorf("the deleted collection's file = %v, want E_NOT_FOUND", err)
	}
	if err := cs.DeleteMany(bridge.CollectionsDeleteManyArgs{ItemIDs: []string{getItem.ID}}); ipcErr(t, err).Code != "E_NOT_FOUND" {
		t.Errorf("DeleteMany of a gone item = %v, want E_NOT_FOUND", err)
	}
	record("tree#deleted")

	// Environments: duplicate, then delete the active one.
	vs := app.W.Variables
	prod := app.SeedEnvironment(t, "Production", flowharness.APIVar{Name: "base_url", Value: "https://prod.example.com"})
	if err := vs.SetActiveEnvironment(bridge.VariablesEnvironmentIDArgs{ID: prod.ID}); err != nil {
		t.Fatal(err)
	}
	envs, _ := vs.ListEnvironments()
	app.StudioContract(t, sc, "environments#seed", envs, paths)
	dup, err := vs.DuplicateEnvironment(bridge.VariablesEnvironmentIDArgs{ID: prod.ID})
	if err != nil || dup.Name != "Production copy" || dup.IsActive || dup.ID == prod.ID {
		t.Fatalf("DuplicateEnvironment = %+v, %v, want an inactive Production copy", dup, err)
	}
	vars, err := vs.List(bridge.VariablesScopeArgs{Scope: model.VariableScopeEnvironment, OwnerID: dup.ID})
	if err != nil || len(vars) != 1 || vars[0].Value != "https://prod.example.com" {
		t.Errorf("copy variables = %+v, %v, want base_url", vars, err)
	}
	envs, _ = vs.ListEnvironments()
	app.StudioContract(t, sc, "environments#duplicated", envs, paths)
	if err := vs.DeleteEnvironment(bridge.VariablesEnvironmentIDArgs{ID: prod.ID}); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}
	envs, _ = vs.ListEnvironments()
	if len(envs) != 1 || envs[0].ID != dup.ID || envs[0].IsActive {
		t.Errorf("environments after deleting the active one = %+v, want the copy, none active", envs)
	}
	app.StudioContract(t, sc, "environments#deleted", envs, paths)
	if err := vs.DeleteEnvironment(bridge.VariablesEnvironmentIDArgs{ID: prod.ID}); ipcErr(t, err).Code != "E_NOT_FOUND" {
		t.Errorf("DeleteEnvironment of a gone one = %v, want E_NOT_FOUND", err)
	}
}
