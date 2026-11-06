package apiflow

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
)

// Contract api-order, read by tests/ui/api-order.spec.ts: collections and environments keep a manual
// order in the rows (new ones append, a rename keeps the place, a restart keeps the order), and items
// still reorder as file edits beside it.
func TestApiOrder(t *testing.T) {
	app := studioflowharness.New(t)
	const sc = "api-order"
	cs, vs := app.W.Collections, app.W.Variables
	paths := studioflowharness.Replace(app.KiraHome, "<home>")

	app.WriteAPIFile(t, "alpha.collection.yaml", []byte(`name: Zeta
items:
  - id: r1
    name: r1
    http:
      method: GET
      url: https://x/1
  - id: r2
    name: r2
    http:
      method: GET
      url: https://x/2
  - id: f
    name: f
    items:
      - id: r3
        name: r3
        http:
          method: GET
          url: https://x/3
`))
	app.WriteAPIFile(t, "beta.collection.yaml", []byte("name: Alpha\nitems: []\n"))
	app.WriteAPIFile(t, "gamma.collection.yaml", []byte("name: Mid\nitems: []\n"))
	for _, n := range []string{"dev", "prod", "stage"} {
		app.WriteAPIFile(t, n+".environment.yaml", []byte("name: "+n+"\n"))
	}

	names := func() string {
		t.Helper()
		tree, err := cs.List()
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, c := range tree.Collections {
			out = append(out, c.Name)
		}
		return strings.Join(out, ",")
	}
	envNames := func() string {
		t.Helper()
		envs, err := vs.ListEnvironments()
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range envs {
			out = append(out, e.Name)
		}
		return strings.Join(out, ",")
	}
	collectionID := func(name string) string {
		t.Helper()
		tree, err := cs.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range tree.Collections {
			if c.Name == name {
				return c.ID
			}
		}
		t.Fatalf("no collection %q", name)
		return ""
	}
	envID := func(name string) string {
		t.Helper()
		envs, err := vs.ListEnvironments()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range envs {
			if e.Name == name {
				return e.ID
			}
		}
		t.Fatalf("no environment %q", name)
		return ""
	}
	record := func(key string) {
		t.Helper()
		tree, err := cs.List()
		if err != nil {
			t.Fatal(err)
		}
		slices.SortFunc(tree.Items, func(a, b model.CollectionItem) int {
			return strings.Compare(a.CollectionID+"/"+fmt.Sprintf("%03d", a.SortOrder)+a.Name, b.CollectionID+"/"+fmt.Sprintf("%03d", b.SortOrder)+b.Name)
		})
		app.Contract(t, sc, key, tree, paths)
	}
	recordEnvs := func(key string) {
		t.Helper()
		envs, err := vs.ListEnvironments()
		if err != nil {
			t.Fatal(err)
		}
		app.Contract(t, sc, key, envs, paths)
	}
	// reorder runs fn and expects exactly one change event of kind.
	oneChange := func(kind string, fn func()) {
		t.Helper()
		mark := app.Events.Mark()
		fn()
		ch := apiChanges(t, app, mark)
		if len(ch) != 1 || !hasChange(ch[0], bridge.ApiDataChange{Kind: kind}) {
			t.Errorf("events = %+v, want one %s change", ch, kind)
		}
	}

	if got := names(); got != "Zeta,Alpha,Mid" {
		t.Fatalf("seed order = %s, want file-load order Zeta,Alpha,Mid", got)
	}
	record("tree#seed")
	if got := envNames(); got != "dev,prod,stage" {
		t.Fatalf("seed environments = %s", got)
	}
	recordEnvs("environments#seed")

	// Drag Mid above Zeta; the id left out keeps its place after the listed ones.
	zeta, mid := collectionID("Zeta"), collectionID("Mid")
	drag := bridge.CollectionsReorderCollectionsArgs{IDs: []string{mid, zeta}}
	app.Contract(t, sc, "args:CollectionsService.ReorderCollections#drag", drag, paths)
	oneChange(bridge.ApiDataTree, func() {
		if err := cs.ReorderCollections(drag); err != nil {
			t.Fatal(err)
		}
	})
	if got := names(); got != "Mid,Zeta,Alpha" {
		t.Errorf("order after the drag = %s, want Mid,Zeta,Alpha", got)
	}
	record("tree#reordered")

	// A rename keeps the place (by name it would sort first).
	if err := cs.Rename(bridge.CollectionsTargetArgs{ID: zeta, Target: "collection", Name: "Aaa"}); err != nil {
		t.Fatal(err)
	}
	if got := names(); got != "Mid,Aaa,Alpha" {
		t.Errorf("order after the rename = %s, want Mid,Aaa,Alpha", got)
	}

	// New rows append; a delete leaves the rest in order.
	newCollection(t, app, "Last")
	if got := names(); got != "Mid,Aaa,Alpha,Last" {
		t.Errorf("order after create = %s, want Mid,Aaa,Alpha,Last", got)
	}
	if err := cs.DeleteMany(bridge.CollectionsDeleteManyArgs{CollectionIDs: []string{mid}}); err != nil {
		t.Fatal(err)
	}
	if got := names(); got != "Aaa,Alpha,Last" {
		t.Errorf("order after delete = %s, want Aaa,Alpha,Last", got)
	}

	app.Restart(t)
	cs, vs = app.W.Collections, app.W.Variables
	if got := names(); got != "Aaa,Alpha,Last" {
		t.Errorf("order after restart = %s, want Aaa,Alpha,Last", got)
	}

	// Items still reorder as file edits beside the collection order.
	tree, err := cs.List()
	if err != nil {
		t.Fatal(err)
	}
	var r1, r2 string
	for _, it := range tree.Items {
		switch it.Name {
		case "r1":
			r1 = it.ID
		case "r2":
			r2 = it.ID
		}
	}
	move := bridge.CollectionsMoveItemsArgs{ItemIDs: []string{r2}, CollectionID: collectionID("Aaa"), BeforeID: &r1}
	app.Contract(t, sc, "args:CollectionsService.MoveItems#items", move, paths)
	if _, err := cs.MoveItems(move); err != nil {
		t.Fatal(err)
	}
	if got := names(); got != "Aaa,Alpha,Last" {
		t.Errorf("collection order after an item move = %s", got)
	}
	record("tree#items")

	// Environments.
	stage, dev := envID("stage"), envID("dev")
	envDrag := bridge.VariablesReorderEnvironmentsArgs{IDs: []string{stage, dev}}
	app.Contract(t, sc, "args:VariablesService.ReorderEnvironments#drag", envDrag, paths)
	oneChange(bridge.ApiDataEnvironments, func() {
		if err := vs.ReorderEnvironments(envDrag); err != nil {
			t.Fatal(err)
		}
	})
	if got := envNames(); got != "stage,dev,prod" {
		t.Errorf("environments after the drag = %s, want stage,dev,prod", got)
	}
	if _, err := vs.DuplicateEnvironment(bridge.VariablesEnvironmentIDArgs{ID: dev}); err != nil {
		t.Fatal(err)
	}
	if got := envNames(); !strings.HasPrefix(got, "stage,dev,prod,") || strings.Count(got, ",") != 3 {
		t.Errorf("environments after a duplicate = %s, want the copy last", got)
	}
	recordEnvs("environments#reordered")

	// Errors change nothing.
	cBefore, eBefore := names(), envNames()
	if err := cs.ReorderCollections(bridge.CollectionsReorderCollectionsArgs{}); ipcErr(t, err).Code != "E_CONTRACT" {
		t.Errorf("empty ids = %v, want E_CONTRACT", err)
	}
	if err := vs.ReorderEnvironments(bridge.VariablesReorderEnvironmentsArgs{}); ipcErr(t, err).Code != "E_CONTRACT" {
		t.Errorf("empty environment ids = %v, want E_CONTRACT", err)
	}
	alpha := collectionID("Alpha")
	for name, ids := range map[string][]string{"unknown": {"nope"}, "repeated": {alpha, alpha}} {
		err := cs.ReorderCollections(bridge.CollectionsReorderCollectionsArgs{IDs: ids})
		if ie := ipcErr(t, err); ie.Code != "E_BAD_REQUEST" || !strings.Contains(ie.Message, "order no longer matches") {
			t.Errorf("%s id = %+v, want E_BAD_REQUEST with the invalidOrder text", name, ie)
		}
		err = vs.ReorderEnvironments(bridge.VariablesReorderEnvironmentsArgs{IDs: ids})
		if ie := ipcErr(t, err); ie.Code != "E_BAD_REQUEST" {
			t.Errorf("%s environment id = %+v, want E_BAD_REQUEST", name, ie)
		}
	}
	if names() != cBefore || envNames() != eBefore {
		t.Errorf("a failed reorder changed the order: %s / %s", names(), envNames())
	}
}
