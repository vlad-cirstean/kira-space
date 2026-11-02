package githostflow_test

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakeaz"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost/azure"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
)

// Contract git-host-azure: tests/ui/git-host-azure.spec.ts reads these.
const (
	azScenario = "git-host-azure"
	azOrigin   = "https://dev.azure.com/acme/Web%20App/_git/api"
	azPR12     = "https://dev.azure.com/acme/Web%20App/_git/api/pullrequest/12"
)

var azKey = fakeaz.Key("acme", "Web App", "api")

// azClock is the discovery cache's time; a test moves it to expire a probe.
type azClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *azClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }

func (c *azClock) Advance(d time.Duration) { c.mu.Lock(); c.now = c.now.Add(d); c.mu.Unlock() }

func day(d int) time.Time { return time.Date(2026, 5, d, 10, 0, 0, 0, time.UTC) }

type azRig struct {
	t      *testing.T
	app    *flowharness.App
	az     *fakeaz.Az
	clock  *azClock
	gs     *flowharness.GitStream
	gitID  string
	repoID string
	tip    string
}

// newAzRig boots an app whose only reachable hosts are the fake az, with a clone of acme/Web App/api
// imported into the ADE workspace. PR 12 is active on feature/login with a required reviewer who
// approved and a running Build; PR 9 is completed on release; PR 15 is a fork's PR on the same
// branch name as PR 12.
func newAzRig(t *testing.T) *azRig {
	t.Helper()
	az := fakeaz.New()
	az.SetDefaults("https://dev.azure.com/acme", "Web App")
	az.AddPR(azKey, fakeaz.PR{
		ID: 9, Title: "Ship v1", Status: "completed", Src: "release", Dst: "main", Head: strings.Repeat("9", 40),
		MergeSha: strings.Repeat("d", 40), Created: day(1), Author: "ann@example.com",
	})
	az.AddPR(azKey, fakeaz.PR{
		ID: 12, Title: "PROJ-7 Add login", Status: "active", Src: "feature/login", Dst: "main", Head: strings.Repeat("a", 40),
		Created: day(3), Author: "ann@example.com", Reviewers: []fakeaz.Reviewer{{Vote: 10, Required: true}},
		Policies: []fakeaz.Policy{{Type: "Build", Status: "running"}, {Type: "Minimum number of reviewers", Status: "approved"}},
	})
	az.AddPR(azKey, fakeaz.PR{
		ID: 15, Title: "Fork: tweak build", Status: "active", Src: "feature/login", Dst: "main", Head: strings.Repeat("f", 40),
		Created: day(4), Author: "mallory@example.com", ForkRepo: "api-fork", ForkProject: "Sandbox",
	})

	clock := &azClock{now: time.Now()}
	app := flowharness.New(t, flowharness.WithAzure(azure.Options{Locator: az, Runner: az, Clock: clock}), flowharness.WithoutGh())
	dir := app.Work + "/api"
	bare := app.NewBare("api-remote")
	seed := app.NewRepo("api-seed")
	seed.Commit("base", map[string]string{"base.txt": "base\n"})
	bare.PushFrom(seed)
	bare.Clone(dir)
	git(t, dir, "remote", "set-url", "origin", azOrigin)
	// A merged pull request lists only while its branch exists.
	git(t, dir, "branch", "feature/login")
	git(t, dir, "branch", "release")
	tip := git(t, dir, "rev-parse", "feature/login")

	rec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	gs := app.OpenGitStream()
	var opened gitrpc.RepoOpenResult
	gs.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: dir}, &opened)
	if opened.Kind != "ok" || opened.Repo == nil {
		t.Fatalf("repo.open = %+v", opened)
	}
	return &azRig{t: t, app: app, az: az, clock: clock, gs: gs, gitID: opened.Repo.RepoID, repoID: rec.ID, tip: tip}
}

func (r *azRig) lookup(branch string) gitrpc.ChangeRequestLookup {
	r.t.Helper()
	return call[gitrpc.ChangeRequestLookup](r.t, r.gs, "branch.resolveChangeRequest", gitrpc.BranchResolveChangeRequestParams{RepoID: r.gitID, Branch: branch})
}

func (r *azRig) index() gitrpc.BranchChangeRequestIndexResult {
	r.t.Helper()
	return call[gitrpc.BranchChangeRequestIndexResult](r.t, r.gs, "branch.changeRequestIndex", gitrpc.BranchChangeRequestIndexParams{RepoID: r.gitID})
}

func (r *azRig) sync() {
	r.t.Helper()
	call[struct{}](r.t, r.gs, "branch.syncChangeRequests", gitrpc.BranchSyncChangeRequestsParams{RepoID: r.gitID, Wait: true})
}

