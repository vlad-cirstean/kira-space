package gitflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// P702: the Git module removes a worktree inline, with no dialog. The preflight tells clean from dirty;
// a clean worktree goes on one op; a dirty one refuses until the Discard changes and remove button
// sends the token. Contract git-worktree-remove: tests/ui/git-worktree-remove.spec.ts reads these.
func TestWorktreeRemoveInline(t *testing.T) {
	const scenario = "git-worktree-remove"
	work := flowharness.Replace
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a\n"})
	clean := repo.LinkedWorktree("wt-clean")
	dirty := repo.LinkedWorktree("wt-dirty")
	locked := repo.LinkedWorktree("wt-locked")
	id := r.open(repo.Dir).RepoID

	pf := r.wtRemovePreflight(id, clean.Dir)
	if pf.Verdict != "clean" || pf.RequiresTypedConfirmation {
		t.Fatalf("clean preflight = %+v, want clean with no token", pf)
	}
	r.app.Contract(t, scenario, "git:preflight.worktreeRemove#clean", pf, work(r.app.Work, "<work>"))
	r.mustOp(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: clean.Dir})
	if _, err := os.Stat(clean.Dir); !os.IsNotExist(err) {
		t.Fatalf("clean worktree still there: %v", err)
	}

	dirty.Write("wip.txt", "wip\n")
	pf = r.wtRemovePreflight(id, dirty.Dir)
	if pf.Verdict != "dirty" || !pf.RequiresTypedConfirmation || pf.ConfirmToken != filepath.Base(dirty.Dir) {
		t.Fatalf("dirty preflight = %+v, want dirty with a token", pf)
	}
	r.app.Contract(t, scenario, "git:preflight.worktreeRemove#dirty", pf, work(r.app.Work, "<work>"))
	res := r.op(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: dirty.Dir})
	if res.OK || res.Error == nil || res.Error.Kind != "ConfirmationRequired" || res.Error.Message != "This worktree has uncommitted changes. Discard them to remove it." {
		t.Fatalf("remove dirty without the token = %+v, want the discard message", res)
	}
	if _, err := os.Stat(filepath.Join(dirty.Dir, "wip.txt")); err != nil {
		t.Fatalf("refused remove lost wip.txt: %v", err)
	}
	r.app.Contract(t, scenario, "git:op.run#dirty-refused", res, work(r.app.Work, "<work>"))
	token := pf.ConfirmToken
	r.mustOp(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: dirty.Dir, Force: true, ConfirmToken: &token})
	if _, err := os.Stat(dirty.Dir); !os.IsNotExist(err) {
		t.Fatalf("dirty worktree still there after the discard: %v", err)
	}

	repo.Git("worktree", "lock", "--reason", "usb", locked.Dir)
	if res := r.op(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: locked.Dir}); res.OK || res.Error == nil || res.Error.Kind != "WorktreeLocked" || !strings.Contains(res.Error.Message, "usb") {
		t.Fatalf("remove locked = %+v, want WorktreeLocked naming the reason", res)
	}
	if _, err := os.Stat(locked.Dir); err != nil {
		t.Fatalf("locked worktree removed: %v", err)
	}
}
