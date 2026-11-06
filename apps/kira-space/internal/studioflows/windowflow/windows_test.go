package windowflow

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
	"github.com/kirathecat/kira-studio/internal/windowsvc"
)

const wait = 20 * time.Second

func tab(id, kind string, order int, active bool) model.TabRecord {
	return model.TabRecord{ID: id, Path: "p/" + id, Kind: kind, State: json.RawMessage(`{}`), Order: order, Active: active}
}

func openTerm(t *testing.T, app *flowharness.App, window, id string) {
	t.Helper()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{TerminalID: id, WindowKey: window, Cwd: app.Home, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
}

// termOut is the decoded output, and whether the terminal exited, seen by one window since mark.
func termOut(app *flowharness.App, mark int, window, id string) (string, bool) {
	var sb strings.Builder
	exited := false
	for _, ev := range app.Events.Since(mark, appevent.ChannelTerminal) {
		te, ok := ev.Data.(terminal.Event)
		if !ok || te.TerminalID != id || ev.Window != window {
			continue
		}
		raw, _ := base64.StdEncoding.DecodeString(te.Data)
		sb.Write(raw)
		exited = exited || te.Exited
	}
	return sb.String(), exited
}

func TestStudioWindows(t *testing.T) {
	app := flowharness.New(t)
	w := app.W.WindowsSvc

	var ie *ipcerr.Error
	if _, err := w.Ensure(windowsvc.EnsureArgs{}); !errors.As(err, &ie) || ie.Code != "E_CONTRACT" {
		t.Fatalf("Ensure without a key = %v, want E_CONTRACT", err)
	}
	for _, key := range []string{"A", "B"} {
		res, err := w.Ensure(windowsvc.EnsureArgs{WindowKey: key})
		if err != nil || res.Mode != model.DefaultWindowMode {
			t.Fatalf("Ensure %s = %+v (%v), want the default mode", key, res, err)
		}
	}
	if err := w.SetMode(windowsvc.SetModeArgs{WindowKey: "A", Mode: "automations"}); err != nil {
		t.Fatal(err)
	}
	if err := w.SetMode(windowsvc.SetModeArgs{WindowKey: "B", Mode: "api"}); err != nil {
		t.Fatal(err)
	}
	if err := w.SetMode(windowsvc.SetModeArgs{Mode: "api"}); err == nil {
		t.Fatal("SetMode without a key succeeded")
	}
	if a, _ := w.Ensure(windowsvc.EnsureArgs{WindowKey: "A"}); a.Mode != "automations" {
		t.Fatalf("window A mode = %q, want terminal", a.Mode)
	}
	if b, _ := w.Ensure(windowsvc.EnsureArgs{WindowKey: "B"}); b.Mode != "api" {
		t.Fatalf("window B mode = %q, want api", b.Mode)
	}
	if err := w.OpenNew(); err != nil || app.NewWindows() != 1 {
		t.Fatalf("OpenNew = %v, shell asked for %d windows, want 1", err, app.NewWindows())
	}

	tabsA := []model.TabRecord{tab("a1", "console", 0, false), tab("a2", "console", 1, true)}
	tabsB := []model.TabRecord{tab("b1", "http-request", 0, true)}
	if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: "A", Tabs: tabsA}); err != nil {
		t.Fatal(err)
	}
	if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: "B", Tabs: tabsB}); err != nil {
		t.Fatal(err)
	}
	tabIDs := func(key string) []string {
		list, err := app.W.Tabs.List(bridge.TabsListArgs{WindowKey: key})
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, len(list))
		for i, r := range list {
			ids[i] = r.ID
		}
		return ids
	}
	if got := tabIDs("A"); !reflect.DeepEqual(got, []string{"a1", "a2"}) {
		t.Fatalf("window A tabs %v", got)
	}
	if got := tabIDs("B"); !reflect.DeepEqual(got, []string{"b1"}) {
		t.Fatalf("window B tabs %v", got)
	}

	// Layout and settings are app-wide: one write, every window hears it.
	mark := app.Events.Mark()
	wide, tall := 400.0, 300.0
	merged, err := app.W.Layout.Set(bridge.LayoutSetArgs{Patch: model.LayoutPatch{Modes: map[string]model.PanelSizesPatch{
		"studio": {Project: &model.ProjectSizePatch{Width: &wide}, Operations: &model.OperationsSizePatch{Height: &tall}},
	}}})
	if err != nil || merged.Modes["studio"].Project.Width != wide || merged.Modes["studio"].Operations.Height != tall {
		t.Fatalf("Layout.Set = %+v (%v)", merged, err)
	}
	// P312: sizes are per module, so another module keeps its defaults.
	if got := merged.Modes["api"]; got != model.DefaultPanelSizes() {
		t.Fatalf("api sizes = %+v, want the defaults untouched", got)
	}
	if got, err := app.W.Layout.GetAll(); err != nil || !reflect.DeepEqual(got, merged) {
		t.Fatalf("Layout.GetAll = %+v (%v), want %+v", got, err, merged)
	}
	if ev := app.Events.WaitAfter(t, mark, appevent.ChannelLayoutChanged, nil, wait); ev.Window != "" {
		t.Fatalf("layout change went to window %q, want every window", ev.Window)
	}
	settingsBefore, err := app.W.Settings.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := app.W.Settings.GetAll(); err != nil || !reflect.DeepEqual(got, settingsBefore) {
		t.Fatalf("Settings.GetAll unstable: %v", err)
	}

	// A window close ends only its own terminals.
	openTerm(t, app, "A", "ta")
	openTerm(t, app, "B", "tb")
	mark = app.Events.Mark()
	app.CloseWindow("A")
	testx.WaitUntil(t, wait, func() bool { _, exited := termOut(app, mark, "A", "ta"); return exited })
	if err := app.W.Terminal.Write(terminal.WriteArgs{TerminalID: "tb", Data: base64.StdEncoding.EncodeToString([]byte("echo still-$((20+22))\n"))}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, wait, func() bool { out, _ := termOut(app, mark, "B", "tb"); return strings.Contains(out, "still-42") })
	if _, exited := termOut(app, mark, "B", "tb"); exited {
		t.Fatal("window B's terminal ended with window A")
	}

	// Every row survives a relaunch.
	app.Restart(t)
	if got := tabIDs("A"); !reflect.DeepEqual(got, []string{"a1", "a2"}) {
		t.Fatalf("window A tabs after a relaunch %v", got)
	}
	if a, _ := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: "A"}); a.Mode != "automations" {
		t.Fatalf("window A mode after a relaunch = %q", a.Mode)
	}
	if got, _ := app.W.Layout.GetAll(); got.Modes["studio"].Project.Width != wide || got.Modes["api"] != model.DefaultPanelSizes() {
		t.Fatalf("layout after a relaunch = %+v", got)
	}
	_, err = app.W.Layout.Set(bridge.LayoutSetArgs{Patch: model.LayoutPatch{Modes: map[string]model.PanelSizesPatch{"bogus": {}}}})
	var coded *ipcerr.Error
	if !errors.As(err, &coded) || coded.Code != string(ipcerr.CodeContract) {
		t.Fatalf("Layout.Set(unknown mode) = %v, want %s", err, ipcerr.CodeContract)
	}
}

