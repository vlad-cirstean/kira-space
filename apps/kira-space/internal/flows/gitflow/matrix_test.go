package gitflow_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func undoLabel(t *testing.T, gs *flowharness.GitStream, id string) (string, bool) {
	t.Helper()
	peek := call[gitrpc.UndoPeekResult](t, gs, "undo.peek", gitrpc.UndoPeekParams{RepoID: id})
	if peek.Slot == nil {
		return "", false
	}
	return peek.Slot.Label, true
}

func TestUndoSlotIsSharedAndAttributed(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("one", map[string]string{"f.txt": "1\n"})
	repo.Tag("t1")
	repo.Tag("t2")
	repo.Branch("sibling", "")
	idA := r.open(repo.Dir).RepoID
	other := r.app.OpenGitStream()
	idB := openOn(t, other, repo.Dir).RepoID

	r.mustOp(idA, gitsession.OpRequest{Kind: "tagDelete", Name: "t1"})
	own, _ := undoLabel(t, r.gs, idA)
	seen, _ := undoLabel(t, other, idB)
	if own != "Deleted tag t1" || seen != "Deleted tag t1 (window: Kira Space)" {
		t.Fatalf("labels after A's delete: own %q, other %q", own, seen)
	}

	var del gitsession.OpResult
	other.MustRequest("op.run", gitrpc.OpRunParams{RepoID: idB, Op: gitsession.OpRequest{Kind: "tagDelete", Name: "t2"}}, &del)
	own, _ = undoLabel(t, other, idB)
	seen, _ = undoLabel(t, r.gs, idA)
	if own != "Deleted tag t2" || seen != "Deleted tag t2 (window: Kira Space)" {
		t.Fatalf("labels after B's delete: own %q, other %q", own, seen)
	}

	r.mustOp(idA, gitsession.OpRequest{Kind: "branchRename", From: "sibling", To: "sibling2"})
	if label, ok := undoLabel(t, other, idB); ok {
		t.Fatalf("slot after another op = %q, want empty", label)
	}
}

// Guards the undo of a branch delete: sha and tracking config come back, and a second undo is refused.
func TestUndoBranchDeleteRestoresTracking(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	tip := repo.Commit("one", map[string]string{"f.txt": "1\n"})
	repo.Branch("tracked", "")
	repo.Git("config", "--local", "branch.tracked.remote", "origin")
	repo.Git("config", "--local", "branch.tracked.merge", "refs/heads/tracked")
	id := r.open(repo.Dir).RepoID

	del := r.mustOp(id, gitsession.OpRequest{Kind: "branchDelete", Name: "tracked", Force: true})
	if del.Undo == nil || del.Undo.Label != "Deleted branch tracked" {
		t.Fatalf("undo snapshot = %+v", del.Undo)
	}
	if label, _ := undoLabel(t, r.gs, id); label != del.Undo.Label {
		t.Fatalf("undo.peek = %q, want %q", label, del.Undo.Label)
	}

	undo := call[gitsession.OpResult](t, r.gs, "undo.run", gitrpc.UndoRunParams{RepoID: id, ID: del.Undo.ID})
	if !undo.OK {
		t.Fatalf("undo.run failed: %+v", undo.Error)
	}
	if got := repo.Git("rev-parse", "--verify", "refs/heads/tracked"); got != tip {
		t.Fatalf("restored branch = %s, want %s", got, tip)
	}
	cfg := repo.Git("config", "--get-regexp", `^branch\.tracked\.`)
	if !strings.Contains(cfg, "branch.tracked.remote origin") || !strings.Contains(cfg, "branch.tracked.merge refs/heads/tracked") {
		t.Fatalf("restored config = %q", cfg)
	}

	again := call[gitsession.OpResult](t, r.gs, "undo.run", gitrpc.UndoRunParams{RepoID: id, ID: del.Undo.ID})
	if again.OK || again.Error == nil || again.Error.Kind != "NotFound" {
		t.Fatalf("second undo.run = %+v, want NotFound", again)
	}
}

