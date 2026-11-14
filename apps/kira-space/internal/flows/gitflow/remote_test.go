package gitflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// remotePair is a bare remote with two clones, x (the app's) and y (another machine).
type remotePair struct {
	bare *flowharness.Bare
	x, y *flowharness.Repo
}

func newRemotePair(r *rig, tag string, files map[string]string) remotePair {
	r.t.Helper()
	seed := r.app.NewRepo("seed-" + tag)
	seed.Commit("seed", files)
	bare := r.app.NewBare("remote-" + tag)
	bare.PushFrom(seed)
	return remotePair{
		bare: bare,
		x:    bare.Clone(filepath.Join(r.app.Work, "x-"+tag)),
		y:    bare.Clone(filepath.Join(r.app.Work, "y-"+tag)),
	}
}

func trackOf(t *testing.T, refs gitsession.RefsResult, branch string) (ahead, behind int) {
	t.Helper()
	for _, b := range refs.Branches {
		if b.ShortName != branch {
			continue
		}
		track, ok := b.Track.(map[string]any)
		if !ok {
			return 0, 0
		}
		a, _ := track["ahead"].(float64)
		bh, _ := track["behind"].(float64)
		return int(a), int(bh)
	}
	t.Fatalf("branch %q not in refs.list", branch)
	return 0, 0
}

