package gitflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
)

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestCheckoutDirtyAutostash(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"f.txt": "base\n"})
	repo.Branch("feature", "")
	repo.Checkout("feature")
	repo.Commit("feature change", map[string]string{"f.txt": "feature\n"})
	repo.Checkout("main")
	repo.Write("f.txt", "local\n")
	id := r.open(repo.Dir).RepoID

	pf := call[gitpreflight.CheckoutPreflight](t, r.gs, "preflight.checkout", gitrpc.PreflightCheckoutParams{RepoID: id, Target: "feature", Mode: "switch"})
	r.app.Contract(t, "git-stash", "git:preflight.checkout#dirty", pf)
	if pf.Verdict != "blocked" || !contains(pf.Routes, "autoStash") {
		t.Fatalf("preflight = verdict %q routes %v, want blocked with the autoStash route", pf.Verdict, pf.Routes)
	}
	on := true
	if snap := r.setSettings(id, gitrpc.RepoSettingsPatchWire{CheckoutAutoStash: &on}); !snap.CheckoutAutoStash {
		t.Fatalf("settings after set = %+v", snap)
	}

	res := r.mustOp(id, gitsession.OpRequest{Kind: "checkout", Target: "feature", Mode: "switch", AutoStash: true})
	if res.Head.Kind != "branch" || res.Head.Name != "feature" {
		t.Fatalf("head after checkout = %+v, want feature", res.Head)
	}
	if got := repo.Git("rev-parse", "--abbrev-ref", "HEAD"); got != "feature" {
		t.Fatalf("git HEAD = %q, want feature", got)
	}
	if out := repo.Git("status", "--porcelain"); out != "" {
		t.Fatalf("tree not clean after autostash checkout: %q", out)
	}
	if got, _ := os.ReadFile(filepath.Join(repo.Dir, "f.txt")); string(got) != "feature\n" {
		t.Fatalf("f.txt = %q, want the feature version", got)
	}
	stashes := call[gitrpc.StashListResult](t, r.gs, "stash.list", gitrpc.StashListParams{RepoID: id})
	r.app.Contract(t, "git-stash", "git:stash.list#autostash", stashes, flowharness.Mask("sha", "baseSha", "indexSha", "timestamp"))
	if len(stashes.Entries) != 1 || stashes.Entries[0].Branch == nil || *stashes.Entries[0].Branch != "main" {
		t.Fatalf("stash.list = %+v, want one entry tagged with main", stashes.Entries)
	}
	if show := repo.Git("show", stashes.Entries[0].Sha+":f.txt"); show != "local" {
		t.Fatalf("stashed f.txt = %q, want the local edit", show)
	}

	recent, err := r.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var found *oplog.Record
	for i := range recent {
		if recent[i].RepoName == "proj" && recent[i].Status == oplog.StatusOK {
			found = &recent[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("ops log has no ok row for proj: %+v", recent)
	}
}

func TestStashFlows(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a\n", "b.txt": "b\n"})
	id := r.open(repo.Dir).RepoID

	repo.Write("a.txt", "a edited\n")
	r.mustOp(id, gitsession.OpRequest{Kind: "stashPush", Message: ptr("first")})
	repo.Write("b.txt", "b edited\n")
	repo.Write("u.txt", "untracked\n")
	r.mustOp(id, gitsession.OpRequest{Kind: "stashPush", Message: ptr("second"), IncludeUntracked: true})

	list := call[gitrpc.StashListResult](t, r.gs, "stash.list", gitrpc.StashListParams{RepoID: id})
	if len(list.Entries) != 2 || !strings.Contains(list.Entries[0].Message, "second") || !list.Entries[0].IncludedUntracked {
		t.Fatalf("stash.list = %+v, want second (with untracked) over first", list.Entries)
	}
	if list.Entries[0].Sha != repo.Git("rev-parse", "stash@{0}") {
		t.Fatalf("top stash sha %s is not stash@{0}", list.Entries[0].Sha)
	}
	show := call[gitsession.StashShowResult](t, r.gs, "stash.show", gitrpc.StashShowParams{RepoID: id, SHA: list.Entries[0].Sha})
	kinds := map[string]porcelain.FileChangeKind{}
	for _, c := range show.Changes {
		kinds[c.Path] = c.Kind
	}
	if kinds["b.txt"] != porcelain.FileModified || kinds["u.txt"] != porcelain.FileAdded {
		t.Fatalf("stash.show changes = %v, want b.txt modified and u.txt added", kinds)
	}

	// A commit that touches the stashed file turns the pop into a conflict.
	r.external(func() { repo.Commit("diverge b", map[string]string{"b.txt": "b committed\n"}) })
	top := list.Entries[0]
	pf := call[gitpreflight.StashPopPreflight](t, r.gs, "preflight.stashPop", gitrpc.PreflightStashPopParams{RepoID: id, SHA: top.Sha, Index: top.Index})
	if pf.Verdict != "willConflict" {
		t.Fatalf("preflight.stashPop = %q, want willConflict", pf.Verdict)
	}
	res := r.op(id, gitsession.OpRequest{Kind: "stashPop", Sha: top.Sha, Index: top.Index})
	if res.OK || res.Error == nil || res.Error.Kind != "StashConflict" {
		t.Fatalf("conflicting pop = %+v err %+v, want a StashConflict error", res, res.Error)
	}
	if st := r.status(id); st.Counts.Unmerged == 0 {
		t.Fatalf("no unmerged paths after the conflicting pop: %+v", st.Counts)
	}
	after := call[gitrpc.StashListResult](t, r.gs, "stash.list", gitrpc.StashListParams{RepoID: id})
	if len(after.Entries) != 2 {
		t.Fatalf("stash list after conflicting pop = %d entries, want 2 (stash kept)", len(after.Entries))
	}
	repo.Git("reset", "--hard", "-q")
	repo.Git("clean", "-fdq")

	bpf := call[gitpreflight.StashBranchPreflight](t, r.gs, "preflight.stashBranch", gitrpc.PreflightStashBranchParams{RepoID: id, SHA: top.Sha, Branch: "from-stash"})
	if bpf.Verdict != "clean" {
		t.Fatalf("preflight.stashBranch = %+v, want clean", bpf)
	}
	r.mustOp(id, gitsession.OpRequest{Kind: "stashBranch", Sha: top.Sha, Index: top.Index, Branch: "from-stash"})
	if got := repo.Git("rev-parse", "--abbrev-ref", "HEAD"); got != "from-stash" {
		t.Fatalf("HEAD = %q after stash branch", got)
	}
	if got, _ := os.ReadFile(filepath.Join(repo.Dir, "u.txt")); string(got) != "untracked\n" {
		t.Fatalf("u.txt after stash branch = %q", got)
	}

	t.Run("global bucket is shared by linked worktrees", func(t *testing.T) {
		repo.Git("reset", "--hard", "-q")
		repo.Git("clean", "-fdq")
		repo.Checkout("main")
		repo.Write("a.txt", "parked\n")
		linked := repo.LinkedWorktree("linked")
		lid := r.open(linked.Dir).RepoID
		r.mustOp(id, gitsession.OpRequest{Kind: "globalStashSave", Label: "parked work"})
		for name, rid := range map[string]string{"main": id, "linked": lid} {
			g := call[gitrpc.StashListResult](t, r.gs, "globalStash.list", gitrpc.GlobalStashListParams{RepoID: rid})
			if len(g.Entries) != 1 || !strings.Contains(g.Entries[0].Message, "parked work") || g.Entries[0].Scope != porcelain.StashScopeGlobal {
				t.Fatalf("globalStash.list for %s = %+v, want the parked entry", name, g.Entries)
			}
		}
	})
}

func ptr[T any](v T) *T { return &v }

func TestWorktreeAddRemove(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a\n"})
	repo.Branch("wt-branch", "")
	id := r.open(repo.Dir).RepoID
	path := filepath.Join(r.app.Work, "wt1")

	add := gitrpc.PreflightWorktreeAddParams{RepoID: id, Path: path, Mode: "existingBranch", Branch: "wt-branch"}
	pf := call[gitpreflight.WorktreeAddPreflight](t, r.gs, "preflight.worktreeAdd", add)
	if pf.Verdict != "clean" {
		t.Fatalf("preflight.worktreeAdd = %+v, want clean", pf)
	}
	r.mustOp(id, gitsession.OpRequest{Kind: "worktreeAdd", Path: path, Mode: "existingBranch", Branch: "wt-branch"})
	if !strings.Contains(repo.Git("worktree", "list", "--porcelain"), "worktree "+path) {
		t.Fatalf("git worktree list lacks %s", path)
	}
	list := call[gitrpc.WorktreeListResult](t, r.gs, "worktree.list", gitrpc.WorktreeListParams{RepoID: id})
	r.app.Contract(t, "git-worktree", "git:worktree.list#after-add", list, flowharness.Mask("head"), flowharness.Replace(r.app.Work, "<work>"))
	var listed *gitsession.WorktreeEntry
	for i, w := range list.Worktrees {
		if w.Path == path {
			listed = &list.Worktrees[i]
		}
	}
	if listed == nil || listed.Branch == nil || !strings.HasSuffix(*listed.Branch, "wt-branch") || len(list.Worktrees) != 2 {
		t.Fatalf("worktree.list = %+v, want main plus wt1 on wt-branch", list.Worktrees)
	}
	if again := call[gitpreflight.WorktreeAddPreflight](t, r.gs, "preflight.worktreeAdd", add); again.Verdict != "blocked" {
		t.Fatalf("second add to the same path = %q, want blocked", again.Verdict)
	}

	if err := os.WriteFile(filepath.Join(path, "dirty.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rm := gitrpc.PreflightWorktreeRemoveParams{RepoID: id, Path: path}
	if dirty := call[gitpreflight.WorktreeRemovePreflight](t, r.gs, "preflight.worktreeRemove", rm); dirty.Verdict != "dirty" {
		t.Fatalf("remove preflight on a dirty worktree = %+v, want dirty", dirty)
	}
	if res := r.op(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: path}); res.OK {
		t.Fatal("removing a dirty worktree without force succeeded")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("dirty worktree vanished: %v", err)
	}
	if err := os.Remove(filepath.Join(path, "dirty.txt")); err != nil {
		t.Fatal(err)
	}
	if clean := call[gitpreflight.WorktreeRemovePreflight](t, r.gs, "preflight.worktreeRemove", rm); clean.Verdict != "clean" {
		t.Fatalf("remove preflight on a clean worktree = %+v, want clean", clean)
	}
	r.mustOp(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: path})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("worktree directory still there: %v", err)
	}
	if strings.Contains(repo.Git("worktree", "list", "--porcelain"), path) {
		t.Fatal("git still lists the removed worktree")
	}
}

