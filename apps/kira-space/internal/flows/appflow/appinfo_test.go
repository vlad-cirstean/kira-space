package appflow_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/appupdate"
)

func TestAppInfo(t *testing.T) {
	app := flowharness.New(t)

	info, err := app.W.App.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.AppVersion == "" || !strings.HasPrefix(info.Go, "go") || info.KiraHome != app.SpaceHome {
		t.Fatalf("Info = %+v, want a version, a Go version and the Space home %q", info, app.SpaceHome)
	}
}

// UI half: tests/ui/settings-about.spec.ts.
func TestAboutGitHubLink(t *testing.T) {
	app := flowharness.New(t)

	info, err := app.W.App.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.RepoURL != appupdate.RepoURL {
		t.Fatalf("RepoURL = %q, want %q", info.RepoURL, appupdate.RepoURL)
	}
	app.Contract(t, "settings-about", "AppService.Info#about", map[string]string{
		"appVersion": info.AppVersion,
		"repoUrl":    info.RepoURL,
	})

	args := bridge.LinkOpenExternalArgs{URL: info.RepoURL}
	app.Contract(t, "settings-about", "args:LinkService.OpenExternal", args)
	if err := app.W.Link.OpenExternal(args); err != nil {
		t.Fatal(err)
	}
	if got := app.Browser.Opened(); !slices.Equal(got, []string{info.RepoURL}) {
		t.Fatalf("opened = %v, want [%s]", got, info.RepoURL)
	}
}
