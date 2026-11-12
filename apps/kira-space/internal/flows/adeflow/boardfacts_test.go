package adeflow_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func TestFolderImportAndFacts(t *testing.T) {
	app := flowharness.New(t)
	folder := filepath.Join(app.Work, "folder")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	remote, _ := cloneWithMain(app, filepath.Join(folder, "withremote"))
	flowharness.NewRepo(t, filepath.Join(folder, "noremote")).Commit("base", map[string]string{"a.txt": "a\n"})
	remote.LinkedWorktree("side")

	res, err := app.W.AdeTask.AddFolder(ctx, adewire.FolderArgs{Path: folder})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Imported) != 2 || res.Folder.RepoCount != 2 || res.Folder.Watch {
		t.Fatalf("AddFolder = %+v, want the two real repos imported, the linked worktree skipped", res)
	}
	repos, err := app.W.AdeTask.Repos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, r := range repos.Repos {
		if r.Source != folder {
			t.Fatalf("repo %s source = %q, want %q", r.Name, r.Source, folder)
		}
		ids[r.Name] = r.CodeRepoID
	}
	if len(ids) != 2 || ids["withremote"] == "" || ids["noremote"] == "" {
		t.Fatalf("ADE repos = %v, want withremote and noremote", ids)
	}

	createTask(t, app, "facts", "", ids["withremote"], ids["noremote"])
	b := board(t, app)
	with, ok1 := repoState(b, ids["withremote"])
	without, ok2 := repoState(b, ids["noremote"])
	if !ok1 || !ok2 {
		t.Fatalf("board repos = %+v, want both", b.Repos)
	}
	if with.Remote != "origin" || with.MainName != "main" || without.Remote != "" {
		t.Fatalf("remote facts = %+v / %+v, want origin and none", with, without)
	}
	if with.LastFetchAt != nil {
		t.Fatalf("lastFetchAt = %d before any fetch", *with.LastFetchAt)
	}
	remote.Git("fetch", "-q")
	// An out-of-band fetch reaches the board through the repo watcher, not a board write.
	testx.WaitUntil(t, waitFor, func() bool {
		with, _ = repoState(board(t, app), ids["withremote"])
		return with.LastFetchAt != nil
	})

	if _, err := app.W.AdeTask.SetFolderWatch(ctx, adewire.FolderArgs{Path: folder, Watch: true}); err != nil {
		t.Fatal(err)
	}
	mark := app.Events.Mark()
	flowharness.NewRepo(t, filepath.Join(folder, "late")).Commit("late", map[string]string{"l.txt": "l\n"})
	app.Events.WaitAfter(t, mark, adewire.ChannelRepos, nil, waitFor)
	repos, _ = app.W.AdeTask.Repos(ctx)
	if len(repos.Repos) != 3 {
		t.Fatalf("watched folder picked up %d repos, want 3 with the late one", len(repos.Repos))
	}
	if err := app.W.AdeTask.RemoveFolder(ctx, adewire.PathArgs{Path: folder}); err != nil {
		t.Fatal(err)
	}
	repos, _ = app.W.AdeTask.Repos(ctx)
	if len(repos.Folders) != 0 || len(repos.Repos) != 3 {
		t.Fatalf("after RemoveFolder: folders=%d repos=%d, want 0 folders and the 3 repos kept", len(repos.Folders), len(repos.Repos))
	}
}

