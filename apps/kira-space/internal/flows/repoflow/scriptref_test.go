package repoflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// Contract repo-script-ref: tests/ui/ade-v2-repo-script.spec.ts reads these.
func TestRepoScriptRefFile(t *testing.T) {
	app := flowharness.New(t)
	r := app.NewRepo("proj")
	r.Commit("init", map[string]string{"a.txt": "a\n"})
	rec := importVia(t, app, r.Dir)
	key := rec.RepoID
	svc := app.W.RepoConfigSvc

	saved, err := svc.SaveFile(repocfg.SaveArgs{Path: key, Text: repoText(key,
		"environments:",
		"  - name: staging",
		"    deployed_sha_run:",
		"      script: Staging sha",
		"  - name: prod",
		"    deployed_sha_script: curl -fsS https://prod.example.com/version",
		"worktree:",
		"  prepare_run:",
		"    script: Install deps",
		"    params:",
		"      mode: ci",
		"      targets: [api, web]",
	)})
	if err != nil || saved.Problem != nil {
		t.Fatalf("SaveFile = %+v, %v, want a valid file", saved, err)
	}
	waitFor(t, 10*time.Second, func() bool { return app.W.RepoConfig.Repo(key).Settings.WorktreePrepareRun != nil })
	c := app.W.RepoConfig.Repo(key)
	wantPrepare := &model.ScriptRef{Script: "Install deps", Params: map[string]model.ParamValues{"mode": {"ci"}, "targets": {"api", "web"}}}
	if !c.Settings.WorktreePrepareRun.Equal(wantPrepare) {
		t.Fatalf("prepare_run = %+v, want %+v", c.Settings.WorktreePrepareRun, wantPrepare)
	}
	if len(c.Environments) != 2 || !c.Environments[0].DeployedShaRun.Equal(&model.ScriptRef{Script: "Staging sha"}) ||
		c.Environments[0].DeployedShaScript != "" || c.Environments[1].DeployedShaRun != nil || c.Environments[1].DeployedShaScript == "" {
		t.Fatalf("environments = %+v, want one _run and one inline", c.Environments)
	}

	hash := saved.Hash
	problem := func(name, text, wantPath string) {
		t.Helper()
		got, err := svc.SaveFile(repocfg.SaveArgs{Path: key, File: saved.File, BaseHash: hash, Text: text})
		if err != nil || got.Problem == nil {
			t.Fatalf("%s: SaveFile = %+v, %v, want a problem", name, got, err)
		}
		waitFor(t, 10*time.Second, func() bool {
			_, p := app.W.RepoConfig.FileInfo(key)
			return p != nil && p.Path == wantPath
		})
		hash = got.Hash
		app.Contract(t, "repo-script-ref", "RepoConfigService.File#"+name, got.Problem)
	}
	problem("prepare-both", repoText(key, "worktree:", "  prepare_script: bun install", "  prepare_run: { script: Install deps }"), "/worktree/prepare_run")
	problem("env-neither", repoText(key, "environments:", "  - name: staging"), "/environments/0")
	problem("env-both", repoText(key, "environments:", "  - name: staging", "    deployed_sha_script: echo x", "    deployed_sha_run: { script: Sha }"), "/environments/0")
	problem("script-blank", repoText(key, "worktree:", "  prepare_run: { script: '  ' }"), "/worktree/prepare_run/script")
	problem("param-name", repoText(key, "worktree:", "  prepare_run: { script: Install deps, params: { Mode: ci } }"), "/worktree/prepare_run/params/Mode")

	docs, err := os.ReadFile(filepath.Join(app.ConfigDir(repocfg.Folder), configfs.DocsFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"prepare_run", "deployed_sha_run"} {
		if !strings.Contains(string(docs), want) {
			t.Fatalf("AGENTS.md does not name %s", want)
		}
	}
}
