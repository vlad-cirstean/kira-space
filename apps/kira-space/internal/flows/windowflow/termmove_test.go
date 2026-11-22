package windowflow_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	spacemodel "github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/flowtest"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const termMoveScenario = "terminal-move"

// echoScript prints a1, then echoes every stdin line as got:<line> until it reads "quit"; "die"
// exits 3.
const echoScript = `echo a1; while read l; do if [ "$l" = die ]; then exit 3; fi; echo "got:$l"; done`

func openScript(t *testing.T, app *flowharness.App, window, id string) {
	t.Helper()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{TerminalID: id, WindowKey: window, Cwd: app.Home, Cols: 80, Rows: 24, Command: echoScript}); err != nil {
		t.Fatal(err)
	}
}

func typeLine(t *testing.T, app *flowharness.App, id, line string) {
	t.Helper()
	if err := app.W.Terminal.Write(terminal.WriteArgs{TerminalID: id, Data: base64.StdEncoding.EncodeToString([]byte(line + "\n"))}); err != nil {
		t.Fatal(err)
	}
}

func termEvent(ev flowtest.Event) terminal.Event {
	te, _ := ev.Data.(terminal.Event)
	return te
}

// waitOut waits for output containing text in window's events for id after mark.
func waitOut(t *testing.T, app *flowharness.App, mark int, window, id, text string) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if out, _ := termOut(app, mark, window, id); strings.Contains(out, text) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	out, _ := termOut(app, mark, window, id)
	t.Fatalf("window %s never saw %q from %s; saw %q", window, text, id, out)
}

func lastEnd(app *flowharness.App, mark int, window, id string) int64 {
	var end int64
	for _, ev := range app.Events.Since(mark, appevent.ChannelTerminal) {
		if te := termEvent(ev); te.TerminalID == id && ev.Window == window && te.End > end {
			end = te.End
		}
	}
	return end
}

func eventsTo(app *flowharness.App, mark int, id string) []flowtest.Event {
	var out []flowtest.Event
	for _, ev := range app.Events.Since(mark, appevent.ChannelTerminal) {
		if termEvent(ev).TerminalID == id {
			out = append(out, ev)
		}
	}
	return out
}

func runsTab(id string) spacemodel.TabRecord {
	r := tab(id, "terminal", 0, true)
	ws := spacemodel.AutomationsWorkspaceID
	r.WorkspaceID = &ws
	return r
}

func moveTerm(app *flowharness.App, id string, to bridge.TabsMoveArgs) (bridge.TabsMoveResult, error) {
	to.WindowKey = "A"
	to.Tabs = []bridge.TabInbound{{Tab: runsTab(id), Terminal: &bridge.TabTerminal{Snapshot: "x", Cols: 80, Rows: 24, Status: "running", Shell: "/bin/sh"}}}
	return app.W.Tabs.Move(to)
}

