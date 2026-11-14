package gitflow_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

func (r *rig) stackList(id string) gitpreflight.StackListResult {
	r.t.Helper()
	return call[gitpreflight.StackListResult](r.t, r.gs, "stack.list", gitrpc.StackListParams{RepoID: id})
}

func (r *rig) stackSet(id, branch, parent string) {
	r.t.Helper()
	p := parent
	r.mustOp(id, gitsession.OpRequest{Kind: "stackSet", Branch: branch, Parent: &p})
}

func branchTips(repo interface{ Git(...string) string }, names ...string) string {
	var out []string
	for _, n := range names {
		out = append(out, n+"="+repo.Git("rev-parse", n))
	}
	return strings.Join(out, " ")
}

// J-STACK-001: a four-level stack with a sibling, amend the bottom, restack, undo restores every
// branch tip.
func TestStackFourLevelsRestackUndo(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"base.txt": "base\n"})
	mk := func(name, from string) {
		repo.Git("checkout", "-q", "-b", name, from)
		repo.Commit("work "+name, map[string]string{name + ".txt": name + "\n"})
	}
	mk("a", "main")
	mk("b", "a")
	mk("c", "b")
	mk("c2", "b")
	repo.Checkout("c")
	id := r.open(repo.Dir).RepoID
	for _, e := range [][2]string{{"a", "main"}, {"b", "a"}, {"c", "b"}, {"c2", "b"}} {
		r.stackSet(id, e[0], e[1])
	}
	r.external(func() {
		repo.Checkout("a")
		repo.Git("commit", "-q", "--amend", "-m", "work a, reworded")
		repo.Checkout("c")
	})

	list := r.stackList(id)
	if len(list.Stacks) != 1 || !list.Stacks[0].NeedsRestack {
		t.Fatalf("stack.list = %+v, want one stack needing restack", list)
	}
	var order []string
	for _, b := range list.Stacks[0].Branches {
		order = append(order, b.Name)
	}
	if strings.Join(order, ",") != "a,b,c,c2" {
		t.Fatalf("stack order = %v, want a,b,c,c2", order)
	}
	pre := branchTips(repo, "a", "b", "c", "c2")
	res := call[gitsession.RestackResult](t, r.gs, "stack.restack", gitrpc.StackRestackParams{RepoID: id, Branch: "c"})
	if !res.OK {
		t.Fatalf("restack c = %+v", res)
	}
	tipA, tipB := repo.Git("rev-parse", "a"), repo.Git("rev-parse", "b")
	if repo.Git("merge-base", "b", "a") != tipA || repo.Git("merge-base", "c", "b") != tipB || repo.Git("merge-base", "c2", "b") != tipB {
		t.Fatalf("levels are not each on their new parent tip: %s", branchTips(repo, "a", "b", "c", "c2"))
	}
	if fresh := r.stackList(id); fresh.Stacks[0].NeedsRestack {
		t.Fatalf("stack still needs restack: %+v", fresh.Stacks[0])
	}

	if res.Undo == nil || len(res.Restacked) != 3 {
		t.Fatalf("restack = %+v, want b, c and c2 restacked with an undo", res)
	}
	if got := undoRun(t, r.gs, id, res.Undo.ID); !got.OK {
		t.Fatalf("undo restack = %+v", got.Error)
	}
	if post := branchTips(repo, "a", "b", "c", "c2"); post != pre {
		t.Fatalf("tips after undo:\n%s\nwant\n%s", post, pre)
	}
}

