package flowharness_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

func TestMain(m *testing.M) { os.Exit(flowharness.Main(m)) }

// Boots the real composition root with default settings and opens a real repo through the git
// stream: an empty git.gitPath must still resolve to a real git.
func TestHarnessBootsWithDefaultSettings(t *testing.T) {
	start := time.Now()
	app := flowharness.New(t)
	t.Logf("harness boot: %s", time.Since(start))

	if got := len(app.W.Bound()); got != 61 {
		t.Fatalf("bound services = %d, want 61", got)
	}
	settings, err := app.W.Settings.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Git.GitPath != "" {
		t.Fatalf("default git.gitPath = %q, want empty", settings.Git.GitPath)
	}

	repo := app.NewRepo("proj")
	want := repo.Commit("first commit", map[string]string{"a.txt": "a\n"})

	gs := app.OpenGitStream()
	var init gitrpc.AppInitResult
	gs.MustRequest("app.init", nil, &init)
	if init.Git.Kind != "ok" || !filepath.IsAbs(init.Git.Path) {
		t.Fatalf("app.init git = %+v, want ok with an absolute path", init.Git)
	}
	var opened gitrpc.RepoOpenResult
	gs.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: repo.Dir}, &opened)
	if opened.Kind != "ok" || opened.Repo == nil {
		t.Fatalf("repo.open = %+v, want ok", opened)
	}
	if got := opened.Repo.Head; got.Kind != "branch" || got.Name != "main" {
		t.Fatalf("repo head = %+v, want branch main", got)
	}

	chunks := gs.Stream("graph.stream", map[string]any{"repoId": opened.Repo.RepoID}, 4).Drain()
	_, rows := chunks[0].Graph(t)
	if len(rows) != 1 || rows[0].Sha[:7] != want[:7] || rows[0].Subject != "first commit" {
		t.Fatalf("graph rows = %+v, want the one commit %s", rows, want)
	}

	gs.Close()
	app.W.Teardown()
}

// Sends a Studio request over the real HTTP stack and drives a real PTY through quit.
func TestHarnessBootsStudioSide(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
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
	if _, ok := app.W.TermRegistry.WindowOf("t1"); !ok {
		t.Fatal("shell not registered after Open")
	}
	app.Quit(t)
	if _, ok := app.W.TermRegistry.WindowOf("t1"); ok {
		t.Fatal("shell still registered after Quit")
	}
}

// A Docker ping through the helper: skips without an engine.
func TestDockerPing(t *testing.T) {
	d := flowharness.RequireDocker(t)
	id := d.Run("ping", []string{"sh", "-c", "sleep 30"}, nil)
	if id == "" {
		t.Fatal("no container id")
	}
}
