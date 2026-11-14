package gitflow_test

import (
	"fmt"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func TestRemoteCancel(t *testing.T) {
	flowharness.Complete(t)
	r := newRig(t)
	p := newRemotePair(r, "slow", map[string]string{"f.txt": "seed\n"})
	p.y.Commit("new upstream work", map[string]string{"y.txt": "y\n"})
	p.y.Git("push", "-q", "origin", "main")

	marks := filepath.Join(r.app.Root, "marks")
	if err := os.MkdirAll(marks, 0o755); err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(r.app.Root, "slow-upload-pack")
	script := fmt.Sprintf("#!/bin/sh\necho $$ > %[1]s/sh.pid\nsleep 120 &\necho $! > %[1]s/sleep.pid\nwait\nexec git upload-pack \"$@\"\n", marks)
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	p.x.Git("config", "remote.origin.uploadpack", wrapper)
	id := r.open(p.x.Dir).RepoID

	done := make(chan gitsession.RemoteOpResult, 1)
	go func() {
		var res gitsession.RemoteOpResult
		_ = r.gs.Request("remote.run", gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: gitsession.RemoteOpParams{Kind: "fetch", Remote: "origin"}}, &res)
		done <- res
	}()
	testx.WaitUntil(t, wait, func() bool { _, err := os.Stat(filepath.Join(marks, "sleep.pid")); return err == nil })

	cancel := call[gitrpc.RemoteCancelResult](t, r.gs, "remote.cancel", gitrpc.RemoteCancelParams{RepoID: id})
	if !cancel.Cancelled {
		t.Fatal("remote.cancel reported nothing cancelled while a fetch was running")
	}
	select {
	case res := <-done:
		if res.OK || res.Error == nil || res.Error.Kind != "Cancelled" {
			t.Fatalf("cancelled fetch = %+v err %+v, want a Cancelled error", res, res.Error)
		}
	case <-time.After(wait):
		t.Fatal("fetch did not return after remote.cancel")
	}
	for _, name := range []string{"sh.pid", "sleep.pid"} {
		raw, _ := os.ReadFile(filepath.Join(marks, name))
		var pid int
		_, _ = fmt.Sscan(string(raw), &pid)
		testx.WaitUntil(t, wait, func() bool { return !testx.ProcessAlive(pid) })
	}
	var locks []string
	_ = filepath.WalkDir(filepath.Join(p.x.Dir, ".git"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".lock") {
			locks = append(locks, path)
		}
		return nil
	})
	if len(locks) != 0 {
		t.Fatalf("lock files left behind: %v", locks)
	}
	recent, err := r.app.W.Ops.Recent(bridgeOpsArgs(10))
	if err != nil {
		t.Fatal(err)
	}
	// The cancelled fetch is the newest user row; a background remote refresh may log after it.
	cancelled := slices.ContainsFunc(recent, func(e oplog.Record) bool {
		return e.Kind == "fetch" && e.Status == oplog.StatusCancelled
	})
	if !cancelled {
		t.Fatalf("ops log = %+v, want a cancelled fetch row", recent)
	}
}