// J-STACK-002: hand-edited config. A cycle and a dangling parent land under orphans, a 70-deep
// chain lists every branch, nothing hangs.
func TestStackHandEditedConfig(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"base.txt": "base\n"})
	for _, b := range []string{"a", "b", "c"} {
		repo.Git("checkout", "-q", "-b", b)
		repo.Commit("work "+b, map[string]string{b + ".txt": b + "\n"})
	}
	repo.Checkout("main")
	id := r.open(repo.Dir).RepoID
	r.stackSet(id, "a", "main")
	r.stackSet(id, "b", "a")
	r.stackSet(id, "c", "b")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	listed := func() gitpreflight.StackListResult {
		done := make(chan gitpreflight.StackListResult, 1)
		go func() { done <- r.stackList(id) }()
		select {
		case l := <-done:
			return l
		case <-ctx.Done():
			t.Fatal("stack.list did not return: a hand-edited config hung it")
			return gitpreflight.StackListResult{}
		}
	}

	repo.Git("config", "branch.a.kirastackparent", "c")
	cyc := listed()
	orphaned := map[string]bool{}
	for _, o := range cyc.Orphans {
		orphaned[o.Name] = true
	}
	if !orphaned["a"] || !orphaned["b"] || !orphaned["c"] {
		t.Fatalf("cycle a-b-c: orphans = %+v stacks = %d, want all three orphaned", cyc.Orphans, len(cyc.Stacks))
	}

	repo.Git("config", "branch.a.kirastackparent", "main")
	repo.Git("update-ref", "-d", "refs/heads/b")
	dangling := listed()
	var c *gitpreflight.StackBranch
	for i, o := range dangling.Orphans {
		if o.Name == "c" {
			c = &dangling.Orphans[i]
		}
	}
	if c == nil || c.State != gitpreflight.StackBranchState("parentMissing") {
		t.Fatalf("dangling parent: orphans = %+v, want c as parentMissing", dangling.Orphans)
	}
	for _, s := range dangling.Stacks {
		for _, b := range s.Branches {
			if b.Name == "c" {
				t.Fatalf("c is grouped under %s although its parent is gone", s.Base)
			}
		}
	}

	// 70 chained branches.
	repo.Git("checkout", "-q", "main")
	prev := "main"
	for i := 0; i < 70; i++ {
		name := fmt.Sprintf("chain%02d", i)
		repo.Git("branch", name, "main")
		repo.Git("config", "branch."+name+".kirastackparent", prev)
		prev = name
	}
	start := time.Now()
	deep := listed()
	listedNames := map[string]bool{}
	for _, s := range deep.Stacks {
		for _, b := range s.Branches {
			listedNames[b.Name] = true
		}
	}
	for _, o := range deep.Orphans {
		listedNames[o.Name] = true
	}
	for i := 0; i < 70; i++ {
		if name := fmt.Sprintf("chain%02d", i); !listedNames[name] {
			t.Fatalf("%s dropped from the stack view (listed %d names)", name, len(listedNames))
		}
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("stack.list over 70 chained branches took %s", d)
	}
}

// J-STACK-003: a stack on a remote-tracking base that the remote deletes. The view reports the
// missing base without rewriting refs; re-parent and its undo move only the config.
func TestStackRemoteBasePruned(t *testing.T) {
	r := newRig(t)
	seed := r.app.NewRepo("seed")
	seed.Commit("base", map[string]string{"f.txt": "1\n"})
	seed.Git("branch", "release")
	bare := r.app.NewBare("remote")
	bare.PushFrom(seed)
	repo := bare.Clone(r.app.Work + "/proj")
	repo.Git("checkout", "-q", "-b", "x", "origin/release")
	repo.Commit("work x", map[string]string{"x.txt": "x\n"})
	id := r.open(repo.Dir).RepoID
	r.stackSet(id, "x", "origin/release")
	if l := r.stackList(id); len(l.Stacks) != 1 || l.Stacks[0].Base != "origin/release" {
		t.Fatalf("stack.list = %+v, want one stack on origin/release", l)
	}

	bare.Git("branch", "-D", "release")
	if res := r.remote(id, gitsession.RemoteOpParams{Kind: "fetch", Remote: "origin", Prune: true}); !res.OK {
		t.Fatalf("fetch --prune = %+v", res.Error)
	}
	refsBefore := repo.Git("for-each-ref", "--format=%(refname) %(objectname)")
	l := r.stackList(id)
	var x *gitpreflight.StackBranch
	for i, o := range l.Orphans {
		if o.Name == "x" {
			x = &l.Orphans[i]
		}
	}
	for _, s := range l.Stacks {
		for i, b := range s.Branches {
			if b.Name == "x" {
				x = &s.Branches[i]
			}
		}
	}
	if x == nil || (x.ParentTip != nil && x.State != gitpreflight.StackBranchState("parentMissing")) {
		t.Fatalf("x after the prune = %+v (stacks %+v), want it listed with the missing base", x, l.Stacks)
	}
	if got := repo.Git("for-each-ref", "--format=%(refname) %(objectname)"); got != refsBefore {
		t.Fatal("listing the stack rewrote refs")
	}

	r.stackSet(id, "x", "main")
	if got := repo.Git("config", "branch.x.kirastackparent"); got != "main" {
		t.Fatalf("parent after re-parent = %q", got)
	}
	peek := r.undoPeek(id)
	if peek.Slot == nil {
		t.Fatal("re-parent offers no undo")
	}
	if got := undoRun(t, r.gs, id, peek.Slot.ID); !got.OK {
		t.Fatalf("undo = %+v", got.Error)
	}
	if got := repo.Git("config", "branch.x.kirastackparent"); got != "origin/release" {
		t.Fatalf("parent after undo = %q, want origin/release", got)
	}
}
