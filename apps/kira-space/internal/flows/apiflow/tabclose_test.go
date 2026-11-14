package apiflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/windowsvc"
)

// Backend half of tests/ui/studio/api-tab-close.spec.ts. Closing an API tab asks Save, Discard or
// Cancel; the UI drives these backend calls. Save writes the request (bound tab: SaveRequest, new
// tab: CreateItem) and the tab row goes away; Discard writes nothing and leaves the file as it was.
func TestTabCloseSaveOrDiscard(t *testing.T) {
	app := flowharness.New(t)
	cs := app.W.Collections
	if _, err := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: "A"}); err != nil {
		t.Fatal(err)
	}
	col := newCollection(t, app, "Tab close")
	file := filepath.Join(app.W.ApiFiles.Dir(), col.File)
	const sc = "api-tab-close"
	mask := flowharness.Mask("itemId", "id", "collectionId", "createdAt", "updatedAt")

	newItem := func(name, url string) (model.CollectionItem, model.SavedRequest) {
		t.Helper()
		req := savedGet()
		req.URL = url
		item, err := cs.CreateItem(bridge.CollectionsCreateItemArgs{CollectionID: col.ID, Kind: "request", Name: name, Request: &req})
		if err != nil {
			t.Fatal(err)
		}
		return item, req
	}
	stored := func(itemID string) string {
		t.Helper()
		r, err := cs.GetRequest(bridge.CollectionsItemArgs{ItemID: itemID})
		if err != nil {
			t.Fatal(err)
		}
		return r.URL
	}
	fileText := func() string {
		t.Helper()
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	openTab := func(itemID *string) string {
		t.Helper()
		id := ids.NewID()
		state, err := json.Marshal(map[string]any{"itemId": itemID})
		if err != nil {
			t.Fatal(err)
		}
		rec := model.TabRecord{ID: id, Path: "request/" + id, Kind: "http-request", State: state, Order: 0, Active: true}
		if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: "A", Tabs: []model.TabRecord{rec}}); err != nil {
			t.Fatal(err)
		}
		if got := tabIDs(t, app); len(got) != 1 || got[0] != id {
			t.Fatalf("open tabs = %v, want [%s]", got, id)
		}
		return id
	}
	closeTab := func() {
		t.Helper()
		if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: "A", Tabs: []model.TabRecord{}}); err != nil {
			t.Fatal(err)
		}
		if got := tabIDs(t, app); len(got) != 0 {
			t.Fatalf("tabs after close = %v, want none", got)
		}
	}

	// Modified tab, Save: the edit is written, then the tab closes.
	shared, read := newItem("shared", "https://x.test/saved")
	openTab(&shared.ID)
	edit := read
	edit.URL = "https://x.test/edited"
	args := bridge.CollectionsSaveRequestArgs{ItemID: shared.ID, Name: "shared", Request: edit, Expect: &read, ExpectName: ptr("shared")}
	app.Contract(t, sc, "args:SaveRequest#close", args, mask)
	got, err := cs.SaveRequest(args)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "SaveRequest#close", got, mask)
	closeTab()
	if u := stored(shared.ID); u != "https://x.test/edited" {
		t.Fatalf("saved url after Save and close = %q", u)
	}
	if !strings.Contains(fileText(), "https://x.test/edited") {
		t.Error("collection file lacks the saved edit")
	}

	// Modified tab, Discard: no write, the file stays byte for byte.
	kept, _ := newItem("kept", "https://x.test/kept")
	openTab(&kept.ID)
	before := fileText()
	closeTab()
	if after := fileText(); after != before {
		t.Error("Discard changed the collection file")
	}
	if u := stored(kept.ID); u != "https://x.test/kept" {
		t.Fatalf("saved url after Discard and close = %q", u)
	}

	// New unsaved tab, Save: Save as… creates the row, then the tab closes.
	openTab(nil)
	fresh := savedGet()
	fresh.URL = "https://x.test/fresh"
	create := bridge.CollectionsCreateItemArgs{CollectionID: col.ID, Kind: "request", Name: "Fresh", Request: &fresh}
	app.Contract(t, sc, "args:CreateItem#close", create, mask)
	item, err := cs.CreateItem(create)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "CreateItem#close", item, mask)
	closeTab()
	if name := itemName(t, cs, item.ID); name != "Fresh" {
		t.Fatalf("created item name = %q", name)
	}
	if u := stored(item.ID); u != "https://x.test/fresh" || !strings.Contains(fileText(), "https://x.test/fresh") {
		t.Fatalf("created request url = %q, file %v", u, fmt.Sprint(strings.Contains(fileText(), "fresh")))
	}
}

func tabIDs(t *testing.T, app *flowharness.App) []string {
	t.Helper()
	list, err := app.W.Tabs.List(bridge.TabsListArgs{WindowKey: "A"})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(list))
	for i, r := range list {
		out[i] = r.ID
	}
	return out
}