// The acks are fire-and-forget: with nothing owed they change nothing, and a quit with no window
// owing one goes straight through its flush.
func TestQuitHandshake(t *testing.T) {
	app := flowharness.New(t)
	app.W.Lifecycle.Flushed(bridge.LifecycleFlushedArgs{WindowKey: "ghost"})
	app.W.Lifecycle.WindowFlushed(bridge.LifecycleWindowFlushedArgs{WindowKey: "ghost"})
	if app.W.Quitter.ShouldQuit() {
		t.Fatal("first ShouldQuit allowed the quit before the flush")
	}
	app.Events.Wait(t, appevent.ChannelFlushBeforeClose, nil, wait)
	if p := quitPrompts(app); p != nil {
		t.Fatalf("quit prompts with nothing live = %+v, want none", p)
	}
	testx.WaitUntil(t, wait, app.W.Quitter.ShouldQuit)
}

func TestDockerStatsEndWithTheirWindow(t *testing.T) {
	d := flowharness.RequireDocker(t)
	app := flowharness.New(t)
	id := d.Run("winstats", []string{"sleep", "600"}, nil)
	for _, key := range []string{"A", "B"} {
		if err := app.W.Docker.StatsSubscribe(docker.StatsArgs{WindowKey: key, IDs: []string{id}}); err != nil {
			t.Fatal(err)
		}
	}
	samples := func(mark int, key string) int {
		n := 0
		for _, ev := range app.Events.Since(mark, docker.ChannelStats) {
			if ev.Window == key {
				n++
			}
		}
		return n
	}
	testx.WaitUntil(t, wait, func() bool { return samples(0, "A") > 0 && samples(0, "B") > 0 })
	app.CloseWindow("A")
	time.Sleep(600 * time.Millisecond)
	mark := app.Events.Mark()
	testx.WaitUntil(t, wait, func() bool { return samples(mark, "B") > 1 })
	if n := samples(mark, "A"); n != 0 {
		t.Fatalf("closed window A still gets %d stats samples", n)
	}
}

// widthUserSet must survive the bridge and a relaunch (the Git review widen reads it).
func TestLayoutWidthUserSetSurvivesRestart(t *testing.T) {
	app := flowharness.New(t)
	width, userSet := 300.0, true
	if _, err := app.W.Layout.Set(bridge.LayoutSetArgs{Patch: model.LayoutPatch{Modes: map[string]model.PanelSizesPatch{
		"studio": {Project: &model.ProjectSizePatch{Width: &width, WidthUserSet: &userSet}},
	}}}); err != nil {
		t.Fatalf("Layout.Set: %v", err)
	}
	app.Restart(t)
	got, err := app.W.Layout.GetAll()
	if err != nil || !got.Modes["studio"].Project.WidthUserSet || got.Modes["api"].Project.WidthUserSet {
		t.Fatalf("Layout.GetAll after restart = %+v, %v, want studio widthUserSet only", got.Modes, err)
	}
}