func (r *azRig) row(number int) gitrpc.ChangeRequestRecord {
	r.t.Helper()
	for _, b := range r.index().Branches {
		if b.ChangeRequest.Number == number {
			return b.ChangeRequest
		}
	}
	r.t.Fatalf("index has no change request %d: %+v", number, r.index())
	return gitrpc.ChangeRequestRecord{}
}

// indexed reports whether the index holds change request number.
func (r *azRig) indexed(number int) bool {
	for _, b := range r.index().Branches {
		if b.ChangeRequest.Number == number {
			return true
		}
	}
	return false
}

func (r *azRig) reviewState() gitrpc.ChangeRequestReviewStateResult {
	r.t.Helper()
	return call[gitrpc.ChangeRequestReviewStateResult](r.t, r.gs, "changeRequest.reviewState",
		gitrpc.ChangeRequestReviewStateParams{RepoID: r.gitID, Branch: "feature/login", Number: 12})
}

func (r *azRig) review(event, body string) gitrpc.ChangeRequestPostResult {
	r.t.Helper()
	return call[gitrpc.ChangeRequestPostResult](r.t, r.gs, "changeRequest.submitReview",
		gitrpc.ChangeRequestSubmitReviewParams{RepoID: r.gitID, Number: 12, Event: event, Body: body, HeadSha: r.tip})
}