// Guards the credential relay end to end: a real git http-backend behind basic auth.
func TestCredentialPrompt(t *testing.T) {
	flowharness.Complete(t)
	r := newRig(t)
	root := filepath.Join(r.app.Root, "http")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := r.app.NewRepo("seed")
	seed.Commit("seed", map[string]string{"f.txt": "seed\n"})
	if out, err := exec.Command("git", "clone", "-q", "--bare", seed.Dir, filepath.Join(root, "repo.git")).CombinedOutput(); err != nil {
		t.Fatalf("bare clone: %v\n%s", err, out)
	}
	backend, err := exec.LookPath("git-http-backend")
	if err != nil {
		backend = "/usr/lib/git-core/git-http-backend"
	}
	if _, err := os.Stat(backend); err != nil {
		t.Skipf("git-http-backend not installed: %v", err)
	}
	cgiHandler := &cgi.Handler{Path: backend, Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if user, pass, ok := req.BasicAuth(); !ok || user != "alice" || pass != "s3cret" {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		cgiHandler.ServeHTTP(w, req)
	}))
	t.Cleanup(srv.Close)

	x := r.app.NewRepo("x")
	x.Commit("local", map[string]string{"x.txt": "x\n"})
	x.AddRemote(strings.Replace(srv.URL, "http://", "http://alice@", 1) + "/repo.git")
	id := r.open(x.Dir).RepoID

	run := func(secret string) gitsession.RemoteOpResult {
		done := make(chan gitsession.RemoteOpResult, 1)
		go func() {
			var res gitsession.RemoteOpResult
			_ = r.gs.Request("remote.run", gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: gitsession.RemoteOpParams{Kind: "fetch", Remote: "origin"}}, &res)
			done <- res
		}()
		req := answerLogin(t, r.app, "alice", secret)
		if !req.Masked || !strings.Contains(req.Prompt, "Password") || req.RepoLabel != "x" {
			t.Fatalf("credential prompt = %+v, want a masked password prompt for repo x", req)
		}
		select {
		case res := <-done:
			return res
		case <-time.After(wait):
			t.Fatal("fetch did not finish after the credential was provided")
			return gitsession.RemoteOpResult{}
		}
	}

	bad := run("wrong")
	if bad.OK || bad.Error == nil || bad.Error.Kind != "AuthFailed" {
		t.Fatalf("fetch with a wrong secret = %+v err %+v, want AuthFailed", bad, bad.Error)
	}
	if good := run("s3cret"); !good.OK {
		t.Fatalf("fetch with the right secret = %+v", good.Error)
	}
	if x.Git("rev-parse", "refs/remotes/origin/main") != seed.Git("rev-parse", "HEAD") {
		t.Fatal("fetch did not bring origin/main")
	}
}

func TestStackRestackConflict(t *testing.T) {
	flowharness.Complete(t)
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"f.txt": "base\n"})
	repo.Git("checkout", "-q", "-b", "a")
	repo.Commit("a edits f", map[string]string{"f.txt": "base\na-line\n"})
	repo.Git("checkout", "-q", "-b", "b")
	repo.Commit("b edits f", map[string]string{"f.txt": "base\na-line\nb-line\n"})
	repo.Git("checkout", "-q", "-b", "c")
	repo.Commit("c adds g", map[string]string{"g.txt": "g\n"})
	id := r.open(repo.Dir).RepoID
	parent := "main"
	for _, b := range []string{"a", "b", "c"} {
		p := parent
		r.mustOp(id, gitsession.OpRequest{Kind: "stackSet", Branch: b, Parent: &p})
		parent = b
	}
	tipB, tipC := repo.Git("rev-parse", "b"), repo.Git("rev-parse", "c")

	r.external(func() {
		repo.Checkout("a")
		repo.Write("f.txt", "base\na-AMENDED\n")
		repo.Git("add", "-A")
		repo.Git("commit", "-q", "--amend", "-m", "a edits f, amended")
		repo.Checkout("c")
	})
	res := call[gitsession.RestackResult](t, r.gs, "stack.restack", gitrpc.StackRestackParams{RepoID: id, Branch: "c"})
	if res.OK || res.StoppedAt == nil || *res.StoppedAt != "b" || res.InProgress == nil || res.InProgress.Kind != gitpreflight.InProgressRebase {
		t.Fatalf("conflicting restack = %+v, want stopped at b mid-rebase", res)
	}
	if c := call[gitrpc.StackCancelRestackResult](t, r.gs, "stack.cancelRestack", gitrpc.StackCancelRestackParams{RepoID: id}); c.Cancelled {
		t.Fatal("stack.cancelRestack reports cancelling a restack that is paused, not running")
	}
	r.mustOp(id, gitsession.OpRequest{Kind: "opAbort"})
	if repo.Git("rev-parse", "b") != tipB || repo.Git("rev-parse", "c") != tipC {
		t.Fatalf("refs not restored after abort: b %s (want %s) c %s (want %s)", repo.Git("rev-parse", "b"), tipB, repo.Git("rev-parse", "c"), tipC)
	}
	if out := repo.Git("status", "--porcelain"); out != "" {
		t.Fatalf("tree not clean after abort: %q", out)
	}
}
