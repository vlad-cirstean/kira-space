package githostflow_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

var ctx = context.Background()

const (
	waitFor = 20 * time.Second
	branch  = "feat/forge"
)

// forgeCR is merge request 7 on branch, owned by the acme repo.
var forgeCR = githost.ChangeRequest{
	Number: 7, Title: "Fix login", State: "open", HeadRef: branch, HeadOwner: "acme", BaseRef: "main",
	URL: "https://forge.test/acme/r/-/merge_requests/7",
}

type rig struct {
	t      *testing.T
	app    *flowharness.App
	forge  *fakehost.Forge
	branch adewire.Branch
	repoID string
	dir    string
	tip    string
	gs     *flowharness.GitStream
	gitID  string
}

// newRig boots an app with forge registered after GitHub and one ADE task branch of an acme/r repo
// whose origin lives on forge.test. Merge request 7 is registered on the branch before any lookup.
func newRig(t *testing.T, forge *fakehost.Forge, opts ...flowharness.Opt) *rig {
	t.Helper()
	forge.AddChangeRequest(forgeCR)
	app := flowharness.New(t, append(opts, flowharness.WithGitHost(forge), flowharness.WithoutGh())...)
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{
		"*": {{Name: "done", Sh: "printf 'a\\nb\\n' > a.txt && git add -A && git commit -q -m add-a"}},
	}})

	dir := filepath.Join(app.Work, "r")
	bare := app.NewBare("r-remote")
	seed := app.NewRepo("r-seed")
	seed.Commit("base", map[string]string{"base.txt": "base\n"})
	bare.PushFrom(seed)
	bare.Clone(dir)
	git(t, dir, "remote", "set-url", "origin", "https://forge.test/acme/r.git")

	rec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	flow := "id: flow\nname: Flow\nsteps:\n  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: work on {branch} in {repo}\n"
	if entry, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "flow.yaml", Yaml: flow}); err != nil || entry.Error != nil {
		t.Fatalf("SaveWorkflowYaml: %+v, %v", entry, err)
	}
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	br := branchIn(t, app, task.ID, rec.ID)
	// finish_step lists the task's artifacts through the host; keep that out of the lookups under test.
	forge.SetOffline(true)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: branch}}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		b, err := app.W.AdeTask.Board(ctx)
		if err != nil {
			return false
		}
		for _, tk := range b.Tasks {
			for _, r := range tk.Runs {
				if r.StepID == "one" && r.State == "done" {
					return true
				}
			}
		}
		return false
	})
	forge.SetOffline(false)
	forge.ResetCalls()
	br = branchIn(t, app, task.ID, rec.ID)
	tip := git(t, br.Worktree, "rev-parse", "HEAD")
	forge.SetHead(7, tip)

	gs := app.OpenGitStream()
	var opened gitrpc.RepoOpenResult
	gs.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: dir}, &opened)
	if opened.Kind != "ok" || opened.Repo == nil {
		t.Fatalf("repo.open = %+v", opened)
	}
	// The agent committed in a worktree; the entry's cached refs follow via the async watcher.
	testx.WaitUntil(t, waitFor, func() bool {
		refs := call[gitsession.RefsResult](t, gs, "refs.list", gitrpc.RefsListParams{RepoID: opened.Repo.RepoID})
		return slices.ContainsFunc(refs.Branches, func(b porcelain.RefRow) bool { return b.ShortName == branch && b.ObjectID == tip })
	})
	return &rig{t: t, app: app, forge: forge, branch: br, repoID: rec.ID, dir: dir, tip: tip, gs: gs, gitID: opened.Repo.RepoID}
}