// Azure Repos registers next to GitHub, drives lookups, the index, reviews and links through the
// fake az, and reports every failure it can meet. Backend half of tests/ui/git-host-azure.spec.ts.
func TestAzureRepos(t *testing.T) {
	r := newAzRig(t)

	// 1. The origin resolves to Azure Repos with its own words.
	list := call[githost.Providers](t, r.gs, "gitHost.providers", struct{}{})
	if len(list.Providers) != 2 || list.Providers[0].ID != "github" || list.Providers[1].ID != "azure" {
		t.Fatalf("gitHost.providers = %+v, want github, azure", list)
	}
	r.app.Contract(t, azScenario, "git:gitHost.providers", list)
	info := call[gitrpc.GitHostInfoResult](t, r.gs, "gitHost.info", gitrpc.GitHostInfoParams{RepoID: r.gitID})
	if info.Host == nil || info.Host.ID != "azure" || info.Host.Label != "Azure Repos" || info.Host.ChangeRequestShort != "PR" {
		t.Fatalf("gitHost.info = %+v, want azure", info)
	}
	r.app.Contract(t, azScenario, "git:gitHost.info", info)

	// 2. The first branch lookup (index incomplete) asks az once and drops the fork's pull request.
	got := r.lookup("feature/login")
	if got.Kind != "ok" || len(got.ChangeRequests) != 1 || got.ChangeRequests[0].Number != 12 ||
		got.ChangeRequests[0].URL != azPR12 || got.ChangeRequests[0].State != "open" {
		t.Fatalf("branch.resolveChangeRequest = %+v, want PR 12 only", got)
	}
	r.app.Contract(t, azScenario, "git:branch.resolveChangeRequest", got)

	// 3. The index: a backfill, then deltas and the checks sweep.
	r.sync()
	ix := r.index()
	if !ix.Complete || len(ix.Branches) != 2 {
		t.Fatalf("index after backfill = %+v, want PR 12 and PR 9 (the fork's PR is not this repo's branch)", ix)
	}
	// The listing marks open rows EXPECTED; the pending pass of the same run reads their policies.
	if p := r.row(12); p.ReviewDecision != "APPROVED" || p.ChecksState != "PENDING" || p.State != "open" || p.URL != azPR12 {
		t.Fatalf("PR 12 row = %+v, want APPROVED and PENDING", p)
	}
	if p := r.row(9); p.State != "merged" || p.ChecksState != "" {
		t.Fatalf("PR 9 row = %+v, want merged", p)
	}

	// The next run asks only whether anything changed (same newest id), then re-reads the pending row.
	probes := r.az.Count("pr-probe:")
	r.sync()
	if r.az.Count("pr-probe:") != probes+1 || r.az.Count("pr-list:"+azKey+":all") != 1 {
		t.Fatalf("calls = %v, want one more probe and no new full listing (etag unchanged)", r.az.Calls())
	}
	r.app.Contract(t, azScenario, "git:branch.changeRequestIndex#pending", r.index(), flowharness.Mask("syncedAt"))

	// An open row whose checks are pending is re-read each run; one that left active is read by id.
	r.az.SetPR(azKey, 12, func(p *fakeaz.PR) { p.Status = "abandoned" })
	r.sync()
	if p := r.row(12); p.State != "closed" || p.ChecksState != "" {
		t.Fatalf("PR 12 row = %+v after abandon, want closed", p)
	}

	// A new pull request lands through the delta, and its build finishing is picked up as pending.
	git(t, r.app.Work+"/api", "branch", "feature/new")
	r.az.AddPR(azKey, fakeaz.PR{
		ID: 16, Title: "New work", Status: "active", Src: "feature/new", Dst: "main", Head: strings.Repeat("e", 40),
		Created: day(5), Author: "bob@example.com", Policies: []fakeaz.Policy{{Type: "Build", Status: "running"}},
	})
	fullListings := r.az.Count("pr-list:" + azKey + ":all")
	r.sync()
	if !r.indexed(16) || r.az.Count("pr-list:"+azKey+":all") != fullListings+1 || r.row(16).ChecksState != "PENDING" {
		t.Fatalf("PR 16 missing from the index after a delta, or no new listing: %+v", r.index())
	}
	r.az.SetPolicies(azKey, 16, fakeaz.Policy{Type: "Build", Status: "approved"})
	r.sync()
	if p := r.row(16); p.ChecksState != "SUCCESS" {
		t.Fatalf("PR 16 checks = %q after the build passed, want SUCCESS", p.ChecksState)
	}
	r.app.Contract(t, azScenario, "git:branch.changeRequestIndex#success", r.index(), flowharness.Mask("syncedAt"))

	// 4. Reviews: the body is a new thread, the vote follows it.
	state := r.reviewState()
	if state.Kind != "ok" || state.ChangeRequestReview == nil {
		t.Fatalf("changeRequest.reviewState = %+v", state)
	}
	if rv := state.ChangeRequestReview; rv.Number != 12 || rv.Host.ID != "azure" || rv.Author != "ann@example.com" ||
		rv.Viewer != "me@acme.test" || rv.Own || rv.State != "closed" {
		t.Fatalf("reviewState payload = %+v", rv)
	}
	r.app.Contract(t, azScenario, "git:changeRequest.reviewState", state, flowharness.Replace(r.tip, strings.Repeat("c", 40)))

	posted := r.review(githost.ReviewApprove, "LGTM")
	if posted.Kind != "ok" {
		t.Fatalf("submitReview approve = %+v", posted)
	}
	r.app.Contract(t, azScenario, "git:changeRequest.submitReview", posted)
	if !slices.Equal(r.az.Threads(12), []string{"LGTM"}) || !slices.Equal(r.az.Votes(12), []string{"approve"}) {
		t.Fatalf("after approve: threads %v votes %v", r.az.Threads(12), r.az.Votes(12))
	}
	calls := r.az.Calls()
	if thread, vote := slices.Index(calls, "invoke:pullRequestThreads:"+azKey), slices.Index(calls, "vote:12:approve"); thread < 0 || vote < thread {
		t.Fatalf("calls = %v, want the thread before the vote", calls)
	}
	if res := r.review(githost.ReviewRequestChanges, "fix the tests"); res.Kind != "ok" ||
		!slices.Equal(r.az.Votes(12), []string{"approve", "wait-for-author"}) {
		t.Fatalf("requestChanges = %+v, votes %v", res, r.az.Votes(12))
	}
	voteCalls := r.az.Count("vote:")
	if res := r.review(githost.ReviewComment, "just a note"); res.Kind != "ok" || r.az.Count("vote:") != voteCalls ||
		!slices.Equal(r.az.Threads(12), []string{"LGTM", "fix the tests", "just a note"}) {
		t.Fatalf("comment review = %+v, threads %v", res, r.az.Threads(12))
	}
	commented := call[gitrpc.ChangeRequestPostResult](t, r.gs, "changeRequest.comment",
		gitrpc.ChangeRequestCommentParams{RepoID: r.gitID, Number: 12, Body: "@ann thanks"})
	if commented.Kind != "ok" || !slices.Equal(r.az.Threads(12)[3:], []string{"@ann thanks"}) {
		t.Fatalf("comment = %+v, threads %v", commented, r.az.Threads(12))
	}
	r.app.Contract(t, azScenario, "git:changeRequest.comment", commented)

	// A line comment is a thread anchored to the file's right side; the head it names must be the host's.
	r.az.SetPR(azKey, 12, func(p *fakeaz.PR) { p.Head = r.tip })
	lined := call[gitrpc.ChangeRequestLineCommentResult](t, r.gs, "changeRequest.lineComment", gitrpc.ChangeRequestLineCommentParams{
		RepoID: r.gitID, Number: 12, Path: "base.txt", Range: gitreview.LineRange{Start: 1, End: 1}, HeadSha: r.tip, Body: "rename?",
	})
	wantCtx := []fakeaz.ThreadContext{{FilePath: "/base.txt", StartLine: 1, End: 1, EndOffset: len("base") + 1}}
	if lined.Kind != "ok" || !reflect.DeepEqual(r.az.ThreadContexts(12)[4:], wantCtx) || r.az.Threads(12)[4] != "rename?" {
		t.Fatalf("lineComment = %+v, contexts %+v, threads %v", lined, r.az.ThreadContexts(12), r.az.Threads(12))
	}

	// 5. The composed URL opens; the API form and a look-alike host are refused.
	open := func(u string) error {
		return r.app.W.GitHost.OpenChangeRequestURL(bridge.OpenChangeRequestURLArgs{URL: u})
	}
	if err := open(azPR12); err != nil {
		t.Fatalf("OpenChangeRequestURL(composed) = %v", err)
	}
	if opened := r.app.Browser.Opened(); len(opened) == 0 || opened[len(opened)-1] != azPR12 {
		t.Fatalf("browser opened %v, want %s", opened, azPR12)
	}
	for _, bad := range []string{
		"https://dev.azure.com/acme/Web%20App/_apis/git/repositories/api/pullRequests/12",
		"https://dev.azure.com.evil/acme/Web%20App/_git/api/pullrequest/12",
		"http://dev.azure.com/acme/Web%20App/_git/api/pullrequest/12",
		"https://dev.azure.com/acme/Web%20App/_git/api/pullrequest/x",
	} {
		if err := open(bad); err == nil {
			t.Errorf("OpenChangeRequestURL(%q) succeeded", bad)
		}
	}

	// 6. ADE host links: a pull request link targets the task repo and branch; a work item names none.
	target, err := r.app.W.AdeTask.ResolveHostLink(ctx, adewire.HostLinkArgs{URL: azPR12 + "?_a=files"})
	if err != nil || target.CodeRepoID != r.repoID || target.Branch != "feature/login" || target.Fork ||
		target.ChangeRequest == nil || target.ChangeRequest.Number != 12 {
		t.Fatalf("ResolveHostLink(PR 12) = %+v, %v", target, err)
	}
	fork, err := r.app.W.AdeTask.ResolveHostLink(ctx, adewire.HostLinkArgs{URL: strings.Replace(azPR12, "/12", "/15", 1)})
	if err != nil || !fork.Fork || fork.HeadOwner != "acme/Sandbox/api-fork" {
		t.Fatalf("ResolveHostLink(fork PR 15) = %+v, %v", fork, err)
	}
	if _, err := r.app.W.AdeTask.ResolveHostLink(ctx, adewire.HostLinkArgs{URL: "https://dev.azure.com/acme/Web%20App/_workitems/edit/55"}); err == nil ||
		!strings.Contains(err.Error(), "An issue link names no branch") {
		t.Fatalf("ResolveHostLink(work item) = %v, want the issue-link refusal", err)
	}

	if v := r.az.Violations(); len(v) != 0 {
		t.Fatalf("az calls without --org and --detect false: %v", v)
	}
}

