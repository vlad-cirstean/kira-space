package adeflow_test

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Contract ade-repo-script: tests/ui/ade-v2-repo-script.spec.ts reads these.

const secretValue = "p513-saved-token"

func normalScript(t *testing.T, app *flowharness.App, fields scripts.CustomScriptFields, secret map[string]*string) {
	t.Helper()
	fields.Kind, fields.Color = scripts.KindScript, "blue"
	if _, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: fields, SecretValues: secret}); err != nil {
		t.Fatalf("create script %q: %v", fields.Name, err)
	}
}

func installDeps(t *testing.T, app *flowharness.App, useAdeDir bool) {
	t.Helper()
	tok := secretValue
	normalScript(t, app, scripts.CustomScriptFields{
		Name:      "Install deps",
		Command:   "echo \"mode=$KIRA_PARAM_MODE task=$KIRA_TASK\"\necho \"token=$KIRA_PARAM_TOKEN\"\necho \"wt=$KIRA_WORKTREE_PATH\"\necho \"cwd=$(pwd)\"",
		UseAdeDir: useAdeDir,
		Params: []scripts.Param{
			{Name: "mode", Type: scripts.ParamText, Required: true},
			{Name: "token", Type: scripts.ParamText, Secret: true},
		},
	}, map[string]*string{"token": &tok})
}

func setPrepareRun(t *testing.T, app *flowharness.App, repoID string, ref *model.ScriptRef) {
	t.Helper()
	setRepoFile(t, app, repoID, func(f *repocfg.File) { f.Worktree = &repocfg.Worktree{PrepareRun: ref} })
}

func runRef(script string, params map[string]model.ParamValues) *model.ScriptRef {
	return &model.ScriptRef{Script: script, Params: params}
}

func waitSetup(t *testing.T, f *runFixture, state string) adewire.Branch {
	t.Helper()
	var br adewire.Branch
	testx.WaitUntil(t, waitFor, func() bool {
		br = branchOf(t, board(t, f.app), f.taskID, f.repoID)
		return br.Setup != nil && br.Setup.State == state
	})
	return br
}

func TestRepoScriptPrepareRun(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("first", ""))
	installDeps(t, f.app, true)
	setPrepareRun(t, f.app, f.repoID, runRef("Install deps", map[string]model.ParamValues{"mode": {"ci"}}))
	f.start(t, "feat/api-work")
	br := waitSetup(t, f, "ready")
	out := logText(t, f.app, "setup", br.ID)
	for _, want := range []string{`running Run script "Install deps"`, "mode=ci task=Fix login", "token=" + scripts.SecretMask, "wt=" + br.Worktree, "cwd=" + br.Worktree} {
		if !strings.Contains(out, want) {
			t.Fatalf("setup log lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, secretValue) {
		t.Fatalf("setup log holds the secret:\n%s", out)
	}
	if !strings.Contains(out, "cwd="+br.Worktree+"\n") {
		t.Fatalf("script did not run in the worktree %q:\n%s", br.Worktree, out)
	}
	f.app.Contract(t, "ade-repo-script", "AdeTaskService.ReadLog#setup-ready", strings.Split(strings.TrimSpace(strings.ReplaceAll(out, br.Worktree, "<worktree>")), "\n"))
}

func TestRepoScriptPrepareRunErrors(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("first", ""))
	setPrepareRun(t, f.app, f.repoID, runRef("Nope", nil))
	f.start(t, "feat/api-work")
	br := waitSetup(t, f, "failed")
	const missing = `Run script "Nope": not found`
	if br.Setup.Note != missing {
		t.Fatalf("setup note = %q, want %q", br.Setup.Note, missing)
	}
	f.app.Contract(t, "ade-repo-script", "AdeTaskService.Branch#setup-failed", map[string]any{"note": br.Setup.Note, "log": logText(t, f.app, "setup", br.ID)})

	retry := func() adewire.Branch {
		t.Helper()
		if err := f.app.W.AdeTask.RetrySetup(ctx, adewire.BranchArgs{BranchID: br.ID}); err != nil {
			t.Fatal(err)
		}
		return br
	}
	failedWith := func(want string) {
		t.Helper()
		retry()
		testx.WaitUntil(t, waitFor, func() bool {
			s := branchOf(t, board(t, f.app), f.taskID, f.repoID).Setup
			return s != nil && s.State == "failed" && strings.Contains(s.Note, want)
		})
	}

	if _, err := f.app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "Smarty", Kind: scripts.KindSmart, Command: "Think", Color: "blue", UseAdeDir: true,
		Smart: &scripts.Smart{Model: "opus", MaxBudgetUSD: 0.5, Timeout: "5m", Tools: []string{"Read"}},
	}}); err != nil {
		t.Fatal(err)
	}
	setPrepareRun(t, f.app, f.repoID, runRef("Smarty", nil))
	failedWith("a repo file runs a normal script")

	installDeps(t, f.app, true)
	setPrepareRun(t, f.app, f.repoID, runRef("Install deps", map[string]model.ParamValues{"mode": {"ci"}, "token": {"plaintext"}}))
	failedWith("token is a secret param")

	setPrepareRun(t, f.app, f.repoID, runRef("Install deps", map[string]model.ParamValues{"mode": {"ci"}}))
	retry()
	waitSetup(t, f, "ready")
}