func TestRemoteFetchPullPush(t *testing.T) {
	r := newRig(t)
	p := newRemotePair(r, "a", map[string]string{"f.txt": "seed\n"})
	id := r.open(p.x.Dir).RepoID

	yTip := p.y.Commit("from y", map[string]string{"y.txt": "y\n"})
	p.y.Git("push", "-q", "origin", "main")

	fetch := r.remote(id, gitsession.RemoteOpParams{Kind: "fetch", Remote: "origin"})
	if !fetch.OK || len(fetch.Updates) == 0 {
		t.Fatalf("fetch = %+v, want ok with ref updates", fetch)
	}
	r.app.Contract(t, "git-remote", "git:refs.list#behind", r.refs(id), flowharness.Mask("objectId", "committerDate"))
	if ahead, behind := trackOf(t, r.refs(id), "main"); ahead != 0 || behind != 1 {
		t.Fatalf("main ahead/behind = %d/%d after fetch, want 0/1", ahead, behind)
	}

	pf := call[gitpreflight.PullPreflight](t, r.gs, "remote.pullPreflight", gitrpc.RemotePullPreflightParams{RepoID: id, Branch: "main"})
	r.app.Contract(t, "git-remote", "git:remote.pullPreflight#behind", pf)
	if pf.Behind != 1 || pf.Ahead != 0 || pf.Strategy != gitpreflight.PullStrategy("ff-only") {
		t.Fatalf("pull preflight = %+v, want behind 1 with ff-only", pf)
	}
	pull := r.remote(id, gitsession.RemoteOpParams{Kind: "pull", Remote: "origin", Branch: "main", Strategy: string(pf.Strategy)})
	if !pull.OK || p.x.Git("rev-parse", "HEAD") != yTip {
		t.Fatalf("pull = %+v, HEAD %s, want %s", pull, p.x.Git("rev-parse", "HEAD"), yTip)
	}

	xTip := p.x.Commit("from x", map[string]string{"x.txt": "x\n"})
	push := call[gitpreflight.PushPreflight](t, r.gs, "remote.pushPreflight", gitrpc.RemotePushPreflightParams{RepoID: id, Branch: "main", Remote: "origin"})
	r.app.Contract(t, "git-remote", "git:remote.pushPreflight#ahead", push, flowharness.Mask("remoteTip", "localTip"))
	if push.Ahead != 1 || push.Behind != 0 || !push.FastForward {
		t.Fatalf("push preflight = %+v, want ahead 1, fast-forward", push)
	}
	if res := r.remote(id, gitsession.RemoteOpParams{Kind: "push", Remote: "origin", Branch: "main"}); !res.OK {
		t.Fatalf("push = %+v", res.Error)
	}
	if got := p.bare.Git("rev-parse", "main"); got != xTip {
		t.Fatalf("remote main = %s, want %s", got, xTip)
	}

	// Rewrite the pushed commit: the plain push is refused, the force push needs the typed token and a fresh lease.
	r.external(func() { gitNow(t, p.x.Dir, "commit", "-q", "--amend", "-m", "from x, amended") })
	amended := p.x.Git("rev-parse", "HEAD")
	push = call[gitpreflight.PushPreflight](t, r.gs, "remote.pushPreflight", gitrpc.RemotePushPreflightParams{RepoID: id, Branch: "main", Remote: "origin"})
	if push.FastForward || push.ProtectedBy == nil || push.RemoteTip == nil || *push.RemoteTip != xTip {
		t.Fatalf("push preflight after amend = %+v, want not fast-forward, protected, remote tip %s", push, xTip)
	}
	if res := r.remote(id, gitsession.RemoteOpParams{Kind: "push", Remote: "origin", Branch: "main"}); res.OK {
		t.Fatal("plain push of a rewritten branch succeeded")
	}
	force := gitsession.RemoteOpParams{Kind: "forcePush", Remote: "origin", Branch: "main", ExpectedRemoteTip: push.RemoteTip}
	if res := r.remote(id, force); res.OK || res.Error == nil || res.Error.Kind != "ProtectedBranch" {
		t.Fatalf("force push without the token = %+v, want ProtectedBranch", res)
	}
	if p.bare.Git("rev-parse", "main") != xTip {
		t.Fatal("refused force push moved the remote")
	}
	force.ConfirmToken = push.ResolvedBranch
	if res := r.remote(id, force); !res.OK {
		t.Fatalf("confirmed force push = %+v", res.Error)
	}
	if got := p.bare.Git("rev-parse", "main"); got != amended {
		t.Fatalf("remote main = %s after force push, want %s", got, amended)
	}

	p.x.Git("branch", "tmp")
	if res := r.remote(id, gitsession.RemoteOpParams{Kind: "push", Remote: "origin", Branch: "tmp", SetUpstream: true}); !res.OK {
		t.Fatalf("push tmp = %+v", res.Error)
	}
	if !strings.Contains(p.bare.Git("branch", "--list", "tmp"), "tmp") {
		t.Fatal("tmp is not on the remote")
	}
	if res := r.remote(id, gitsession.RemoteOpParams{Kind: "deleteRemoteBranch", Remote: "origin", Branch: "tmp"}); !res.OK {
		t.Fatalf("delete remote branch = %+v", res.Error)
	}
	if out := p.bare.Git("branch", "--list", "tmp"); out != "" {
		t.Fatalf("tmp still on the remote: %q", out)
	}

	t.Run("pull with merge and rebase on a diverged branch", func(t *testing.T) {
		flowharness.Complete(t)
		for _, strategy := range []string{"merge", "rebase"} {
			for _, conflict := range []bool{false, true} {
				name := strategy + map[bool]string{false: " clean", true: " conflict"}[conflict]
				t.Run(name, func(t *testing.T) {
					rr := newRig(t)
					tag := strings.ReplaceAll(name, " ", "-")
					pair := newRemotePair(rr, tag, map[string]string{"f.txt": "seed\n"})
					rid := rr.open(pair.x.Dir).RepoID
					theirs, ours := "y.txt", "x.txt"
					if conflict {
						theirs, ours = "f.txt", "f.txt"
					}
					pair.y.Commit("theirs", map[string]string{theirs: "theirs\n"})
					pair.y.Git("push", "-q", "origin", "main")
					localTip := pair.x.Commit("ours", map[string]string{ours: "ours\n"})
					if res := rr.remote(rid, gitsession.RemoteOpParams{Kind: "fetch", Remote: "origin"}); !res.OK {
						t.Fatalf("fetch = %+v", res.Error)
					}
					res := rr.remote(rid, gitsession.RemoteOpParams{Kind: "pull", Remote: "origin", Branch: "main", Strategy: strategy})
					if !conflict {
						if !res.OK || res.InProgress != nil {
							t.Fatalf("clean %s pull = %+v", strategy, res)
						}
						parents := strings.Fields(pair.x.Git("log", "-1", "--format=%P"))
						if (strategy == "merge") != (len(parents) == 2) {
							t.Fatalf("%s pull left %d parents on HEAD", strategy, len(parents))
						}
						return
					}
					if res.OK || res.InProgress == nil {
						t.Fatalf("conflicting %s pull = %+v, want an operation in progress", strategy, res)
					}
					rr.mustOp(rid, gitsession.OpRequest{Kind: "opAbort"})
					if got := pair.x.Git("rev-parse", "HEAD"); got != localTip {
						t.Fatalf("HEAD after abort = %s, want %s", got, localTip)
					}
					res = rr.remote(rid, gitsession.RemoteOpParams{Kind: "pull", Remote: "origin", Branch: "main", Strategy: strategy})
					if res.OK || res.InProgress == nil {
						t.Fatalf("second conflicting pull = %+v", res)
					}
					if err := os.WriteFile(filepath.Join(pair.x.Dir, "f.txt"), []byte("resolved\n"), 0o644); err != nil {
						t.Fatal(err)
					}
					pair.x.Git("add", "f.txt")
					done := rr.mustOp(rid, gitsession.OpRequest{Kind: "opContinue"})
					if done.InProgress != nil || pair.x.Git("status", "--porcelain") != "" {
						t.Fatalf("after continue: %+v, status %q", done.InProgress, pair.x.Git("status", "--porcelain"))
					}
				})
			}
		}
	})
}