func decodeData(t *testing.T, s string) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTerminalMove(t *testing.T) {
	app := flowharness.New(t)
	ensureWindows(t, app, "A", "B")
	app.WindowMgr.SetGeometry(geom("A", 0, 0, 0, 1000, 800, 0), geom("B", 1, 1200, 0, 1000, 800, 1))
	reg := app.W.TermRegistry
	term := app.W.Terminal

	t.Run("hold move adopt", func(t *testing.T) {
		mark := app.Events.Mark()
		openScript(t, app, "A", "t1")
		waitOut(t, app, mark, "A", "t1", "a1")
		time.Sleep(60 * time.Millisecond)
		want := lastEnd(app, mark, "A", "t1")

		hold, err := term.Hold(terminal.HoldArgs{TerminalID: "t1"})
		if err != nil || !hold.Live || hold.End != want {
			t.Fatalf("Hold = %+v, %v, want live end %d", hold, err, want)
		}
		app.Contract(t, termMoveScenario, "TerminalService.Hold", hold, flowharness.Mask("end"))
		held := app.Events.Mark()
		typeLine(t, app, "t1", "b2")
		time.Sleep(300 * time.Millisecond)
		if evs := eventsTo(app, held, "t1"); len(evs) != 0 {
			t.Fatalf("%d terminal events while held, want none", len(evs))
		}

		args := bridge.TabsMoveArgs{WindowKey: "A", Tabs: []bridge.TabInbound{{Tab: runsTab("t1"), Terminal: &bridge.TabTerminal{Snapshot: "x", Cols: 80, Rows: 24, Status: "running", Shell: "/bin/sh"}}}, Target: "B"}
		app.Contract(t, termMoveScenario, "args:TabsService.Move:terminal", args, flowharness.Mask("windowKey"))
		if _, err := moveTerm(app, "t1", bridge.TabsMoveArgs{Target: "B"}); err != nil {
			t.Fatal(err)
		}
		if w, ok := reg.WindowOf("t1"); !ok || w != "B" {
			t.Fatalf("WindowOf = %q, %v, want B", w, ok)
		}
		in := inbound(t, app, "B")
		if len(in) != 1 || !in[0].Focus || in[0].Terminal == nil || in[0].Terminal.Snapshot != "x" {
			t.Fatalf("inbox %+v, want the terminal payload", in)
		}
		res, err := term.Adopt(terminal.AdoptArgs{TerminalID: "t1", WindowKey: "B"})
		if err != nil {
			t.Fatal(err)
		}
		data := decodeData(t, res.Data)
		if !strings.Contains(data, "got:b2") || res.End != hold.End+int64(len(data)) || res.Exited {
			t.Fatalf("Adopt = %+v data %q, want got:b2 and end %d", res, data, hold.End+int64(len(data)))
		}
		app.Contract(t, termMoveScenario, "TerminalService.Adopt", res, flowharness.Mask("data", "end"))

		live := app.Events.Mark()
		typeLine(t, app, "t1", "c3")
		waitOut(t, app, live, "B", "t1", "got:c3")
		for _, ev := range eventsTo(app, live, "t1") {
			if ev.Window != "B" {
				t.Fatalf("live event to %q, want only B", ev.Window)
			}
			if te := termEvent(ev); te.End <= res.End {
				t.Fatalf("live end %d does not continue past adopt end %d", te.End, res.End)
			}
		}

		// Source close keeps the session; target close ends it with CauseWindow.
		app.CloseWindow("A")
		if w, ok := reg.WindowOf("t1"); !ok || w != "B" {
			t.Fatalf("after closing A: WindowOf = %q, %v, want B", w, ok)
		}
		after := app.Events.Mark()
		typeLine(t, app, "t1", "d4")
		waitOut(t, app, after, "B", "t1", "got:d4")
		app.CloseWindow("B")
		deadline := time.Now().Add(wait)
		for {
			if _, ok := reg.WindowOf("t1"); !ok {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("session survived closing its owner window")
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}

func TestTerminalMoveCases(t *testing.T) {
	app := flowharness.New(t)
	ensureWindows(t, app, "A", "B")
	app.WindowMgr.SetGeometry(geom("A", 0, 0, 0, 1000, 800, 0), geom("B", 1, 1200, 0, 1000, 800, 1))
	reg := app.W.TermRegistry
	term := app.W.Terminal

	started := func(t *testing.T, id string) int {
		t.Helper()
		mark := app.Events.Mark()
		openScript(t, app, "A", id)
		waitOut(t, app, mark, "A", id, "a1")
		time.Sleep(60 * time.Millisecond)
		return mark
	}
	hold := func(t *testing.T, id string) {
		t.Helper()
		if h, err := term.Hold(terminal.HoldArgs{TerminalID: id}); err != nil || !h.Live {
			t.Fatalf("Hold %s = %+v, %v", id, h, err)
		}
	}

	t.Run("exit while held", func(t *testing.T) {
		started(t, "t3")
		hold(t, "t3")
		typeLine(t, app, "t3", "die")
		deadline := time.Now().Add(wait)
		for {
			if _, ok := reg.WindowOf("t3"); !ok {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("script did not exit")
			}
			time.Sleep(10 * time.Millisecond)
		}
		if _, err := moveTerm(app, "t3", bridge.TabsMoveArgs{Target: "B"}); err != nil {
			t.Fatal(err)
		}
		res, err := term.Adopt(terminal.AdoptArgs{TerminalID: "t3", WindowKey: "B"})
		if err != nil || !res.Exited || res.ExitCode == nil || *res.ExitCode != 3 {
			t.Fatalf("Adopt = %+v, %v, want exited code 3", res, err)
		}
		if h, _ := term.Hold(terminal.HoldArgs{TerminalID: "t3"}); h.Live {
			t.Fatal("pump still tracked after adopting the exit")
		}
	})

	// A shell that exited before the move began: Hold reports not live, so the source window sends the
	// exit it saw, never a running terminal for the target to adopt.
	t.Run("exit before hold", func(t *testing.T) {
		started(t, "t8")
		typeLine(t, app, "t8", "die")
		deadline := time.Now().Add(wait)
		for {
			if _, ok := reg.WindowOf("t8"); !ok {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("script did not exit")
			}
			time.Sleep(10 * time.Millisecond)
		}
		h, err := term.Hold(terminal.HoldArgs{TerminalID: "t8"})
		if err != nil || h.Live {
			t.Fatalf("Hold after exit = %+v, %v, want not live", h, err)
		}
		app.Contract(t, termMoveScenario, "TerminalService.Hold:exited", h)
	})

	t.Run("release", func(t *testing.T) {
		started(t, "t4")
		hold(t, "t4")
		mark := app.Events.Mark()
		typeLine(t, app, "t4", "h1")
		time.Sleep(300 * time.Millisecond)
		if err := term.Release(terminal.HoldArgs{TerminalID: "t4"}); err != nil {
			t.Fatal(err)
		}
		evs := eventsTo(app, mark, "t4")
		if len(evs) != 1 || evs[0].Window != "A" || !strings.Contains(decodeData(t, termEvent(evs[0]).Data), "got:h1") {
			t.Fatalf("release events %+v, want one to A with the held bytes", evs)
		}
		live := app.Events.Mark()
		typeLine(t, app, "t4", "h2")
		waitOut(t, app, live, "A", "t4", "got:h2")
		if err := term.Release(terminal.HoldArgs{TerminalID: "t4"}); err != nil {
			t.Fatalf("Release with no hold = %v, want no-op", err)
		}
	})

	t.Run("refusals", func(t *testing.T) {
		started(t, "t5")
		hold(t, "t5")
		_, err := term.Hold(terminal.HoldArgs{TerminalID: "t5"})
		wantContract(t, err)
		if _, err := moveTerm(app, "t5", bridge.TabsMoveArgs{Target: "B"}); err != nil {
			t.Fatal(err)
		}
		_, err = term.Adopt(terminal.AdoptArgs{TerminalID: "t5", WindowKey: "A"})
		wantContract(t, err)
		app.Contract(t, termMoveScenario, "TerminalService.Adopt:error", testx.AsIpcErr(t, err))
		if _, err := term.Adopt(terminal.AdoptArgs{TerminalID: "t5", WindowKey: "B"}); err != nil {
			t.Fatal(err)
		}
		_, err = term.Adopt(terminal.AdoptArgs{TerminalID: "t5", WindowKey: "B"})
		wantContract(t, err)
	})

	t.Run("forget", func(t *testing.T) {
		started(t, "t6")
		hold(t, "t6")
		if _, err := moveTerm(app, "t6", bridge.TabsMoveArgs{Target: "B"}); err != nil {
			t.Fatal(err)
		}
		app.CloseWindow("B")
		if _, ok := reg.WindowOf("t6"); ok {
			t.Fatal("session survived closing the window that owns it")
		}
		if h, _ := term.Hold(terminal.HoldArgs{TerminalID: "t6"}); h.Live {
			t.Fatal("pump still tracked after the target window closed")
		}
	})

	t.Run("new window miss", func(t *testing.T) {
		started(t, "t7")
		hold(t, "t7")
		typeLine(t, app, "t7", "n1")
		time.Sleep(300 * time.Millisecond)
		res, err := moveTerm(app, "t7", bridge.TabsMoveArgs{Drop: &bridge.TabDrop{ClientX: -80, ClientY: 100, InnerWidth: 1000, InnerHeight: 800}})
		if err != nil || !res.Created {
			t.Fatalf("drop miss = %+v, %v", res, err)
		}
		if w, ok := reg.WindowOf("t7"); !ok || w != res.WindowKey {
			t.Fatalf("WindowOf = %q, %v, want %s", w, ok, res.WindowKey)
		}
		got, err := term.Adopt(terminal.AdoptArgs{TerminalID: "t7", WindowKey: res.WindowKey})
		if err != nil || !strings.Contains(decodeData(t, got.Data), "got:n1") {
			t.Fatalf("Adopt = %+v, %v, want the output held while the window booted", got, err)
		}
	})
}