func TestDetailCacheDropsForEveryWindowOnRefsChanged(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	root := repo.Commit("root", map[string]string{"f.txt": "1\n"})
	idA := r.open(repo.Dir).RepoID
	other := r.app.OpenGitStream()
	idB := openOn(t, other, repo.Dir).RepoID
	hasTag := func(gs *flowharness.GitStream, id string) bool {
		d := call[porcelain.CommitDetail](t, gs, "commit.detail", gitrpc.CommitDetailParams{RepoID: id, SHA: root})
		for _, dec := range d.Decoration {
			if dec.Kind == porcelain.DecorationTag && dec.Name == "v-shared" {
				return true
			}
		}
		return false
	}
	if hasTag(r.gs, idA) || hasTag(other, idB) {
		t.Fatal("tag decoration before the tag exists")
	}

	externalOn(t, func() { repo.Tag("v-shared") }, r.gs, other)
	if !hasTag(r.gs, idA) || !hasTag(other, idB) {
		t.Fatal("a window still serves the stale cached detail after refsChanged")
	}
}

func TestCommitDetailMergeParentSelector(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("root", map[string]string{"f.txt": "1\n"})
	repo.Branch("side", "")
	repo.Checkout("side")
	repo.Commit("side work", map[string]string{"side.txt": "s\n"})
	repo.Checkout("main")
	repo.Commit("main work", map[string]string{"main.txt": "m\n"})
	merge := repo.Merge("side", "merge side")
	id := r.open(repo.Dir).RepoID

	paths := func(parent int) []string {
		d := call[porcelain.CommitDetail](t, r.gs, "commit.detail", gitrpc.CommitDetailParams{RepoID: id, SHA: merge, ParentIndex: &parent})
		r.app.Contract(t, "git-commit-detail", "git:commit.detail#merge-parent-"+strconv.Itoa(parent), d, flowharness.Mask("sha", "parents", "timestamp"))
		var out []string
		for _, f := range d.Files {
			out = append(out, f.Path)
		}
		return out
	}
	if first, second := paths(0), paths(1); reflect.DeepEqual(first, second) {
		t.Fatalf("parentIndex 0 and 1 list the same files: %v", first)
	}
	bad := 7
	if we := wireErr(t, r.gs, "commit.detail", gitrpc.CommitDetailParams{RepoID: id, SHA: merge, ParentIndex: &bad}); we.Code != "E_BAD_REQUEST" {
		t.Fatalf("out-of-range parentIndex error = %+v, want E_BAD_REQUEST", we)
	}
}

func TestUnservedOpKindIsRefused(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("one", map[string]string{"f.txt": "1\n"})
	id := r.open(repo.Dir).RepoID

	we := wireErr(t, r.gs, "op.run", gitrpc.OpRunParams{RepoID: id, Op: gitsession.OpRequest{Kind: "tagPush"}})
	if we.Code != "E_UNKNOWN_METHOD" || !strings.Contains(we.Message, "tagPush") {
		t.Fatalf("error = %+v, want E_UNKNOWN_METHOD naming tagPush", we)
	}
}

func TestStalledStreamBlocksOnlyItsOwnConnection(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	for i := 0; i < 3; i++ {
		repo.Commit(fmt.Sprintf("c%d", i), map[string]string{"f.txt": fmt.Sprintf("%d\n", i)})
	}
	idA := r.open(repo.Dir).RepoID
	other := r.app.OpenGitStream()
	idB := openOn(t, other, repo.Dir).RepoID

	stalled := r.gs.Stream("graph.stream", m{"repoId": idA}, 0)
	// No signal says the stream holds its walk yet; a git log plus parse of 3 commits takes a few ms.
	time.Sleep(150 * time.Millisecond)
	loaded := make(chan error, 1)
	go func() { loaded <- r.gs.Request("graph.loadMore", gitrpc.GraphLoadMoreParams{RepoID: idA}, nil) }()
	select {
	case err := <-loaded:
		t.Fatalf("A's graph.loadMore returned (%v) while its own stream is stalled on credits", err)
	case <-time.After(300 * time.Millisecond):
	}

	if got := graphAll(t, other, idB, 10); len(got) != 3 {
		t.Fatalf("B streamed %d rows, want 3", len(got))
	}
	call[gitsession.RefsResult](t, other, "refs.list", gitrpc.RefsListParams{RepoID: idB})

	stalled.Cancel()
	select {
	case <-loaded:
	case <-time.After(wait):
		t.Fatal("A's graph.loadMore never returned after cancelling the stalled stream")
	}
}

