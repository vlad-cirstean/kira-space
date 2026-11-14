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

func (r *rig) stashes(id string) gitrpc.StashListResult {
	r.t.Helper()
	return call[gitrpc.StashListResult](r.t, r.gs, "stash.list", gitrpc.StashListParams{RepoID: id})
}

func (r *rig) undoPeek(id string) gitrpc.UndoPeekResult {
	r.t.Helper()
	return call[gitrpc.UndoPeekResult](r.t, r.gs, "undo.peek", gitrpc.UndoPeekParams{RepoID: id})
}

func undoRun(t *testing.T, gs *flowharness.GitStream, id, undoID string) gitsession.OpResult {
	t.Helper()
	return call[gitsession.OpResult](t, gs, "undo.run", gitrpc.UndoRunParams{RepoID: id, ID: undoID})
}

func stashShas(repo *flowharness.Repo) string { return repo.Git("stash", "list", "--format=%H") }

// J-GIT-020 (+ OQ-GIT-1): auto-stash switch, hotfix, come back, pop. Status comes back byte for
// byte, staged-new file still staged.
func TestAutoStashSwitchBackPopRestoresStatus(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a\n", "b.txt": "b\n", "c.txt": "c\n"})
	repo.Branch("feature", "")
	repo.Checkout("feature")
	repo.Write("a.txt", "a edited\n")
	repo.Write("b.txt", "b edited\n")
	repo.Write("u.txt", "untracked\n")
	repo.Write("s.txt", "staged new\n")
	repo.Git("add", "s.txt")
	repo.Write("c.txt", "c staged edit\n")
	repo.Git("add", "c.txt")
	before := repo.Git("status", "--porcelain")
	id := r.open(repo.Dir).RepoID

	r.mustOp(id, gitsession.OpRequest{Kind: "checkout", Target: "main", Mode: "switch", AutoStash: true})
	list := r.stashes(id)
	if len(list.Entries) != 1 || list.Entries[0].Message != "On feature: auto-stash: switching to main" || !list.Entries[0].IncludedUntracked {
		t.Fatalf("stash.list = %+v, want one auto-stash entry tagged feature with untracked", list.Entries)
	}
	if got := repo.Git("stash", "list"); strings.Count(got, "\n") != 0 || !strings.Contains(got, "auto-stash: switching to main") {
		t.Fatalf("git stash list = %q, want exactly the auto-stash entry", got)
	}
	if out := repo.Git("status", "--porcelain"); out != "" {
		t.Fatalf("tree not clean on main: %q", out)
	}

	hotfix := repo.Commit("hotfix", map[string]string{"hot.txt": "fix\n"})
	r.mustOp(id, gitsession.OpRequest{Kind: "checkout", Target: "feature", Mode: "switch"})
	if got := repo.Git("rev-parse", "main"); got != hotfix {
		t.Fatalf("main = %s, want the hotfix %s", got, hotfix)
	}
	if n := len(r.stashes(id).Entries); n != 1 {
		t.Fatalf("stash entries after the clean switch back = %d, want 1 (never auto-popped)", n)
	}

	top := r.stashes(id).Entries[0]
	r.mustOp(id, gitsession.OpRequest{Kind: "stashPop", Sha: top.Sha, Index: top.Index, RestoreIndex: true})
	if after := repo.Git("status", "--porcelain"); after != before {
		t.Fatalf("status after pop differs\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if n := len(r.stashes(id).Entries); n != 0 {
		t.Fatalf("stash entries after pop = %d, want 0", n)
	}

	t.Run("second auto-stash on dirt made on the other branch", func(t *testing.T) {
		repo.Checkout("main")
		repo.Write("hot.txt", "dirty main\n")
		r.mustOp(id, gitsession.OpRequest{Kind: "checkout", Target: "feature", Mode: "switch", AutoStash: true})
		got := r.stashes(id).Entries
		if len(got) != 1 || got[0].Message != "On main: auto-stash: switching to feature" || got[0].Branch == nil || *got[0].Branch != "main" {
			t.Fatalf("stash.list = %+v, want one entry tagged main", got)
		}
	})
}

// J-GIT-024 (+ J-GIT-025, OQ-GIT-1): stash matrix, pop conflict keeps the entry, undo of a drop
// restores the same sha at the top, a later op voids the stale undo.
func TestStashMatrixAndDropUndo(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a\n", "b.txt": "b\n", "c.txt": "c\n"})
	id := r.open(repo.Dir).RepoID

	repo.Write("a.txt", "a stashed\n")
	r.mustOp(id, gitsession.OpRequest{Kind: "stashPush", Message: ptr("A")})
	repo.Write("b.txt", "b stashed\n")
	repo.Write("u.txt", "untracked\n")
	r.mustOp(id, gitsession.OpRequest{Kind: "stashPush", Message: ptr("B"), IncludeUntracked: true})
	repo.Write("c.txt", "c staged\n")
	repo.Git("add", "c.txt")
	r.mustOp(id, gitsession.OpRequest{Kind: "stashPush", Message: ptr("C")})
	if n := len(r.stashes(id).Entries); n != 3 {
		t.Fatalf("stash entries = %d, want 3", n)
	}
	a := r.stashes(id)
	stashA, stashB := a.Entries[2], a.Entries[1]
	if !strings.HasSuffix(stashA.Message, "A") || !strings.HasSuffix(stashB.Message, "B") {
		t.Fatalf("stash order = %+v", a.Entries)
	}

	// Apply B: stash stays, files come back.
	r.mustOp(id, gitsession.OpRequest{Kind: "stashApply", Sha: stashB.Sha})
	if got, _ := os.ReadFile(filepath.Join(repo.Dir, "u.txt")); string(got) != "untracked\n" {
		t.Fatalf("u.txt after apply = %q", got)
	}
	if n := len(r.stashes(id).Entries); n != 3 {
		t.Fatalf("apply dropped an entry: %d left", n)
	}
	repo.Git("checkout", "-q", "--", ".")
	repo.Git("clean", "-fdq")

	// Same lines changed and committed: popping A conflicts, git keeps the entry.
	r.external(func() { repo.Commit("diverge a", map[string]string{"a.txt": "a committed\n"}) })
	list := r.stashes(id)
	var idxA int
	for _, e := range list.Entries {
		if e.Sha == stashA.Sha {
			idxA = e.Index
		}
	}
	res := r.op(id, gitsession.OpRequest{Kind: "stashPop", Sha: stashA.Sha, Index: idxA})
	if res.OK || res.Error == nil || res.Error.Kind != "StashConflict" {
		t.Fatalf("conflicting pop = %+v, want StashConflict", res)
	}
	if got, _ := os.ReadFile(filepath.Join(repo.Dir, "a.txt")); !strings.Contains(string(got), "<<<<<<<") {
		t.Fatalf("a.txt = %q, want conflict markers", got)
	}
	if !strings.Contains(stashShas(repo), stashA.Sha) {
		t.Fatalf("stash A gone after a conflicting pop: %q", stashShas(repo))
	}
	repo.Git("reset", "--hard", "-q")
	repo.Git("clean", "-fdq")

	// Drop A, undo: same sha back at the top.
	drop := r.mustOp(id, gitsession.OpRequest{Kind: "stashDrop", Sha: stashA.Sha, Index: idxA})
	if drop.Undo == nil {
		t.Fatalf("stash drop offered no undo: %+v", drop)
	}
	if strings.Contains(stashShas(repo), stashA.Sha) {
		t.Fatal("stash A still listed after drop")
	}
	undone := undoRun(t, r.gs, id, drop.Undo.ID)
	if !undone.OK {
		t.Fatalf("undo drop = %+v", undone.Error)
	}
	if top := strings.Fields(stashShas(repo))[0]; top != stashA.Sha {
		t.Fatalf("top stash after undo = %s, want A's sha %s", top, stashA.Sha)
	}
	if got := repo.Git("stash", "list", "--format=%gs", "-1"); !strings.HasSuffix(got, "A") {
		t.Fatalf("restored stash message = %q, want A's", got)
	}

	// Another op, then a stale undo: nothing replays.
	drop2 := r.mustOp(id, gitsession.OpRequest{Kind: "stashDrop", Sha: stashA.Sha, Index: 0})
	stale := drop2.Undo.ID
	r.mustOp(id, gitsession.OpRequest{Kind: "branchCreate", Name: "other", StartPoint: "HEAD"})
	if peek := r.undoPeek(id); peek.Slot != nil && peek.Slot.ID == stale {
		t.Fatalf("undo slot still holds the stale record: %+v", peek.Slot)
	}
	before := stashShas(repo)
	if res := undoRun(t, r.gs, id, stale); res.OK || res.Error == nil || res.Error.Kind != "NotFound" {
		t.Fatalf("stale undo = %+v, want NotFound", res)
	}
	if after := stashShas(repo); after != before {
		t.Fatalf("stale undo changed the stash list:\n%s\n->\n%s", before, after)
	}
}

// J-GIT-025: global stash across a worktree, removal undo, removal of a removed entry.
func TestGlobalStashRemoveUndo(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a\n"})
	id := r.open(repo.Dir).RepoID
	repo.Write("a.txt", "parked\n")
	r.mustOp(id, gitsession.OpRequest{Kind: "globalStashSave", Label: "parked work"})
	linked := repo.LinkedWorktree("linked")
	gs2 := r.app.OpenGitStream()
	lid := openOn(t, gs2, linked.Dir).RepoID

	g := call[gitrpc.StashListResult](t, gs2, "globalStash.list", gitrpc.GlobalStashListParams{RepoID: lid})
	if len(g.Entries) != 1 {
		t.Fatalf("window 2 global list = %+v, want the parked entry", g.Entries)
	}
	sha := g.Entries[0].Sha
	rm := call[gitsession.OpResult](t, gs2, "op.run", gitrpc.OpRunParams{RepoID: lid, Op: gitsession.OpRequest{Kind: "globalStashRemove", Sha: sha}})
	if !rm.OK || rm.Undo == nil {
		t.Fatalf("globalStashRemove = %+v", rm)
	}
	if out := repo.Git("for-each-ref", "refs/kira/globalstash/"); out != "" {
		t.Fatalf("global refs after removal = %q", out)
	}
	if res := call[gitsession.OpResult](t, gs2, "op.run", gitrpc.OpRunParams{RepoID: lid, Op: gitsession.OpRequest{Kind: "globalStashRemove", Sha: sha}}); res.OK || res.Error == nil || res.Error.Kind != "NotFound" {
		t.Fatalf("removing a removed entry = %+v, want NotFound", res)
	}
}

// J-GIT-026 (+ J-GIT-003): typed confirmation, a commit made outside the app while the dialog is
// open, undo keeps the new work, per-mode undo restores HEAD and index.
func TestHardResetStaleDialogAndUndoPerMode(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	c1 := repo.Commit("one", map[string]string{"f.txt": "1\n", "g.txt": "1\n"})
	repo.Commit("two", map[string]string{"f.txt": "2\n"})
	repo.Commit("three", map[string]string{"f.txt": "3\n"})
	c4 := repo.Commit("four", map[string]string{"g.txt": "4\n"})
	repo.Write("f.txt", "unstaged\n")
	repo.Write("g.txt", "staged\n")
	repo.Git("add", "g.txt")
	repo.Write("n.txt", "new\n")
	repo.Git("add", "n.txt")
	id := r.open(repo.Dir).RepoID

	pf := call[gitpreflight.ResetPreflight](t, r.gs, "preflight.reset", gitrpc.PreflightResetParams{RepoID: id, Target: c1, Mode: "hard"})
	if !pf.RequiresTypedConfirmation {
		t.Fatalf("preflight = %+v, want typed confirmation", pf)
	}
	wrong := "zzzzzzz"
	if res := r.op(id, gitsession.OpRequest{Kind: "reset", Target: c1, Mode: "hard", ConfirmToken: &wrong}); res.OK || res.Error == nil || res.Error.Kind != "ConfirmationRequired" {
		t.Fatalf("wrong token = %+v, want ConfirmationRequired", res)
	}
	if repo.Git("rev-parse", "HEAD") != c4 {
		t.Fatal("refused reset moved HEAD")
	}

	// Commit everything outside the app while the dialog is open.
	var c5 string
	r.external(func() {
		repo.Git("add", "-A")
		c5 = repo.Commit("outside", nil)
	})
	token := c1[:7]
	hard := r.mustOp(id, gitsession.OpRequest{Kind: "reset", Target: c1, Mode: "hard", ConfirmToken: &token})
	if repo.Git("rev-parse", "HEAD") != c1 || repo.Git("status", "--porcelain") != "" {
		t.Fatalf("hard reset left HEAD %s status %q", repo.Git("rev-parse", "HEAD"), repo.Git("status", "--porcelain"))
	}
	if hard.Undo == nil {
		t.Fatal("hard reset has no undo")
	}
	if back := undoRun(t, r.gs, id, hard.Undo.ID); !back.OK || repo.Git("rev-parse", "HEAD") != c5 {
		t.Fatalf("undo = %+v, HEAD %s, want the outside commit %s", back, repo.Git("rev-parse", "HEAD"), c5)
	}
	if got, _ := os.ReadFile(filepath.Join(repo.Dir, "f.txt")); string(got) != "unstaged\n" {
		t.Fatalf("f.txt after undo = %q, want the committed outside edit", got)
	}

	for _, mode := range []string{"soft", "mixed", "hard"} {
		t.Run("undo "+mode, func(t *testing.T) {
			head := repo.Git("rev-parse", "HEAD")
			repo.Write("f.txt", "work "+mode+"\n")
			repo.Git("add", "f.txt")
			headBefore := repo.Git("rev-parse", "HEAD")
			indexBefore := repo.Git("diff", "--cached", "--name-status")
			treeBefore := repo.Git("status", "--porcelain")
			op := gitsession.OpRequest{Kind: "reset", Target: c1, Mode: mode}
			if mode == "hard" {
				op.ConfirmToken = &token
			}
			res := r.mustOp(id, op)
			if res.Undo == nil {
				t.Fatalf("%s reset has no undo", mode)
			}
			if got := undoRun(t, r.gs, id, res.Undo.ID); !got.OK {
				t.Fatalf("undo %s = %+v", mode, got.Error)
			}
			if got := repo.Git("rev-parse", "HEAD"); got != headBefore {
				t.Fatalf("HEAD after undo %s = %s, want %s", mode, got, headBefore)
			}
			if mode != "hard" {
				if got := repo.Git("diff", "--cached", "--name-status"); got != indexBefore {
					t.Fatalf("index after undo %s = %q, want %q", mode, got, indexBefore)
				}
				if got := repo.Git("status", "--porcelain"); got != treeBefore {
					t.Fatalf("status after undo %s = %q, want %q", mode, got, treeBefore)
				}
			}
			repo.Git("reset", "--hard", "-q", head)
		})
	}
}

// J-GIT-027 (+ J-BC-062, OQ-GIT-2): undo after new local edits. A colliding edit refuses with the
// file list and changes nothing; a non-colliding one succeeds and keeps the edit. The slot is per
// repo and the next op, local or remote, clears it. A ref moved outside refuses.
func TestUndoAfterLocalEdits(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("one", map[string]string{"f.txt": "1\n", "g.txt": "g\n"})
	c2 := repo.Commit("two", map[string]string{"f.txt": "2\n"})
	id := r.open(repo.Dir).RepoID

	t.Run("colliding edit refuses with file list, undo stays armed", func(t *testing.T) {
		res := r.mustOp(id, gitsession.OpRequest{Kind: "reset", Target: "HEAD~1", Mode: "hard"})
		repo.Write("f.txt", "my edit\n")
		headBefore, statusBefore, idxBefore := repo.Git("rev-parse", "HEAD"), repo.Git("status", "--porcelain"), repo.Git("diff", "--cached")
		got := undoRun(t, r.gs, id, res.Undo.ID)
		if got.OK || got.Error == nil {
			t.Fatalf("undo over a colliding edit = %+v, want refusal", got)
		}
		if !strings.Contains(got.Error.Message, "f.txt") {
			t.Fatalf("refusal %q does not name f.txt", got.Error.Message)
		}
		if repo.Git("rev-parse", "HEAD") != headBefore || repo.Git("status", "--porcelain") != statusBefore || repo.Git("diff", "--cached") != idxBefore {
			t.Fatal("refused undo changed HEAD, index or tree")
		}
		if b, _ := os.ReadFile(filepath.Join(repo.Dir, "f.txt")); string(b) != "my edit\n" {
			t.Fatalf("f.txt = %q after refusal", b)
		}
		if peek := r.undoPeek(id); peek.Slot == nil || peek.Slot.ID != res.Undo.ID {
			t.Fatalf("slot after a refused undo = %+v, want it still armed", peek.Slot)
		}
		repo.Git("checkout", "-q", "--", "f.txt")
		if again := undoRun(t, r.gs, id, res.Undo.ID); !again.OK || repo.Git("rev-parse", "HEAD") != c2 {
			t.Fatalf("undo after clearing the collision = %+v", again)
		}
	})

	t.Run("mixed undo keeps an edit to the same file", func(t *testing.T) {
		res := r.mustOp(id, gitsession.OpRequest{Kind: "reset", Target: "HEAD~1", Mode: "mixed"})
		repo.Write("f.txt", "my edit\n")
		if got := undoRun(t, r.gs, id, res.Undo.ID); !got.OK {
			t.Fatalf("undo = %+v", got.Error)
		}
		if b, _ := os.ReadFile(filepath.Join(repo.Dir, "f.txt")); string(b) != "my edit\n" {
			t.Fatalf("f.txt = %q, want the edit kept", b)
		}
		repo.Git("reset", "--hard", "-q", c2)
	})

	t.Run("non-colliding edit survives the undo", func(t *testing.T) {
		res := r.mustOp(id, gitsession.OpRequest{Kind: "reset", Target: "HEAD~1", Mode: "mixed"})
		repo.Write("g.txt", "g edited\n")
		if got := undoRun(t, r.gs, id, res.Undo.ID); !got.OK {
			t.Fatalf("undo = %+v", got.Error)
		}
		if repo.Git("rev-parse", "HEAD") != c2 {
			t.Fatal("HEAD not restored")
		}
		if b, _ := os.ReadFile(filepath.Join(repo.Dir, "g.txt")); string(b) != "g edited\n" {
			t.Fatalf("g.txt = %q, want the edit kept", b)
		}
		repo.Git("checkout", "-q", "--", "g.txt")
	})

	t.Run("slot is per repo", func(t *testing.T) {
		other := r.app.NewRepo("other")
		other.Commit("o1", map[string]string{"o.txt": "1\n"})
		other.Commit("o2", map[string]string{"o.txt": "2\n"})
		oid := r.open(other.Dir).RepoID
		res := r.mustOp(id, gitsession.OpRequest{Kind: "reset", Target: "HEAD~1", Mode: "mixed"})
		r.mustOp(oid, gitsession.OpRequest{Kind: "branchCreate", Name: "side", StartPoint: "HEAD"})
		if peek := r.undoPeek(id); peek.Slot == nil || peek.Slot.ID != res.Undo.ID {
			t.Fatalf("an op in another repo cleared this repo's slot: %+v", peek.Slot)
		}
		r.mustOp(id, gitsession.OpRequest{Kind: "branchCreate", Name: "same-repo", StartPoint: "HEAD"})
		if peek := r.undoPeek(id); peek.Slot != nil && peek.Slot.ID == res.Undo.ID {
			t.Fatal("a later op in the same repo left the old slot armed")
		}
		repo.Git("reset", "--hard", "-q", c2)
	})

	t.Run("ref moved outside refuses", func(t *testing.T) {
		res := r.mustOp(id, gitsession.OpRequest{Kind: "reset", Target: "HEAD~1", Mode: "mixed"})
		var moved string
		r.external(func() { moved = repo.Commit("outside", map[string]string{"x.txt": "x\n"}) })
		got := undoRun(t, r.gs, id, res.Undo.ID)
		if got.OK || got.Error == nil {
			t.Fatalf("undo after an outside commit = %+v, want refusal", got)
		}
		if repo.Git("rev-parse", "HEAD") != moved {
			t.Fatal("refused undo moved HEAD")
		}
	})
}

// J-GIT-029: deleting a stack parent re-parents its child; undo restores the ref, the branch
// config and the child's parent.
func TestDeleteStackParentAndUndo(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"base.txt": "b\n"})
	for _, b := range []string{"a", "b", "c"} {
		repo.Git("checkout", "-q", "-b", b)
		repo.Commit("work "+b, map[string]string{b + ".txt": b + "\n"})
	}
	repo.Checkout("main")
	repo.Git("config", "branch.a.remote", "origin")
	repo.Git("config", "branch.a.merge", "refs/heads/a")
	repo.Git("config", "branch.a.description", "the a branch")
	id := r.open(repo.Dir).RepoID
	parent := "main"
	for _, b := range []string{"a", "b", "c"} {
		p := parent
		r.mustOp(id, gitsession.OpRequest{Kind: "stackSet", Branch: b, Parent: &p})
		parent = b
	}
	tipA := repo.Git("rev-parse", "a")

	res := r.mustOp(id, gitsession.OpRequest{Kind: "branchDelete", Name: "a", Force: true})
	if got := repo.Git("config", "branch.b.kirastackparent"); got != "main" {
		t.Fatalf("branch.b.kirastackparent after delete = %q, want main", got)
	}
	if res.Undo == nil {
		t.Fatal("branch delete has no undo")
	}
	if got := undoRun(t, r.gs, id, res.Undo.ID); !got.OK {
		t.Fatalf("undo = %+v", got.Error)
	}
	if got := repo.Git("rev-parse", "refs/heads/a"); got != tipA {
		t.Fatalf("refs/heads/a = %s, want %s", got, tipA)
	}
	for k, want := range map[string]string{"branch.a.remote": "origin", "branch.a.merge": "refs/heads/a", "branch.a.description": "the a branch", "branch.b.kirastackparent": "a"} {
		if got := repo.Git("config", k); got != want {
			t.Fatalf("%s = %q, want %q", k, got, want)
		}
	}
	list := call[gitpreflight.StackListResult](t, r.gs, "stack.list", gitrpc.StackListParams{RepoID: id})
	if len(list.Stacks) != 1 || len(list.Stacks[0].Branches) != 3 {
		t.Fatalf("stack.list after undo = %+v, want three levels", list)
	}
}