// Default settings: git.gitPath is empty, so every call must resolve git through Discovery (P230).
func TestRefreshDefaultSettings(t *testing.T) {
	app := flowharness.New(t)

	withRemote, bare := cloneWithMain(app, filepath.Join(app.Work, "tracked"))
	withRemote.Git("checkout", "-q", "-b", "feat")
	withRemote.Commit("feat 1", map[string]string{"f.txt": "1\n"})
	withRemote.Git("push", "-q", "-u", "origin", "feat")
	withRemote.Checkout("main")

	other := bare.Clone(filepath.Join(app.Work, "other"))
	other.Commit("main 2", map[string]string{"m.txt": "2\n"})
	other.Git("push", "-q", "origin", "main")

	noRemote := app.NewRepo("noremote")
	noRemote.Commit("base", map[string]string{"a.txt": "a\n"})
	noRemote.Branch("feat", "")

	bad := app.NewRepo("badremote")
	bad.Commit("base", map[string]string{"a.txt": "a\n"})
	bad.Branch("feat", "")
	bad.BadRemote()

	wtHost, _ := cloneWithMain(app, filepath.Join(app.Work, "wthost"))
	wtRoot := wtHost.LinkedWorktree("wt-feat")

	recs := map[string]string{}
	for name, dir := range map[string]string{"tracked": withRemote.Dir, "noremote": noRemote.Dir, "badremote": bad.Dir, "wtroot": wtRoot.Dir} {
		recs[name] = importRepo(t, app, dir).ID
	}
	branch := map[string]string{"tracked": "feat", "noremote": "feat", "badremote": "feat", "wtroot": "wt-feat"}
	for name, id := range recs {
		if _, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: id, Name: branch[name]}); err != nil {
			t.Fatalf("AddExistingBranch %s: %v", name, err)
		}
	}

	// Contract ade-board: tests/ui/ade-v2-plan.spec.ts "contract: Refresh all ..." reads these.
	before := board(t, app)
	for _, name := range []string{"tracked", "noremote"} {
		st, _ := repoState(before, recs[name])
		app.Contract(t, "ade-board", "AdeTaskService.Board#repo-"+name+"-before", st)
	}
	res, err := app.W.AdeTask.Refresh(ctx, adewire.RefreshArgs{})
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]adewire.RepoRefresh{}
	for _, r := range res.Repos {
		rows[r.CodeRepoID] = r
	}
	for _, name := range []string{"tracked", "noremote"} {
		app.Contract(t, "ade-board", "AdeTaskService.Refresh#"+name, rows[recs[name]])
		st, _ := repoState(board(t, app), recs[name])
		app.Contract(t, "ade-board", "AdeTaskService.Board#repo-"+name+"-after", st, flowharness.Mask("lastFetchAt"))
	}
	if len(rows) != 4 {
		t.Fatalf("refresh-all answered %d repos, want 4", len(rows))
	}
	if r := rows[recs["tracked"]]; r.Error != nil || r.RefsChanged != 1 {
		t.Fatalf("tracked refresh = %+v, want no error and main reported changed after the push from another clone", r)
	}
	if r := rows[recs["noremote"]]; r.Error != nil || r.RefsChanged != 0 {
		t.Fatalf("no-remote refresh = %+v, want no error", r)
	}
	r := rows[recs["badremote"]]
	if r.Error == nil || r.Error.Kind == "" || r.Error.Kind == "Unknown" {
		t.Fatalf("bad-remote refresh = %+v, want a classified error", r)
	}
	if r := rows[recs["wtroot"]]; r.Error != nil {
		t.Fatalf("linked-worktree-root refresh = %+v, want no error", r)
	}

	b := board(t, app)
	if st, _ := repoState(b, recs["wtroot"]); st.LastFetchAt == nil {
		t.Fatal("linked-worktree root has no lastFetchAt after refresh")
	}
	if st, _ := repoState(b, recs["tracked"]); st.Remote != "origin" || st.LastFetchAt == nil {
		t.Fatalf("tracked repo state = %+v, want origin and a fetch time", st)
	}
	if st, _ := repoState(b, recs["noremote"]); st.Remote != "" || st.LastFetchAt != nil {
		t.Fatalf("no-remote repo state = %+v, want no remote and no fetch time", st)
	}
	if _, err := os.Stat(filepath.Join(noRemote.Dir, ".git", "FETCH_HEAD")); err == nil {
		t.Fatal("no-remote refresh ran a fetch")
	}
}