// slowFetch makes every fetch from origin in dir run for secs seconds first, and returns the marker
// file the wrapper creates once it is running.
func slowFetch(t *testing.T, root string, repo *flowharness.Repo, secs int) string {
	t.Helper()
	started := filepath.Join(root, fmt.Sprintf("fetch-started-%d", secs))
	wrapper := filepath.Join(root, fmt.Sprintf("slow-upload-pack-%d", secs))
	script := fmt.Sprintf("#!/bin/sh\ntouch %s\nsleep %d &\nwait\nexec git upload-pack \"$@\"\n", started, secs)
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	repo.Git("config", "remote.origin.uploadpack", wrapper)
	return started
}

func fileExists(path string) func() bool {
	return func() bool { _, err := os.Stat(path); return err == nil }
}

var fetchOp = gitsession.RemoteOpParams{Kind: "fetch", Remote: "origin"}

func asyncFetch(gs *flowharness.GitStream, id string) <-chan gitsession.RemoteOpResult {
	out := make(chan gitsession.RemoteOpResult, 1)
	go func() {
		var res gitsession.RemoteOpResult
		_ = gs.Request("remote.run", gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: fetchOp}, &res)
		out <- res
	}()
	return out
}

func TestSimultaneousRemoteRunsAdmitExactlyOne(t *testing.T) {
	r := newRig(t)
	p := newRemotePair(r, "one", map[string]string{"f.txt": "seed\n"})
	started := slowFetch(t, r.app.Root, p.x, 120)
	id := r.open(p.x.Dir).RepoID
	const n = 4
	streams := []*flowharness.GitStream{r.gs}
	for i := 1; i < n; i++ {
		gs := r.app.OpenGitStream()
		openOn(t, gs, p.x.Dir)
		streams = append(streams, gs)
	}

	replies := make(chan gitsession.RemoteOpResult, n)
	var wg sync.WaitGroup
	for _, gs := range streams {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var res gitsession.RemoteOpResult
			_ = gs.Request("remote.run", gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: fetchOp}, &res)
			replies <- res
		}()
	}
	testx.WaitUntil(t, wait, fileExists(started))
	for i := 0; i < n-1; i++ {
		select {
		case res := <-replies:
			remoteFailure(t, res, "OperationInProgress")
		case <-time.After(wait):
			t.Fatalf("only %d of %d losers reported OperationInProgress", i, n-1)
		}
	}
	select {
	case res := <-replies:
		t.Fatalf("a second connection was admitted: %+v", res)
	case <-time.After(200 * time.Millisecond):
	}

	controller := r.app.OpenGitStream()
	openOn(t, controller, p.x.Dir)
	if !call[gitrpc.RemoteCancelResult](t, controller, "remote.cancel", gitrpc.RemoteCancelParams{RepoID: id}).Cancelled {
		t.Fatal("remote.cancel from another window reported nothing running")
	}
	select {
	case res := <-replies:
		remoteFailure(t, res, "Cancelled")
	case <-time.After(wait):
		t.Fatal("the winner never answered after remote.cancel")
	}
	wg.Wait()
	if call[gitrpc.RemoteCancelResult](t, controller, "remote.cancel", gitrpc.RemoteCancelParams{RepoID: id}).Cancelled {
		t.Fatal("remote.cancel with the slot free reported true")
	}
}

func TestLocalWriteAndRemoteOpAreNotMutuallyExclusive(t *testing.T) {
	r := newRig(t)
	p := newRemotePair(r, "mutex", map[string]string{"f.txt": "seed\n"})
	p.x.Git("branch", "side")
	started := slowFetch(t, r.app.Root, p.x, 120)
	id := r.open(p.x.Dir).RepoID
	b := r.app.OpenGitStream()
	openOn(t, b, p.x.Dir)

	fetching := asyncFetch(r.gs, id)
	testx.WaitUntil(t, wait, fileExists(started))

	var res gitsession.OpResult
	b.MustRequest("op.run", gitrpc.OpRunParams{RepoID: id, Op: gitsession.OpRequest{Kind: "checkout", Target: "side", Mode: "switch"}}, &res)
	if !res.OK || res.Head.Name != "side" {
		t.Fatalf("checkout during a fetch = %+v", res)
	}
	var busy gitsession.RemoteOpResult
	b.MustRequest("remote.run", gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: fetchOp}, &busy)
	remoteFailure(t, busy, "OperationInProgress")

	if !call[gitrpc.RemoteCancelResult](t, b, "remote.cancel", gitrpc.RemoteCancelParams{RepoID: id}).Cancelled {
		t.Fatal("cancelling the running fetch reported false")
	}
	select {
	case got := <-fetching:
		remoteFailure(t, got, "Cancelled")
	case <-time.After(wait):
		t.Fatal("the fetch never answered after remote.cancel")
	}
}