func TestCherryPickRevertConflict(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"c.txt": "base\n"})
	repo.Branch("feat", "")
	repo.Checkout("feat")
	picked := repo.Commit("feat edits c", map[string]string{"c.txt": "feat\n"})
	repo.Checkout("main")
	onMain := repo.Commit("main edits c", map[string]string{"c.txt": "main\n"})
	id := r.open(repo.Dir).RepoID

	pf := call[gitpreflight.CherryPickPreflight](t, r.gs, "preflight.cherryPick", gitrpc.PreflightCherryPickParams{RepoID: id, SHA: picked})
	if pf.Verdict != "willConflict" {
		t.Fatalf("preflight.cherryPick = %+v, want willConflict", pf)
	}
	res := r.op(id, gitsession.OpRequest{Kind: "cherryPick", Sha: picked})
	if res.OK || res.InProgress == nil || res.InProgress.Kind != gitpreflight.InProgressCherryPick {
		t.Fatalf("conflicting pick = %+v, want a cherry-pick in progress", res)
	}
	st := r.status(id)
	if st.InProgress == nil || st.InProgress.Kind != gitpreflight.InProgressCherryPick {
		t.Fatalf("status.get = %+v, want a cherry-pick in progress", st.InProgress)
	}
	// Contract git-view-head: tests/ui/repo-commit-meta.spec.ts shows the operation in the view head.
	r.app.Contract(t, "git-view-head", "git:status.get#cherry-pick", st, flowharness.Mask("otherSha"))
	r.mustOp(id, gitsession.OpRequest{Kind: "opAbort"})
	if got := repo.Git("rev-parse", "HEAD"); got != onMain {
		t.Fatalf("HEAD after abort = %s, want %s", got, onMain)
	}
	if out := repo.Git("status", "--porcelain"); out != "" {
		t.Fatalf("tree not clean after abort: %q", out)
	}

	res = r.op(id, gitsession.OpRequest{Kind: "cherryPick", Sha: picked})
	if res.OK || res.InProgress == nil {
		t.Fatalf("second conflicting pick = %+v", res)
	}
	repo.Write("c.txt", "resolved\n")
	repo.Git("add", "c.txt")
	done := r.mustOp(id, gitsession.OpRequest{Kind: "opContinue"})
	if done.InProgress != nil {
		t.Fatalf("still in progress after continue: %+v", done.InProgress)
	}
	if got := repo.Git("log", "-1", "--format=%s"); got != "feat edits c" {
		t.Fatalf("picked commit subject = %q", got)
	}
	if repo.Git("rev-parse", "HEAD~1") != onMain {
		t.Fatal("picked commit is not on top of main's tip")
	}

	t.Run("revert a merge with mainline", func(t *testing.T) {
		repo.Branch("side", "")
		repo.Checkout("side")
		repo.Commit("side adds s", map[string]string{"s.txt": "s\n"})
		repo.Checkout("main")
		repo.Commit("main adds m", map[string]string{"m.txt": "m\n"})
		merge := repo.Merge("side", "merge side")
		rp := call[gitpreflight.RevertPreflight](t, r.gs, "preflight.revert", gitrpc.PreflightRevertParams{RepoID: id, Shas: []string{merge}})
		if len(rp.MainlineRequired) == 0 {
			t.Fatalf("preflight.revert of a merge = %+v, want mainline choices", rp)
		}
		mainline := 1
		r.mustOp(id, gitsession.OpRequest{Kind: "revert", Shas: []string{merge}, Mainline: &mainline})
		if _, err := os.Stat(filepath.Join(repo.Dir, "s.txt")); !os.IsNotExist(err) {
			t.Fatalf("s.txt survived reverting the merge against mainline 1: %v", err)
		}
		if got := repo.Git("log", "-1", "--format=%s"); !strings.HasPrefix(got, "Revert") {
			t.Fatalf("revert commit subject = %q", got)
		}
	})
}

