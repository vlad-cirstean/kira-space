package configsyncflow_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/configsync"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

const reposScenario = "config-sync-repos"

const acmeShared = `origin: https://example.com/acme/app.git
worktree:
  prepare_script: bun install
  prepare_timeout: 15m
integration_branches: [develop]
environments:
  - name: staging
    deployed_sha_script: curl -fsS https://staging.example.com/version
review:
  base_candidates: [main, develop]
`

const leakyShared = `origin: https://example.com/leak/x.git
environments:
  - name: prod
    deployed_sha_script: echo ghp_0123456789abcdefghijklmnopqrstuvwxyz
`

func importRepoAt(t *testing.T, app *flowharness.App, r *flowharness.Repo) (id, repoID string) {
	t.Helper()
	rec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: r.Dir})
	if err != nil {
		t.Fatalf("ImportRepo: %v", err)
	}
	return rec.ID, rec.RepoID
}

func repoWire(t *testing.T, app *flowharness.App, codeRepoID string) adewire.Repo {
	t.Helper()
	res, err := app.W.AdeTask.Repos(ctx)
	if err != nil {
		t.Fatalf("Repos: %v", err)
	}
	for _, r := range res.Repos {
		if r.CodeRepoID == codeRepoID {
			return r
		}
	}
	t.Fatalf("Repos has no %s", codeRepoID)
	return adewire.Repo{}
}

func rowOf(p configsync.Preview, id string) (configsync.Row, bool) {
	for _, r := range p.Rows {
		if r.ID == id {
			return r, true
		}
	}
	return configsync.Row{}, false
}

func fetchPreview(t *testing.T, svc *bridge.ConfigSyncService) configsync.Preview {
	t.Helper()
	if _, err := svc.FetchNow(bridge.ConfigSyncFetchArgs{WindowKey: "w1"}); err != nil {
		t.Fatal(err)
	}
	pv, err := svc.Preview()
	if err != nil {
		t.Fatal(err)
	}
	return pv
}

