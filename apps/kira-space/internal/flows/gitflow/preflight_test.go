package gitflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// J-GIT-021 (+ J-GIT-022): a switch preflight goes stale. The tree turning clean means no empty
// stash and no error; a merge started outside means the op refuses, with stash and discard alike.
func TestCheckoutPreflightGoesStale(t *testing.T) {
	setup := func(t *testing.T) (*rig, *flowharness.Repo, string) {
		r := newRig(t)
		repo := r.app.NewRepo("proj")
		repo.Commit("base", map[string]string{"f.txt": "base\n"})
		repo.Branch("other", "")
		repo.Checkout("other")
		repo.Commit("other edit", map[string]string{"f.txt": "other\n"})
		repo.Checkout("main")
		repo.Write("f.txt", "dirty\n")
		repo.Write("u.txt", "untracked\n")
		return r, repo, r.open(repo.Dir).RepoID
	}

	t.Run("tree goes clean", func(t *testing.T) {
		r, repo, id := setup(t)
		pf := call[gitpreflight.CheckoutPreflight](t, r.gs, "preflight.checkout", gitrpc.PreflightCheckoutParams{RepoID: id, Target: "other", Mode: "switch"})
		if !contains(pf.Routes, "autoStash") {
			t.Fatalf("preflight routes = %v, want autoStash", pf.Routes)
		}
		r.external(func() {
			repo.Git("checkout", "-q", "--", ".")
			_ = os.Remove(filepath.Join(repo.Dir, "u.txt"))
		})
		res := r.mustOp(id, gitsession.OpRequest{Kind: "checkout", Target: "other", Mode: "switch", AutoStash: true})
		if res.Head.Name != "other" {
			t.Fatalf("head after the stale-dialog switch = %+v, want other", res.Head)
		}
		if out := repo.Git("stash", "list"); out != "" {
			t.Fatalf("stash list = %q, want none (no empty stash)", out)
		}
	})

	t.Run("merge starts outside", func(t *testing.T) {
		r, repo, id := setup(t)
		repo.Git("checkout", "-q", "--", ".")
		_ = os.Remove(filepath.Join(repo.Dir, "u.txt"))
		repo.Branch("clash", "")
		repo.Checkout("clash")
		repo.Commit("clash edit", map[string]string{"f.txt": "clash\n"})
		repo.Checkout("main")
		repo.Commit("main edit", map[string]string{"f.txt": "main\n"})
		call[gitpreflight.CheckoutPreflight](t, r.gs, "preflight.checkout", gitrpc.PreflightCheckoutParams{RepoID: id, Target: "other", Mode: "switch"})
		r.external(func() { _, _ = repo.GitErr("merge", "clash") })
		head := repo.Git("rev-parse", "HEAD")
		for name, op := range map[string]gitsession.OpRequest{
			"stash":   {Kind: "checkout", Target: "other", Mode: "switch", AutoStash: true},
			"discard": {Kind: "checkout", Target: "other", Mode: "switch", DiscardLocalChanges: true},
		} {
			res := r.op(id, op)
			if res.OK || res.Error == nil || res.Error.Kind != "OperationInProgress" || !strings.Contains(strings.ToLower(res.Error.Message), "merg") {
				t.Fatalf("%s with a merge in progress = %+v, want OperationInProgress naming the merge (error %+v)", name, res, res.Error)
			}
		}
		if repo.Git("stash", "list") != "" {
			t.Fatal("a refused switch left a stash entry")
		}
		if _, err := os.Stat(filepath.Join(repo.Dir, ".git", "MERGE_HEAD")); err != nil {
			t.Fatalf("MERGE_HEAD gone: %v", err)
		}
		if repo.Git("rev-parse", "HEAD") != head {
			t.Fatal("HEAD moved")
		}
		if st := r.status(id); st.InProgress == nil || st.InProgress.Kind != gitpreflight.InProgressMerge {
			t.Fatalf("status = %+v, want the merge still in progress", st.InProgress)
		}
	})
}

// J-GIT-023: checkout of a tag and a typed sha detaches, a remote-only branch creates a tracking
// branch, an existing diverged local branch is not reset, an option-shaped target never spawns.
func TestCheckoutTargets(t *testing.T) {
	r := newRig(t)
	seed := r.app.NewRepo("seed")
	seed.Commit("base", map[string]string{"f.txt": "1\n"})
	seed.Tag("v1.0")
	seed.Git("checkout", "-q", "-b", "topic")
	seed.Commit("topic work", map[string]string{"t.txt": "t\n"})
	seed.Checkout("main")
	bare := r.app.NewBare("remote")
	bare.PushFrom(seed)
	bare.Git("tag", "v1.0", seed.Git("rev-parse", "v1.0"))
	repo := bare.Clone(filepath.Join(r.app.Work, "proj"))
	repo.Git("fetch", "-q", "--tags", "origin")
	id := r.open(repo.Dir).RepoID
	headBranch := func() (string, error) { return repo.GitErr("symbolic-ref", "-q", "HEAD") }

	r.mustOp(id, gitsession.OpRequest{Kind: "checkout", Target: "v1.0", Mode: "switch"})
	if _, err := headBranch(); err == nil {
		t.Fatal("checkout of a tag left HEAD on a branch")
	}
	if repo.Git("rev-parse", "HEAD") != repo.Git("rev-parse", "v1.0^{commit}") {
		t.Fatal("HEAD is not at the tag")
	}

	repo.Checkout("main")
	short := repo.Git("rev-parse", "--short=7", "HEAD")
	r.mustOp(id, gitsession.OpRequest{Kind: "checkout", Target: short, Mode: "switch"})
	if _, err := headBranch(); err == nil {
		t.Fatal("checkout of a sha left HEAD on a branch")
	}

	r.mustOp(id, gitsession.OpRequest{Kind: "checkout", Target: "origin/topic", Mode: "switch"})
	if got := repo.Git("rev-parse", "--abbrev-ref", "HEAD"); got != "topic" {
		t.Fatalf("HEAD = %q, want a local topic", got)
	}
	if got := repo.Git("config", "branch.topic.remote") + "/" + repo.Git("config", "branch.topic.merge"); got != "origin/refs/heads/topic" {
		t.Fatalf("topic upstream = %q, want origin/refs/heads/topic", got)
	}

	// Local topic diverges; checking out origin/topic again must not reset it.
	local := repo.Commit("local only", map[string]string{"l.txt": "l\n"})
	repo.Checkout("main")
	res := r.op(id, gitsession.OpRequest{Kind: "checkout", Target: "origin/topic", Mode: "switch"})
	if repo.Git("rev-parse", "topic") != local {
		t.Fatalf("local topic was reset to %s (op %+v), want %s", repo.Git("rev-parse", "topic"), res, local)
	}

	before := repo.Git("rev-parse", "HEAD")
	we := wireErr(t, r.gs, "op.run", gitrpc.OpRunParams{RepoID: id, Op: gitsession.OpRequest{Kind: "checkout", Target: "-f", Mode: "switch"}})
	if we.Code != "E_BAD_REQUEST" {
		t.Fatalf("option-shaped target = %+v, want E_BAD_REQUEST", we)
	}
	if repo.Git("rev-parse", "HEAD") != before {
		t.Fatal("refused target moved HEAD")
	}
}