func TestResetAndUndo(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	c1 := repo.Commit("one", map[string]string{"f.txt": "1\n"})
	repo.Commit("two", map[string]string{"f.txt": "2\n"})
	c3 := repo.Commit("three", map[string]string{"f.txt": "3\n"})
	repo.Write("f.txt", "dirty\n")
	id := r.open(repo.Dir).RepoID

	for _, mode := range []string{"soft", "mixed", "hard"} {
		pf := call[gitpreflight.ResetPreflight](t, r.gs, "preflight.reset", gitrpc.PreflightResetParams{RepoID: id, Target: c1, Mode: mode})
		if pf.Leaving != 2 || pf.Mode != mode {
			t.Fatalf("preflight.reset %s = %+v, want leaving 2", mode, pf)
		}
		if (mode == "hard") != (len(pf.Destroys) > 0) || (mode == "hard") != pf.RequiresTypedConfirmation {
			t.Fatalf("preflight.reset %s destroys %v typed %v", mode, pf.Destroys, pf.RequiresTypedConfirmation)
		}
	}

	if res := r.op(id, gitsession.OpRequest{Kind: "reset", Target: c1, Mode: "hard"}); res.OK || res.Error == nil || res.Error.Kind != "ConfirmationRequired" {
		t.Fatalf("hard reset without the typed token = %+v, want ConfirmationRequired", res)
	}
	if repo.Git("rev-parse", "HEAD") != c3 {
		t.Fatal("refused hard reset moved HEAD")
	}

	mixed := r.mustOp(id, gitsession.OpRequest{Kind: "reset", Target: c1, Mode: "mixed"})
	if mixed.Undo == nil || repo.Git("rev-parse", "HEAD") != c1 {
		t.Fatalf("mixed reset = %+v, HEAD %s", mixed, repo.Git("rev-parse", "HEAD"))
	}
	if got, _ := os.ReadFile(filepath.Join(repo.Dir, "f.txt")); string(got) != "dirty\n" {
		t.Fatalf("mixed reset touched the working tree: %q", got)
	}
	peek := call[gitrpc.UndoPeekResult](t, r.gs, "undo.peek", gitrpc.UndoPeekParams{RepoID: id})
	if peek.Slot == nil || !strings.HasPrefix(peek.Slot.Label, "Reset (mixed)") || peek.Slot.RecoverySha != c3 {
		t.Fatalf("undo.peek = %+v, want the mixed reset recovering %s", peek.Slot, c3)
	}
	undone := call[gitsession.OpResult](t, r.gs, "undo.run", gitrpc.UndoRunParams{RepoID: id, ID: peek.Slot.ID})
	if !undone.OK || repo.Git("rev-parse", "HEAD") != c3 || undone.Head.Name != "main" {
		t.Fatalf("undo = %+v, HEAD %s, want main back at %s", undone, repo.Git("rev-parse", "HEAD"), c3)
	}
	if got := repo.Git("rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Fatalf("branch after undo = %q", got)
	}

	token := c1[:7]
	hard := r.mustOp(id, gitsession.OpRequest{Kind: "reset", Target: c1, Mode: "hard", ConfirmToken: &token})
	if repo.Git("rev-parse", "HEAD") != c1 || repo.Git("status", "--porcelain") != "" || hard.Undo == nil {
		t.Fatalf("hard reset left HEAD %s status %q", repo.Git("rev-parse", "HEAD"), repo.Git("status", "--porcelain"))
	}
	back := call[gitsession.OpResult](t, r.gs, "undo.run", gitrpc.UndoRunParams{RepoID: id, ID: hard.Undo.ID})
	if !back.OK || repo.Git("rev-parse", "HEAD") != c3 {
		t.Fatalf("undo of the hard reset = %+v", back)
	}
}

