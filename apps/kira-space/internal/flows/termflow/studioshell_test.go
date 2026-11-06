package termflow_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func TestShellTab(t *testing.T) {
	app := flowharness.New(t)
	svc := app.W.Terminal

	cwd := svc.DefaultCwd().Path
	if cwd != app.Home {
		t.Fatalf("DefaultCwd = %q, want the temp HOME %q", cwd, app.Home)
	}
	// Contract terminal: tests/ui/automations-module.spec.ts "contract: an unscoped terminal opens".
	app.StudioContract(t, "terminal", "TerminalService.DefaultCwd", svc.DefaultCwd())
	app.StudioContract(t, "terminal", "args:TerminalService.Open", terminal.OpenArgs{
		TerminalID: "t1", WindowKey: "main", Cwd: cwd, Cols: 80, Rows: 24,
	})
	mark := app.Events.Mark()
	open(t, app, "w1", "t1", cwd)
	write(t, app, "t1", "pwd; echo $TERM_PROGRAM\n")
	waitMatch(t, app, mark, "w1", "t1", line(cwd))
	waitMatch(t, app, mark, "w1", "t1", line(terminal.TermProgram))

	if err := svc.Resize(terminal.ResizeArgs{TerminalID: "t1", Cols: 100, Rows: 30}); err != nil {
		t.Fatal(err)
	}
	write(t, app, "t1", "stty size\n")
	waitMatch(t, app, mark, "w1", "t1", line("30 100"))

	if err := svc.Close(terminal.CloseArgs{TerminalID: "t1"}); err != nil {
		t.Fatal(err)
	}
	waitExit(t, app, mark, "w1", "t1")
	testx.WaitUntil(t, wait, func() bool { _, ok := app.W.TermRegistry.WindowOf("t1"); return !ok })
	if err := svc.Write(terminal.WriteArgs{TerminalID: "t1", Data: "ZWNobyBoaQo="}); err != nil {
		t.Fatalf("Write after Close = %v, want a no-op", err)
	}
}

func TestOpenRefusals(t *testing.T) {
	app := flowharness.New(t)
	svc := app.W.Terminal
	open(t, app, "w1", "live", app.Home)

	base := terminal.OpenArgs{TerminalID: "r", WindowKey: "w1", Cwd: app.Home, Cols: 80, Rows: 24}
	cases := map[string]struct {
		mutate func(a terminal.OpenArgs) terminal.OpenArgs
		code   string
	}{
		"relative cwd": {func(a terminal.OpenArgs) terminal.OpenArgs { a.Cwd = "relative/dir"; return a }, "E_CONTRACT"},
		"missing cwd":  {func(a terminal.OpenArgs) terminal.OpenArgs { a.Cwd = app.Home + "/nope"; return a }, "E_BAD_REQUEST"},
		"zero cols":    {func(a terminal.OpenArgs) terminal.OpenArgs { a.Cols = 0; return a }, "E_CONTRACT"},
		"long command": {func(a terminal.OpenArgs) terminal.OpenArgs {
			a.Command = strings.Repeat("x", terminal.MaxCommandBytes+1)
			return a
		}, "E_BAD_REQUEST"},
		"duplicate id": {func(a terminal.OpenArgs) terminal.OpenArgs { a.TerminalID = "live"; a.WindowKey = "w2"; return a }, "E_CONTRACT"},
	}
	for name, c := range cases {
		_, err := svc.Open(c.mutate(base))
		if err == nil || testx.AsIpcErr(t, err).Code != c.code {
			t.Fatalf("%s: err = %v, want %s", name, err, c.code)
		}
	}
	if _, ok := app.W.TermRegistry.WindowOf("r"); ok {
		t.Fatal("a refused Open registered a session")
	}
	if w, _ := app.W.TermRegistry.WindowOf("live"); w != "w1" {
		t.Fatalf("duplicate Open moved the live session to window %q", w)
	}
}

func TestWindowCloseKillsOnlyItsShells(t *testing.T) {
	app := flowharness.New(t)
	pidRe := regexp.MustCompile(`(?m)(?:^|\r)(\d+)\r?$`)
	mark := app.Events.Mark()
	pids := map[string]int{}
	for _, w := range []string{"A", "B"} {
		open(t, app, w, "t-"+w, app.Home)
		write(t, app, "t-"+w, "echo $$\n")
		pid, err := strconv.Atoi(waitMatch(t, app, mark, w, "t-"+w, pidRe)[1])
		if err != nil {
			t.Fatal(err)
		}
		pids[w] = pid
	}

	app.CloseWindow("A")
	waitExit(t, app, mark, "A", "t-A")
	testx.WaitUntil(t, wait, func() bool { return !testx.ProcessAlive(pids["A"]) })
	if !testx.ProcessAlive(pids["B"]) {
		t.Fatal("window B's shell died with window A")
	}
	write(t, app, "t-B", "echo alive$((1+1))\n")
	waitMatch(t, app, mark, "B", "t-B", line("alive2"))
}

func TestQuitTearsDownTerminals(t *testing.T) {
	app := flowharness.New(t)
	pidRe := regexp.MustCompile(`(?m)(?:^|\r)(\d+)\r?$`)
	mark := app.Events.Mark()
	pids := map[string]int{}
	for _, w := range []string{"A", "B"} {
		open(t, app, w, "t-"+w, app.Home)
		write(t, app, "t-"+w, "echo $$\n")
		pid, _ := strconv.Atoi(waitMatch(t, app, mark, w, "t-"+w, pidRe)[1])
		pids[w] = pid
	}

	app.Quit(t)
	for _, w := range []string{"A", "B"} {
		waitExit(t, app, mark, w, "t-"+w)
		testx.WaitUntil(t, wait, func() bool { return !testx.ProcessAlive(pids[w]) })
	}
	_, err := app.W.Terminal.Open(terminal.OpenArgs{TerminalID: "late", WindowKey: "A", Cwd: app.Home, Cols: 80, Rows: 24})
	if err == nil || testx.AsIpcErr(t, err).Code != "E_CONFLICT" {
		t.Fatalf("Open after Quit = %v, want E_CONFLICT", err)
	}
}

func TestLargeOutput(t *testing.T) {
	flowharness.Complete(t)
	app := flowharness.New(t)
	mark := app.Events.Mark()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "big", WindowKey: "w1", Cwd: app.Home, Cols: 80, Rows: 24, Command: "seq 1 200000", LaunchKind: terminal.LaunchKindScript,
	}); err != nil {
		t.Fatal(err)
	}
	waitExit(t, app, mark, "w1", "big")
	text, _, _ := out(app, mark, "w1", "big")
	next := 1
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSuffix(l, "\r")
		if n, err := strconv.Atoi(l); err == nil {
			if n != next {
				t.Fatalf("line %d where %d expected", n, next)
			}
			next++
		}
	}
	if next != 200001 {
		t.Fatalf("output ends at %d, want 200000", next-1)
	}
}