// J-REM-011: the lease is the tip the user saw. A colleague pushes after the fetch, the force push
// is refused and the remote keeps the colleague's commit. After a fetch the same push lands, and
// plain force needs the typed token.
func TestForcePushLeaseTipSeen(t *testing.T) {
	r := newRig(t)
	p := featPair(r, "seen")
	id := r.open(p.x.Dir).RepoID
	if res := r.remote(id, gitsession.RemoteOpParams{Kind: "fetch", Remote: "origin"}); !res.OK {
		t.Fatalf("fetch = %+v", res.Error)
	}
	seen := p.x.Git("rev-parse", "origin/feat")
	colleague := p.y.Commit("colleague", map[string]string{"y.txt": "y\n"})
	p.y.Git("push", "-q", "origin", "feat")
	r.external(func() { gitNow(t, p.x.Dir, "commit", "-q", "--amend", "-m", "feat work, amended") })
	amended := p.x.Git("rev-parse", "HEAD")

	res := r.remote(id, gitsession.RemoteOpParams{Kind: "forcePush", Remote: "origin", Branch: "feat", ExpectedRemoteTip: &seen})
	if res.OK || res.Error == nil {
		t.Fatalf("force push with a stale lease = %+v, want refusal", res)
	}
	if k := res.Error.Kind; k != "LeaseViolation" && k != "RemoteRefUpdated" {
		t.Fatalf("error kind = %q, want a lease refusal", k)
	}
	if got := p.bare.Git("rev-parse", "feat"); got != colleague {
		t.Fatalf("remote feat = %s after the refused push, want the colleague's %s", got, colleague)
	}

	if f := r.remote(id, gitsession.RemoteOpParams{Kind: "fetch", Remote: "origin"}); !f.OK {
		t.Fatalf("fetch = %+v", f.Error)
	}
	fresh := p.x.Git("rev-parse", "origin/feat")
	if fresh != colleague {
		t.Fatalf("origin/feat = %s after the fetch, want %s", fresh, colleague)
	}
	// Fetched is not integrated: --force-if-includes still refuses until the user overwrites on purpose.
	remoteFailure(t, r.remote(id, gitsession.RemoteOpParams{Kind: "forcePush", Remote: "origin", Branch: "feat", ExpectedRemoteTip: &fresh}), "RemoteRefUpdated")
	if got := p.bare.Git("rev-parse", "feat"); got != colleague {
		t.Fatalf("remote feat = %s after the second refusal, want %s", got, colleague)
	}
	ok := r.remote(id, gitsession.RemoteOpParams{Kind: "forcePush", Remote: "origin", Branch: "feat", ExpectedRemoteTip: &fresh, PlainForce: true})
	if !ok.OK {
		t.Fatalf("plain force after inspecting the incoming commit = %+v", ok.Error)
	}
	if got := p.bare.Git("rev-parse", "feat"); got != amended {
		t.Fatalf("remote feat = %s, want the amended %s", got, amended)
	}

	// A third round: plain force on release/* needs the typed confirmation.
	p.x.Git("branch", "release/1", "HEAD")
	if res := r.remote(id, gitsession.RemoteOpParams{Kind: "push", Remote: "origin", Branch: "release/1", SetUpstream: true}); !res.OK {
		t.Fatalf("push release/1 = %+v", res.Error)
	}
	r.external(func() { gitNow(t, p.x.Dir, "checkout", "-q", "release/1") })
	r.external(func() { gitNow(t, p.x.Dir, "commit", "-q", "--amend", "-m", "release, amended") })
	before := p.bare.Git("rev-parse", "release/1")
	plain := gitsession.RemoteOpParams{Kind: "forcePush", Remote: "origin", Branch: "release/1", PlainForce: true, ExpectedRemoteTip: &before}
	remoteFailure(t, r.remote(id, plain), "ProtectedBranch")
	if got := p.bare.Git("rev-parse", "release/1"); got != before {
		t.Fatal("plain force without the token moved the remote")
	}
	plain.ConfirmToken = "release/1"
	if res := r.remote(id, plain); !res.OK {
		t.Fatalf("plain force with the token = %+v", res.Error)
	}
	if got := p.bare.Git("rev-parse", "release/1"); got != p.x.Git("rev-parse", "HEAD") {
		t.Fatal("remote release/1 is not at the local tip after the confirmed plain force")
	}
}