func branchIn(t *testing.T, app *flowharness.App, taskID, repoID string) adewire.Branch {
	t.Helper()
	b, err := app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, br := range b.Branches {
		if br.TaskID == taskID && br.CodeRepoID == repoID {
			return br
		}
	}
	t.Fatalf("board has no branch of task %s", taskID)
	return adewire.Branch{}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func call[T any](t *testing.T, gs *flowharness.GitStream, method string, params any) T {
	t.Helper()
	var out T
	gs.MustRequest(method, params, &out)
	return out
}

func (r *rig) setProvider(value string) {
	r.t.Helper()
	call[gitrpc.RepoSettingsSnapshot](r.t, r.gs, "repoSettings.set", gitrpc.RepoSettingsSetParams{
		RepoID: r.gitID, Patch: gitrpc.RepoSettingsPatchWire{GitHostProvider: &value},
	})
}

func (r *rig) hostInfo() *githost.Info {
	r.t.Helper()
	return call[gitrpc.GitHostInfoResult](r.t, r.gs, "gitHost.info", gitrpc.GitHostInfoParams{RepoID: r.gitID}).Host
}

func (r *rig) branchLookup() gitrpc.ChangeRequestLookup {
	r.t.Helper()
	return call[gitrpc.ChangeRequestLookup](r.t, r.gs, "branch.resolveChangeRequest",
		gitrpc.BranchResolveChangeRequestParams{RepoID: r.gitID, Branch: branch})
}

// A second host registered next to GitHub drives lookups, links and URLs with its own
// words, and no step reaches `gh` (the app boots without it).
func TestSecondGitHostRegistered(t *testing.T) {
	forge := fakehost.New()
	r := newRig(t, forge)

	list := call[githost.Providers](t, r.gs, "gitHost.providers", struct{}{})
	if len(list.Providers) != 3 || list.Providers[0].ID != "github" || list.Providers[1].ID != "azure" ||
		list.Providers[2].ID != "forge" || list.Providers[2].ChangeRequestShort != "MR" {
		t.Fatalf("gitHost.providers = %+v, want github, azure, forge", list)
	}
	if got := r.app.W.GitHost.Providers(); len(got.Providers) != 3 {
		t.Fatalf("GitHostService.Providers = %+v, want 3", got)
	}

	if info := r.hostInfo(); info == nil || info.ID != "forge" || info.Label != "Forge" || info.ChangeRequestNoun != "merge request" {
		t.Fatalf("gitHost.info = %+v, want forge", info)
	}

	// A branch lookup and a commit lookup find merge request 7; repeats read the cache.
	for range 2 {
		if lk := r.branchLookup(); lk.Kind != "ok" || len(lk.ChangeRequests) != 1 || lk.ChangeRequests[0].Number != 7 {
			t.Fatalf("branch.resolveChangeRequest = %+v, want MR 7", lk)
		}
		lk := call[gitrpc.ChangeRequestLookup](t, r.gs, "commit.resolveChangeRequests", gitrpc.CommitResolveChangeRequestsParams{RepoID: r.gitID, SHA: r.tip})
		if lk.Kind != "ok" || len(lk.ChangeRequests) != 1 || lk.ChangeRequests[0].Number != 7 {
			t.Fatalf("commit.resolveChangeRequests = %+v, want MR 7", lk)
		}
	}
	if n := forge.Count("Open"); n != 1 {
		t.Fatalf("host Open calls = %d, want 1 (snapshot cached)", n)
	}
	if n := forge.Count("ForCommit:"); n != 1 {
		t.Fatalf("host ForCommit calls = %d, want 1", n)
	}

	// Browser links come from the provider, and only its own merge request URLs open.
	want := "https://forge.test/acme/r/-/merge_requests/7"
	if u := call[gitrpc.ChangeRequestBrowserUrlResult](t, r.gs, "changeRequest.browserUrl", gitrpc.ChangeRequestBrowserUrlParams{RepoID: r.gitID, Number: 7}); u.URL == nil || *u.URL != want {
		t.Fatalf("changeRequest.browserUrl = %v, want %s", u.URL, want)
	}
	if err := r.app.W.GitHost.OpenChangeRequestURL(bridge.OpenChangeRequestURLArgs{URL: want}); err != nil {
		t.Fatal(err)
	}
	if err := r.app.W.GitHost.OpenChangeRequestURL(bridge.OpenChangeRequestURLArgs{URL: "https://forge.test/acme/r/pull/7"}); err == nil {
		t.Fatal("a /pull/ URL on the forge host opened")
	}
	if got := r.app.Browser.Opened(); !slices.Equal(got, []string{want}) {
		t.Fatalf("browser opened %v, want only %s", got, want)
	}

	// The ADE board names the host, the branch's merge request and the branch page.
	crs, err := r.app.W.AdeTask.ChangeRequests(ctx)
	if err != nil || len(crs.Repos) != 1 || crs.Repos[0].Kind != "ok" || crs.Repos[0].Host == nil || crs.Repos[0].Host.ID != "forge" {
		t.Fatalf("ChangeRequests = %+v, %v, want ok on forge", crs, err)
	}
	if cr := crs.Branches[r.branch.ID]; cr.Number != 7 || cr.State != "open" || cr.URL != want {
		t.Fatalf("branch change request = %+v, want MR 7", cr)
	}
	if u := crs.BranchURLs[r.branch.ID]; u != "https://forge.test/acme/r/-/tree/"+branch {
		t.Fatalf("BranchURLs = %v, want the forge tree URL", crs.BranchURLs)
	}

	// Task links: a merge request link is recognised, a GitHub or unknown link is refused.
	task := r.taskID()
	link := want
	if _, err := r.app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: task, Patch: adewire.TaskPatch{HostURL: &link}}); err != nil {
		t.Fatal(err)
	}
	b, err := r.app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := taskOf(b, task); got.HostLink == nil || *got.HostLink != (githost.Link{Provider: "forge", Kind: "changeRequest", Ref: "r!7"}) {
		t.Fatalf("task hostLink = %+v, want forge MR r!7", got.HostLink)
	}
	for _, bad := range []string{"https://example.com/acme/r/-/merge_requests/7", "https://forge.test/acme/r/pull/7"} {
		_, err := r.app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: task, Patch: adewire.TaskPatch{HostURL: &bad}})
		if err == nil || !strings.Contains(err.Error(), "Not a GitHub PR or issue link or Azure Repos PR or issue link or Forge MR or issue link.") {
			t.Fatalf("UpdateTask hostUrl %q = %v, want the not-a-link error naming both hosts", bad, err)
		}
	}

	// Settings: off disables lookups without a host call, a provider id pins it, an unknown id falls back to auto.
	before := len(forge.Calls())
	r.setProvider("off")
	if info := r.hostInfo(); info != nil {
		t.Fatalf("gitHost.info with off = %+v, want null", info)
	}
	if lk := r.branchLookup(); lk.Kind != "disabled" {
		t.Fatalf("lookup with off = %+v, want disabled", lk)
	}
	if after := len(forge.Calls()); after != before {
		t.Fatalf("host called %d times while off, want none", after-before)
	}
	r.setProvider("github")
	if info := r.hostInfo(); info == nil || info.ID != "github" {
		t.Fatalf("gitHost.info pinned to github = %+v, want github", info)
	}
	if lk := r.branchLookup(); lk.Kind != "unavailable" || lk.Status == nil || lk.Status.Kind != "notFound" {
		t.Fatalf("lookup pinned to github without gh = %+v, want unavailable notFound", lk)
	}
	if after := len(forge.Calls()); after != before {
		t.Fatalf("forge called %d times while pinned to github, want none", after-before)
	}
	r.setProvider("nope")
	if info := r.hostInfo(); info == nil || info.ID != "forge" {
		t.Fatalf("gitHost.info with an unknown id = %+v, want the auto-detected forge", info)
	}
}

