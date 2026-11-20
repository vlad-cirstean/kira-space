package appflow_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/testx"
	"github.com/kirathecat/kira-studio/internal/windowsvc"
)

func ensureWindow(t *testing.T, app *flowharness.App, key string) windowsvc.EnsureResult {
	t.Helper()
	res, err := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: key})
	if err != nil {
		t.Fatalf("Ensure %s: %v", key, err)
	}
	return res
}

func tab(id, workspace string) model.TabRecord {
	ws := workspace
	return model.TabRecord{ID: id, Path: "/" + id, Kind: "terminal", State: json.RawMessage(`{}`), Active: true, WorkspaceID: &ws}
}

func tabIDs(t *testing.T, app *flowharness.App, key string) []string {
	t.Helper()
	tabs, err := app.W.Tabs.List(bridge.TabsListArgs{WindowKey: key})
	if err != nil {
		t.Fatalf("Tabs.List %s: %v", key, err)
	}
	ids := make([]string, 0, len(tabs))
	for _, tb := range tabs {
		ids = append(ids, tb.ID)
	}
	slices.Sort(ids)
	return ids
}

func TestWindowScopedState(t *testing.T) {
	app := flowharness.New(t)
	for _, key := range []string{"w-a", "w-b"} {
		if res := ensureWindow(t, app, key); res.Mode != "git" {
			t.Fatalf("window %s starts in mode %q, want git", key, res.Mode)
		}
	}
	if err := app.W.WindowsSvc.SetMode(windowsvc.SetModeArgs{WindowKey: "w-a", Mode: "ade"}); err != nil {
		t.Fatal(err)
	}
	if err := app.W.WindowsSvc.SetMode(windowsvc.SetModeArgs{WindowKey: "w-b", Mode: "automations"}); err != nil {
		t.Fatal(err)
	}
	save := func(key string, tabs ...model.TabRecord) {
		t.Helper()
		if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: key, Tabs: tabs}); err != nil {
			t.Fatalf("Tabs.Save %s: %v", key, err)
		}
	}
	save("w-a", tab("a1", "ws"), tab("a2", "ws"))
	save("w-b", tab("b1", "ws"))
	if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: "w-ghost", Tabs: nil}); err == nil {
		t.Fatal("Tabs.Save for an unregistered window succeeded")
	}

	check := func() {
		t.Helper()
		if got := tabIDs(t, app, "w-a"); !slices.Equal(got, []string{"a1", "a2"}) {
			t.Fatalf("w-a tabs = %v", got)
		}
		if got := tabIDs(t, app, "w-b"); !slices.Equal(got, []string{"b1"}) {
			t.Fatalf("w-b tabs = %v", got)
		}
		if got := ensureWindow(t, app, "w-a").Mode; got != "ade" {
			t.Fatalf("w-a mode = %q, want ade", got)
		}
		if got := ensureWindow(t, app, "w-b").Mode; got != "automations" {
			t.Fatalf("w-b mode = %q, want automations", got)
		}
	}
	check()
	save("w-a", tab("a2", "ws"))
	if got := tabIDs(t, app, "w-a"); !slices.Equal(got, []string{"a2"}) {
		t.Fatalf("w-a tabs after replace = %v, want only a2", got)
	}
	if got := tabIDs(t, app, "w-b"); !slices.Equal(got, []string{"b1"}) {
		t.Fatalf("saving w-a touched w-b: %v", got)
	}
	save("w-a", tab("a1", "ws"), tab("a2", "ws"))

	app.Restart()
	check()
}

// A fake window registers under the real registry, as a native window would.
func addWindows(t *testing.T, app *flowharness.App, keys ...string) map[string]int {
	t.Helper()
	detached := map[string]int{}
	for _, key := range keys {
		app.W.Windows.Add(key, 0, nil, func() { detached[key]++ })
	}
	return detached
}

func TestQuitFlushHandshake(t *testing.T) {
	app := flowharness.New(t)
	detached := addWindows(t, app, "w-a", "w-b")
	mark := app.Events.Mark()

	if app.W.Quitter.ShouldQuit() {
		t.Fatal("ShouldQuit allowed the first quit before any flush")
	}
	app.Events.WaitAfter(t, mark, appevent.ChannelFlushBeforeClose, nil, waitFor)
	if _, err := app.W.Settings.GetAll(); err != nil {
		t.Fatalf("DB closed before the windows flushed: %v", err)
	}

	// A negative check: teardown must not start while w-b still owes its ack.
	app.W.Lifecycle.Flushed(bridge.LifecycleFlushedArgs{WindowKey: "w-a"})
	time.Sleep(200 * time.Millisecond)
	if _, err := app.W.Settings.GetAll(); err != nil {
		t.Fatalf("teardown ran with one window still owing its ack: %v", err)
	}
	app.W.Lifecycle.Flushed(bridge.LifecycleFlushedArgs{WindowKey: "w-ghost"})
	app.W.Lifecycle.Flushed(bridge.LifecycleFlushedArgs{WindowKey: "w-b"})

	testx.WaitUntil(t, waitFor, app.W.Quitter.ShouldQuit)
	if _, err := app.W.Settings.GetAll(); err == nil {
		t.Fatal("DB still open after teardown")
	}
	if detached["w-a"] != 1 || detached["w-b"] != 1 {
		t.Fatalf("window detaches = %v, want one each", detached)
	}

	// A second quit and the harness's own stop change nothing.
	before := len(app.Events.Since(0, appevent.ChannelFlushBeforeClose))
	if !app.W.Quitter.ShouldQuit() {
		t.Fatal("second ShouldQuit refused")
	}
	app.W.Quitter.Shutdown()
	app.W.Teardown()
	if after := len(app.Events.Since(0, appevent.ChannelFlushBeforeClose)); after != before {
		t.Fatalf("flush signal re-sent: %d -> %d", before, after)
	}
	if detached["w-a"] != 1 || detached["w-b"] != 1 {
		t.Fatalf("window detaches after repeat = %v, want one each", detached)
	}
}
