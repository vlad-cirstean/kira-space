package gitflow_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/testx"
)

type porcelainV2 struct {
	staged, unstaged, untracked, unmerged int
	paths                                 []string
}

// parsePorcelainV2 is the oracle for status.get: git status --porcelain=v2 -z.
func parsePorcelainV2(raw string) porcelainV2 {
	var out porcelainV2
	recs := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if rec == "" {
			continue
		}
		switch rec[0] {
		case '1':
			f := strings.SplitN(rec, " ", 9)
			out.countXY(f[1])
			out.paths = append(out.paths, f[8])
		case '2':
			f := strings.SplitN(rec, " ", 10)
			out.countXY(f[1])
			out.paths = append(out.paths, f[9])
			i++
		case 'u':
			f := strings.SplitN(rec, " ", 11)
			out.unmerged++
			out.paths = append(out.paths, f[10])
		case '?':
			out.untracked++
			out.paths = append(out.paths, rec[2:])
		}
	}
	sort.Strings(out.paths)
	return out
}

func (p *porcelainV2) countXY(xy string) {
	if xy[0] != '.' {
		p.staged++
	}
	if xy[1] != '.' {
		p.unstaged++
	}
}

// Guards working.detail and status.get against git status for every dirty state.
func TestWorkingTreeStates(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{
		"staged.txt": "one\n", "unstaged.txt": "one\n", "gone.txt": "one\n", "keep.txt": "one\n",
	})
	repo.Write("staged.txt", "two\n")
	repo.Git("add", "staged.txt")
	repo.Write("new-staged.txt", "new\n")
	repo.Git("add", "new-staged.txt")
	repo.Write("unstaged.txt", "two\n")
	if err := os.Remove(filepath.Join(repo.Dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	repo.Write("untracked.txt", "u\n")
	id := r.open(repo.Dir).RepoID

	want := parsePorcelainV2(repo.Git("status", "--porcelain=v2", "-z", "--untracked-files=all"))
	st := call[gitpreflight.StatusSummary](t, r.gs, "status.get", gitrpc.StatusGetParams{RepoID: id})
	if st.IsClean {
		t.Fatal("status.get isClean on a dirty tree")
	}
	if st.Counts.Staged != want.staged || st.Counts.Unstaged != want.unstaged || st.Counts.Untracked != want.untracked {
		t.Fatalf("counts = %+v, git: staged %d unstaged %d untracked %d", st.Counts, want.staged, want.unstaged, want.untracked)
	}
	got := append([]string(nil), st.DirtyPaths...)
	sort.Strings(got)
	if strings.Join(got, "|") != strings.Join(want.paths, "|") {
		t.Fatalf("dirtyPaths = %v, git: %v", got, want.paths)
	}

	detail := call[struct {
		Files []porcelain.FileChange `json:"files"`
	}](t, r.gs, "working.detail", gitrpc.WorkingDetailParams{RepoID: id})
	kinds := map[string]porcelain.FileChangeKind{}
	for _, f := range detail.Files {
		kinds[f.Path] = f.Kind
	}
	for path, kind := range map[string]porcelain.FileChangeKind{
		"staged.txt": porcelain.FileModified, "new-staged.txt": porcelain.FileAdded,
		"unstaged.txt": porcelain.FileModified, "gone.txt": porcelain.FileDeleted,
	} {
		if kinds[path] != kind {
			t.Fatalf("working.detail %q = %q, want %q (all: %v)", path, kinds[path], kind, kinds)
		}
	}
	if _, ok := kinds["keep.txt"]; ok {
		t.Fatalf("working.detail lists the unchanged keep.txt: %v", kinds)
	}

	t.Run("conflicted", func(t *testing.T) {
		c := r.app.NewRepo("conflict")
		c.Commit("base", map[string]string{"c.txt": "base\n"})
		c.Conflict("left", "right", "c.txt")
		cid := r.open(c.Dir).RepoID
		wantC := parsePorcelainV2(c.Git("status", "--porcelain=v2", "-z"))
		stC := call[gitpreflight.StatusSummary](t, r.gs, "status.get", gitrpc.StatusGetParams{RepoID: cid})
		if wantC.unmerged != 1 || stC.Counts.Unmerged != wantC.unmerged {
			t.Fatalf("unmerged = %d, git: %d", stC.Counts.Unmerged, wantC.unmerged)
		}
		if stC.InProgress == nil || stC.InProgress.Kind != gitpreflight.InProgressMerge {
			t.Fatalf("inProgress = %+v, want a merge", stC.InProgress)
		}
		if !strings.Contains(strings.Join(stC.InProgress.ConflictedPaths, ","), "c.txt") || stC.InProgress.UnmergedCount != 1 {
			t.Fatalf("inProgress = %+v, want c.txt conflicted, one unmerged", stC.InProgress)
		}
	})
}

func (r *rig) worktrees(id string) []gitsession.WorktreeEntry {
	r.t.Helper()
	return call[gitrpc.WorktreeListResult](r.t, r.gs, "worktree.list", gitrpc.WorktreeListParams{RepoID: id}).Worktrees
}

func (r *rig) wtRemovePreflight(id, path string) gitpreflight.WorktreeRemovePreflight {
	r.t.Helper()
	return call[gitpreflight.WorktreeRemovePreflight](r.t, r.gs, "preflight.worktreeRemove", gitrpc.PreflightWorktreeRemoveParams{RepoID: id, Path: path})
}

func blockerKinds(bs []gitpreflight.WorktreeRemoveBlocker) []string {
	out := []string{}
	for _, b := range bs {
		out = append(out, b.Kind)
	}
	return out
}

// J-WT-011 (+ J-WT-012): every remove blocker refuses and leaves files alone; a worktree that goes
// dirty between dialog and confirm is refused at confirm and needs the typed token.
func TestWorktreeRemoveBlockersAndRecheck(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a\n"})
	wt := repo.LinkedWorktree("wt-a")
	id := r.open(repo.Dir).RepoID

	pf := r.wtRemovePreflight(id, repo.Dir)
	if pf.Verdict != "blocked" || !strings.Contains(strings.Join(blockerKinds(pf.Blockers), ","), "mainWorktree") {
		t.Fatalf("remove main = %+v, want blocked by mainWorktree", pf)
	}
	if !strings.Contains(strings.Join(blockerKinds(pf.Blockers), ","), "currentWorktree") {
		t.Fatalf("remove current = %+v, want currentWorktree", pf.Blockers)
	}
	if res := r.op(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: repo.Dir}); res.OK {
		t.Fatal("removed the main worktree")
	}
	if _, err := os.Stat(filepath.Join(repo.Dir, "a.txt")); err != nil {
		t.Fatalf("main worktree damaged: %v", err)
	}
	if res := r.op(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: filepath.Join(r.app.Work, "not-a-worktree")}); res.OK || res.Error == nil {
		t.Fatalf("removing a path that is no worktree = %+v, want refusal", res)
	}

	// Open in another window.
	gs2 := r.app.OpenGitStream()
	openOn(t, gs2, wt.Dir)
	if got := r.wtRemovePreflight(id, wt.Dir); got.Verdict != "blocked" || blockerKinds(got.Blockers)[0] != "openInAnotherWindow" {
		t.Fatalf("remove while open elsewhere = %+v", got)
	}
	gs2.Close()
	testx.WaitUntil(t, wait, func() bool { return r.wtRemovePreflight(id, wt.Dir).Verdict == "clean" })

	// Locked.
	repo.Git("worktree", "lock", "--reason", "usb", wt.Dir)
	lockRes := r.op(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: wt.Dir})
	if lockRes.OK || lockRes.Error == nil || lockRes.Error.Kind != "WorktreeLocked" || !strings.Contains(lockRes.Error.Message, "usb") {
		t.Fatalf("remove locked = %+v, want WorktreeLocked naming the reason", lockRes)
	}
	repo.Git("worktree", "unlock", wt.Dir)

	// Dialog opened clean, then a file lands in the worktree.
	if got := r.wtRemovePreflight(id, wt.Dir); got.Verdict != "clean" || got.RequiresTypedConfirmation {
		t.Fatalf("clean preflight = %+v", got)
	}
	wt.Write("fresh.txt", "agent output\n")
	stale := r.op(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: wt.Dir})
	if stale.OK || stale.Error == nil || stale.Error.Kind != "ConfirmationRequired" {
		t.Fatalf("remove after the worktree went dirty = %+v, want ConfirmationRequired", stale)
	}
	if _, err := os.Stat(filepath.Join(wt.Dir, "fresh.txt")); err != nil {
		t.Fatalf("fresh.txt lost: %v", err)
	}
	dirty := r.wtRemovePreflight(id, wt.Dir)
	if dirty.Verdict != "dirty" || !dirty.RequiresTypedConfirmation || dirty.ConfirmToken == "" {
		t.Fatalf("dirty preflight = %+v, want a token form", dirty)
	}
	wrong := "nope"
	if res := r.op(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: wt.Dir, Force: true, ConfirmToken: &wrong}); res.OK || res.Error == nil || res.Error.Kind != "ConfirmationRequired" {
		t.Fatalf("wrong token = %+v, want ConfirmationRequired (force from the client is never trusted)", res)
	}
	if _, err := os.Stat(filepath.Join(wt.Dir, "fresh.txt")); err != nil {
		t.Fatalf("fresh.txt lost after a wrong token: %v", err)
	}
	// The token fingerprints the set the user saw: a file written after the preflight voids it.
	oldToken := dirty.ConfirmToken
	wt.Write("later.txt", "written after the user looked\n")
	if res := r.op(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: wt.Dir, ConfirmToken: &oldToken}); res.OK || res.Error == nil || res.Error.Kind != "ConfirmationRequired" {
		t.Fatalf("stale token = %+v, want ConfirmationRequired", res)
	}
	if _, err := os.Stat(filepath.Join(wt.Dir, "later.txt")); err != nil {
		t.Fatalf("later.txt lost after a stale token: %v", err)
	}
	tip := repo.Git("rev-parse", "refs/heads/wt-a")
	token := r.wtRemovePreflight(id, wt.Dir).ConfirmToken
	r.mustOp(id, gitsession.OpRequest{Kind: "worktreeRemove", Path: wt.Dir, ConfirmToken: &token})
	if _, err := os.Stat(wt.Dir); !os.IsNotExist(err) {
		t.Fatalf("worktree directory survived the confirmed remove: %v", err)
	}
	if strings.Contains(repo.Git("worktree", "list", "--porcelain"), wt.Dir) {
		t.Fatal("git still lists the removed worktree")
	}
	if got := repo.Git("rev-parse", "refs/heads/wt-a"); got != tip {
		t.Fatalf("branch wt-a = %s after the remove, want kept at %s", got, tip)
	}
}

