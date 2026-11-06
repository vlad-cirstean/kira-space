package windowflow

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/windowsvc"
)

// Backend half of tests/ui/tabs-scope.spec.ts.
func TestTabsScope(t *testing.T) {
	app := studioflowharness.New(t)
	for _, key := range []string{"A", "B"} {
		if _, err := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: key}); err != nil {
			t.Fatal(err)
		}
	}
	listIDs := func(key string) []string {
		list, err := app.W.Tabs.List(bridge.TabsListArgs{WindowKey: key})
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(list))
		for i, r := range list {
			out[i] = r.ID
		}
		return out
	}
	wantContract := func(what string, err error) {
		t.Helper()
		var ie *ipcerr.Error
		if !errors.As(err, &ie) || ie.Code != "E_CONTRACT" {
			t.Fatalf("%s = %v, want E_CONTRACT", what, err)
		}
	}

	v7, v4 := ids.NewID(), "5f2c1c0e-8f0a-4a52-9d6e-0b6c2f7a1d11"
	if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: "A", Tabs: []model.TabRecord{tab(v7, "console", 0, true), tab(v4, "console", 1, false)}}); err != nil {
		t.Fatal(err)
	}
	if got := listIDs("A"); !reflect.DeepEqual(got, []string{v7, v4}) {
		t.Fatalf("window A tabs %v", got)
	}

	b := tab("b-own", "console", 0, true)
	err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: "B", Tabs: []model.TabRecord{b, tab(v7, "console", 1, false)}})
	wantContract("B saving A's tab", err)
	var ie *ipcerr.Error
	errors.As(err, &ie)
	app.Contract(t, "tabs-scope", "TabsService.Save#ownedElsewhere", map[string]any{"code": ie.Code, "message": ie.Message})
	if got := listIDs("B"); len(got) != 0 {
		t.Fatalf("window B kept %v after a refused batch", got)
	}
	if got := listIDs("A"); !reflect.DeepEqual(got, []string{v7, v4}) {
		t.Fatalf("window A lost tabs: %v", got)
	}

	for _, bad := range []string{strings.Repeat("a", 65), "has space", "a/b", "a\nb"} {
		wantContract("tab id "+bad, app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: "B", Tabs: []model.TabRecord{tab(bad, "console", 0, true)}}))
	}
	if got := listIDs("B"); len(got) != 0 {
		t.Fatalf("window B kept %v after refused ids", got)
	}
}