func reposFiles(t *testing.T, app *flowharness.App) []string {
	t.Helper()
	ents, err := os.ReadDir(app.ConfigDir("repos"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".yaml") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestConfigSyncRepos(t *testing.T) {
	app := flowharness.New(t)
	svc := app.W.ConfigSyncSvc
	reposDir := app.ConfigDir("repos")

	// Local repositories: A matches the shared origin; B has no origin; C has two origin URLs.
	a := app.NewRepo("a")
	a.Commit("init", map[string]string{"a.txt": "a\n"})
	a.AddRemote("git@example.com:acme/app.git")
	aID, aPath := importRepoAt(t, app, a)
	b := app.NewRepo("b")
	b.Commit("init", map[string]string{"b.txt": "b\n"})
	_, bPath := importRepoAt(t, app, b)
	c := app.NewRepo("c")
	c.Commit("init", map[string]string{"c.txt": "c\n"})
	c.AddRemote("https://example.com/acme/app.git")
	c.Git("config", "--add", "remote.origin.url", "https://other.example/acme/app")
	_, cPath := importRepoAt(t, app, c)

	aLocal := "# local notes\npath: " + aPath + "\nnickname: Alpha\ngraph:\n  page_size: 150\nworktree:\n  base_path: " + filepath.Join(app.Work, "trees") + "\n"
	write(t, filepath.Join(reposDir, "a.yaml"), aLocal)
	app.W.RepoConfig.Reload()

	shared := app.NewRepo("shared")
	shared.Commit("init", map[string]string{
		"kira-config.yaml":     "version: 1\nmodules: [repos]\n",
		"repos/acme-app.yaml":  acmeShared,
		"repos/other.yaml":     "origin: https://example.com/other/thing.git\nintegration_branches: [develop]\n",
		"repos/leaky.yaml":     leakyShared,
		"repos/local-key.yaml": "path: /somewhere/else\norigin: https://example.com/local/key.git\n",
	})
	st, err := svc.SetRepo(bridge.ConfigSyncSetRepoArgs{URL: "file://" + shared.Dir, Branch: "main", WindowKey: "w1"})
	if err != nil {
		t.Fatalf("SetRepo: %v", err)
	}

	// Status lists the repos module in combined mode, without whole-file sharing.
	var mod *configsync.ModuleInfo
	for i := range st.Modules {
		if st.Modules[i].Name == "repos" {
			mod = &st.Modules[i]
		}
	}
	if mod == nil || mod.Mode != configsync.ModeCombined || strings.Join(mod.Modes, ",") != "local,combined" {
		t.Fatalf("repos module = %+v", mod)
	}
	app.Contract(t, reposScenario, "ConfigSyncService.Status#repos", st,
		flowharness.Omit("url", "fetched", "fetchedAt", "appliedAt"))
	if _, err := svc.SetMode(bridge.ConfigSyncModeArgs{Module: "repos", Mode: configsync.ModeShared}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("repos shared mode = %v, want E_BAD_REQUEST", err)
	}

	// Preview: A gets entry rows; C is blocked by its origins; unmatched, leaky and local-key files show.
	pv, err := svc.Preview()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, reposScenario, "ConfigSyncService.Preview#repos", pv, flowharness.Omit("token", "commit"))
	for _, e := range []struct {
		entry string
		runs  bool
	}{{"worktree/prepare", true}, {"worktree/prepare_timeout", false}, {"integration_branches", false}, {"environments/staging", true}, {"review/base_candidates", false}} {
		id := "repos/acme-app.yaml#" + e.entry + "@a.yaml"
		r, ok := rowOf(pv, id)
		if !ok || r.Kind != configsync.KindAdd || r.RunsCommands != e.runs || r.Target != "a.yaml" || r.Repo != aPath || r.Origin != "example.com/acme/app" {
			t.Fatalf("row %s = %+v (found %v)", id, r, ok)
		}
	}
	if r, ok := rowOf(pv, "repos/acme-app.yaml@"+cPath); !ok || r.Kind != configsync.KindBlocked || r.Repo != cPath {
		t.Fatalf("several-origins row = %+v (found %v)", r, ok)
	}
	if r, ok := rowOf(pv, "repos/other.yaml"); !ok || r.Kind != configsync.KindUnmatched || r.Origin != "example.com/other/thing" || r.Repo != "" {
		t.Fatalf("unmatched row = %+v (found %v)", r, ok)
	}
	if r, ok := rowOf(pv, "repos/leaky.yaml"); !ok || r.Kind != configsync.KindBlocked || strings.Contains(r.Problem, "ghp_") {
		t.Fatalf("leaky row = %+v (found %v)", r, ok)
	}
	if r, ok := rowOf(pv, "repos/local-key.yaml"); !ok || r.Kind != configsync.KindBlocked {
		t.Fatalf("local-key row = %+v (found %v)", r, ok)
	}
	for _, r := range pv.Rows {
		if r.Repo == bPath {
			t.Fatalf("repo without an origin has a row: %+v", r)
		}
	}
	if len(pv.Files) != 1 || pv.Files[0].File != "a.yaml" || pv.Files[0].Module != "repos" {
		t.Fatalf("diffs = %+v, want the one target file", pv.Files)
	}

	// Apply: the shared keys land, local keys and the comment stay, the board sees the new config.
	res, err := svc.Apply(bridge.ConfigSyncApplyArgs{Token: pv.Token})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	app.Contract(t, reposScenario, "ConfigSyncService.Apply#repos", res, flowharness.Omit("backup"))
	if res.Applied != 1 || res.Backup == "" {
		t.Fatalf("apply result = %+v, want one file and a backup of the replaced one", res)
	}
	text := read(t, filepath.Join(reposDir, "a.yaml"))
	for _, want := range []string{"# local notes", "path: " + aPath, "nickname: Alpha", "page_size: 150", "base_path:", "prepare_script: bun install", "prepare_timeout: 15m", "name: staging", "base_candidates"} {
		if !strings.Contains(text, want) {
			t.Fatalf("a.yaml lost %q:\n%s", want, text)
		}
	}
	if !strings.Contains(read(t, filepath.Join(res.Backup, "repos", "a.yaml")), "# local notes") {
		t.Fatal("backup does not hold the replaced file")
	}
	if cfg := app.W.RepoConfig.Repo(aPath); cfg.Settings.WorktreePrepareScript != "bun install" {
		t.Fatalf("repo config = %+v, want the shared prepare script", cfg.Settings)
	}
	if got := repoWire(t, app, aID); len(got.Environments) != 1 || got.Environments[0].Name != "staging" || got.PrepareScript != "bun install" {
		t.Fatalf("board repo = %+v, want the shared environment", got)
	}
	if files := reposFiles(t, app); len(files) != 1 {
		t.Fatalf("repo files = %v, want only a.yaml (B and C have no file)", files)
	}

	// Copy for shared config: origin plus the shared keys, never a local key.
	shText, err := svc.RepoSharedText(bridge.ConfigSyncRepoArgs{Path: aPath})
	if err != nil {
		t.Fatalf("RepoSharedText: %v", err)
	}
	app.Contract(t, reposScenario, "ConfigSyncService.RepoSharedText", shText)
	if !strings.Contains(shText, "origin: https://example.com/acme/app\n") || !strings.Contains(shText, "prepare_script: bun install") {
		t.Fatalf("shared text:\n%s", shText)
	}
	for _, local := range []string{"nickname", "page_size", "base_path", "path:", "# local notes"} {
		if strings.Contains(shText, local) {
			t.Fatalf("shared text holds local key %q:\n%s", local, shText)
		}
	}
	if _, err := svc.RepoSharedText(bridge.ConfigSyncRepoArgs{Path: bPath}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("RepoSharedText without origin = %v, want E_BAD_REQUEST", err)
	}
	if _, err := svc.RepoSharedText(bridge.ConfigSyncRepoArgs{Path: cPath}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("RepoSharedText with two origins = %v, want E_BAD_REQUEST", err)
	}

	// Conflict: local and shared both change the prepare script; the default keeps mine and the base advances.
	write(t, filepath.Join(reposDir, "a.yaml"), strings.Replace(text, "prepare_script: bun install", "prepare_script: make deps", 1))
	app.W.RepoConfig.Reload()
	shared.Commit("new prepare", map[string]string{"repos/acme-app.yaml": strings.Replace(acmeShared, "bun install", "bun ci", 1)})
	pv = fetchPreview(t, svc)
	conflictID := "repos/acme-app.yaml#worktree/prepare@a.yaml"
	if r, ok := rowOf(pv, conflictID); !ok || r.Kind != configsync.KindConflict {
		t.Fatalf("conflict row = %+v (found %v)", r, ok)
	}
	if res, err = svc.Apply(bridge.ConfigSyncApplyArgs{Token: pv.Token}); err != nil || res.Kept != 1 {
		t.Fatalf("keep mine = %+v, %v", res, err)
	}
	if !strings.Contains(read(t, filepath.Join(reposDir, "a.yaml")), "prepare_script: make deps") {
		t.Fatal("keep mine changed the local prepare script")
	}
	if pv = fetchPreview(t, svc); func() bool { _, ok := rowOf(pv, conflictID); return ok }() {
		t.Fatal("conflict returned after the base advanced")
	}

	// Deactivated entry: a new shared environment that is switched off is listed, never applied.
	if _, err := svc.SetDeactivated(bridge.ConfigSyncDeactivateArgs{Module: "repos", Unit: "acme-app.yaml#environments/qa", Off: true}); err != nil {
		t.Fatal(err)
	}
	withQA := strings.Replace(strings.Replace(acmeShared, "bun install", "bun ci", 1), "review:", "  - name: qa\n    deployed_sha_script: echo qa\nreview:", 1)
	shared.Commit("add qa", map[string]string{"repos/acme-app.yaml": withQA})
	pv = fetchPreview(t, svc)
	if r, ok := rowOf(pv, "repos/acme-app.yaml#environments/qa@a.yaml"); !ok || !r.Deactivated {
		t.Fatalf("qa row = %+v (found %v)", r, ok)
	}
	if _, err := svc.Apply(bridge.ConfigSyncApplyArgs{Token: pv.Token}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, filepath.Join(reposDir, "a.yaml")), "name: qa") {
		t.Fatal("deactivated environment applied")
	}

	// A repository registered later gets its own file on the next refresh.
	d := app.NewRepo("d")
	d.Commit("init", map[string]string{"d.txt": "d\n"})
	d.AddRemote("https://example.com/acme/app")
	_, dPath := importRepoAt(t, app, d)
	pv, err = svc.Preview()
	if err != nil {
		t.Fatal(err)
	}
	addD := "repos/acme-app.yaml#worktree/prepare@d.yaml"
	if r, ok := rowOf(pv, addD); !ok || r.Kind != configsync.KindAdd || r.Target != "d.yaml" || r.Repo != dPath {
		t.Fatalf("late repo row = %+v (found %v)", r, ok)
	}
	if _, ok := rowOf(pv, "repos/acme-app.yaml#environments/staging@a.yaml"); ok {
		t.Fatal("repo A has a staging row although it matches shared")
	}
	if _, err := svc.Apply(bridge.ConfigSyncApplyArgs{Token: pv.Token}); err != nil {
		t.Fatal(err)
	}
	dText := read(t, filepath.Join(reposDir, "d.yaml"))
	for _, want := range []string{"path: " + dPath, "prepare_script: bun ci", "name: staging"} {
		if !strings.Contains(dText, want) {
			t.Fatalf("d.yaml lacks %q:\n%s", want, dText)
		}
	}
	if strings.Contains(dText, "name: qa") {
		t.Fatal("d.yaml holds the deactivated environment")
	}

	// Upstream deletes the shared file: unchanged entries go, the conflicting local prepare script stays.
	shared.Git("rm", "-q", "repos/acme-app.yaml")
	shared.Commit("drop acme", nil)
	pv = fetchPreview(t, svc)
	if r, ok := rowOf(pv, "repos/acme-app.yaml#environments/staging@a.yaml"); !ok || r.Kind != configsync.KindDelete {
		t.Fatalf("delete row for A = %+v (found %v)", r, ok)
	}
	if r, ok := rowOf(pv, "repos/acme-app.yaml#worktree/prepare@a.yaml"); !ok || r.Kind != configsync.KindConflict {
		t.Fatalf("prepare row for A = %+v (found %v)", r, ok)
	}
	if r, ok := rowOf(pv, "repos/acme-app.yaml#worktree/prepare@d.yaml"); !ok || r.Kind != configsync.KindDelete {
		t.Fatalf("delete row for D = %+v (found %v)", r, ok)
	}
	if _, err := svc.Apply(bridge.ConfigSyncApplyArgs{Token: pv.Token}); err != nil {
		t.Fatal(err)
	}
	aAfter := read(t, filepath.Join(reposDir, "a.yaml"))
	for _, want := range []string{"# local notes", "nickname: Alpha", "page_size: 150", "prepare_script: make deps"} {
		if !strings.Contains(aAfter, want) {
			t.Fatalf("a.yaml lost %q after the delete:\n%s", want, aAfter)
		}
	}
	for _, gone := range []string{"name: staging", "base_candidates", "integration_branches", "prepare_timeout"} {
		if strings.Contains(aAfter, gone) {
			t.Fatalf("a.yaml keeps %q after the delete:\n%s", gone, aAfter)
		}
	}
	dAfter := read(t, filepath.Join(reposDir, "d.yaml"))
	if !strings.Contains(dAfter, "path: "+dPath) || strings.Contains(dAfter, "prepare_script") || strings.Contains(dAfter, "staging") {
		t.Fatalf("d.yaml after the delete:\n%s", dAfter)
	}
	pv, err = svc.Preview()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range pv.Rows {
		if strings.HasPrefix(r.ID, "repos/acme-app.yaml") {
			t.Fatalf("row of the deleted shared file after apply: %+v", r)
		}
	}
}

func errCode(err error) string {
	var e *ipcerr.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
