package appflow_test

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
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
