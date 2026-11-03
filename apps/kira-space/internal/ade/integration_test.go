package ade

import (
	"context"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitcred"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adeflow"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg/repocfgtest"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

// countingRunner counts git spawns by subcommand.
type countingRunner struct {
	gitclient.Runner
	cherry, patchID atomic.Int32
}

func (r *countingRunner) Start(ctx context.Context, gitPath string, spec gitclient.Spec) (gitclient.Process, error) {
	switch spec.Args[0] {
	case "cherry":
		r.cherry.Add(1)
	case "patch-id":
		r.patchID.Add(1)
	}
	return r.Runner.Start(ctx, gitPath, spec)
}

type integFixture struct {
	*boardHarness
	runner *countingRunner
	dir    string
	origin string
}

func newIntegFixture(t *testing.T) *integFixture {
	t.Helper()
	skipWithoutGitQueue(t)
	db, err := storage.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r, err := repos.New(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	cr := &countingRunner{Runner: gitclient.NewExecRunner()}
	registry := gitsession.NewRegistry(cr)
	t.Cleanup(registry.Close)
	h := &boardHarness{t: t, repos: r}
	h.files = repocfgtest.New(t, db.DB, filepath.Join(t.TempDir(), "repos"))
	r.AdeRepoConfig.Files = h.files
	h.status.Store(gitclient.GitStatus{Kind: "ok", Path: "git"})
	h.board = NewTaskBoard(TaskBoardDeps{
		Agent:       testAgents,
		Credentials: gitcred.New(),
		Tasks:       r.AdeTasks, Statuses: r.AdeStatuses, RepoConfig: r.AdeRepoConfig, Facts: r.AdeFacts, CodeRepos: r.CodeRepos,
		GitRepoSettings: h.files.Settings,
		Runner:          cr, Registry: registry,
		GitStatus: func(context.Context) gitclient.GitStatus { return h.status.Load().(gitclient.GitStatus) },
		Workflows: &adeflow.Reader{Dir: t.TempDir(), Store: r.AdeTasks},
		OnBoard:   func() { h.emitted.Add(1) }, HomeDir: "/home/u",
		AutofetchMinutes: func() int { return 5 },
	})
	t.Cleanup(h.board.Close)
	h.files.OnChange = h.board.RepoConfigChanged

	origin, dir := initQueueRepo(t)
	h.addRepoFromPath("r", dir)
	runGitQueue(t, dir, "checkout", "-q", "-b", "develop", "main")
	runGitQueue(t, dir, "push", "-q", "-u", "origin", "develop")
	runGitQueue(t, dir, "checkout", "-q", "main")
	f := &integFixture{boardHarness: h, runner: cr, dir: dir, origin: origin}
	h.setRepoFile("r", func(rf *repocfg.File) { rf.IntegrationBranches = []string{"develop"} })
	return f
}

// feature creates a branch off main with one commit per file name and tracks it as a task.
func (f *integFixture) feature(name string, files ...string) {
	f.t.Helper()
	runGitQueue(f.t, f.dir, "checkout", "-q", "-b", name, "main")
	for _, file := range files {
		commitFile(f.t, f.dir, file, name+file+"\n", name+" "+file)
	}
	runGitQueue(f.t, f.dir, "checkout", "-q", "main")
	f.addTask("T-"+name, "task", branchSpec{id: "b-" + name, repo: "r", name: name, kind: "mine"})
}

func (f *integFixture) onDevelop(args ...string) {
	f.t.Helper()
	runGitQueue(f.t, f.dir, "checkout", "-q", "develop")
	for _, a := range args {
		runGitQueue(f.t, f.dir, strings.Fields(a)...)
	}
	runGitQueue(f.t, f.dir, "push", "-q", "-f", "origin", "develop")
	runGitQueue(f.t, f.dir, "checkout", "-q", "main")
}

func (f *integFixture) integration(name string) adewire.Integration {
	f.t.Helper()
	b, err := f.board.Board(context.Background())
	if err != nil {
		f.t.Fatalf("Board: %v", err)
	}
	in := boardBranch(f.t, b, "b-"+name).Integration
	if len(in) != 1 || in[0].Target != "develop" {
		f.t.Fatalf("%s integration = %+v", name, in)
	}
	return in[0]
}

func (f *integFixture) expect(name, status, note string) {
	f.t.Helper()
	got := f.integration(name)
	if got.Status != status || got.Note != note {
		f.t.Fatalf("%s: got %s %q, want %s %q", name, got.Status, got.Note, status, note)
	}
}

func TestIntegration_mergeShapes(t *testing.T) {
	f := newIntegFixture(t)
	f.feature("not-yet", "a.txt")
	f.feature("ff", "f.txt")
	f.feature("mc", "m.txt")
	f.feature("sq", "s1.txt", "s2.txt")
	f.feature("cp", "c1.txt", "c2.txt")
	f.onDevelop("merge -q --ff-only ff", "merge -q --no-ff -m mc mc", "merge -q --squash sq", "commit -q -m squash-sq",
		"cherry-pick cp~1")

	f.expect("not-yet", "not merged", "")
	f.expect("ff", "merged", "up to date")
	f.expect("mc", "merged", "up to date")
	f.expect("sq", "merged", "up to date")
	f.expect("cp", "stale", "1 commit since it was merged")
}

func TestIntegration_staleAfterMarks(t *testing.T) {
	f := newIntegFixture(t)
	f.feature("nc", "n1.txt")
	f.feature("rb", "r1.txt")
	f.feature("rv", "v1.txt")
	f.onDevelop("merge -q --ff-only nc", "merge -q --no-ff -m rb rb", "merge -q --no-ff -m rv rv")
	f.expect("nc", "merged", "up to date")
	f.expect("rb", "merged", "up to date")
	f.expect("rv", "merged", "up to date")

	runGitQueue(t, f.dir, "checkout", "-q", "nc")
	commitFile(t, f.dir, "n2.txt", "more\n", "more")
	runGitQueue(t, f.dir, "checkout", "-q", "main")
	f.expect("nc", "stale", "1 commit since it was merged")

	commitFile(t, f.dir, "main2.txt", "m\n", "main moves")
	runGitQueue(t, f.dir, "push", "-q", "origin", "main")
	runGitQueue(t, f.dir, "checkout", "-q", "rb")
	runGitQueue(t, f.dir, "rebase", "-q", "main")
	runGitQueue(t, f.dir, "checkout", "-q", "main")
	f.expect("rb", "stale", "rebased since it was merged")

	f.onDevelop("reset -q --hard main~1")
	f.expect("rv", "stale", "no longer in develop")
}

func TestIntegration_recordMerge(t *testing.T) {
	f := newIntegFixture(t)
	f.feature("rec", "r.txt")
	ctx := context.Background()
	if err := f.board.RecordMerge(ctx, "b-rec", "staging"); err == nil {
		t.Fatal("unconfigured target accepted")
	}
	if err := f.board.RecordMerge(ctx, "b-rec", "develop"); err == nil {
		t.Fatal("merge recorded while the target lacks the tip")
	}
	f.onDevelop("merge -q --ff-only rec")
	if err := f.board.RecordMerge(ctx, "b-rec", "develop"); err != nil {
		t.Fatalf("RecordMerge: %v", err)
	}
	got := f.integration("rec")
	if got.Status != "merged" || !got.Recorded {
		t.Fatalf("recorded then merged: %+v", got)
	}
	f.onDevelop("reset -q --hard main")
	got = f.integration("rec")
	if got.Status != "stale" || !got.Recorded || got.Note != "no longer in develop" {
		t.Fatalf("recorded merge dropped from target: %+v", got)
	}
}

func TestIntegration_cacheAndRefreshMergedInto(t *testing.T) {
	f := newIntegFixture(t)
	f.feature("sq", "s1.txt", "s2.txt")
	f.expect("sq", "not merged", "")
	cherry, patchID := f.runner.cherry.Load(), f.runner.patchID.Load()
	f.expect("sq", "not merged", "")
	if f.runner.cherry.Load() != cherry || f.runner.patchID.Load() != patchID {
		t.Fatal("unchanged shas spawned git cherry/patch-id again")
	}

	// Another clone squash-merges the pushed branch into develop; Refresh reports it once.
	runGitQueue(t, f.dir, "push", "-q", "origin", "sq")
	other := t.TempDir()
	runGitQueue(t, other, "clone", "-q", f.origin, ".")
	runGitQueue(t, other, "checkout", "-q", "develop")
	runGitQueue(t, other, "merge", "-q", "--squash", "origin/sq")
	runGitQueue(t, other, "commit", "-q", "-m", "squash")
	runGitQueue(t, other, "push", "-q", "origin", "develop")

	res, err := f.board.Refresh(context.Background(), []string{"r"})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Repos[0].MergedInto; len(got) != 1 || got[0].BranchID != "b-sq" || got[0].Target != "develop" {
		t.Fatalf("mergedInto = %+v", got)
	}
	f.expect("sq", "merged", "up to date")
	res, err = f.board.Refresh(context.Background(), []string{"r"})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Repos[0].MergedInto; len(got) != 0 {
		t.Fatalf("second refresh mergedInto = %+v", got)
	}
}

func (f *integFixture) setEnv(script string) {
	f.t.Helper()
	f.setRepoFile("r", func(rf *repocfg.File) {
		rf.Environments = []repocfg.Environment{{Name: "staging", DeployedSHAScript: script}}
	})
	if err := f.board.RunEnvScripts(context.Background(), "r"); err != nil {
		f.t.Fatalf("RunEnvScripts: %v", err)
	}
}

func (f *integFixture) deployment(name string) adewire.Deployment {
	f.t.Helper()
	b, err := f.board.Board(context.Background())
	if err != nil {
		f.t.Fatalf("Board: %v", err)
	}
	d := boardBranch(f.t, b, "b-"+name).Deployments
	if len(d) != 1 || d[0].Env != "staging" {
		f.t.Fatalf("%s deployments = %+v", name, d)
	}
	return d[0]
}

func TestDeployment_scriptStates(t *testing.T) {
	f := newIntegFixture(t)
	f.feature("dep", "d1.txt", "d2.txt")
	sha := func(rev string) string { return strings.TrimSpace(runGitQueue(t, f.dir, "rev-parse", rev)) }
	tip, first, mainTip := sha("dep"), sha("dep~1"), sha("main")

	f.setEnv("echo noise; echo " + tip)
	if d := f.deployment("dep"); d.Status != "deployed" || d.DeployedSha != tip {
		t.Fatalf("deployed: %+v", d)
	}

	f.setEnv("echo " + first[:10])
	if d := f.deployment("dep"); d.Status != "stale" || d.MissingCommits != 1 {
		t.Fatalf("partial: %+v", d)
	}

	f.setEnv("echo " + mainTip)
	if d := f.deployment("dep"); d.Status != "stale" || d.MissingCommits != 2 {
		t.Fatalf("none of the branch: %+v", d)
	}

	f.setEnv("echo " + tip)
	f.setEnv("echo " + first)
	if d := f.deployment("dep"); d.Status != "stale" || !strings.Contains(d.Note, "moved back to") {
		t.Fatalf("moved back: %+v", d)
	}

	f.setEnv("exit 3")
	if d := f.deployment("dep"); d.Status != "unknown" || d.Error == "" || d.DeployedSha != first {
		t.Fatalf("script failure keeps last sha: %+v", d)
	}

	f.setEnv("echo " + strings.Repeat("a", 40))
	if d := f.deployment("dep"); d.Status != "unknown" || !strings.Contains(d.Error, "not in this clone") {
		t.Fatalf("unknown object: %+v", d)
	}
}
