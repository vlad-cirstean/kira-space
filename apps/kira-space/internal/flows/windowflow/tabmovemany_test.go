package windowflow_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	spacemodel "github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

const tabMoveManyScenario = "tab-move-many"

func entries(recs ...spacemodel.TabRecord) []bridge.TabInbound {
	out := make([]bridge.TabInbound, len(recs))
	for i, r := range recs {
		out[i] = bridge.TabInbound{Tab: r}
	}
	return out
}

func windowRows(t *testing.T, app *flowharness.App) int {
	t.Helper()
	var n int
	if err := app.DB().QueryRow(`SELECT count(*) FROM windows`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func runningPayload() *bridge.TabTerminal {
	return &bridge.TabTerminal{Snapshot: "x", Cols: 80, Rows: 24, Status: "running", Shell: "/bin/sh"}
}

func TestTabMoveMany(t *testing.T) {
	app := flowharness.New(t)
	ensureWindows(t, app, "A", "B", "C")
	app.WindowMgr.SetGeometry(
		geom("A", 0, 0, 0, 1000, 800, 0),
		geom("B", 1, 1200, 0, 1000, 800, 1),
		geom("C", 2, 1300, 100, 800, 600, 2),
	)
	tabs := app.W.Tabs
	term := app.W.Terminal
	reg := app.W.TermRegistry
	move := func(args bridge.TabsMoveArgs) (bridge.TabsMoveResult, error) {
		if args.WindowKey == "" {
			args.WindowKey = "A"
		}
		return tabs.Move(args)
	}
	reset := func(t *testing.T) {
		t.Helper()
		for _, k := range []string{"A", "B", "C"} {
			saveTabs(t, app, k)
			inbound(t, app, k)
		}
	}

	t.Run("set by key", func(t *testing.T) {
		reset(t)
		saveTabs(t, app, "B", tab("b0", "data", 0, true))
		d1, h1, d2 := tab("d1", "data", 0, true), tab("h1", "http-request", 1, false), tab("d2", "data", 2, false)
		x3 := tab("x3", "data", 3, false)
		saveTabs(t, app, "A", d1, h1, d2, x3)
		mark := app.Events.Mark()
		args := bridge.TabsMoveArgs{Tabs: entries(d1, h1, x3), Target: "B"}
		args.Tabs[2].Incognito = true
		app.Contract(t, tabMoveManyScenario, "args:TabsService.Move:many", bridge.TabsMoveArgs{WindowKey: "A", Tabs: args.Tabs, Target: "B"}, flowharness.Mask("windowKey"))
		res, err := move(args)
		if err != nil || res.WindowKey != "B" || res.Created {
			t.Fatalf("Move = %+v, %v", res, err)
		}
		app.Contract(t, tabMoveManyScenario, "TabsService.Move", res)
		if got := tabIDsOf(listTabs(t, app, "A")); !reflect.DeepEqual(got, []string{"d2"}) {
			t.Fatalf("A tabs %v", got)
		}
		b := listTabs(t, app, "B")
		if got := tabIDsOf(b); !reflect.DeepEqual(got, []string{"b0", "d1", "h1"}) {
			t.Fatalf("B tabs %v, want b0 d1 h1 (no row for incognito x3)", got)
		}
		if b[1].Active || b[2].Active {
			t.Fatalf("moved rows active: %+v", b)
		}
		in := inbound(t, app, "B")
		if len(in) != 3 || in[0].Tab.ID != "d1" || in[1].Tab.ID != "h1" || in[2].Tab.ID != "x3" || !in[2].Incognito {
			t.Fatalf("inbox %+v", in)
		}
		if !in[0].Focus || in[1].Focus || in[2].Focus {
			t.Fatalf("focus %v %v %v, want only the first (the active one)", in[0].Focus, in[1].Focus, in[2].Focus)
		}
		app.Contract(t, tabMoveManyScenario, "TabsService.Inbound", in)
		nudges := app.Events.Since(mark, bridge.ChannelTabsInbound)
		if len(nudges) != 1 || nudges[0].Window != "B" {
			t.Fatalf("nudges %+v, want one for B", nudges)
		}
	})

	t.Run("focus first and ignores caller", func(t *testing.T) {
		reset(t)
		a, b := tab("f1", "data", 0, false), tab("f2", "data", 1, false)
		saveTabs(t, app, "A", a, b)
		args := bridge.TabsMoveArgs{Tabs: entries(a, b), Target: "B"}
		args.Tabs[1].Focus = true
		if _, err := move(args); err != nil {
			t.Fatal(err)
		}
		in := inbound(t, app, "B")
		if len(in) != 2 || !in[0].Focus || in[1].Focus {
			t.Fatalf("inbox %+v, want focus on the first entry only", in)
		}
	})

	t.Run("second active wins", func(t *testing.T) {
		reset(t)
		a, b := tab("g1", "data", 0, false), tab("g2", "data", 1, true)
		saveTabs(t, app, "A", a, b)
		if _, err := move(bridge.TabsMoveArgs{Tabs: entries(a, b), Target: "B"}); err != nil {
			t.Fatal(err)
		}
		in := inbound(t, app, "B")
		if len(in) != 2 || in[0].Focus || !in[1].Focus {
			t.Fatalf("inbox %+v, want focus on the active second entry", in)
		}
	})

	t.Run("atomic", func(t *testing.T) {
		reset(t)
		a1, c1 := tab("a1", "data", 0, true), tab("c1", "data", 0, true)
		saveTabs(t, app, "A", a1)
		saveTabs(t, app, "C", c1)
		for _, args := range []bridge.TabsMoveArgs{
			{Tabs: entries(a1, c1), Target: "B"},
			{Tabs: entries(a1, c1), NewWindow: true},
		} {
			windows := len(app.WindowMgr.OpenedTorn)
			before := windowRows(t, app)
			mark := app.Events.Mark()
			_, err := move(args)
			wantContract(t, err)
			if got := tabIDsOf(listTabs(t, app, "A")); !reflect.DeepEqual(got, []string{"a1"}) {
				t.Fatalf("A tabs %v, want a1 kept", got)
			}
			if got := tabIDsOf(listTabs(t, app, "C")); !reflect.DeepEqual(got, []string{"c1"}) {
				t.Fatalf("C tabs %v, want c1 kept", got)
			}
			if len(inbound(t, app, "B")) != 0 {
				t.Fatal("inbox filled by a refused move")
			}
			if n := len(app.Events.Since(mark, bridge.ChannelTabsInbound)); n != 0 {
				t.Fatalf("%d nudges after a refused move", n)
			}
			if after := windowRows(t, app); after != before || len(app.WindowMgr.OpenedTorn) != windows {
				t.Fatalf("windows %d -> %d, torn %d -> %d: a refused move leaked a window", before, windowRows(t, app), windows, len(app.WindowMgr.OpenedTorn))
			}
		}
	})

	t.Run("refusals", func(t *testing.T) {
		reset(t)
		other := "ws1"
		many := make([]spacemodel.TabRecord, 101)
		for i := range many {
			many[i] = tab("m"+string(rune('a'+i/26))+string(rune('a'+i%26)), "data", i, false)
		}
		pay := func(n int) *bridge.TabTerminal {
			p := runningPayload()
			p.Snapshot = strings.Repeat("s", n)
			return p
		}
		big := entries(runsTab("r1"), runsTab("r2"))
		big[0].Terminal, big[1].Terminal = pay(9<<20), pay(9<<20)
		withPayload := entries(tab("d9", "data", 0, false))
		withPayload[0].Terminal = runningPayload()
		cases := map[string]bridge.TabsMoveArgs{
			"empty list":       {Tabs: []bridge.TabInbound{}, Target: "B"},
			"101 entries":      {Tabs: entries(many...), Target: "B"},
			"repeated id":      {Tabs: entries(tab("d1", "data", 0, false), tab("d1", "data", 1, false)), Target: "B"},
			"repo kind":        {Tabs: entries(tab("d2", "data", 0, false), tab("f1", "repo-file", 1, false)), Target: "B"},
			"terminal outside": {Tabs: entries(spacemodel.TabRecord{ID: "r9", Path: "p", Kind: "terminal", State: tab("x", "data", 0, false).State, WorkspaceID: &other}), Target: "B"},
			"payload on data":  {Tabs: withPayload, Target: "B"},
			"snapshots 16 MiB": {Tabs: big, Target: "B"},
		}
		saveTabs(t, app, "A", tab("d1", "data", 0, true), tab("d2", "data", 1, false))
		for name, args := range cases {
			t.Run(name, func(t *testing.T) {
				_, err := move(args)
				wantContract(t, err)
			})
		}
		if got := tabIDsOf(listTabs(t, app, "A")); !reflect.DeepEqual(got, []string{"d1", "d2"}) {
			t.Fatalf("A tabs %v, want unchanged", got)
		}
		if len(inbound(t, app, "B")) != 0 {
			t.Fatal("inbox filled by a refused move")
		}
	})

	t.Run("drop miss", func(t *testing.T) {
		reset(t)
		m1, m2, m3 := tab("p1", "data", 0, true), tab("p2", "data", 1, false), tab("p3", "data", 2, false)
		saveTabs(t, app, "A", m1, m2, m3)
		before := len(app.WindowMgr.OpenedTorn)
		res, err := move(bridge.TabsMoveArgs{Tabs: entries(m1, m2, m3), Drop: &bridge.TabDrop{ClientX: -60, ClientY: 200, GrabX: 120, GrabY: 15, InnerWidth: 1000, InnerHeight: 800}})
		if err != nil || !res.Created || res.WindowKey == "A" {
			t.Fatalf("drop miss = %+v, %v", res, err)
		}
		if len(app.WindowMgr.OpenedTorn) != before+1 {
			t.Fatalf("torn windows %d, want %d", len(app.WindowMgr.OpenedTorn), before+1)
		}
		if got := tabIDsOf(listTabs(t, app, res.WindowKey)); !reflect.DeepEqual(got, []string{"p1", "p2", "p3"}) {
			t.Fatalf("new window tabs %v, want p1 p2 p3", got)
		}
	})

	started := func(t *testing.T, id string) {
		t.Helper()
		mark := app.Events.Mark()
		openScript(t, app, "A", id)
		waitOut(t, app, mark, "A", id, "a1")
		time.Sleep(60 * time.Millisecond)
	}
	hold := func(t *testing.T, id string) {
		t.Helper()
		if h, err := term.Hold(terminal.HoldArgs{TerminalID: id}); err != nil || !h.Live {
			t.Fatalf("Hold %s = %+v, %v", id, h, err)
		}
	}
	liveEntries := func(recs ...spacemodel.TabRecord) []bridge.TabInbound {
		out := entries(recs...)
		for i, r := range recs {
			if r.Kind == "terminal" {
				out[i].Terminal = runningPayload()
			}
		}
		return out
	}

	t.Run("two live terminals", func(t *testing.T) {
		reset(t)
		started(t, "tm1")
		started(t, "tm2")
		hold(t, "tm1")
		hold(t, "tm2")
		typeLine(t, app, "tm1", "b1")
		typeLine(t, app, "tm2", "b2")
		time.Sleep(300 * time.Millisecond)
		d1 := tab("td1", "data", 1, false)
		args := bridge.TabsMoveArgs{Tabs: liveEntries(runsTab("tm1"), d1, runsTab("tm2")), Target: "B"}
		app.Contract(t, tabMoveManyScenario, "args:TabsService.Move:terminals", bridge.TabsMoveArgs{WindowKey: "A", Tabs: args.Tabs, Target: "B"}, flowharness.Mask("windowKey"))
		saveTabs(t, app, "A", d1)
		if _, err := move(args); err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{"tm1", "tm2"} {
			if w, ok := reg.WindowOf(id); !ok || w != "B" {
				t.Fatalf("WindowOf %s = %q, %v, want B", id, w, ok)
			}
		}
		for id, want := range map[string]string{"tm1": "got:b1", "tm2": "got:b2"} {
			res, err := term.Adopt(terminal.AdoptArgs{TerminalID: id, WindowKey: "B"})
			if err != nil {
				t.Fatal(err)
			}
			data := decodeData(t, res.Data)
			other := "got:b2"
			if id == "tm2" {
				other = "got:b1"
			}
			if !strings.Contains(data, want) || strings.Contains(data, other) {
				t.Fatalf("Adopt %s data %q, want only %s", id, data, want)
			}
		}
		app.Contract(t, tabMoveManyScenario, "TabsService.Move:terminals", bridge.TabsMoveResult{WindowKey: "B"})
		live := app.Events.Mark()
		typeLine(t, app, "tm1", "c1")
		typeLine(t, app, "tm2", "c2")
		waitOut(t, app, live, "B", "tm1", "got:c1")
		waitOut(t, app, live, "B", "tm2", "got:c2")
		for _, id := range []string{"tm1", "tm2"} {
			for _, ev := range eventsTo(app, live, id) {
				if ev.Window != "B" {
					t.Fatalf("live event for %s to %q, want only B", id, ev.Window)
				}
			}
		}
		app.CloseWindow("A")
		for _, id := range []string{"tm1", "tm2"} {
			if w, ok := reg.WindowOf(id); !ok || w != "B" {
				t.Fatalf("after closing A: WindowOf %s = %q, %v, want B", id, w, ok)
			}
		}
	})

	t.Run("terminal set refused", func(t *testing.T) {
		reset(t)
		ensureWindows(t, app, "A")
		started(t, "tr1")
		started(t, "tr2")
		hold(t, "tr1")
		hold(t, "tr2")
		mark := app.Events.Mark()
		typeLine(t, app, "tr1", "h1")
		typeLine(t, app, "tr2", "h2")
		time.Sleep(300 * time.Millisecond)
		c1 := tab("tc1", "data", 0, true)
		saveTabs(t, app, "C", c1)
		_, err := move(bridge.TabsMoveArgs{Tabs: liveEntries(runsTab("tr1"), runsTab("tr2"), c1), Target: "B"})
		wantContract(t, err)
		for _, id := range []string{"tr1", "tr2"} {
			if w, ok := reg.WindowOf(id); !ok || w != "A" {
				t.Fatalf("WindowOf %s = %q, %v, want A after a refused move", id, w, ok)
			}
		}
		for id, want := range map[string]string{"tr1": "got:h1", "tr2": "got:h2"} {
			if err := term.Release(terminal.HoldArgs{TerminalID: id}); err != nil {
				t.Fatal(err)
			}
			waitOut(t, app, mark, "A", id, want)
		}
	})

	t.Run("mixed exited", func(t *testing.T) {
		reset(t)
		ensureWindows(t, app, "A")
		started(t, "te1")
		started(t, "te2")
		hold(t, "te1")
		hold(t, "te2")
		typeLine(t, app, "te2", "die")
		deadline := time.Now().Add(wait)
		for {
			if _, ok := reg.WindowOf("te2"); !ok {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("script did not exit")
			}
			time.Sleep(10 * time.Millisecond)
		}
		code := 3
		in := liveEntries(runsTab("te1"), runsTab("te2"))
		in[1].Terminal = &bridge.TabTerminal{Snapshot: "x", Cols: 80, Rows: 24, Status: "exited", ExitCode: &code, Shell: "/bin/sh"}
		if _, err := move(bridge.TabsMoveArgs{Tabs: in, Target: "B"}); err != nil {
			t.Fatal(err)
		}
		if w, ok := reg.WindowOf("te1"); !ok || w != "B" {
			t.Fatalf("WindowOf te1 = %q, %v, want B", w, ok)
		}
		got := inbound(t, app, "B")
		if len(got) != 2 || got[1].Terminal == nil || got[1].Terminal.Status != "exited" || got[1].Terminal.ExitCode == nil || *got[1].Terminal.ExitCode != 3 {
			t.Fatalf("inbox %+v, want te2 to carry its exit status", got)
		}
		if _, ok := reg.WindowOf("te2"); ok {
			t.Fatal("exited te2 still has a session")
		}
		if _, err := term.Adopt(terminal.AdoptArgs{TerminalID: "te1", WindowKey: "B"}); err != nil {
			t.Fatal(err)
		}
	})
}