// J-WT-010 (+ J-WT-013, J-GIT-004): add in every mode, every blocker refuses with no directory
// created, and a worktree directory deleted outside shows prunable and frees its branch.
func TestWorktreeAddModesBlockersAndPrunable(t *testing.T) {
	r := newRig(t)
	seed := r.app.NewRepo("seed")
	seed.Commit("base", map[string]string{"a.txt": "a\n"})
	bare := r.app.NewBare("remote")
	bare.PushFrom(seed)
	repo := bare.Clone(filepath.Join(r.app.Work, "proj"))
	repo.Branch("topic", "")
	repo.Tag("v1")
	id := r.open(repo.Dir).RepoID
	base := filepath.Join(r.app.Work, "trees")
	add := func(op gitsession.OpRequest) gitsession.OpResult {
		op.Kind = "worktreeAdd"
		return r.op(id, op)
	}

	wtA := filepath.Join(base, "repo-wt-a")
	r.mustOp(id, gitsession.OpRequest{Kind: "worktreeAdd", Path: wtA, Mode: "newBranch", Branch: "wt-a", StartPoint: "origin/main"})
	wtTopic := filepath.Join(base, "repo-topic")
	r.mustOp(id, gitsession.OpRequest{Kind: "worktreeAdd", Path: wtTopic, Mode: "existingBranch", Branch: "topic"})
	wtTag := filepath.Join(base, "repo-v1")
	r.mustOp(id, gitsession.OpRequest{Kind: "worktreeAdd", Path: wtTag, Mode: "detach", StartPoint: "v1"})
	wtUni := filepath.Join(base, "wt ünï cödé")
	r.mustOp(id, gitsession.OpRequest{Kind: "worktreeAdd", Path: wtUni, Mode: "newBranch", Branch: "uni", StartPoint: "main"})
	list := repo.Git("worktree", "list", "--porcelain")
	for _, p := range []string{wtA, wtTopic, wtTag, wtUni} {
		if !strings.Contains(list, "worktree "+p) {
			t.Fatalf("git worktree list lacks %s:\n%s", p, list)
		}
	}
	if n := strings.Count(list, "worktree "); n != 5 {
		t.Fatalf("worktree count = %d, want main plus four", n)
	}
	if got := repo.Git("rev-parse", "wt-a"); got != repo.Git("rev-parse", "origin/main") {
		t.Fatal("wt-a does not start at origin/main")
	}

	refuse := func(name string, op gitsession.OpRequest, kind, createdPath string) {
		t.Helper()
		res := add(op)
		if res.OK || res.Error == nil || res.Error.Kind != kind {
			t.Fatalf("%s = %+v, want %s", name, res, kind)
		}
		if createdPath != "" {
			if _, err := os.Stat(createdPath); !os.IsNotExist(err) {
				t.Fatalf("%s left %s behind: %v", name, createdPath, err)
			}
		}
	}
	filePath := filepath.Join(base, "a-file")
	if err := os.WriteFile(filePath, []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}
	refuse("path is a file", gitsession.OpRequest{Path: filePath, Mode: "newBranch", Branch: "x1", StartPoint: "main"}, "AlreadyExists", "")
	if b, _ := os.ReadFile(filePath); string(b) != "f" {
		t.Fatalf("the file at the path was touched: %q", b)
	}
	pfFile := call[gitpreflight.WorktreeAddPreflight](t, r.gs, "preflight.worktreeAdd", gitrpc.PreflightWorktreeAddParams{RepoID: id, Path: filePath, Mode: "newBranch", Branch: "x1", StartPoint: "main"})
	if pfFile.Verdict != "blocked" {
		t.Fatalf("preflight over an existing path = %+v, want blocked", pfFile)
	}
	p5 := filepath.Join(base, "dup-branch")
	refuse("branch name exists", gitsession.OpRequest{Path: p5, Mode: "newBranch", Branch: "topic", StartPoint: "main"}, "AlreadyExists", p5)
	p6 := filepath.Join(base, "main-again")
	refuse("branch checked out in main", gitsession.OpRequest{Path: p6, Mode: "existingBranch", Branch: "main"}, "WorktreeConflict", p6)
	p7 := filepath.Join(base, "nowhere")
	refuse("unknown start point", gitsession.OpRequest{Path: p7, Mode: "newBranch", Branch: "x7", StartPoint: "nope"}, "NotFound", p7)
	if out := repo.Git("branch", "--list", "x7", "x1"); out != "" {
		t.Fatalf("a refused add created branches: %q", out)
	}
	p8 := filepath.Join(repo.Dir, ".git", "inside")
	if res := add(gitsession.OpRequest{Path: p8, Mode: "newBranch", Branch: "x8", StartPoint: "main"}); res.OK {
		t.Fatalf("a worktree inside .git was created: %+v", res)
	}

	// Directory deleted outside: listed prunable, its branch is not reported as checked out elsewhere.
	if err := os.RemoveAll(wtTopic); err != nil {
		t.Fatal(err)
	}
	var prunable *gitsession.WorktreeEntry
	for _, w := range r.worktrees(id) {
		if w.Path == wtTopic {
			w := w
			prunable = &w
		}
	}
	if prunable == nil || prunable.Prunable == nil || prunable.Prunable.Reason == "" {
		t.Fatalf("worktree.list entry for the deleted directory = %+v, want prunable with a reason", prunable)
	}
	repo.Git("worktree", "prune")
	if strings.Contains(repo.Git("worktree", "list", "--porcelain"), wtTopic) {
		t.Fatal("prune left the registration")
	}
	if got := repo.Git("rev-parse", "refs/heads/topic"); got == "" {
		t.Fatal("prune removed the branch")
	}
	r.mustOp(id, gitsession.OpRequest{Kind: "worktreeAdd", Path: wtTopic, Mode: "existingBranch", Branch: "topic"})
}