// J-GIT-030 (OQ-GIT-3): annotated tag comes back as the same tag object, lightweight as same
// commit, and a tag moved again since the delete is never overwritten.
func TestDeleteTagsAndUndo(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	c1 := repo.Commit("one", map[string]string{"f.txt": "1\n"})
	c2 := repo.Commit("two", map[string]string{"f.txt": "2\n"})
	repo.Git("tag", "-a", "v2", "-m", "release two", c1)
	repo.Git("tag", "tmp", c1)
	obj := repo.Git("rev-parse", "refs/tags/v2")
	id := r.open(repo.Dir).RepoID

	res := r.mustOp(id, gitsession.OpRequest{Kind: "tagDelete", Name: "v2"})
	if got := undoRun(t, r.gs, id, res.Undo.ID); !got.OK {
		t.Fatalf("undo annotated = %+v", got.Error)
	}
	if got := repo.Git("cat-file", "-t", "refs/tags/v2"); got != "tag" {
		t.Fatalf("v2 object type after undo = %q, want tag", got)
	}
	if got := repo.Git("rev-parse", "refs/tags/v2"); got != obj {
		t.Fatalf("v2 object = %s, want %s", got, obj)
	}
	if got := repo.Git("tag", "-l", "--format=%(contents:subject)", "v2"); got != "release two" {
		t.Fatalf("v2 message = %q", got)
	}

	res = r.mustOp(id, gitsession.OpRequest{Kind: "tagDelete", Name: "tmp"})
	if got := undoRun(t, r.gs, id, res.Undo.ID); !got.OK {
		t.Fatalf("undo lightweight = %+v", got.Error)
	}
	if got := repo.Git("cat-file", "-t", "refs/tags/tmp"); got != "commit" || repo.Git("rev-parse", "tmp") != c1 {
		t.Fatalf("tmp after undo: type %q at %s, want a lightweight tag at %s", got, repo.Git("rev-parse", "tmp"), c1)
	}

	res = r.mustOp(id, gitsession.OpRequest{Kind: "tagDelete", Name: "tmp"})
	repo.Git("tag", "tmp", c2)
	got := undoRun(t, r.gs, id, res.Undo.ID)
	if got.OK {
		t.Fatalf("undo over a re-created tag succeeded: %+v", got)
	}
	if repo.Git("rev-parse", "tmp") != c2 {
		t.Fatal("undo moved the newer tag")
	}
}