func TestTwoRepositoriesShareNothing(t *testing.T) {
	r := newRig(t)
	repoA := r.app.NewRepo("repo-a")
	repoA.Commit("a1", map[string]string{"a.txt": "a\n"})
	repoB := r.app.NewRepo("repo-b")
	repoB.Commit("b1", map[string]string{"b.txt": "b\n"})
	other := r.app.OpenGitStream()
	idA := r.open(repoA.Dir).RepoID
	idB := openOn(t, other, repoB.Dir).RepoID
	if idA == idB {
		t.Fatal("two repositories got one id")
	}

	var onlyInA string
	externalOn(t, func() { onlyInA = repoA.Commit("a2", map[string]string{"a.txt": "a2\n"}) }, r.gs)
	time.Sleep(300 * time.Millisecond)
	if n := len(other.Events("repo.changed")); n != 0 {
		t.Fatalf("B heard %d events for a write to repository A", n)
	}

	r.mustOp(idA, gitsession.OpRequest{Kind: "branchCreate", Name: "tmp", StartPoint: "main"})
	r.mustOp(idA, gitsession.OpRequest{Kind: "branchDelete", Name: "tmp"})
	if label, ok := undoLabel(t, other, idB); ok {
		t.Fatalf("B's undo slot = %q after A's delete, want empty", label)
	}
	for gs, id := range map[*flowharness.GitStream]string{r.gs: idA, other: idB} {
		if call[gitrpc.RemoteCancelResult](t, gs, "remote.cancel", gitrpc.RemoteCancelParams{RepoID: id}).Cancelled {
			t.Fatal("remote.cancel reported true with nothing running")
		}
	}
	wireErr(t, other, "commit.detail", gitrpc.CommitDetailParams{RepoID: idB, SHA: onlyInA})
}

func goroutinesSettled(before int) (after int, ok bool) {
	const margin = 5
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		after = runtime.NumGoroutine()
		if after <= before+margin {
			return after, true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return after, false
}

func gitProcessesUnder(t *testing.T, dir string) int {
	t.Helper()
	abs, err := filepath.EvalSymlinks(dir)
	if err != nil {
		abs = dir
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Skip("/proc unavailable")
	}
	n := 0
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil || strings.TrimSpace(string(comm)) != "git" {
			continue
		}
		cwd, err := os.Readlink(filepath.Join("/proc", e.Name(), "cwd"))
		if err == nil && (cwd == abs || strings.HasPrefix(cwd, abs+string(filepath.Separator))) {
			n++
		}
	}
	return n
}

func TestConnectionCyclesLeakNothing(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/proc is Linux-only")
	}
	r := newRig(t)
	repoA := r.app.NewRepo("repo-a")
	repoA.Commit("a1", map[string]string{"a.txt": "a\n"})
	repoB := r.app.NewRepo("repo-b")
	repoB.Commit("b1", map[string]string{"b.txt": "b\n"})

	cycle := func() {
		gs := r.app.OpenGitStream()
		id := openOn(t, gs, repoA.Dir).RepoID
		openOn(t, gs, repoB.Dir)
		gs.Stream("graph.stream", m{"repoId": id}, 1).Cancel()
		gs.Close()
	}
	cycle()
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()
	for i := 0; i < 20; i++ {
		cycle()
	}
	if after, ok := goroutinesSettled(before); !ok {
		t.Fatalf("goroutines grew from %d to %d over 20 connection cycles", before, after)
	}
	testx.WaitUntil(t, wait, func() bool { return gitProcessesUnder(t, repoA.Dir)+gitProcessesUnder(t, repoB.Dir) == 0 })
}

