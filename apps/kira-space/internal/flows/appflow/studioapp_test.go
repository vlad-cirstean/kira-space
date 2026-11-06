package appflow_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

func TestAppInfo(t *testing.T) {
	app := flowharness.New(t)

	info, err := app.W.App.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.AppVersion == "" || !strings.HasPrefix(info.Go, "go") || info.KiraHome != app.KiraHome {
		t.Fatalf("Info = %+v, want a version, a Go version and KIRA_HOME %q", info, app.KiraHome)
	}
}

func TestDataGripScan(t *testing.T) {
	app := flowharness.New(t)
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
