package adeflow_test

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func TestRepoEnvironments(t *testing.T) {
	app := flowharness.New(t)
	repo, _ := cloneWithMain(app, app.Work+"/envs")
	rec := importRepo(t, app, repo.Dir)
	repo.Git("checkout", "-q", "-b", "feat")
	repo.Commit("feat 1", map[string]string{"f.txt": "1\n"})
	repo.Git("checkout", "-q", "main")
	if _, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: rec.ID, Name: "feat"}); err != nil {
		t.Fatal(err)
	}
	// main now holds the branch tip, so the prod script's HEAD contains it.
	repo.Git("merge", "-q", "--ff-only", "feat")

	setRepoFile(t, app, rec.ID, func(rf *repocfg.File) {
		rf.Environments = []repocfg.Environment{
			{Name: "prod", DeployedSHAScript: "git rev-parse HEAD"},
			{Name: "broken", DeployedSHAScript: "true"},
		}
	})

	head := gitOut(t, repo.Dir, "rev-parse", "HEAD")
	var br adewire.Branch
	testx.WaitUntil(t, waitFor, func() bool {
		for _, b := range board(t, app).Branches {
			if b.CodeRepoID == rec.ID && b.Name == "feat" {
				br = b
			}
		}
		return len(br.Deployments) == 2 && br.Deployments[0].CheckedAt != 0 && br.Deployments[1].CheckedAt != 0
	})
	byEnv := map[string]adewire.Deployment{}
	for _, d := range br.Deployments {
		byEnv[d.Env] = d
	}
	if d := byEnv["prod"]; d.DeployedSha != head || d.Error != "" {
		t.Fatalf("prod = %+v, want the repo HEAD %s", d, head)
	}
	if d := byEnv["broken"]; !strings.Contains(d.Error, "the script printed no commit sha") {
		t.Fatalf("broken = %+v, want the no-sha error", d)
	}
	app.Contract(t, "repo-env", "AdeTaskService.Branch#deployments", br.Deployments, flowharness.Mask("checkedAt", "deployedSha"))
}