// J-REM-010: a local branch tracking a differently named remote branch pushes to that name, never
// creates a same-name branch, and the protected-pattern check runs on the remote name.
func TestPushUpstreamWithOtherName(t *testing.T) {
	r := newRig(t)
	p := newRemotePair(r, "other-name", map[string]string{"f.txt": "seed\n"})
	p.x.Git("push", "-q", "origin", "main:release/123")
	p.x.Git("fetch", "-q", "origin")
	p.x.Git("checkout", "-q", "-b", "fix", "--track", "origin/release/123")
	p.x.Commit("fix work", map[string]string{"fix.txt": "fix\n"})
	id := r.open(p.x.Dir).RepoID

	pf := call[gitpreflight.PushPreflight](t, r.gs, "remote.pushPreflight", gitrpc.RemotePushPreflightParams{RepoID: id, Branch: "fix", Remote: "origin"})
	if pf.WouldSetUpstream || pf.Ahead != 1 || pf.Behind != 0 || pf.ResolvedBranch != "release/123" {
		t.Fatalf("push preflight = %+v, want ahead 1 against release/123, no upstream to set", pf)
	}
	if res := r.remote(id, gitsession.RemoteOpParams{Kind: "push", Remote: "origin", Branch: "fix"}); !res.OK {
		t.Fatalf("push = %+v", res.Error)
	}
	if got := p.bare.Git("rev-parse", "refs/heads/release/123"); got != p.x.Git("rev-parse", "fix") {
		t.Fatalf("bare release/123 = %s, want the local tip", got)
	}
	if out := p.bare.Git("branch", "--list", "fix"); out != "" {
		t.Fatalf("a stray same-name branch was created: %q", out)
	}

	r.external(func() { gitNow(t, p.x.Dir, "commit", "-q", "--amend", "-m", "fix work, amended") })
	pf = call[gitpreflight.PushPreflight](t, r.gs, "remote.pushPreflight", gitrpc.RemotePushPreflightParams{RepoID: id, Branch: "fix", Remote: "origin"})
	if pf.ProtectedBy == nil {
		t.Fatalf("push preflight = %+v, want the protected gate: the remote name matches release/*", pf)
	}
	tip := *pf.RemoteTip
	force := gitsession.RemoteOpParams{Kind: "forcePush", Remote: "origin", Branch: "fix", ExpectedRemoteTip: &tip}
	remoteFailure(t, r.remote(id, force), "ProtectedBranch")
	if out := p.bare.Git("branch", "--list", "fix"); out != "" {
		t.Fatalf("force push created a stray branch: %q", out)
	}
}