func TestRepoScriptPrepareRunScriptDir(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("first", ""))
	installDeps(t, f.app, false)
	setPrepareRun(t, f.app, f.repoID, runRef("Install deps", map[string]model.ParamValues{"mode": {"ci"}}))
	f.start(t, "feat/api-work")
	br := waitSetup(t, f, "ready")
	out := logText(t, f.app, "setup", br.ID)
	if !strings.Contains(out, "wt="+br.Worktree) {
		t.Fatalf("KIRA_WORKTREE_PATH is not the worktree %q:\n%s", br.Worktree, out)
	}
	if strings.Contains(out, "cwd="+br.Worktree+"\n") || !strings.Contains(out, "cwd=/") {
		t.Fatalf("script ran in the worktree, want its own folder:\n%s", out)
	}
}

func TestRepoScriptDeployedShaRun(t *testing.T) {
	app := flowharness.New(t)
	repo, _ := cloneWithMain(app, app.Work+"/envs")
	rec := importRepo(t, app, repo.Dir)
	repo.Git("checkout", "-q", "-b", "feat")
	repo.Commit("feat 1", map[string]string{"f.txt": "1\n"})
	repo.Git("checkout", "-q", "main")
	if _, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: rec.ID, Name: "feat"}); err != nil {
		t.Fatal(err)
	}
	repo.Git("merge", "-q", "--ff-only", "feat")
	normalScript(t, app, scripts.CustomScriptFields{
		Name:      "Staging sha",
		Command:   "if [ \"$KIRA_PARAM_TAG\" = ok ]; then git rev-parse HEAD; fi",
		UseAdeDir: true,
		Params:    []scripts.Param{{Name: "tag", Type: scripts.ParamText, Required: true}},
	}, nil)
	env := func(tag string) {
		setRepoFile(t, app, rec.ID, func(rf *repocfg.File) {
			rf.Environments = []repocfg.Environment{
				{Name: "staging", DeployedSHARun: runRef("Staging sha", map[string]model.ParamValues{"tag": {tag}})},
				{Name: "qa", DeployedSHARun: runRef("Missing sha", nil)},
			}
		})
	}
	deployments := func(ok func(map[string]adewire.Deployment) bool) map[string]adewire.Deployment {
		t.Helper()
		byEnv := map[string]adewire.Deployment{}
		testx.WaitUntil(t, waitFor, func() bool {
			byEnv = map[string]adewire.Deployment{}
			for _, b := range board(t, app).Branches {
				if b.CodeRepoID == rec.ID && b.Name == "feat" {
					for _, d := range b.Deployments {
						byEnv[d.Env] = d
					}
				}
			}
			return len(byEnv) == 2 && ok(byEnv)
		})
		return byEnv
	}

	env("bad")
	got := deployments(func(m map[string]adewire.Deployment) bool {
		return m["staging"].CheckedAt != 0 && m["qa"].CheckedAt != 0
	})
	if d := got["staging"]; !strings.Contains(d.Error, "the script printed no commit sha") {
		t.Fatalf("staging = %+v, want the no-sha error", d)
	}
	if d := got["qa"]; d.Status != "unknown" || d.Error != `Run script "Missing sha": not found` {
		t.Fatalf("qa = %+v, want unknown with the not-found error", d)
	}

	env("ok")
	head := gitOut(t, repo.Dir, "rev-parse", "HEAD")
	got = deployments(func(m map[string]adewire.Deployment) bool { return m["staging"].DeployedSha == head })
	if d := got["staging"]; d.Status != "deployed" || d.Error != "" {
		t.Fatalf("staging = %+v, want deployed at %s", d, head)
	}
	app.Contract(t, "ade-repo-script", "AdeTaskService.Branch#deployments", []adewire.Deployment{got["staging"], got["qa"]}, flowharness.Mask("checkedAt", "deployedSha"))
}
