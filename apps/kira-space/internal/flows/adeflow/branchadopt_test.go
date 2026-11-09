package adeflow_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P583 existing branches for an agent. Contract ade-branch-adopt: tests/ui/ade-v2-branch-adopt.spec.ts reads these.

const adoptScenario = "ade-branch-adopt"

var adoptMask = flowharness.Mask("id", "taskId", "branchId", "codeRepoId", "worktree", "addedAt", "tip", "base", "setup", "createdAt", "expiresAt", "session")

type adoptFixture struct {
	app     *flowharness.App
	repo    *flowharness.Repo
	apiID   string
	adopter adewire.Task // asks for the branch
	donor   adewire.Task // holds it
}

// newAdoptFixture registers api and makes two tasks on it: the adopter with a waiting draft row, the donor
// with no rows. Its agent runs end at once, or wait for hold.
func newAdoptFixture(t *testing.T, hold string) *adoptFixture {
	t.Helper()
	app := flowharness.New(t)
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done", WaitFile: hold}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	api := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	return &adoptFixture{
		app: app, repo: repo, apiID: api.ID,
		adopter: createTask(t, app, "Adopter", "flow", api.ID),
		donor:   createTask(t, app, "Donor", "flow"),
	}
}

// donorHolds puts name on the donor task, checked out in its own worktree.
func (f *adoptFixture) donorHolds(t *testing.T, name string) adewire.Branch {
	t.Helper()
	gitOut(t, f.repo.Dir, "branch", name)
	if _, err := f.app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: f.apiID, Name: name, TaskID: f.donor.ID}); err != nil {
		t.Fatalf("AddExistingBranch %s: %v", name, err)
	}
	return f.rowOf(t, f.donor.ID, name)
}

func (f *adoptFixture) rowOf(t *testing.T, taskID, name string) adewire.Branch {
	t.Helper()
	for _, br := range board(t, f.app).Branches {
		if br.TaskID == taskID && br.Name == name {
			return br
		}
	}
	t.Fatalf("task %s has no branch %q", taskID, name)
	return adewire.Branch{}
}

// asked is one request_branches call in flight.
type asked struct {
	f    *adoptFixture
	task adewire.Task
	done chan struct {
		res agentmcp.RequestBranchesResult
		err error
	}
}

func (f *adoptFixture) ask(task adewire.Task, specs ...agentmcp.BranchSpec) *asked {
	a := &asked{f: f, task: task, done: make(chan struct {
		res agentmcp.RequestBranchesResult
		err error
	}, 1)}
	go func() {
		res, err := f.app.W.AdeBoard.RequestBranches(ctx, agentmcp.SpaceCall{TaskID: task.ID}, specs)
		a.done <- struct {
			res agentmcp.RequestBranchesResult
			err error
		}{res, err}
	}()
	return a
}

func existing(branch string) agentmcp.BranchSpec {
	return agentmcp.BranchSpec{Repo: "api", Branch: branch, Existing: true}
}

// request waits for the open request of the asking task.
func (a *asked) request(t *testing.T) adewire.BranchRequest {
	t.Helper()
	var id string
	testx.WaitUntil(t, waitFor, func() bool {
		id = taskOf(t, board(t, a.f.app), a.task.ID).BranchRequestID
		return id != ""
	})
	req, err := branchRequest(a.f.app, id)
	if err != nil {
		t.Fatalf("BranchRequest: %v", err)
	}
	return req
}

// result waits for the call to end.
func (a *asked) result(t *testing.T) agentmcp.RequestedBranch {
	t.Helper()
	select {
	case r := <-a.done:
		if r.err != nil || len(r.res.Branches) != 1 {
			t.Fatalf("RequestBranches = %+v, %v; want one branch", r.res, r.err)
		}
		return r.res.Branches[0]
	case <-time.After(waitFor):
		t.Fatal("request_branches did not end")
	}
	return agentmcp.RequestedBranch{}
}

func (a *asked) approve(t *testing.T, req adewire.BranchRequest) {
	t.Helper()
	ids := make([]string, len(req.Items))
	for i, it := range req.Items {
		ids[i] = it.ID
	}
	if ans, err := approveBR(a.f.app, req.ID, ids, nil); err != nil || !ans.Resolved {
		t.Fatalf("Approve = %+v, %v", ans, err)
	}
}

// refused expects the call to end at once with a failure holding text and no prompt.
func (a *asked) refused(t *testing.T, text string) {
	t.Helper()
	r := a.result(t)
	if r.Outcome != agentmcp.BranchFailed || !strings.Contains(r.Next, text) {
		t.Fatalf("result = %+v, want a failure holding %q", r, text)
	}
	if id := taskOf(t, board(t, a.f.app), a.task.ID).BranchRequestID; id != "" {
		t.Fatalf("task has an open request %s, want none", id)
	}
}

