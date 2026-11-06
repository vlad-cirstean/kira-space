package appflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

func TestInfoAndKeepAwake(t *testing.T) {
	app := studioflowharness.New(t)

	info, err := app.W.App.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.AppVersion == "" || !strings.HasPrefix(info.Go, "go") || info.KiraHome != app.KiraHome {
		t.Fatalf("Info = %+v, want a version, a Go version and KIRA_HOME %q", info, app.KiraHome)
	}

	if st := app.W.KeepAwake.Status(); st.Manual || app.KeepAwake.Held() {
		t.Fatalf("keep-awake starts on: %+v", st)
	}
	mark := app.Events.Mark()
	on := app.W.KeepAwake.SetManual(bridge.KeepAwakeSetManualArgs{Enabled: true})
	if !on.Manual || !on.Supported || on.Error != "" || !app.KeepAwake.Held() {
		t.Fatalf("SetManual(true) = %+v, held=%v", on, app.KeepAwake.Held())
	}
	if ev := app.Events.WaitAfter(t, mark, bridge.ChannelKeepAwake, nil, 10*time.Second); ev.Window != "" {
		t.Fatalf("keep-awake change went to window %q, want every window", ev.Window)
	}
	if st := app.W.KeepAwake.Status(); !st.Manual {
		t.Fatalf("Status after SetManual(true) = %+v", st)
	}
	off := app.W.KeepAwake.SetManual(bridge.KeepAwakeSetManualArgs{Enabled: false})
	if off.Manual || app.KeepAwake.Held() || app.KeepAwake.Acquires != 1 || app.KeepAwake.Releases < 1 {
		t.Fatalf("SetManual(false) = %+v, held=%v acquires=%d releases=%d", off, app.KeepAwake.Held(), app.KeepAwake.Acquires, app.KeepAwake.Releases)
	}
}

func TestUpdateRefusedInDevBuild(t *testing.T) {
	app := studioflowharness.New(t)
	st, err := app.W.Update.Status(context.Background())
	if err != nil || st.UpdateAvailable {
		t.Fatalf("Status = %+v (%v), want no update in a dev build", st, err)
	}
	if err := app.W.Update.InstallUpdate(context.Background()); err == nil {
		t.Fatal("InstallUpdate succeeded in a dev build")
	}
	if err := app.W.Update.CancelInstall(); err != nil {
		t.Fatalf("CancelInstall with nothing installing: %v", err)
	}
}

func TestCheckForUpdatesRefusedInDevBuild(t *testing.T) {
	app := studioflowharness.New(t)
	var ie *ipcerr.Error
	if _, err := app.W.Update.Check(context.Background()); !errors.As(err, &ie) || ie.Code != "E_CONFLICT" || ie.Message != "Updates are available only in release builds." {
		t.Fatalf("Check in a dev build = %v, want the coded notRelease error", err)
	}
}

func TestDataGripScan(t *testing.T) {
	app := studioflowharness.New(t)
	var ie *ipcerr.Error
	if _, err := app.W.DataGrip.Scan(bridge.DataGripScanArgs{}); !errors.As(err, &ie) || ie.Code != "E_CONTRACT" {
		t.Fatalf("Scan without a path = %v, want E_CONTRACT", err)
	}
	if _, err := app.W.DataGrip.Scan(bridge.DataGripScanArgs{Path: filepath.Join(app.Home, "no-project")}); !errors.As(err, &ie) || ie.Code != "E_BAD_REQUEST" {
		t.Fatalf("Scan of a folder with no .idea = %v, want E_BAD_REQUEST", err)
	}

	project := filepath.Join(app.Home, "proj")
	src := filepath.Join("..", "..", "datagrip", "testdata", "project-six", ".idea")
	if err := os.MkdirAll(filepath.Join(project, ".idea"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"dataSources.xml", "dataSources.local.xml"} {
		b, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, ".idea", f), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prev, err := app.W.DataGrip.Scan(bridge.DataGripScanArgs{Path: project})
	if err != nil {
		t.Fatal(err)
	}
	if len(prev.Rows) != 6 {
		t.Fatalf("Scan found %d rows, want 6: %+v", len(prev.Rows), prev.Rows)
	}
	kinds := map[string]string{}
	for _, r := range prev.Rows {
		if !r.Importable {
			t.Fatalf("row %q not importable: %s %s", r.Name, r.SkipReason, r.SkipDetail)
		}
		kinds[r.Name] = r.Kind
	}
	if kinds["pg-main"] != "postgres" || kinds["app-coverage"] != "sqlite" || kinds["mongo-catalog"] != "mongodb" {
		t.Fatalf("scanned kinds %v", kinds)
	}
	if list, err := app.W.Connections.List(); err != nil || len(list) != 0 {
		t.Fatalf("Scan created connections: %v %v", list, err)
	}

	if _, err := app.W.DataGrip.Import(bridge.DataGripImportArgs{Path: project}); !errors.As(err, &ie) || ie.Code != "E_CONTRACT" {
		t.Fatalf("Import without a selection = %v, want E_CONTRACT", err)
	}
	var uuids []string
	for _, r := range prev.Rows {
		uuids = append(uuids, r.UUID)
	}
	report, err := app.W.DataGrip.Import(bridge.DataGripImportArgs{Path: project, SelectedUUIDs: uuids})
	if err != nil {
		t.Fatal(err)
	}
	list, err := app.W.Connections.List()
	if err != nil || len(list) != len(uuids) {
		t.Fatalf("Import created %d connections (%v), want %d; report %+v", len(list), err, len(uuids), report)
	}
}

func TestOpenExternal(t *testing.T) {
	app := studioflowharness.New(t)
	open := func(url string) error { return app.W.Link.OpenExternal(bridge.LinkOpenExternalArgs{URL: url}) }

	app.Contract(t, "link-open", "args:LinkService.OpenExternal", bridge.LinkOpenExternalArgs{URL: "https://hub.docker.com/_/alpine"})
	for _, ok := range []string{"https://hub.docker.com/_/alpine", "http://localhost:3000/"} {
		if err := open(ok); err != nil {
			t.Fatal(err)
		}
	}
	for _, bad := range []string{"javascript:alert(1)", "file:///etc/passwd", "ftp://example.com/x", "https://", "example.com", ""} {
		if err := open(bad); err == nil {
			t.Errorf("OpenExternal(%q) succeeded", bad)
		}
	}
	want := []string{"https://hub.docker.com/_/alpine", "http://localhost:3000/"}
	if got := app.Browser.Opened(); !slices.Equal(got, want) {
		t.Fatalf("browser opened %v, want only %v", got, want)
	}
}

func TestTerminalLinkOpen(t *testing.T) {
	app := studioflowharness.New(t)
	const url = "https://github.com/acme/api/pull/7"
	args := bridge.LinkOpenExternalArgs{URL: url}
	app.Contract(t, "terminal-link", "args:LinkService.OpenExternal", args)
	if err := app.W.Link.OpenExternal(args); err != nil {
		t.Fatal(err)
	}
	if err := app.W.Link.OpenExternal(bridge.LinkOpenExternalArgs{URL: "mailto:dev@example.com"}); err == nil {
		t.Error("OpenExternal(mailto) succeeded")
	}
	if got := app.Browser.Opened(); !slices.Equal(got, []string{url}) {
		t.Fatalf("browser opened %v, want only [%s]", got, url)
	}
}
