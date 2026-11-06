package studioflowharness_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

func TestMain(m *testing.M) { os.Exit(studioflowharness.Main(m)) }

// Boots the real composition root with default settings and sends a request over real sockets.
func TestHarnessBootsAndSends(t *testing.T) {
	start := time.Now()
	app := studioflowharness.New(t)
	t.Logf("harness boot: %s", time.Since(start))

	if got := len(app.W.Bound()); got != 39 {
		t.Fatalf("bound services = %d, want 39", got)
	}
	srv := studioflowharness.HTTP(t)
	res, err := app.W.Http.Send(context.Background(), bridge.HttpSendArgs{
		Method: "GET", URL: srv.URL + "/echo", OpID: "35aa4fd1-8a08-76b1-8044-5e6cf39f36fa", TabID: "tab-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 {
		t.Fatalf("status = %d, want 200", res.Status)
	}
	if reqs := srv.Requests(); len(reqs) != 1 || reqs[0].Path != "/echo" {
		t.Fatalf("server saw %+v, want one /echo", reqs)
	}

	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "t1", WindowKey: "w1", Cwd: app.Home, Cols: 80, Rows: 24,
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := app.W.TerminalRegistry.WindowOf("t1"); !ok {
		t.Fatal("shell not registered after Open")
	}
	app.Quit(t)
	if _, ok := app.W.TerminalRegistry.WindowOf("t1"); ok {
		t.Fatal("shell still registered after Quit")
	}
}

// A Docker ping through the helper: skips without an engine.
func TestDockerPing(t *testing.T) {
	d := studioflowharness.RequireDocker(t)
	id := d.Run("ping", []string{"sh", "-c", "sleep 30"}, nil)
	if id == "" {
		t.Fatal("no container id")
	}
}