// J-REM-012: pull strategies keep a dirty unrelated file. ff-only refuses a diverged branch
// without moving refs; merge creates a two-parent commit; rebase is linear with the local commit on
// top; abort returns HEAD and tree to the pre-pull state.
func TestPullStrategiesDirtyTree(t *testing.T) {
	for _, strategy := range []string{"ff-only", "merge", "rebase"} {
		t.Run(strategy, func(t *testing.T) {
			r := newRig(t)
			p := newRemotePair(r, "pull-"+strategy, map[string]string{"f.txt": "seed\n", "d.txt": "dirty base\n"})
			id := r.open(p.x.Dir).RepoID
			p.y.Commit("theirs 1", map[string]string{"y1.txt": "1\n"})
			theirs := p.y.Commit("theirs 2", map[string]string{"y2.txt": "2\n"})
			p.y.Git("push", "-q", "origin", "main")
			local := p.x.Commit("ours", map[string]string{"x.txt": "x\n"})
			p.x.Write("d.txt", "dirty edit\n")
			if res := r.remote(id, gitsession.RemoteOpParams{Kind: "fetch", Remote: "origin"}); !res.OK {
				t.Fatalf("fetch = %+v", res.Error)
			}
			res := r.remote(id, gitsession.RemoteOpParams{Kind: "pull", Remote: "origin", Branch: "main", Strategy: strategy})
			dirty, _ := os.ReadFile(filepath.Join(p.x.Dir, "d.txt"))
			switch strategy {
			case "ff-only":
				if res.OK || res.Error == nil {
					t.Fatalf("ff-only pull of a diverged branch = %+v, want refusal", res)
				}
				if got := p.x.Git("rev-parse", "HEAD"); got != local {
					t.Fatalf("refused pull moved HEAD to %s", got)
				}
			case "merge":
				if !res.OK {
					t.Fatalf("merge pull = %+v", res.Error)
				}
				if parents := strings.Fields(p.x.Git("log", "-1", "--format=%P")); len(parents) != 2 {
					t.Fatalf("merge pull left %d parents", len(parents))
				}
			case "rebase":
				// git refuses to rebase over an unstaged edit; the app reports it and changes nothing.
				if res.OK || res.Error == nil || res.Error.Kind != "DirtyWorktree" {
					t.Fatalf("rebase pull over a dirty tree = %+v, want DirtyWorktree", res)
				}
				if got := p.x.Git("rev-parse", "HEAD"); got != local {
					t.Fatalf("refused rebase moved HEAD to %s", got)
				}
				p.x.Git("checkout", "-q", "--", "d.txt")
				if res := r.remote(id, gitsession.RemoteOpParams{Kind: "pull", Remote: "origin", Branch: "main", Strategy: "rebase"}); !res.OK {
					t.Fatalf("rebase pull on a clean tree = %+v", res.Error)
				}
				if parents := strings.Fields(p.x.Git("log", "-1", "--format=%P")); len(parents) != 1 {
					t.Fatalf("rebase pull left %d parents", len(parents))
				}
				if p.x.Git("log", "-1", "--format=%s") != "ours" || p.x.Git("rev-parse", "HEAD~1") != theirs {
					t.Fatal("local commit is not on top of theirs")
				}
				return
			}
			if string(dirty) != "dirty edit\n" {
				t.Fatalf("d.txt = %q after %s pull, want the dirty edit untouched", dirty, strategy)
			}
			if got := p.x.Git("status", "--porcelain"); got != " M d.txt" {
				t.Fatalf("status = %q, want only the dirty file", got)
			}
		})
	}

	t.Run("rebase conflict abort", func(t *testing.T) {
		r := newRig(t)
		p := newRemotePair(r, "pull-abort", map[string]string{"f.txt": "seed\n"})
		id := r.open(p.x.Dir).RepoID
		p.y.Commit("theirs", map[string]string{"f.txt": "theirs\n"})
		p.y.Git("push", "-q", "origin", "main")
		local := p.x.Commit("ours", map[string]string{"f.txt": "ours\n"})
		if res := r.remote(id, gitsession.RemoteOpParams{Kind: "fetch", Remote: "origin"}); !res.OK {
			t.Fatalf("fetch = %+v", res.Error)
		}
		res := r.remote(id, gitsession.RemoteOpParams{Kind: "pull", Remote: "origin", Branch: "main", Strategy: "rebase"})
		if res.OK || res.InProgress == nil {
			t.Fatalf("conflicting rebase pull = %+v, want an operation in progress", res)
		}
		r.mustOp(id, gitsession.OpRequest{Kind: "opAbort"})
		if got := p.x.Git("rev-parse", "HEAD"); got != local {
			t.Fatalf("HEAD after abort = %s, want %s", got, local)
		}
		if got := p.x.Git("status", "--porcelain"); got != "" {
			t.Fatalf("status after abort = %q, want clean", got)
		}
	})
}