func TestStackRestack(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"base.txt": "base\n"})
	for _, b := range []string{"a", "b", "c"} {
		repo.Git("checkout", "-q", "-b", b)
		repo.Commit("work on "+b, map[string]string{b + ".txt": b + "\n"})
	}
	id := r.open(repo.Dir).RepoID
	parent := "main"
	for _, b := range []string{"a", "b", "c"} {
		p := parent
		r.mustOp(id, gitsession.OpRequest{Kind: "stackSet", Branch: b, Parent: &p})
		parent = b
	}
	before := call[gitpreflight.StackListResult](t, r.gs, "stack.list", gitrpc.StackListParams{RepoID: id})
	if len(before.Stacks) != 1 || len(before.Stacks[0].Branches) != 3 || before.Stacks[0].NeedsRestack {
		t.Fatalf("stack.list = %+v, want one fresh stack of three", before)
	}

	r.external(func() {
		repo.Checkout("a")
		repo.Git("commit", "-q", "--amend", "-m", "work on a, reworded")
		repo.Checkout("c")
	})

	after := call[gitpreflight.StackListResult](t, r.gs, "stack.list", gitrpc.StackListParams{RepoID: id})
	if len(after.Stacks) != 1 || !after.Stacks[0].NeedsRestack {
		t.Fatalf("stack.list after amending the base = %+v, want needsRestack", after)
	}
	pf := call[gitpreflight.RestackPreflight](t, r.gs, "preflight.restack", gitrpc.PreflightRestackParams{RepoID: id, Branch: "c"})
	if pf.Verdict != "clean" || len(pf.Plan) != 2 {
		t.Fatalf("preflight.restack = %+v, want clean with b and c planned", pf)
	}
	res := call[gitsession.RestackResult](t, r.gs, "stack.restack", gitrpc.StackRestackParams{RepoID: id, Branch: "c"})
	if !res.OK || strings.Join(res.Restacked, ",") != "b,c" {
		t.Fatalf("stack.restack = %+v, want b and c restacked", res)
	}
	tipA := repo.Git("rev-parse", "a")
	if repo.Git("merge-base", "--is-ancestor", tipA, "b"); repo.Git("rev-parse", "b~1") != tipA {
		t.Fatalf("b is not on the amended a")
	}
	if repo.Git("rev-parse", "c~1") != repo.Git("rev-parse", "b") {
		t.Fatal("c is not on the restacked b")
	}
	if fresh := call[gitpreflight.StackListResult](t, r.gs, "stack.list", gitrpc.StackListParams{RepoID: id}); fresh.Stacks[0].NeedsRestack {
		t.Fatalf("stack still needs restack: %+v", fresh)
	}
}

