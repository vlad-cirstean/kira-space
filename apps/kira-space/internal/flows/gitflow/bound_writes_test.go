package gitflow_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const gitEventRemoteProgress = bridge.GitEventPrefix + "remote.progress"

// boundHead reads the checked-out branch name through the bound GitService.
func boundHead(t *testing.T, svc *bridge.GitService, window, id string) string {
	t.Helper()
	refs, err := svc.RefsList(context.Background(), bridge.GitRefsListArgs{WindowKey: window, Params: gitrpc.RefsListParams{RepoID: id}})
	if err != nil {
		t.Fatal(err)
	}
	return refs.Head.Name
}

// GitService carries the write methods too, with the stream's results: every preflight, stash, op,
// undo, remote, worktree and stack method answers bound; cancel, window close and credential
// prompts behave as they do over a stream.
func TestGitServiceBoundWrites(t *testing.T) {
	const scenario = "git-bound-writes"
	app := flowharness.New(t)
	svc := app.W.GitSvc
	ctx := context.Background()
	mask := flowharness.Mask("sha", "parents", "timestamp", "authorTimeSeconds", "committerDate", "objectId", "id", "createdAt", "baseSha", "indexSha", "oldSha", "newSha", "old", "new")
	chk := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	for i, key := range []string{"w1", "w4"} {
		app.W.Windows.Add(key, i, nil, func() {})
	}

	seed := app.NewRepo("seed")
	seed.Commit("base", map[string]string{"f.txt": "1\n"})
	bare := app.NewBare("remote")
	bare.PushFrom(seed)
	local := bare.Clone(filepath.Join(app.Work, "local"))
	local.Branch("sibling", "")
	local.Branch("feat", "")
	local.Checkout("feat")
	featTip := local.Commit("feat work", map[string]string{"feat.txt": "x\n"})
	local.Checkout("main")
	mainTip := local.Commit("main moves", map[string]string{"m.txt": "m\n"})

	open, err := svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: "w1", Params: gitrpc.RepoOpenParams{Path: local.Dir}})
	chk(err)
	id := open.Repo.RepoID
	run := func(op gitsession.OpRequest) gitsession.OpResult {
		t.Helper()
		res, err := svc.OpRun(ctx, bridge.GitOpRunArgs{WindowKey: "w1", Params: gitrpc.OpRunParams{RepoID: id, Op: op}})
		chk(err)
		if !res.OK {
			t.Fatalf("op.run %s = %+v", op.Kind, res.Error)
		}
		return res
	}

	// Preflights.
	_, err = svc.PreflightCheckout(ctx, bridge.GitPreflightCheckoutArgs{WindowKey: "w1", Params: gitrpc.PreflightCheckoutParams{RepoID: id, Target: "sibling", Mode: "switch"}})
	chk(err)
	_, err = svc.PreflightRevert(ctx, bridge.GitPreflightRevertArgs{WindowKey: "w1", Params: gitrpc.PreflightRevertParams{RepoID: id, Shas: []string{mainTip}}})
	chk(err)
	_, err = svc.PreflightReset(ctx, bridge.GitPreflightResetArgs{WindowKey: "w1", Params: gitrpc.PreflightResetParams{RepoID: id, Target: "sibling", Mode: "mixed"}})
	chk(err)
	_, err = svc.PreflightCherryPick(ctx, bridge.GitPreflightCherryPickArgs{WindowKey: "w1", Params: gitrpc.PreflightCherryPickParams{RepoID: id, SHA: featTip}})
	chk(err)

	// Stash.
	local.Write("f.txt", "dirty\n")
	msg := "wip"
	run(gitsession.OpRequest{Kind: "stashPush", Message: &msg})
	stashes, err := svc.StashList(ctx, bridge.GitStashListArgs{WindowKey: "w1", Params: gitrpc.StashListParams{RepoID: id}})
	chk(err)
	if len(stashes.Entries) != 1 {
		t.Fatalf("StashList = %+v, want one entry", stashes)
	}
	app.Contract(t, scenario, "git:stash.list#bound", stashes, mask)
	sha := stashes.Entries[0].Sha
	_, err = svc.StashShow(ctx, bridge.GitStashShowArgs{WindowKey: "w1", Params: gitrpc.StashShowParams{RepoID: id, SHA: sha}})
	chk(err)
	_, err = svc.PreflightStashPop(ctx, bridge.GitPreflightStashPopArgs{WindowKey: "w1", Params: gitrpc.PreflightStashPopParams{RepoID: id, SHA: sha}})
	chk(err)
	_, err = svc.PreflightStashBranch(ctx, bridge.GitPreflightStashBranchArgs{WindowKey: "w1", Params: gitrpc.PreflightStashBranchParams{RepoID: id, SHA: sha, Branch: "from-stash"}})
	chk(err)
	_, err = svc.GlobalStashList(ctx, bridge.GitGlobalStashListArgs{WindowKey: "w1", Params: gitrpc.GlobalStashListParams{RepoID: id}})
	chk(err)
	run(gitsession.OpRequest{Kind: "stashDrop", Sha: sha})

	// Ops and undo.
	checkout := run(gitsession.OpRequest{Kind: "checkout", Target: "sibling", Mode: "switch"})
	app.Contract(t, scenario, "git:op.run#bound", checkout, mask)
	run(gitsession.OpRequest{Kind: "checkout", Target: "main", Mode: "switch"})
	local.Tag("t1")
	created := run(gitsession.OpRequest{Kind: "tagDelete", Name: "t1"})
	peek, err := svc.UndoPeek(ctx, bridge.GitUndoPeekArgs{WindowKey: "w1", Params: gitrpc.UndoPeekParams{RepoID: id}})
	chk(err)
	if peek.Slot == nil || created.Undo == nil {
		t.Fatalf("UndoPeek = %+v after tagDelete (undo %+v)", peek, created.Undo)
	}
	app.Contract(t, scenario, "git:undo.peek#bound", peek, mask)
	undone, err := svc.UndoRun(ctx, bridge.GitUndoRunArgs{WindowKey: "w1", Params: gitrpc.UndoRunParams{RepoID: id, ID: created.Undo.ID}})
	chk(err)
	if !undone.OK {
		t.Fatalf("UndoRun = %+v", undone.Error)
	}

	// Stack.
	parent := "main"
	run(gitsession.OpRequest{Kind: "stackSet", Branch: "feat", Parent: &parent})
	stacks, err := svc.StackList(ctx, bridge.GitStackListArgs{WindowKey: "w1", Params: gitrpc.StackListParams{RepoID: id}})
	chk(err)
	app.Contract(t, scenario, "git:stack.list#bound", stacks, mask)
	_, err = svc.PreflightRestack(ctx, bridge.GitPreflightRestackArgs{WindowKey: "w1", Params: gitrpc.PreflightRestackParams{RepoID: id, Branch: "feat"}})
	chk(err)
	restacked, err := svc.StackRestack(ctx, bridge.GitStackRestackArgs{WindowKey: "w1", Params: gitrpc.StackRestackParams{RepoID: id, Branch: "feat"}})
	chk(err)
	if !restacked.OK {
		t.Fatalf("StackRestack = %+v", restacked)
	}
	idle, err := svc.StackCancelRestack(ctx, bridge.GitStackCancelRestackArgs{WindowKey: "w1", Params: gitrpc.StackCancelRestackParams{RepoID: id}})
	chk(err)
	if idle.Cancelled {
		t.Fatal("StackCancelRestack reported cancelling an idle repo")
	}

	// Worktree.
	wtPath := filepath.Join(app.Work, "wt")
	_, err = svc.PreflightWorktreeAdd(ctx, bridge.GitPreflightWorktreeAddArgs{WindowKey: "w1", Params: gitrpc.PreflightWorktreeAddParams{RepoID: id, Path: wtPath, Mode: "newBranch", Branch: "wt-b", StartPoint: "main"}})
	chk(err)
	run(gitsession.OpRequest{Kind: "worktreeAdd", Path: wtPath, Mode: "newBranch", Branch: "wt-b", StartPoint: "main"})
	trees, err := svc.WorktreeList(ctx, bridge.GitWorktreeListArgs{WindowKey: "w1", Params: gitrpc.WorktreeListParams{RepoID: id}})
	chk(err)
	if len(trees.Worktrees) != 2 {
		t.Fatalf("WorktreeList = %+v, want 2", trees)
	}
	app.Contract(t, scenario, "git:worktree.list#bound", trees, flowharness.Mask("path", "head", "sha"))
	_, err = svc.PreflightWorktreeRemove(ctx, bridge.GitPreflightWorktreeRemoveArgs{WindowKey: "w1", Params: gitrpc.PreflightWorktreeRemoveParams{RepoID: id, Path: wtPath}})
	chk(err)

	// Remote, with progress addressed to the window that holds the repo.
	_, err = svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: "w2", Params: gitrpc.RepoOpenParams{Path: local.Dir}})
	chk(err)
	seed.Commit("upstream", map[string]string{"u.txt": "u\n"})
	bare.PushFrom(seed)
	_, err = svc.RemotePullPreflight(ctx, bridge.GitRemotePullPreflightArgs{WindowKey: "w1", Params: gitrpc.RemotePullPreflightParams{RepoID: id, Branch: "main"}})
	chk(err)
	_, err = svc.RemotePushPreflight(ctx, bridge.GitRemotePushPreflightArgs{WindowKey: "w1", Params: gitrpc.RemotePushPreflightParams{RepoID: id, Branch: "main", Remote: "origin"}})
	chk(err)
	mark := app.Events.Mark()
	fetched, err := svc.RemoteRun(ctx, bridge.GitRemoteRunArgs{WindowKey: "w1", Params: gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: fetchOp}})
	chk(err)
	if !fetched.OK || len(fetched.Updates) == 0 {
		t.Fatalf("RemoteRun fetch = %+v", fetched)
	}
	app.Contract(t, scenario, "git:remote.run#bound", fetched, mask)
	if len(app.Events.Since(mark, gitEventRemoteProgress)) == 0 {
		t.Fatal("fetch emitted no remote.progress")
	}
	for _, e := range app.Events.Since(mark, gitEventRemoteProgress) {
		if e.Window != "w1" {
			t.Fatalf("remote.progress went to window %q, want w1 only", e.Window)
		}
	}
	noop, err := svc.RemoteCancel(ctx, bridge.GitRemoteCancelArgs{WindowKey: "w1", Params: gitrpc.RemoteCancelParams{RepoID: id}})
	chk(err)
	app.Contract(t, scenario, "git:remote.cancel#bound", noop)

	// Cancel: a running fetch ends through RemoteCancel.
	started := slowFetch(t, app.Root, local, 120)
	done := make(chan gitsession.RemoteOpResult, 1)
	go func() {
		res, _ := svc.RemoteRun(ctx, bridge.GitRemoteRunArgs{WindowKey: "w1", Params: gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: fetchOp}})
		done <- res
	}()
	testx.WaitUntil(t, wait, fileExists(started))
	cancelled, err := svc.RemoteCancel(ctx, bridge.GitRemoteCancelArgs{WindowKey: "w1", Params: gitrpc.RemoteCancelParams{RepoID: id}})
	chk(err)
	if !cancelled.Cancelled {
		t.Fatal("RemoteCancel reported nothing running during a fetch")
	}
	select {
	case res := <-done:
		remoteFailure(t, res, "Cancelled")
	case <-time.After(wait):
		t.Fatal("fetch did not end after RemoteCancel")
	}

	// A cancelled binding call abandons the reply, not the write.
	cancelRepo := app.NewRepo("cancel-proj")
	cancelRepo.Commit("one", map[string]string{"f.txt": "1\n"})
	cancelRepo.Branch("sibling", "")
	cancelRoot := filepath.Join(app.Root, "cancel")
	chk(os.MkdirAll(cancelRoot, 0o755))
	blocked, release := blockCheckouts(t, cancelRepo, cancelRoot)
	cancelID, err := svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: "w1", Params: gitrpc.RepoOpenParams{Path: cancelRepo.Dir}})
	chk(err)
	callCtx, abandon := context.WithCancel(ctx)
	go func() {
		_, _ = svc.OpRun(callCtx, bridge.GitOpRunArgs{WindowKey: "w1", Params: gitrpc.OpRunParams{RepoID: cancelID.Repo.RepoID, Op: gitsession.OpRequest{Kind: "checkout", Target: "sibling", Mode: "switch"}}})
	}()
	testx.WaitUntil(t, wait, fileExists(blocked))
	abandon()
	release()
	testx.WaitUntil(t, wait, func() bool { return boundHead(t, svc, "w1", cancelID.Repo.RepoID) == "sibling" })

	// Window close mid-write: the checkout lands and the op log records it.
	closeRepo := app.NewRepo("close-proj")
	closeRepo.Commit("one", map[string]string{"f.txt": "1\n"})
	closeRepo.Branch("sibling", "")
	closeRoot := filepath.Join(app.Root, "close")
	chk(os.MkdirAll(closeRoot, 0o755))
	blocked, release = blockCheckouts(t, closeRepo, closeRoot)
	closeOpen, err := svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: "w3", Params: gitrpc.RepoOpenParams{Path: closeRepo.Dir}})
	chk(err)
	observer, err := svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: "w5", Params: gitrpc.RepoOpenParams{Path: closeRepo.Dir}})
	chk(err)
	go func() {
		_, _ = svc.OpRun(ctx, bridge.GitOpRunArgs{WindowKey: "w3", Params: gitrpc.OpRunParams{RepoID: closeOpen.Repo.RepoID, Op: gitsession.OpRequest{Kind: "checkout", Target: "sibling", Mode: "switch"}}})
	}()
	testx.WaitUntil(t, wait, fileExists(blocked))
	app.W.GitWindows.Close("w3")
	release()
	testx.WaitUntil(t, wait, func() bool { return boundHead(t, svc, "w5", observer.Repo.RepoID) == "sibling" })
	testx.WaitUntil(t, wait, func() bool {
		recent, err := app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 20})
		chk(err)
		for _, r := range recent {
			if r.RepoName == "close-proj" && r.Status == oplog.StatusOK && r.Module == string(oplog.ModuleGit) {
				return true
			}
		}
		return false
	})

	// Window close mid-prompt withdraws the credential prompt and frees the op slot.
	url, _ := smartHTTP(t, app.Work, "ana", "s3cret")
	authBare := app.NewBare("auth-remote")
	authBare.PushFrom(seed)
	authLocal := authBare.Clone(filepath.Join(app.Work, "auth-local"))
	authLocal.Git("remote", "set-url", "origin", strings.Replace(url, "http://", "http://ana@", 1)+"/auth-remote.git")
	authOpen, err := svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: "w4", Params: gitrpc.RepoOpenParams{Path: authLocal.Dir}})
	chk(err)
	_, err = svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: "w1", Params: gitrpc.RepoOpenParams{Path: authLocal.Dir}})
	chk(err)
	authID := authOpen.Repo.RepoID
	prompted := make(chan gitsession.RemoteOpResult, 1)
	go func() {
		res, _ := svc.RemoteRun(ctx, bridge.GitRemoteRunArgs{WindowKey: "w4", Params: gitrpc.RemoteRunParams{RepoID: authID, RemoteOpParams: fetchOp}})
		prompted <- res
	}()
	e := credentialEntry(t, app)
	if e.Origin != "w4" || e.Target != "w4" {
		t.Fatalf("prompt entry = %+v, want origin and target w4", e)
	}
	app.W.GitWindows.Close("w4")
	noCredentialEntry(t, app)
	testx.WaitUntil(t, wait, func() bool { return len(app.W.GitCredential.Pending()) == 0 })
	select {
	case res := <-prompted:
		if res.OK || res.Error == nil {
			t.Fatalf("fetch after window close = %+v, want an auth failure", res)
		}
	case <-time.After(wait):
		t.Fatal("fetch never ended after its window closed")
	}
	next := make(chan gitsession.RemoteOpResult, 1)
	go func() {
		for {
			res, _ := svc.RemoteRun(ctx, bridge.GitRemoteRunArgs{WindowKey: "w1", Params: gitrpc.RemoteRunParams{RepoID: authID, RemoteOpParams: fetchOp}})
			if res.Error == nil || res.Error.Kind != "OperationInProgress" {
				next <- res
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	answerLogin(t, app, "ana", "s3cret")
	select {
	case res := <-next:
		if !res.OK {
			t.Fatalf("w1 fetch after the slot freed = %+v", res.Error)
		}
	case <-time.After(wait):
		t.Fatal("w1 never got the slot")
	}
}
