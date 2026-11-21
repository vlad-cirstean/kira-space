package windowflow_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	spacemodel "github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
	"github.com/kirathecat/kira-studio/internal/windowsvc"
)

const tabMoveScenario = "tab-move"

func ensureWindows(t *testing.T, app *flowharness.App, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, err := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: k}); err != nil {
			t.Fatal(err)
		}
	}
}

func saveTabs(t *testing.T, app *flowharness.App, key string, tabs ...spacemodel.TabRecord) {
	t.Helper()
	if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: key, Tabs: tabs}); err != nil {
		t.Fatal(err)
	}
}

func listTabs(t *testing.T, app *flowharness.App, key string) []spacemodel.TabRecord {
	t.Helper()
	list, err := app.W.Tabs.List(bridge.TabsListArgs{WindowKey: key})
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func tabIDsOf(list []spacemodel.TabRecord) []string {
	ids := make([]string, len(list))
	for i, r := range list {
		ids[i] = r.ID
	}
	return ids
}

func inbound(t *testing.T, app *flowharness.App, key string) []bridge.TabInbound {
	t.Helper()
	in, err := app.W.Tabs.Inbound(bridge.TabsListArgs{WindowKey: key})
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func wantContract(t *testing.T, err error) {
	t.Helper()
	if e := testx.AsIpcErr(t, err); e.Code != string(ipcerr.CodeContract) {
		t.Fatalf("code = %s (%s), want E_CONTRACT", e.Code, e.Message)
	}
}

func geom(key string, order int, x, y, w, h float64, seq uint64) shell.WindowGeom {
	return shell.WindowGeom{Key: key, Order: order, Bounds: shell.WindowBounds{X: x, Y: y, Width: w, Height: h}, FocusSeq: seq}
}

func TestTabMove(t *testing.T) {
	app := flowharness.New(t)
	ensureWindows(t, app, "A", "B", "C", "E")
	app.WindowMgr.SetGeometry(
		geom("A", 0, 0, 0, 1000, 800, 0),
		geom("B", 1, 1200, 0, 1000, 800, 1),
		geom("C", 2, 1300, 100, 800, 600, 2),
		shell.WindowGeom{Key: "E", Order: 3, Ephemeral: true, Bounds: shell.WindowBounds{X: 1300, Y: 100, Width: 800, Height: 600}, FocusSeq: 3},
	)
	tabs := app.W.Tabs
	move := func(args bridge.TabsMoveArgs) (bridge.TabsMoveResult, error) {
		if args.WindowKey == "" {
			args.WindowKey = "A"
		}
		return tabs.Move(args)
	}

	t.Run("by key", func(t *testing.T) {
		h1 := tab("h1", "http-request", 1, false)
		saveTabs(t, app, "A", tab("d1", "data", 0, true), h1)
		mark := app.Events.Mark()
		res, err := move(bridge.TabsMoveArgs{Tab: h1, Target: "B"})
		if err != nil || res.WindowKey != "B" || res.Created {
			t.Fatalf("Move = %+v, %v", res, err)
		}
		app.Contract(t, tabMoveScenario, "TabsService.Move", res)
		if got := tabIDsOf(listTabs(t, app, "A")); !reflect.DeepEqual(got, []string{"d1"}) {
			t.Fatalf("A tabs %v", got)
		}
		b := listTabs(t, app, "B")
		if got := tabIDsOf(b); !reflect.DeepEqual(got, []string{"h1"}) || b[0].Active {
			t.Fatalf("B tabs %+v, want h1 inactive", b)
		}
		in := inbound(t, app, "B")
		if len(in) != 1 || in[0].Tab.ID != "h1" || in[0].Incognito || in[0].Terminal != nil {
			t.Fatalf("inbox %+v", in)
		}
		app.Contract(t, tabMoveScenario, "TabsService.Inbound", in)
		if again := inbound(t, app, "B"); len(again) != 0 {
			t.Fatalf("second Inbound = %+v, want empty", again)
		}
		nudges := app.Events.Since(mark, bridge.ChannelTabsInbound)
		if len(nudges) != 1 || nudges[0].Window != "B" {
			t.Fatalf("inbound nudges %+v, want one for B", nudges)
		}
		if f := app.WindowMgr.Focused; len(f) == 0 || f[len(f)-1] != "B" {
			t.Fatalf("Focused %v, want B last", f)
		}
		// A stale save still naming the moved tab is refused.
		wantContract(t, tabs.Save(bridge.TabsSaveArgs{WindowKey: "A", Tabs: []spacemodel.TabRecord{tab("d1", "data", 0, true), h1}}))
	})

	t.Run("incognito", func(t *testing.T) {
		x1 := tab("x1", "data", 0, true)
		saveTabs(t, app, "A", x1)
		if _, err := move(bridge.TabsMoveArgs{Tab: x1, Target: "B", Incognito: true}); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"A", "B"} {
			for _, r := range listTabs(t, app, k) {
				if r.ID == "x1" {
					t.Fatalf("incognito tab x1 persisted in %s", k)
				}
			}
		}
		in := inbound(t, app, "B")
		if len(in) != 1 || !in[0].Incognito {
			t.Fatalf("inbox %+v, want one incognito entry", in)
		}
	})

	t.Run("refusals", func(t *testing.T) {
		owned := tab("o1", "data", 0, true)
		saveTabs(t, app, "C", owned)
		other := "ws1"
		cases := map[string]bridge.TabsMoveArgs{
			"id owned by a third window": {Tab: owned, Target: "B"},
			"terminal outside Runs":      {Tab: spacemodel.TabRecord{ID: "r1", Path: "p", Kind: "terminal", State: json.RawMessage(`{}`), WorkspaceID: &other}, Target: "B"},
			"repo kind":                  {Tab: tab("f1", "repo-file", 0, false), Target: "B"},
			"terminal payload on data":   {Tab: tab("d9", "data", 0, false), Target: "B", Terminal: &bridge.TabTerminal{Status: "running"}},
			"unknown target":             {Tab: tab("d8", "data", 0, false), Target: "nope"},
			"target is source":           {Tab: tab("d7", "data", 0, false), Target: "A"},
			"two destinations":           {Tab: tab("d6", "data", 0, false), Target: "B", NewWindow: true},
			"no destination":             {Tab: tab("d5", "data", 0, false)},
			"ephemeral target":           {Tab: tab("d4", "data", 0, false), Target: "E"},
			"no window key":              {WindowKey: "", Tab: tab("d3", "data", 0, false), Target: "B"},
		}
		for name, args := range cases {
			t.Run(name, func(t *testing.T) {
				args := args
				if name == "no window key" {
					_, err := tabs.Move(args)
					wantContract(t, err)
					return
				}
				_, err := move(args)
				wantContract(t, err)
			})
		}
		if got := tabIDsOf(listTabs(t, app, "C")); !reflect.DeepEqual(got, []string{"o1"}) {
			t.Fatalf("C tabs %v after refusals", got)
		}
		if got := listTabs(t, app, "B"); len(got) != 1 {
			t.Fatalf("B tabs %v after refusals", tabIDsOf(got))
		}
	})

	t.Run("drop hit", func(t *testing.T) {
		caps := app.W.Tabs.Capabilities()
		if !caps.DragOut {
			t.Fatalf("Capabilities = %+v, want dragOut on X11", caps)
		}
		app.Contract(t, tabMoveScenario, "TabsService.Capabilities:dragOut", caps)
		d1 := tab("hit1", "console", 0, true)
		saveTabs(t, app, "A", d1)
		args := bridge.TabsMoveArgs{Tab: d1, Drop: &bridge.TabDrop{ClientX: 1400, ClientY: 300, GrabX: 10, GrabY: 10, InnerWidth: 1000, InnerHeight: 800}}
		res, err := move(args)
		if err != nil || res.WindowKey != "C" || res.Created {
			t.Fatalf("drop into C = %+v, %v", res, err)
		}
		inbound(t, app, "C")
		d2 := tab("hit2", "console", 0, true)
		saveTabs(t, app, "A", d2)
		res, err = move(bridge.TabsMoveArgs{Tab: d2, Drop: &bridge.TabDrop{ClientX: 500, ClientY: 300, InnerWidth: 1000, InnerHeight: 800}})
		if err != nil || res.WindowKey != "A" {
			t.Fatalf("drop inside source = %+v, %v, want source key", res, err)
		}
		if got := tabIDsOf(listTabs(t, app, "A")); !reflect.DeepEqual(got, []string{"hit2"}) {
			t.Fatalf("A tabs %v, want hit2 kept", got)
		}
		if len(inbound(t, app, "A")) != 0 {
			t.Fatal("source inbox holds the in-source drop")
		}
	})

	t.Run("drop miss", func(t *testing.T) {
		m1 := tab("miss1", "data", 0, true)
		saveTabs(t, app, "A", m1)
		drop := &bridge.TabDrop{ClientX: -60, ClientY: 200, GrabX: 120, GrabY: 15, InnerWidth: 1000, InnerHeight: 800}
		app.Contract(t, tabMoveScenario, "args:TabsService.Move:drop", bridge.TabsMoveArgs{WindowKey: "A", Tab: m1, Drop: drop}, flowharness.Mask("windowKey"))
		before := len(app.WindowMgr.OpenedTorn)
		res, err := move(bridge.TabsMoveArgs{Tab: m1, Drop: drop})
		if err != nil || !res.Created || res.WindowKey == "A" {
			t.Fatalf("drop miss = %+v, %v", res, err)
		}
		app.Contract(t, tabMoveScenario, "TabsService.Move:created", res, flowharness.Mask("windowKey"))
		if len(app.WindowMgr.OpenedTorn) != before+1 {
			t.Fatalf("torn windows %d, want %d", len(app.WindowMgr.OpenedTorn), before+1)
		}
		rec := app.WindowMgr.OpenedTorn[before]
		want := shell.TornBounds(shell.WindowBounds{X: 0, Y: 0, Width: 1000, Height: 800}, shell.DropInput{
			ClientX: -60, ClientY: 200, GrabX: 120, GrabY: 15, InnerWidth: 1000, InnerHeight: 800})
		if rec.Key != res.WindowKey || rec.Order != 4 || rec.Bounds == nil || *rec.Bounds != want {
			t.Fatalf("torn record %+v, want order 4 bounds %+v", rec, want)
		}
		if got := tabIDsOf(listTabs(t, app, res.WindowKey)); !reflect.DeepEqual(got, []string{"miss1"}) {
			t.Fatalf("new window tabs %v", got)
		}
		if in := inbound(t, app, res.WindowKey); len(in) != 1 {
			t.Fatalf("new window inbox %+v", in)
		}
	})

	t.Run("menu new window", func(t *testing.T) {
		n1 := tab("new1", "data", 0, true)
		saveTabs(t, app, "A", n1)
		before := len(app.WindowMgr.OpenedTorn)
		res, err := move(bridge.TabsMoveArgs{Tab: n1, NewWindow: true})
		if err != nil || !res.Created {
			t.Fatalf("NewWindow = %+v, %v", res, err)
		}
		if rec := app.WindowMgr.OpenedTorn[before]; rec.Bounds != nil {
			t.Fatalf("menu path bounds %+v, want nil (cascade)", rec.Bounds)
		}
	})

	t.Run("script-run", func(t *testing.T) {
		sr := tab("sr1", "script-run", 0, true)
		sr.State = json.RawMessage(`{"runId":"run1"}`)
		auto := spacemodel.AutomationsWorkspaceID
		sr.WorkspaceID = &auto
		args := bridge.TabsMoveArgs{Tab: sr, Target: "B"}
		app.Contract(t, tabMoveScenario, "args:TabsService.Move:scriptRun", bridge.TabsMoveArgs{WindowKey: "A", Tab: sr, Target: "B"}, flowharness.Mask("windowKey"))
		if _, err := move(args); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"A", "B"} {
			for _, r := range listTabs(t, app, k) {
				if r.ID == "sr1" {
					t.Fatalf("script-run row persisted in %s", k)
				}
			}
		}
		in := inbound(t, app, "B")
		if len(in) != 1 || in[0].Tab.ID != "sr1" || !strings.Contains(string(in[0].Tab.State), "run1") {
			t.Fatalf("inbox %+v", in)
		}
	})

	t.Run("targets and close", func(t *testing.T) {
		got, err := tabs.Targets(bridge.TabsListArgs{WindowKey: "A"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) < 2 || got[0].Key != "B" || got[0].Number != 2 || got[1].Key != "C" || got[1].Number != 3 {
			t.Fatalf("Targets(A) = %+v, want B=2 C=3 then torn windows", got)
		}
		for _, g := range got {
			if g.Key == "E" || g.Key == "A" {
				t.Fatalf("Targets lists %s", g.Key)
			}
		}
		app.Contract(t, tabMoveScenario, "TabsService.Targets", got[:2], flowharness.Mask("key"))

		saveTabs(t, app, "A")
		closed, err := tabs.CloseEmptyWindow(bridge.TabsListArgs{WindowKey: "A"})
		if err != nil || !closed {
			t.Fatalf("CloseEmptyWindow(empty A) = %v, %v", closed, err)
		}
		app.Contract(t, tabMoveScenario, "TabsService.CloseEmptyWindow", closed)
		if c := app.WindowMgr.Closed; len(c) == 0 || c[len(c)-1] != "A" {
			t.Fatalf("Closed %v, want A", c)
		}
		if closed, _ := tabs.CloseEmptyWindow(bridge.TabsListArgs{WindowKey: "C"}); closed {
			t.Fatal("C holds a row; must not close")
		}
		saveTabs(t, app, "C")
		saveTabs(t, app, "B")
		// Sole real window left: B, C and the torn ones still live, so shrink geometry to one.
		app.WindowMgr.SetGeometry(geom("C", 2, 0, 0, 800, 600, 0))
		if closed, _ := tabs.CloseEmptyWindow(bridge.TabsListArgs{WindowKey: "C"}); closed {
			t.Fatal("sole real window must not close")
		}
	})

	t.Run("forget", func(t *testing.T) {
		app.WindowMgr.SetGeometry(geom("A", 0, 0, 0, 1000, 800, 0), geom("B", 1, 1200, 0, 1000, 800, 1))
		f1 := tab("fg1", "data", 0, true)
		saveTabs(t, app, "A", f1)
		if _, err := move(bridge.TabsMoveArgs{Tab: f1, Target: "B"}); err != nil {
			t.Fatal(err)
		}
		bridge.ForgetTabWindow(tabs, "B")
		if got := inbound(t, app, "B"); len(got) != 0 {
			t.Fatalf("inbox after Forget = %+v", got)
		}
	})
}

func TestTabMoveWayland(t *testing.T) {
	app := flowharness.New(t, flowharness.WithWayland())
	ensureWindows(t, app, "A", "B")
	app.WindowMgr.SetGeometry(geom("A", 0, 0, 0, 1000, 800, 0), geom("B", 1, 1200, 0, 1000, 800, 1))
	caps := app.W.Tabs.Capabilities()
	if caps.DragOut {
		t.Fatalf("Capabilities = %+v, want dragOut false on Wayland", caps)
	}
	app.Contract(t, tabMoveScenario, "TabsService.Capabilities", caps)
	d1 := tab("w1", "data", 0, true)
	saveTabs(t, app, "A", d1)
	_, err := app.W.Tabs.Move(bridge.TabsMoveArgs{WindowKey: "A", Tab: d1, Drop: &bridge.TabDrop{ClientX: -10, InnerWidth: 1000, InnerHeight: 800}})
	wantContract(t, err)
	if res, err := app.W.Tabs.Move(bridge.TabsMoveArgs{WindowKey: "A", Tab: d1, NewWindow: true}); err != nil || !res.Created {
		t.Fatalf("NewWindow on Wayland = %+v, %v", res, err)
	}
}