// A repo whose origin is on github.com resolves to GitHub even with the forge registered.
func TestOriginPicksTheProvider(t *testing.T) {
	forge := fakehost.New()
	app := flowharness.New(t, flowharness.WithGitHost(forge), flowharness.WithoutGh())
	gs := app.OpenGitStream()
	repo := app.NewRepo("gh")
	repo.Commit("first", map[string]string{"a.txt": "a\n"})
	repo.AddRemote("https://github.com/acme/r.git")
	var opened gitrpc.RepoOpenResult
	gs.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: repo.Dir}, &opened)
	if opened.Kind != "ok" || opened.Repo == nil {
		t.Fatalf("repo.open = %+v", opened)
	}
	info := call[gitrpc.GitHostInfoResult](t, gs, "gitHost.info", gitrpc.GitHostInfoParams{RepoID: opened.Repo.RepoID}).Host
	if info == nil || info.ID != "github" || info.ChangeRequestShort != "PR" {
		t.Fatalf("gitHost.info = %+v, want github", info)
	}
	if n := len(forge.Calls()); n != 0 {
		t.Fatalf("forge called %d times for a github.com repo, want none", n)
	}
}

func (r *rig) taskID() string { return r.branch.TaskID }

func taskOf(b adewire.Board, id string) adewire.Task {
	for _, tk := range b.Tasks {
		if tk.ID == id {
			return tk
		}
	}
	return adewire.Task{}
}
