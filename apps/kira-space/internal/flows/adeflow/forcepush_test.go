package adeflow_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// J-ADE-057. Force push uses a lease on the remote tip last seen: a remote that moved since the last
// fetch refuses the push and keeps the other pusher's commit; a fetch lets the second attempt through.
// Contract ade-force-push: tests/ui/ade-v2-force-push.spec.ts reads the refused result.
func TestForcePushLease(t *testing.T) {
	f, task, br := newRbFx(t)
	app := f.app
	gitOut(t, br.Worktree, "commit", "-q", "--amend", "-m", "amended locally")
	local := gitOut(t, br.Worktree, "rev-parse", "HEAD")

	// A teammate pushes to the same branch; this clone has not fetched.
	mate := f.repo.Dir + "-mate"
	gitOut(t, filepath.Dir(f.repo.Dir), "clone", "-q", gitOut(t, f.repo.Dir, "remote", "get-url", "origin"), mate)
	gitOut(t, mate, "checkout", "-q", "feat/a")
	commitIn(t, mate, "mate.txt")
	gitOut(t, mate, "push", "-q", "origin", "feat/a")
	mateTip := gitOut(t, mate, "rev-parse", "HEAD")

	args := adewire.BranchArgs{BranchID: br.ID}
	app.Contract(t, "ade-force-push", "args:AdeTaskService.ForcePush", args)
	res, err := app.W.AdeTask.ForcePush(ctx, args)
	if err != nil {
		t.Fatalf("ForcePush: %v", err)
	}
	remote := func() string {
		return gitOut(t, f.repo.Dir, "ls-remote", "origin", "refs/heads/feat/a")
	}
	if res.Error == nil {
		t.Fatalf("ForcePush over a moved remote = %+v, want a refusal", res)
	}
	if !strings.HasPrefix(remote(), mateTip) {
		t.Fatalf("remote tip = %q, want the teammate's %s kept", remote(), mateTip)
	}
	app.Contract(t, "ade-force-push", "AdeTaskService.ForcePush#stale-lease", res)
	if got := gitOut(t, br.Worktree, "rev-parse", "HEAD"); got != local {
		t.Fatalf("local branch moved to %s by a refused push", got)
	}

	// A fetch alone does not free the push: --force-if-includes refuses to overwrite a tip the local branch never integrated.
	gitOut(t, br.Worktree, "fetch", "-q")
	_ = task
	again, err := app.W.AdeTask.ForcePush(ctx, args)
	if err != nil || again.Error == nil || again.Error.Kind != "RemoteRefUpdated" {
		t.Fatalf("ForcePush after fetch = %+v, %v, want RemoteRefUpdated", again, err)
	}
	app.Contract(t, "ade-force-push", "AdeTaskService.ForcePush#unintegrated", again)
	if !strings.HasPrefix(remote(), mateTip) {
		t.Fatalf("remote tip = %q, want the teammate's %s kept", remote(), mateTip)
	}

	// Once the local branch contains the teammate's tip the push goes through.
	gitOut(t, br.Worktree, "rebase", "origin/feat/a")
	local = gitOut(t, br.Worktree, "rev-parse", "HEAD")
	ok, err := app.W.AdeTask.ForcePush(ctx, args)
	if err != nil || ok.Error != nil {
		t.Fatalf("ForcePush after integrating = %+v, %v", ok, err)
	}
	if !strings.HasPrefix(remote(), local) {
		t.Fatalf("remote tip = %q, want the local %s", remote(), local)
	}
	// A plain force never replaces a tip the lease did not name: the teammate's commit is only in the reflog now.
}