// J-GIT-028 (+ J-GIT-002): a cherry-pick series onto release. A clean pick undoes, a conflicting
// pick continues, an empty pick surfaces and skips, a no-commit pick leaves the staged diff and no
// undo, a merge picks against mainline 1. Quitting mid-conflict leaves the operation detectable.
func TestCherryPickSeries(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	base := repo.Commit("base", map[string]string{"c.txt": "base\n", "z.txt": "z0\n"})
	repo.Branch("release", "")
	repo.Git("checkout", "-q", "-b", "dev")
	x := repo.Commit("add x", map[string]string{"x.txt": "x\n"})
	y := repo.Commit("edit c on dev", map[string]string{"c.txt": "dev\n"})
	z := repo.Commit("edit z", map[string]string{"z.txt": "z1\n"})
	w := repo.Commit("add w", map[string]string{"w.txt": "w\n"})
	repo.Git("checkout", "-q", "-b", "side", base)
	repo.Commit("side adds s", map[string]string{"s.txt": "s\n"})
	repo.Checkout("dev")
	merge := repo.Merge("side", "merge side")
	repo.Checkout("release")
	releaseBase := repo.Commit("release edit c", map[string]string{"c.txt": "release\n"})
	repo.Commit("release already has z", map[string]string{"z.txt": "z1\n"})
	id := r.open(repo.Dir).RepoID

	pick := r.mustOp(id, gitsession.OpRequest{Kind: "cherryPick", Sha: x})
	if pick.Undo == nil {
		t.Fatal("a clean pick offers no undo")
	}
	before := repo.Git("rev-parse", "HEAD~1")
	if got := undoRun(t, r.gs, id, pick.Undo.ID); !got.OK || repo.Git("rev-parse", "HEAD") != before || repo.Git("status", "--porcelain") != "" {
		t.Fatalf("undo of the pick = %+v, HEAD %s, status %q", got, repo.Git("rev-parse", "HEAD"), repo.Git("status", "--porcelain"))
	}

	// Conflict at the second pick, resolve and continue.
	r.mustOp(id, gitsession.OpRequest{Kind: "cherryPick", Sha: x})
	res := r.op(id, gitsession.OpRequest{Kind: "cherryPick", Sha: y})
	if res.OK || res.InProgress == nil || res.InProgress.Kind != gitpreflight.InProgressCherryPick {
		t.Fatalf("conflicting pick = %+v, want a cherry-pick in progress", res)
	}
	if got := r.op(id, gitsession.OpRequest{Kind: "cherryPick", Sha: z}); got.OK || got.Error == nil || got.Error.Kind != "OperationInProgress" {
		t.Fatalf("a second pick mid-conflict = %+v, want OperationInProgress", got)
	}
	repo.Write("c.txt", "resolved\n")
	repo.Git("add", "c.txt")
	if done := r.mustOp(id, gitsession.OpRequest{Kind: "opContinue"}); done.InProgress != nil {
		t.Fatalf("still in progress after continue: %+v", done.InProgress)
	}
	if repo.Git("log", "-1", "--format=%s") != "edit c on dev" {
		t.Fatal("continued pick is not on top")
	}

	// Empty pick: already applied.
	empty := r.op(id, gitsession.OpRequest{Kind: "cherryPick", Sha: z})
	if empty.OK || empty.InProgress == nil {
		t.Fatalf("empty pick = %+v, want in progress", empty)
	}
	if st := r.status(id); st.Counts.Unmerged != 0 {
		t.Fatalf("empty pick shows %d unmerged files, want none", st.Counts.Unmerged)
	}
	r.mustOp(id, gitsession.OpRequest{Kind: "opSkip"})
	if _, err := os.Stat(filepath.Join(repo.Dir, ".git", "CHERRY_PICK_HEAD")); !os.IsNotExist(err) {
		t.Fatalf("CHERRY_PICK_HEAD after skip: %v", err)
	}

	// No-commit pick: staged diff equals the picked commit's diff, no undo.
	headBefore := repo.Git("rev-parse", "HEAD")
	nc := r.mustOp(id, gitsession.OpRequest{Kind: "cherryPick", Sha: w, NoCommit: true})
	if nc.Undo != nil {
		t.Fatalf("no-commit pick offers an undo: %+v", nc.Undo)
	}
	if repo.Git("rev-parse", "HEAD") != headBefore {
		t.Fatal("no-commit pick moved HEAD")
	}
	if got, want := repo.Git("diff", "--cached"), repo.Git("diff", w+"^", w); got != want {
		t.Fatalf("staged diff:\n%s\nwant W's diff:\n%s", got, want)
	}
	repo.Git("reset", "--hard", "-q")

	// Merge commit with mainline 1.
	mainline := 1
	r.mustOp(id, gitsession.OpRequest{Kind: "cherryPick", Sha: merge, Mainline: &mainline})
	if got, want := repo.Git("diff", "HEAD~1", "HEAD"), repo.Git("diff", merge+"^1", merge); got != want {
		t.Fatalf("mainline pick diff:\n%s\nwant\n%s", got, want)
	}
	_ = releaseBase

	// Quit mid-conflict: the operation is still there after a restart.
	repo.Git("reset", "--hard", "-q", releaseBase)
	conflict := r.op(id, gitsession.OpRequest{Kind: "cherryPick", Sha: y})
	if conflict.OK || conflict.InProgress == nil {
		t.Fatalf("second conflicting pick = %+v", conflict)
	}
	r.app.Restart()
	gs := r.app.OpenGitStream()
	nid := openOn(t, gs, repo.Dir).RepoID
	st := call[gitpreflight.StatusSummary](t, gs, "status.get", gitrpc.StatusGetParams{RepoID: nid})
	if st.InProgress == nil || st.InProgress.Kind != gitpreflight.InProgressCherryPick || st.Counts.Unmerged == 0 {
		t.Fatalf("status after restart = %+v, want the cherry-pick still in progress with conflicts", st)
	}
	abort := call[gitsession.OpResult](t, gs, "op.run", gitrpc.OpRunParams{RepoID: nid, Op: gitsession.OpRequest{Kind: "opAbort"}})
	if !abort.OK || repo.Git("rev-parse", "HEAD") != releaseBase || repo.Git("status", "--porcelain") != "" {
		t.Fatalf("abort after restart = %+v, HEAD %s", abort, repo.Git("rev-parse", "HEAD"))
	}
}