func headRef(t *testing.T, dir string) string {
	t.Helper()
	return gitOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
}

func (f *adoptFixture) listContract(t *testing.T, key string, req adewire.BranchRequest) {
	t.Helper()
	f.app.Contract(t, adoptScenario, "ApprovalsService.List#"+key, []approvals.Item{*branchItem(f.app, req.ID)}, adoptMask)
}

func TestBranchAdopt(t *testing.T) {
	t.Run("a free branch: checkout item, approve, worktree in the task folder", func(t *testing.T) {
		f := newAdoptFixture(t, "")
		gitOut(t, f.repo.Dir, "branch", "feat/free")
		a := f.ask(f.adopter, existing("feat/free"))
		req := a.request(t)
		if len(req.Items) != 1 || req.Items[0].Kind != "checkout" || req.Items[0].Holder != nil || req.Items[0].Dirty != 0 {
			t.Fatalf("request = %+v, want one checkout item with no holder", req)
		}
		f.listContract(t, "checkout", req)
		a.approve(t, req)
		got := a.result(t)
		if got.Outcome != agentmcp.BranchCheckedOut || !nanoFolder.MatchString(filepath.Base(got.Worktree)) || filepath.Base(filepath.Dir(got.Worktree)) != "api" {
			t.Fatalf("result = %+v, want checkedOut in wt/api/<nanoid>", got)
		}
		row := f.rowOf(t, f.adopter.ID, "feat/free")
		if row.Kind != "mine" || row.Origin != "agent" || row.Worktree != got.Worktree || row.Detached != nil || len(row.Shared) != 0 {
			t.Fatalf("row = %+v, want an agent row of kind mine, attached, not shared", row)
		}
		if n := len(branchesOf(t, f.app, f.adopter.ID)); n != 1 {
			t.Fatalf("adopter rows = %d, want the draft filled, not a second row", n)
		}
		if head := headRef(t, got.Worktree); head != "feat/free" {
			t.Fatalf("worktree HEAD = %q, want feat/free", head)
		}
	})

	t.Run("a branch on another task: adopt names the holder, approve detaches it there", func(t *testing.T) {
		f := newAdoptFixture(t, "")
		donorRow := f.donorHolds(t, "feat/x")
		tip := headOf(t, donorRow.Worktree)
		a := f.ask(f.adopter, existing("feat/x"))
		req := a.request(t)
		it := req.Items[0]
		if it.Kind != "adopt" || it.Holder == nil || it.Holder.Kind != "task" || it.Holder.TaskID != f.donor.ID || it.Holder.BranchID != donorRow.ID || it.Holder.Worktree != donorRow.Worktree {
			t.Fatalf("item = %+v, want an adopt item held by the donor task", it)
		}
		f.listContract(t, "adopt", req)
		a.approve(t, req)
		got := a.result(t)
		if got.Outcome != agentmcp.BranchCheckedOut || !strings.Contains(got.Next, "Detached it in "+donorRow.Worktree) {
			t.Fatalf("result = %+v, want checkedOut after detaching the donor", got)
		}
		if head := headRef(t, donorRow.Worktree); head != "HEAD" || headOf(t, donorRow.Worktree) != tip {
			t.Fatalf("donor worktree HEAD = %q at %s, want detached at %s", head, headOf(t, donorRow.Worktree), tip)
		}
		if head := headRef(t, got.Worktree); head != "feat/x" {
			t.Fatalf("adopter worktree HEAD = %q, want feat/x", head)
		}
		adopter, donor := f.rowOf(t, f.adopter.ID, "feat/x"), f.rowOf(t, f.donor.ID, "feat/x")
		if adopter.Worktree != got.Worktree || len(adopter.Shared) != 1 || adopter.Shared[0].TaskID != f.donor.ID || adopter.Shared[0].BranchID != donor.ID || adopter.Detached != nil {
			t.Fatalf("adopter row = %+v, want attached and shared with the donor", adopter)
		}
		if donor.Worktree != "" || donor.Detached == nil || donor.Detached.Worktree != donorRow.Worktree ||
			donor.Detached.Holder.Kind != "task" || donor.Detached.Holder.TaskID != f.adopter.ID || donor.Detached.Holder.BranchID != adopter.ID ||
			len(donor.Shared) != 1 || donor.Shared[0].BranchID != adopter.ID {
			t.Fatalf("donor row = %+v, want detached with the adopter as holder, name %q kept", donor, donor.Name)
		}
		f.app.Contract(t, adoptScenario, "AdeTaskBoard.Board#adopted", []adewire.Branch{donor, adopter}, adoptMask)
	})

	t.Run("the main checkout holder: adopt with a dirty warning", func(t *testing.T) {
		f := newAdoptFixture(t, "")
		gitOut(t, f.repo.Dir, "checkout", "-q", "-b", "feat/main-held")
		scratch := filepath.Join(f.repo.Dir, "scratch.txt")
		if err := os.WriteFile(scratch, []byte("wip\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		a := f.ask(f.adopter, existing("feat/main-held"))
		req := a.request(t)
		it := req.Items[0]
		if it.Kind != "adopt" || it.Holder == nil || it.Holder.Kind != "main" || it.Dirty != 1 {
			t.Fatalf("item = %+v, want an adopt item held by the main checkout with 1 dirty file", it)
		}
		f.listContract(t, "adoptMain", req)
		a.approve(t, req)
		if got := a.result(t); got.Outcome != agentmcp.BranchCheckedOut {
			t.Fatalf("result = %+v, want checkedOut", got)
		}
		if head := headRef(t, f.repo.Dir); head != "HEAD" {
			t.Fatalf("main checkout HEAD = %q, want detached", head)
		}
		if _, err := os.Stat(scratch); err != nil {
			t.Fatalf("dirty file is gone after the detach: %v", err)
		}
	})

	t.Run("a busy holder is refused before any prompt", func(t *testing.T) {
		hold := filepath.Join(t.TempDir(), "release")
		f := newAdoptFixture(t, hold)
		f.donorHolds(t, "feat/busy")
		if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.donor.ID}); err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		waitRun(t, f.app, f.donor.ID, "one", "running")
		f.ask(f.adopter, existing("feat/busy")).refused(t, "where a run is working on it")
		if err := os.WriteFile(hold, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a git operation in the holder is refused before any prompt", func(t *testing.T) {
		f := newAdoptFixture(t, "")
		donorRow := f.donorHolds(t, "feat/merging")
		commitFile := func(dir, content string) {
			if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			gitOut(t, dir, "add", "-A")
			gitOut(t, dir, "commit", "-q", "-m", "write "+content)
		}
		commitFile(donorRow.Worktree, "donor\n")
		gitOut(t, f.repo.Dir, "checkout", "-q", "-b", "feat/other", "main")
		commitFile(f.repo.Dir, "other\n")
		gitOut(t, f.repo.Dir, "checkout", "-q", "main")
		merge := exec.Command("git", "merge", "feat/other")
		merge.Dir = donorRow.Worktree
		if out, err := merge.CombinedOutput(); err == nil {
			t.Fatalf("merge should conflict:\n%s", out)
		}
		f.ask(f.adopter, existing("feat/merging")).refused(t, "in progress")
	})

	t.Run("deny leaves the holder attached", func(t *testing.T) {
		f := newAdoptFixture(t, "")
		donorRow := f.donorHolds(t, "feat/x")
		a := f.ask(f.adopter, existing("feat/x"))
		req := a.request(t)
		if _, err := denyBR(f.app, req.ID); err != nil {
			t.Fatal(err)
		}
		if got := a.result(t); got.Outcome != agentmcp.BranchDenied {
			t.Fatalf("result = %+v, want denied", got)
		}
		if head := headRef(t, donorRow.Worktree); head != "feat/x" {
			t.Fatalf("donor worktree HEAD = %q, want feat/x still", head)
		}
		if row := f.rowOf(t, f.donor.ID, "feat/x"); row.Detached != nil || row.Worktree != donorRow.Worktree || len(row.Shared) != 0 {
			t.Fatalf("donor row = %+v, want attached and not shared", row)
		}
	})

	t.Run("existing without the flag fails with the hint, existing with a base fails", func(t *testing.T) {
		f := newAdoptFixture(t, "")
		gitOut(t, f.repo.Dir, "branch", "feat/free")
		f.ask(f.adopter, agentmcp.BranchSpec{Repo: "api", Branch: "feat/free"}).refused(t, "pass existing: true to work on it")
		f.ask(f.adopter, agentmcp.BranchSpec{Repo: "api", Branch: "feat/free", Existing: true, Base: "main"}).refused(t, "base only applies to a new branch")
		f.ask(f.adopter, existing("feat/nope")).refused(t, "Omit existing to create it")
	})

	t.Run("a reserved name is refused", func(t *testing.T) {
		f := newAdoptFixture(t, "")
		f.ask(f.adopter, existing("main")).refused(t, "belongs to the repo itself")
	})

	t.Run("the donor's start refuses while another task holds the branch, then reattaches its own folder", func(t *testing.T) {
		f := newAdoptFixture(t, "")
		donorRow := f.donorHolds(t, "feat/x")
		a := f.ask(f.adopter, existing("feat/x"))
		a.approve(t, a.request(t))
		a.result(t)

		_, serr := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.donor.ID})
		if serr == nil {
			run := waitRun(t, f.app, f.donor.ID, "one", "stuck", "failed", "pending")
			if run.State == "done" || run.State == "running" {
				t.Fatalf("donor run = %+v, want it refused while the adopter holds the branch", run)
			}
		} else if !strings.Contains(serr.Error(), "another task") {
			t.Fatalf("StartRun = %v, want the checked-out-in-another-task refusal", serr)
		}
		if head := headRef(t, donorRow.Worktree); head != "HEAD" {
			t.Fatalf("donor worktree HEAD = %q, want it left detached by the refused start", head)
		}

		if err := f.app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: f.adopter.ID}); err != nil {
			t.Fatalf("ArchiveTask adopter: %v", err)
		}
		if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.donor.ID}); err != nil {
			t.Fatalf("StartRun after the adopter left: %v", err)
		}
		waitRun(t, f.app, f.donor.ID, "one", "done")
		if head := headRef(t, donorRow.Worktree); head != "feat/x" {
			t.Fatalf("donor worktree HEAD = %q, want feat/x reattached in its own folder", head)
		}
		row := f.rowOf(t, f.donor.ID, "feat/x")
		if row.Worktree != donorRow.Worktree || row.Detached != nil {
			t.Fatalf("donor row = %+v, want attached in %s", row, donorRow.Worktree)
		}
		entries, err := os.ReadDir(filepath.Dir(donorRow.Worktree))
		if err != nil || len(entries) != 1 {
			t.Fatalf("worktree folders = %v, %v; want only the donor's own", entries, err)
		}
	})

	t.Run("archiving the donor removes only its own folder", func(t *testing.T) {
		f := newAdoptFixture(t, "")
		donorRow := f.donorHolds(t, "feat/x")
		a := f.ask(f.adopter, existing("feat/x"))
		a.approve(t, a.request(t))
		got := a.result(t)
		if err := f.app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: f.donor.ID}); err != nil {
			t.Fatalf("ArchiveTask donor: %v", err)
		}
		gone(t, donorRow.Worktree)
		if head := headRef(t, got.Worktree); head != "feat/x" {
			t.Fatalf("adopter worktree HEAD = %q, want feat/x untouched by the donor's archive", head)
		}
		if row := f.rowOf(t, f.adopter.ID, "feat/x"); row.Worktree != got.Worktree || len(row.Shared) != 0 {
			t.Fatalf("adopter row = %+v, want attached and no longer shared", row)
		}
	})

	t.Run("a holder that moved during the wait fails the branch", func(t *testing.T) {
		f := newAdoptFixture(t, "")
		donorRow := f.donorHolds(t, "feat/x")
		a := f.ask(f.adopter, existing("feat/x"))
		req := a.request(t)
		elsewhere := filepath.Join(f.app.Work, "elsewhere")
		gitOut(t, donorRow.Worktree, "switch", "-q", "--detach")
		gitOut(t, f.repo.Dir, "worktree", "add", "-q", elsewhere, "feat/x")
		a.approve(t, req)
		got := a.result(t)
		if got.Outcome != agentmcp.BranchFailed || !strings.Contains(got.Next, "moved to "+elsewhere+" while waiting") {
			t.Fatalf("result = %+v, want a moved failure", got)
		}
		if head := headRef(t, elsewhere); head != "feat/x" {
			t.Fatalf("elsewhere HEAD = %q, want feat/x untouched", head)
		}
		for _, br := range branchesOf(t, f.app, f.adopter.ID) {
			if br.Name != "" {
				t.Fatalf("adopter row %+v, want the draft left unnamed", br)
			}
		}
	})

	t.Run("rename_branch refuses a shared branch", func(t *testing.T) {
		f := newAdoptFixture(t, "")
		f.donorHolds(t, "feat/x")
		a := f.ask(f.adopter, existing("feat/x"))
		a.approve(t, a.request(t))
		a.result(t)
		res, err := f.app.W.AdeBoard.RenameBranch(ctx, agentmcp.SpaceCall{TaskID: f.adopter.ID}, agentmcp.RenameBranchArgs{Repo: "api", From: "feat/x", To: "feat/y"})
		if err != nil || res.Outcome != agentmcp.BranchFailed || !strings.Contains(res.Next, `Branch feat/x is also on task "Donor"; it cannot be renamed while shared.`) {
			t.Fatalf("RenameBranch = %+v, %v; want the shared refusal", res, err)
		}
	})
}