// Every way az can be unusable reads as an unavailable lookup with the step that fixes it, and a
// throttled call arms the breaker so no further spawn happens.
func TestAzureReposFailures(t *testing.T) {
	r := newAzRig(t)
	expire := func() { r.clock.Advance(6 * time.Minute) }
	unavailable := func(branch string) gitrpc.ChangeRequestLookup {
		t.Helper()
		res := r.lookup(branch)
		if res.Kind != "unavailable" || res.Status == nil {
			t.Fatalf("lookup(%s) = %+v, want unavailable", branch, res)
		}
		return res
	}

	r.az.SetLoggedIn(false)
	expire()
	if st := unavailable("signed-out").Status; st.Kind != githost.KindUnauthenticated || !strings.Contains(st.Reason, "az login") {
		t.Fatalf("signed out status = %+v", st)
	}
	r.app.Contract(t, azScenario, "git:branch.resolveChangeRequest#unavailable", unavailable("signed-out-again"))

	r.az.SetLoggedIn(true)
	r.az.SetExtension(false)
	expire()
	if st := unavailable("no-extension").Status; st.Kind != githost.KindNotFound || !strings.Contains(st.Reason, "az extension add") {
		t.Fatalf("missing extension status = %+v", st)
	}

	r.az.SetExtension(true)
	expire()
	if res := r.lookup("feature/login"); res.Kind != "ok" || len(res.ChangeRequests) != 1 {
		t.Fatalf("lookup after recovery = %+v", res)
	}

	r.az.SetFailure("ERROR: TF400733: The request has been canceled: Request was blocked due to exceeding usage of resource 'Count' in namespace 'WIT'.")
	if st := unavailable("throttled").Status; !strings.Contains(st.Reason, "throttled") {
		t.Fatalf("throttled status = %+v", st)
	}
	spawned := r.az.Count("")
	unavailable("throttled-again")
	if got := r.az.Count(""); got != spawned {
		t.Fatalf("a lookup spawned az %d times inside the breaker window", got-spawned)
	}
	if v := r.az.Violations(); len(v) != 0 {
		t.Fatalf("az calls without --org and --detect false: %v", v)
	}
}
