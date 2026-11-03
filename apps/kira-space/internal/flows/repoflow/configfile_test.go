package repoflow_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const eventWait = 5 * time.Second

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

func waitRepo(t *testing.T, app *flowharness.App, codeRepoID string, cond func(adewire.Repo) bool) adewire.Repo {
	t.Helper()
	var r adewire.Repo
	waitFor(t, 10*time.Second, func() bool {
		r = repoWire(t, app, codeRepoID)
		return cond(r)
	})
	return r
}

func repoText(key string, lines ...string) string {
	return "path: " + key + "\n" + strings.Join(lines, "\n") + "\n"
}

func TestRepoConfigFile(t *testing.T) {
	app := flowharness.New(t)
	r := app.NewRepo("proj")
	r.Commit("init", map[string]string{"a.txt": "a\n"})
	rec := importVia(t, app, r.Dir)
	key := rec.RepoID
	svc := app.W.RepoConfigSvc
	dir := filepath.Join(app.SpaceHome, repocfg.Folder)
	mask := flowharness.Mask("hash")

	var file string
	var hash string

	t.Run("a repo without a file gets a template and Save creates the file", func(t *testing.T) {
		if got := repoWire(t, app, rec.ID); got.File != "" || got.Problem != nil || got.Nickname != "" {
			t.Fatalf("fresh repo = %+v, want defaults and no file", got)
		}
		tpl, err := svc.File(repocfg.FileArgs{Path: key})
		if err != nil || tpl.Exists || tpl.File != "" || !strings.Contains(tpl.Text, "path: "+key) {
			t.Fatalf("File = %+v, %v, want a template for %s", tpl, err, key)
		}
		app.Contract(t, "repo-config-file", "RepoConfigService.File#none", tpl, mask)

		mark := app.Events.Mark()
		saved, err := svc.SaveFile(repocfg.SaveArgs{Path: key, Text: repoText(key,
			"nickname: Web",
			"integration_branches: [develop]",
			"environments:",
			"  - name: prod",
			"    deployed_sha_script: git rev-parse HEAD",
			"worktree:",
			"  prepare_script: bun install",
			"graph:",
			"  page_size: 150",
		)})
		if err != nil || !saved.Exists || saved.Problem != nil || saved.Hash == "" {
			t.Fatalf("SaveFile = %+v, %v, want a created, valid file", saved, err)
		}
		file, hash = saved.File, saved.Hash
		app.Contract(t, "repo-config-file", "RepoConfigService.SaveFile", saved, mask)
		app.Events.WaitAfter(t, mark, bridge.ChannelRepoConfigChanged, nil, eventWait)

		got := waitRepo(t, app, rec.ID, func(r adewire.Repo) bool { return r.Nickname == "Web" })
		if got.File != strings.TrimSuffix(file, ".yaml") || got.PrepareScript != "bun install" ||
			!slices.Equal(got.IntegrationBranches, []string{"develop"}) || len(got.Environments) != 1 || got.Environments[0].Name != "prod" {
			t.Fatalf("Repos = %+v, want the saved values", got)
		}
		app.Contract(t, "repo-config-file", "AdeTaskService.Repos#repo", got)
		if s, err := app.W.RepoConfig.Settings(key); err != nil || s.GraphPageSize != 150 {
			t.Fatalf("Settings = %+v, %v, want graph page size 150", s, err)
		}

		list, err := svc.List()
		if err != nil || len(list.Files) != 1 || list.Files[0].File != file || list.Files[0].Repo != rec.ID {
			t.Fatalf("List = %+v, %v, want the one file bound to %s", list, err, rec.ID)
		}
		app.Contract(t, "repo-config-file", "RepoConfigService.List", list)

		_, err = svc.SaveFile(repocfg.SaveArgs{Path: key, Text: repoText(key)})
		if errCode(err) != "E_CONFLICT" {
			t.Fatalf("second create for the same path = %v, want E_CONFLICT", err)
		}
	})

	t.Run("an edit on disk reloads and pushes", func(t *testing.T) {
		mark := app.Events.Mark()
		mustWrite(t, filepath.Join(dir, file), []byte(repoText(key, "nickname: Site", "integration_branches: [develop]")))
		app.Events.WaitAfter(t, mark, bridge.ChannelRepoConfigChanged, nil, eventWait)
		got := waitRepo(t, app, rec.ID, func(r adewire.Repo) bool { return r.Nickname == "Site" })
		if got.PrepareScript != "" || len(got.Environments) != 0 {
			t.Fatalf("Repos = %+v, want leaves the file dropped back at their defaults", got)
		}
		v, err := svc.File(repocfg.FileArgs{Path: key})
		if err != nil || v.Hash == hash || !strings.Contains(v.Text, "nickname: Site") {
			t.Fatalf("File after the edit = %+v, %v", v, err)
		}
		hash = v.Hash
	})

	t.Run("an invalid save is kept, flagged, and the last good values stay in use", func(t *testing.T) {
		broken, err := svc.SaveFile(repocfg.SaveArgs{Path: key, File: file, BaseHash: hash, Text: repoText(key, "nickname: Broken", "bogus: 1")})
		if err != nil || broken.Problem == nil || !broken.LastGood || broken.Problem.Line == 0 {
			t.Fatalf("SaveFile = %+v, %v, want the file kept with a problem and a last good", broken, err)
		}
		app.Contract(t, "repo-config-file", "RepoConfigService.File#broken", broken, mask)
		got := waitRepo(t, app, rec.ID, func(r adewire.Repo) bool { return r.Problem != nil })
		if got.Nickname != "Site" || got.Problem.Line != broken.Problem.Line {
			t.Fatalf("Repos = %+v, want nickname Site (last good) and the file's problem", got)
		}
		app.Contract(t, "repo-config-file", "AdeTaskService.Repos#problem", got.Problem)

		fixed, err := svc.SaveFile(repocfg.SaveArgs{Path: key, File: file, BaseHash: broken.Hash, Text: repoText(key, "nickname: Fixed")})
		if err != nil || fixed.Problem != nil {
			t.Fatalf("SaveFile fix = %+v, %v", fixed, err)
		}
		hash = fixed.Hash
		waitRepo(t, app, rec.ID, func(r adewire.Repo) bool { return r.Problem == nil && r.Nickname == "Fixed" })
	})

	t.Run("a save against a changed file is refused", func(t *testing.T) {
		mustWrite(t, filepath.Join(dir, file), []byte(repoText(key, "nickname: Elsewhere")))
		_, err := svc.SaveFile(repocfg.SaveArgs{Path: key, File: file, BaseHash: hash, Text: repoText(key, "nickname: Mine")})
		if errCode(err) != "E_CONFLICT" {
			t.Fatalf("stale save = %v, want E_CONFLICT", err)
		}
		app.Contract(t, "repo-config-file", "RepoConfigService.SaveFile#conflict", map[string]string{"code": errCode(err)})
		v, err := svc.File(repocfg.FileArgs{Path: key})
		if err != nil || !strings.Contains(v.Text, "Elsewhere") {
			t.Fatalf("File = %+v, %v, want the disk text kept", v, err)
		}
		hash = v.Hash
	})

	t.Run("a second file for the same path applies nothing and is flagged", func(t *testing.T) {
		mark := app.Events.Mark()
		dup := filepath.Join(dir, "zz-duplicate.yaml")
		mustWrite(t, dup, []byte(repoText(key, "nickname: Duplicate")))
		app.Events.WaitAfter(t, mark, bridge.ChannelRepoConfigChanged, nil, eventWait)
		var item repocfg.FileItem
		waitFor(t, 10*time.Second, func() bool {
			list, err := svc.List()
			if err != nil {
				t.Fatal(err)
			}
			for _, it := range list.Files {
				if it.File == "zz-duplicate.yaml" {
					item = it
				}
			}
			return item.Problem != nil
		})
		if item.Repo != "" {
			t.Fatalf("duplicate item = %+v, want no repo bound", item)
		}
		app.Contract(t, "repo-config-file", "RepoConfigService.List#duplicate", item)
		if got := repoWire(t, app, rec.ID); got.Nickname != "Elsewhere" || got.Problem != nil {
			t.Fatalf("Repos = %+v, want the first file to apply", got)
		}
		if err := os.Remove(dup); err != nil {
			t.Fatal(err)
		}
		waitFor(t, 10*time.Second, func() bool {
			list, _ := svc.List()
			return len(list.Files) == 1
		})
	})

	t.Run("integration branches are read as written", func(t *testing.T) {
		mustWrite(t, filepath.Join(dir, file), []byte(repoText(key, "integration_branches: [develop, staging]")))
		got := waitRepo(t, app, rec.ID, func(r adewire.Repo) bool { return len(r.IntegrationBranches) == 2 })
		if !slices.Equal(got.IntegrationBranches, []string{"develop", "staging"}) {
			t.Fatalf("integration branches = %v, want [develop staging]", got.IntegrationBranches)
		}
	})

	t.Run("the files survive a restart, and deleting one resets the repo", func(t *testing.T) {
		app.Restart()
		if got := repoWire(t, app, rec.ID); !slices.Equal(got.IntegrationBranches, []string{"develop", "staging"}) {
			t.Fatalf("after restart = %+v, want the file applied", got)
		}
		if err := os.Remove(filepath.Join(dir, file)); err != nil {
			t.Fatal(err)
		}
		got := waitRepo(t, app, rec.ID, func(r adewire.Repo) bool { return r.File == "" && len(r.IntegrationBranches) == 0 })
		if len(got.IntegrationBranches) != 0 || got.Nickname != "" || got.Problem != nil {
			t.Fatalf("Repos after the delete = %+v, want defaults", got)
		}
		if s, err := app.W.RepoConfig.Settings(key); err != nil || !reflect.DeepEqual(s, model.DefaultGitRepoSettings()) {
			t.Fatalf("Settings after the delete = %+v, %v, want the defaults", s, err)
		}
	})
}