// J-GIT-031: undo attribution across two windows on one repo. One undo executes, the second
// finds the slot taken.
func TestUndoAttributionAcrossWindows(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"f.txt": "1\n"})
	repo.Branch("x", "")
	tipX := repo.Git("rev-parse", "x")
	id := r.open(repo.Dir).RepoID
	gs2 := r.app.OpenGitStream()
	openOn(t, gs2, repo.Dir)

	del := r.mustOp(id, gitsession.OpRequest{Kind: "branchDelete", Name: "x", Force: true})
	if del.Undo == nil || strings.Contains(del.Undo.Label, "(window:") {
		t.Fatalf("own label = %+v, want no attribution suffix", del.Undo)
	}
	peek2 := call[gitrpc.UndoPeekResult](t, gs2, "undo.peek", gitrpc.UndoPeekParams{RepoID: id})
	if peek2.Slot == nil || !strings.Contains(peek2.Slot.Label, "(window:") {
		t.Fatalf("window 2 slot = %+v, want a label naming window 1", peek2.Slot)
	}
	first := undoRun(t, gs2, id, peek2.Slot.ID)
	if !first.OK {
		t.Fatalf("undo from window 2 = %+v", first.Error)
	}
	if repo.Git("rev-parse", "refs/heads/x") != tipX {
		t.Fatal("branch x not restored")
	}
	second := undoRun(t, r.gs, id, del.Undo.ID)
	if second.OK || second.Error == nil || second.Error.Kind != "NotFound" {
		t.Fatalf("second undo = %+v, want NotFound (slot taken)", second)
	}
}