func TestTaskBranches(t *testing.T) {
	app := flowharness.New(t)
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "proj"))
	repo.Branch("local-topic", "")
	repo.Git("checkout", "-q", "-b", "remote-topic")
	repo.Commit("remote only", map[string]string{"r.txt": "r\n"})
	repo.Git("push", "-q", "origin", "remote-topic")
	repo.Checkout("main")
	repo.Git("branch", "-q", "-D", "remote-topic")
	repo.Git("fetch", "-q")
	rec := importRepo(t, app, repo.Dir)

	cands, err := app.W.AdeTask.CandidateBranches(ctx, adewire.CandidateBranchesArgs{})
	if err != nil {
		t.Fatal(err)
	}
	remoteOnly := map[string]bool{}
	for _, c := range cands.Branches {
		remoteOnly[c.Name] = c.RemoteOnly
	}
	if len(remoteOnly) != 2 || remoteOnly["local-topic"] || !remoteOnly["remote-topic"] {
		t.Fatalf("candidates = %v, want local-topic (local) and remote-topic (remote only), no main", remoteOnly)
	}

	// A local branch nobody has checked out becomes a task branch in its own real worktree.
	added, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: rec.ID, Name: "local-topic"})
	if err != nil {
		t.Fatal(err)
	}
	wt := branchOf(t, board(t, app), added.Task.ID, rec.ID).Worktree
	if wt == "" {
		t.Fatal("existing branch has no worktree on the board")
	}
	if got := repo.Git("worktree", "list", "--porcelain"); !strings.Contains(got, "worktree "+wt) {
		t.Fatalf("git worktree list lacks %s:\n%s", wt, got)
	}
	if gitOut(t, wt, "rev-parse", "--abbrev-ref", "HEAD") != "local-topic" {
		t.Fatal("worktree is not on local-topic")
	}

	t.Run("remote-only branch", func(t *testing.T) {
		added, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: rec.ID, Name: "remote-topic"})
		if err != nil {
			t.Fatal(err)
		}
		if added.Task.Kind != "task" {
			t.Fatalf("remote-only branch by the user's own author = %+v, want a plain task", added.Task)
		}
		br := branchOf(t, board(t, app), added.Task.ID, rec.ID)
		if br.Worktree == "" {
			t.Fatalf("remote-only branch has no worktree, setup = %+v", br.Setup)
		}
	})

	// A new branch: the not-created branch of a second task gets its worktree on StartRun.
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
	task := createTask(t, app, "Ship it", "flow", rec.ID)
	br := branchOf(t, board(t, app), task.ID, rec.ID)
	if br.Name != "" || br.Worktree != "" {
		t.Fatalf("not-created branch = %+v, want no name or worktree yet", br)
	}
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/ship-it"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, task.ID, "one", "done")
	b := board(t, app)
	br = branchOf(t, b, task.ID, rec.ID)
	if br.Name != "feat/ship-it" || br.Worktree == "" {
		t.Fatalf("started branch = %+v, want feat/ship-it with a worktree", br)
	}
	if got := repo.Git("worktree", "list", "--porcelain"); !strings.Contains(got, "worktree "+br.Worktree) {
		t.Fatalf("git worktree list lacks %s", br.Worktree)
	}

	// Ahead/behind against base equals git's own count.
	commitIn(t, br.Worktree, "ahead.txt")
	repo.Commit("main moves", map[string]string{"m.txt": "m\n"})
	repo.Git("push", "-q", "origin", "main")
	if _, err := app.W.AdeTask.Refresh(ctx, adewire.RefreshArgs{CodeRepoIDs: []string{rec.ID}}); err != nil {
		t.Fatal(err)
	}
	br = branchOf(t, board(t, app), task.ID, rec.ID)
	counts := strings.Fields(gitOut(t, repo.Dir, "rev-list", "--left-right", "--count", "feat/ship-it...main"))
	if counts[0] != strconv.Itoa(br.Ahead) || counts[1] != strconv.Itoa(br.Behind) || br.Ahead == 0 || br.Behind == 0 {
		t.Fatalf("board ahead/behind = %d/%d, git = %v", br.Ahead, br.Behind, counts)
	}
}