// blockCheckouts installs a post-checkout hook that holds every checkout until the returned
// release function runs, and returns the marker created once a checkout is held.
func blockCheckouts(t *testing.T, repo *flowharness.Repo, root string) (started string, release func()) {
	t.Helper()
	started = filepath.Join(root, "checkout-started")
	gate := filepath.Join(root, "checkout-release")
	hook := fmt.Sprintf("#!/bin/sh\ntouch %s\nwhile [ ! -e %s ]; do sleep 0.05; done\n", started, gate)
	if err := os.WriteFile(filepath.Join(repo.Dir, ".git", "hooks", "post-checkout"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	return started, func() {
		if err := os.WriteFile(gate, nil, 0o644); err != nil {
			t.Error(err)
		}
	}
}

func TestWriteSurvivesCancelAndDisconnect(t *testing.T) {
	for _, how := range []string{"client cancel", "disconnect", "window close"} {
		t.Run(how, func(t *testing.T) {
			r := newRig(t)
			repo := r.app.NewRepo("proj")
			repo.Commit("one", map[string]string{"f.txt": "1\n"})
			repo.Branch("sibling", "")
			started, release := blockCheckouts(t, repo, r.app.Root)
			idA := r.open(repo.Dir).RepoID
			observer := r.app.OpenGitStream()
			idB := openOn(t, observer, repo.Dir).RepoID

			cancel := r.gs.Fire("op.run", gitrpc.OpRunParams{RepoID: idA, Op: gitsession.OpRequest{Kind: "checkout", Target: "sibling", Mode: "switch"}})
			testx.WaitUntil(t, wait, fileExists(started))

			closed := make(chan struct{})
			switch how {
			case "client cancel":
				cancel()
				close(closed)
			case "window close":
				r.app.W.GitWindows.Close(r.gs.Key())
				close(closed)
			default:
				go func() { r.gs.Close(); close(closed) }()
			}
			release()
			<-closed

			testx.WaitUntil(t, wait, func() bool {
				return call[gitsession.RefsResult](t, observer, "refs.list", gitrpc.RefsListParams{RepoID: idB}).Head.Name == "sibling"
			})
		})
	}
}

func TestDisconnectDuringRemoteOpKeepsTheSlotUntilItEnds(t *testing.T) {
	r := newRig(t)
	p := newRemotePair(r, "gone", map[string]string{"f.txt": "seed\n"})
	started := slowFetch(t, r.app.Root, p.x, 2)
	id := r.open(p.x.Dir).RepoID
	b := r.app.OpenGitStream()
	openOn(t, b, p.x.Dir)

	r.gs.Fire("remote.run", gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: fetchOp})
	testx.WaitUntil(t, wait, fileExists(started))
	go r.gs.Close()

	var busy gitsession.RemoteOpResult
	b.MustRequest("remote.run", gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: fetchOp}, &busy)
	remoteFailure(t, busy, "OperationInProgress")

	testx.WaitUntil(t, wait, func() bool {
		var res gitsession.RemoteOpResult
		b.MustRequest("remote.run", gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: fetchOp}, &res)
		if res.Error != nil && res.Error.Kind != "OperationInProgress" {
			t.Fatalf("remote.run = %+v, want success or OperationInProgress", res)
		}
		return res.Error == nil
	})
	if runtime.GOOS == "linux" {
		testx.WaitUntil(t, wait, func() bool { return gitProcessesUnder(t, p.x.Dir) == 0 })
	}
}

func TestDisconnectDuringCredentialPromptFreesTheSlot(t *testing.T) {
	r := newRig(t)
	seed := r.app.NewRepo("seed")
	seed.Commit("base", map[string]string{"a.txt": "a\n"})
	bare := r.app.NewBare("remote")
	bare.PushFrom(seed)
	url, _ := smartHTTP(t, r.app.Work, "ana", "s3cret")
	local := bare.Clone(filepath.Join(r.app.Work, "local"))
	local.Git("remote", "set-url", "origin", strings.Replace(url, "http://", "http://ana@", 1)+"/remote.git")
	id := r.open(local.Dir).RepoID
	b := r.app.OpenGitStream()
	openOn(t, b, local.Dir)

	r.gs.Fire("remote.run", gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: fetchOp})
	var first string
	testx.WaitUntil(t, wait, func() bool {
		p := r.app.W.GitCredential.Pending()
		if len(p) == 0 {
			return false
		}
		first = p[0].RequestID
		return true
	})
	go r.gs.Close()

	// B retries until A's disconnect frees the slot; its attempt then raises its own prompt.
	res := make(chan gitsession.RemoteOpResult, 1)
	go func() {
		for {
			var got gitsession.RemoteOpResult
			_ = b.Request("remote.run", gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: fetchOp}, &got)
			if got.Error == nil || got.Error.Kind != "OperationInProgress" {
				res <- got
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	answerLogin(t, r.app, "ana", "wrong", first)
	select {
	case got := <-res:
		remoteFailure(t, got, "AuthFailed")
	case <-time.After(wait):
		t.Fatal("B's prompted fetch never finished")
	}
}
