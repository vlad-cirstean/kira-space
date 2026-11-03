package appflow_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
)

func gitStatusOf(gs *flowharness.GitStream) gitrpc.AppInitResult {
	var init gitrpc.AppInitResult
	gs.MustRequest("app.init", nil, &init)
	return init
}

func refreshErrors(t *testing.T, app *flowharness.App) []*adewire.RemoteOpError {
	t.Helper()
	res, err := app.W.AdeTask.Refresh(ctx, adewire.RefreshArgs{})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	out := make([]*adewire.RemoteOpError, 0, len(res.Repos))
	for _, r := range res.Repos {
		out = append(out, r.Error)
	}
	return out
}

func importErr(app *flowharness.App, dir string) error {
	_, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: dir})
	return err
}

// Guards Discovery's cache and the settings propagation: one git.gitPath change reaches the git
// stream, the repo import and the ADE refresh without a restart.
func TestGitPathSettingEverywhere(t *testing.T) {
	app := flowharness.New(t)
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	createTask(t, app, "Fix login", rec.ID)
	gs := app.OpenGitStream()
	if got := gitStatusOf(gs).Git; got.Kind != "ok" {
		t.Fatalf("default git status = %+v, want ok", got)
	}

	// Linux falls back to PATH for a missing file, so an executable that is not git stands in for a bad path.
	bad := filepath.Join(app.Work, "not-git")
	writeExecutable(t, bad, "#!/bin/sh\necho not-git\n")
	setGitPath(t, app, bad)

	app.Contract(t, "git-path", "git:app.init#unusable", gitStatusOf(gs), flowharness.Mask("serverVersion"))
	if got := gitStatusOf(gs).Git; got.Kind != "unusable" || got.Path != bad {
		t.Fatalf("git status after bad path = %+v, want unusable at %s", got, bad)
	}
	extra := app.NewRepo("extra-0")
	extra.Commit("c", map[string]string{"a": "a"})
	if err := importErr(app, extra.Dir); err == nil || !strings.Contains(err.Error(), "E_GIT_UNAVAILABLE") {
		t.Fatalf("ImportRepo with bad git = %v, want E_GIT_UNAVAILABLE", err)
	}
	errs := refreshErrors(t, app)
	if len(errs) != 1 || errs[0] == nil || errs[0].Message == "" {
		t.Fatalf("refresh rows with bad git = %+v, want one classified error", errs)
	}

	for i, path := range []string{realGit(t), ""} {
		setGitPath(t, app, path)
		if got := gitStatusOf(gs).Git; got.Kind != "ok" {
			t.Fatalf("git status after path %q = %+v, want ok", path, got)
		}
		next := app.NewRepo("extra-" + string(rune('1'+i)))
		next.Commit("c", map[string]string{"a": "a"})
		importRepo(t, app, next.Dir)
		if e := refreshErrors(t, app)[0]; e != nil {
			t.Fatalf("refresh after path %q: %+v", path, e)
		}
	}

	t.Run("refresh of an open repo follows the setting", func(t *testing.T) {
		setGitPath(t, app, bad)
		if got := gitStatusOf(gs).Git; got.Kind != "unusable" {
			t.Fatalf("git status = %+v, want unusable", got)
		}
		for i, e := range refreshErrors(t, app) {
			if e == nil {
				t.Fatalf("refresh row %d reports no error with an unusable git", i)
			}
		}
	})
}