// J-REM-014: deleting a remote branch that needs a credential. Cancelling the prompt leaves the
// bare ref and the local refs alone and ends the op as cancelled; the right answer deletes it.
func TestDeleteRemoteBranchCredentialCancel(t *testing.T) {
	app := flowharness.New(t)
	seed := app.NewRepo("seed")
	seed.Commit("base", map[string]string{"a.txt": "a\n"})
	seed.Branch("tmp", "")
	bare := app.NewBare("remote")
	bare.PushFrom(seed)
	bare.Git("config", "http.receivepack", "true")
	url, _ := smartHTTP(t, app.Work, "ana", "s3cret")
	local := bare.Clone(filepath.Join(app.Work, "local"))
	local.Git("remote", "set-url", "origin", strings.Replace(url, "http://", "http://ana@", 1)+"/remote.git")
	local.Git("branch", "tmp-local")
	refsBefore := local.Git("for-each-ref", "--format=%(refname) %(objectname)")
	gs := app.OpenGitStream()
	id := openOn(t, gs, local.Dir).RepoID
	del := gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: gitsession.RemoteOpParams{Kind: "deleteRemoteBranch", Remote: "origin", Branch: "tmp"}}

	run := func() <-chan gitsession.RemoteOpResult {
		ch := make(chan gitsession.RemoteOpResult, 1)
		go func() {
			var got gitsession.RemoteOpResult
			_ = gs.Request("remote.run", del, &got)
			ch <- got
		}()
		return ch
	}

	done := run()
	testx.WaitUntil(t, wait, func() bool { return len(app.W.GitCredential.Pending()) > 0 })
	req := app.W.GitCredential.Pending()[0]
	if ok, err := app.W.GitCredential.Provide(bridge.GitCredentialProvideArgs{RequestID: req.RequestID}); err != nil || !ok {
		t.Fatalf("cancel = %v, %v", ok, err)
	}
	res := <-done
	if res.OK || res.Error == nil || res.Error.Kind == "" {
		t.Fatalf("cancelled delete = %+v, want a classified failure", res)
	}
	if res.Error.Kind != "Cancelled" && res.Error.Kind != "AuthFailed" {
		t.Fatalf("cancelled delete kind = %q, want Cancelled or AuthFailed, never a raw error", res.Error.Kind)
	}
	if !strings.Contains(bare.Git("branch", "--list", "tmp"), "tmp") {
		t.Fatal("the bare ref vanished after a cancelled delete")
	}
	if got := local.Git("for-each-ref", "--format=%(refname) %(objectname)"); got != refsBefore {
		t.Fatalf("local refs changed after a cancelled delete:\n%s\n->\n%s", refsBefore, got)
	}

	done = run()
	answerLogin(t, app, "ana", "s3cret")
	if res := <-done; !res.OK {
		t.Fatalf("delete with the right password = %+v", res.Error)
	}
	if out := bare.Git("branch", "--list", "tmp"); out != "" {
		t.Fatalf("tmp still on the remote: %q", out)
	}
}
