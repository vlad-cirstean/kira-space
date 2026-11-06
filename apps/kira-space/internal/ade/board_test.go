package ade

import (
	"context"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitcred"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adeflow"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg/repocfgtest"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

type boardHarness struct {
	t       *testing.T
	repos   *repos.Repos
	board   *TaskBoard
	files   *repocfg.Store
	emitted atomic.Int32
	status  atomic.Value // gitclient.GitStatus
	mid     string
}

func newBoardHarness(t *testing.T) *boardHarness {
	t.Helper()
	db, err := storage.OpenAt(t.TempDir())
	if err != nil {
		t.Fatalf("storage.OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r, err := repos.New(db.DB)
	if err != nil {
		t.Fatalf("repos.New: %v", err)
	}
	registry := gitsession.NewRegistry(gitclient.NewExecRunner())
	t.Cleanup(registry.Close)
	h := &boardHarness{t: t, repos: r}
	h.files = repocfgtest.New(t, db.DB, filepath.Join(t.TempDir(), "repos"))
	r.AdeRepoConfig.Files = h.files
	h.status.Store(gitclient.GitStatus{Kind: "ok", Path: "git"})
	h.board = NewTaskBoard(TaskBoardDeps{
		Agent:       testAgents,
		Credentials: gitcred.New(),
		Tasks:       r.AdeTasks, Statuses: r.AdeStatuses, RepoConfig: r.AdeRepoConfig, Facts: r.AdeFacts, CodeRepos: r.CodeRepos, Runner: gitclient.NewExecRunner(),
		GitRepoSettings: h.files.Settings,
		Registry:        registry,
		GitStatus:       func(context.Context) gitclient.GitStatus { return h.status.Load().(gitclient.GitStatus) },
		Workflows:       &adeflow.Reader{Dir: t.TempDir(), Store: r.AdeTasks},
		OnBoard:         func() { h.emitted.Add(1) }, HomeDir: "/home/u",
		AutofetchMinutes: func() int { return 5 },
	})
	t.Cleanup(h.board.Close)
	h.files.OnChange = h.board.RepoConfigChanged
	return h
}

// setRepoFile edits the repo file of a code repo added with addRepoFromPath.
func (h *boardHarness) setRepoFile(id string, edit func(*repocfg.File)) {
	h.t.Helper()
	rec, err := h.repos.CodeRepos.Get(id)
	if err != nil || rec == nil {
		h.t.Fatalf("code repo %s: %v", id, err)
	}
	repocfgtest.Set(h.t, h.files, rec.RepoID, edit)
}

func (h *boardHarness) addRepo(id, root string) {
	h.t.Helper()
	if _, err := h.repos.CodeRepos.Create(model.CodeRepo{ID: id, Name: id, Root: root, RepoID: id}); err != nil {
		h.t.Fatalf("create code repo: %v", err)
	}
}

// addTask creates a task with one branch per spec; specs are {repo, name, kind, base, hadCommits}.
type branchSpec struct {
	id, repo, name, kind, base string
	hadCommits                 bool
}

func (h *boardHarness) addTask(id, kind string, specs ...branchSpec) {
	h.t.Helper()
	var bs []model.AdeTaskBranch
	for i, s := range specs {
		bs = append(bs, model.AdeTaskBranch{
			ID: s.id, TaskID: id, CodeRepoID: s.repo, Name: s.name, Kind: s.kind, Base: s.base,
			Position: i, HadCommits: s.hadCommits, AddedAt: 1,
		})
	}
	if _, err := h.repos.AdeTasks.CreateTask(model.AdeTask{ID: id, Kind: kind, Title: id, CreatedAt: 1}, bs, nil, nil); err != nil {
		h.t.Fatalf("create task %s: %v", id, err)
	}
}

func boardBranch(t *testing.T, b adewire.Board, id string) adewire.Branch {
	t.Helper()
	for _, br := range b.Branches {
		if br.ID == id {
			return br
		}
	}
	t.Fatalf("branch %s not on board", id)
	return adewire.Branch{}
}

func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	writeQueueFile(t, dir, name, content)
	runGitQueue(t, dir, "add", name)
	runGitQueue(t, dir, "commit", "-q", "-m", msg)
}

func TestTaskBoard_snapshotAssembly(t *testing.T) {
	skipWithoutGitQueue(t)
	h := newBoardHarness(t)
	_, dir := initQueueRepo(t)
	h.addRepo("r", dir)

	// feat-a and feat-p both rewrite shared.txt (a real pair conflict); feat-b stacks on feat-a;
	// their-x is another author's; feat-c builds on their-x; done-m is already inside main.
	commitFile(t, dir, "shared.txt", "one\n", "shared")
	runGitQueue(t, dir, "push", "-q", "origin", "main")
	runGitQueue(t, dir, "checkout", "-q", "-b", "feat-a", "main")
	commitFile(t, dir, "shared.txt", "a side\n", "a")
	runGitQueue(t, dir, "checkout", "-q", "-b", "feat-b", "feat-a")
	commitFile(t, dir, "b.txt", "b\n", "b")
	runGitQueue(t, dir, "checkout", "-q", "-b", "feat-p", "main")
	commitFile(t, dir, "shared.txt", "p side\n", "p")
	runGitQueue(t, dir, "checkout", "-q", "-b", "their-x", "main")
	writeQueueFile(t, dir, "x.txt", "x\n")
	runGitQueue(t, dir, "add", "x.txt")
	runGitQueueAs(t, dir, queueOtherName, queueOtherEmail, "commit", "-q", "-m", "x")
	runGitQueue(t, dir, "checkout", "-q", "-b", "feat-c", "their-x")
	commitFile(t, dir, "c.txt", "c\n", "c")
	runGitQueue(t, dir, "checkout", "-q", "-b", "done-m", "main")
	commitFile(t, dir, "m.txt", "m\n", "m")
	runGitQueue(t, dir, "checkout", "-q", "main")
	runGitQueue(t, dir, "merge", "-q", "--ff-only", "done-m")
	runGitQueue(t, dir, "push", "-q", "origin", "main")

	h.addTask("T1", "task", branchSpec{id: "b1", repo: "r", name: "feat-a", kind: "mine"})
	h.addTask("T2", "task", branchSpec{id: "b2", repo: "r", name: "feat-b", kind: "mine", base: "feat-a"})
	h.addTask("T3", "task", branchSpec{id: "b3", repo: "r", name: "feat-p", kind: "mine"})
	h.addTask("T4", "review", branchSpec{id: "b4", repo: "r", name: "their-x", kind: "review"})
	h.addTask("T5", "task", branchSpec{id: "b5", repo: "r", name: "feat-c", kind: "mine", base: "their-x"})
	h.addTask("T6", "task", branchSpec{id: "b6", repo: "r", name: "done-m", kind: "mine", hadCommits: true})

	board, err := h.board.Board(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Tasks) != 6 || board.WorktreeBasePath != "/home/u/wt" || board.AutofetchMinutes != 5 {
		t.Fatalf("board shell: %d tasks, %q, %d", len(board.Tasks), board.WorktreeBasePath, board.AutofetchMinutes)
	}
	if got := board.Repos[0]; got.MainName != "main" || got.Remote != "origin" {
		t.Fatalf("repo state %+v", got)
	}

	b1, b2, b5 := boardBranch(t, board, "b1"), boardBranch(t, board, "b2"), boardBranch(t, board, "b5")
	if b1.Base != "main" || b1.BaseBranchID != "" || b1.Ahead != 1 || b1.Owner != "" {
		t.Fatalf("b1 %+v", b1)
	}
	if b2.Base != "feat-a" || b2.BaseBranchID != "b1" || b2.BaseOwner != "" || b2.Ahead != 1 || b2.MergedIntoMain {
		t.Fatalf("b2 stacked on my branch: %+v", b2)
	}
	if b5.BaseBranchID != "b4" || b5.BaseOwner != queueOtherName {
		t.Fatalf("b5 base on someone else's branch: id %q owner %q", b5.BaseBranchID, b5.BaseOwner)
	}
	if b4 := boardBranch(t, board, "b4"); b4.Owner != queueOtherName {
		t.Fatalf("b4 owner %q", b4.Owner)
	}
	if b6 := boardBranch(t, board, "b6"); !b6.MergedIntoMain || b6.MergedAt == nil {
		t.Fatalf("b6 should be merged via ancestor: %+v", b6)
	}
	// b6 merged: no check; b1 unseen: checking.
	if b6 := boardBranch(t, board, "b6"); b6.ConflictCheck != conflictDone || len(b6.ConflictsIfRebased) != 0 {
		t.Fatalf("merged branch check state %+v", b6)
	}
	if b1.ConflictCheck != conflictChecking {
		t.Fatalf("first Board must not block on the check: %q", b1.ConflictCheck)
	}

	// pairs are keyed by branch id: b1 (feat-a) and b3 (feat-p) both edit shared.txt.
	var found bool
	for _, p := range board.Pairs {
		if p.A == "b1" && p.B == "b3" {
			found = slices.Equal(p.Conflicts, []string{"shared.txt"})
		}
	}
	if !found {
		t.Fatalf("pair b1/b3 with conflict missing: %+v", board.Pairs)
	}
	stored, _ := h.repos.AdeTasks.BranchesLive()
	for _, s := range stored {
		if s.ID == "b6" && s.MergedAt == nil {
			t.Fatalf("merged write-back missing: %+v", s)
		}
	}

	// main moves with a conflicting edit; Refresh settles the checks before returning.
	commitFile(t, dir, "shared.txt", "main moved\n", "main moves")
	runGitQueue(t, dir, "push", "-q", "origin", "main")
	if _, err := h.board.Refresh(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	board, _ = h.board.Board(context.Background())
	b1, b2 = boardBranch(t, board, "b1"), boardBranch(t, board, "b2")
	if b1.ConflictCheck != conflictDone || !slices.Equal(b1.ConflictsIfRebased, []string{"shared.txt"}) {
		t.Fatalf("b1 after refresh: %q %v (%s)", b1.ConflictCheck, b1.ConflictsIfRebased, b1.ConflictCheckReason)
	}
	// b2 checks against feat-a's tip, not main: stacked branch has no conflict with its base.
	if b2.ConflictCheck != conflictDone || len(b2.ConflictsIfRebased) != 0 {
		t.Fatalf("b2 after refresh: %q %v (%s)", b2.ConflictCheck, b2.ConflictsIfRebased, b2.ConflictCheckReason)
	}
	if h.emitted.Load() == 0 {
		t.Fatal("Refresh did not emit a board change")
	}
}

func TestTaskBoard_notCreatedAndTooOldGit(t *testing.T) {
	skipWithoutGitQueue(t)
	h := newBoardHarness(t)
	_, dir := initQueueRepo(t)
	h.addRepo("r", dir)
	runGitQueue(t, dir, "checkout", "-q", "-b", "feat", "main")
	commitFile(t, dir, "f.txt", "f\n", "f")
	h.addTask("T1", "task", branchSpec{id: "b1", repo: "r", name: "feat", kind: "mine"})
	h.addTask("T2", "task", branchSpec{id: "b2", repo: "r", name: "", kind: "mine"})

	// Opens (and caches) the repo while git is still ok; a later tooOld only fails the checks.
	if _, err := h.board.openRepo(context.Background(), "r"); err != nil {
		t.Fatal(err)
	}
	h.status.Store(gitclient.GitStatus{Kind: "tooOld", Detected: "2.30.0", Required: "2.38.0"})
	board, err := h.board.Board(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b2 := boardBranch(t, board, "b2")
	if b2.Name != "" || b2.Tip != "" || b2.Base != "main" || b2.ConflictCheck != conflictDone || b2.Dirty == nil {
		t.Fatalf("not-created branch %+v", b2)
	}

	if _, err := h.board.Refresh(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	board, _ = h.board.Board(context.Background())
	b1 := boardBranch(t, board, "b1")
	if b1.ConflictCheck != conflictFailed || b1.ConflictCheckReason == "" || len(b1.ConflictsIfRebased) != 0 {
		t.Fatalf("tooOld git: %+v", b1)
	}
}

// addRepoFromPath imports a real checkout the way the Git module would.
func (h *boardHarness) addRepoFromPath(id, root string) {
	h.t.Helper()
	sum, err := gitclient.Identify(context.Background(), gitclient.NewExecRunner(), "git", root)
	if err != nil {
		h.t.Fatalf("identify: %v", err)
	}
	if _, err := h.repos.CodeRepos.Create(model.CodeRepo{ID: id, Name: id, Root: sum.Root, RepoID: sum.RepoID}); err != nil {
		h.t.Fatalf("create code repo: %v", err)
	}
}
