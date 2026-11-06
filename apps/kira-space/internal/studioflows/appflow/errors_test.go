package appflow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

func sqliteInput(name, path string) connections.Input {
	db := path
	return connections.Input{ConnectionFields: model.ConnectionFields{
		Name: name, Kind: "sqlite", Color: "blue", Mode: "fields", Database: &db,
		Options: map[string]any{}, McpReadMode: "deny", McpWriteMode: "deny", McpDdlMode: "deny",
	}}
}

func wantCode(t *testing.T, err error, code string) *ipcerr.Error {
	t.Helper()
	var ie *ipcerr.Error
	if !errors.As(err, &ie) || ie.Code != code {
		t.Fatalf("error = %v, want code %s", err, code)
	}
	return ie
}

// The UI spec tests/ui/consistency.spec.ts asserts the same relative-path text in the dialog.
func TestRelativeSQLitePathRefused(t *testing.T) {
	app := studioflowharness.New(t)
	_, err := app.W.Connections.Create(sqliteInput("rel", "data/app.db"))
	ie := wantCode(t, err, "E_BAD_REQUEST")
	if want := i18n.T("backend.studio.connection.databasePathAbsolute.body", nil); ie.Message != want {
		t.Fatalf("message = %q, want %q", ie.Message, want)
	}
}

func TestDeletedSavedQueryIsNotFound(t *testing.T) {
	app := studioflowharness.New(t)
	conn, err := app.W.Connections.Create(sqliteInput("q", filepath.Join(t.TempDir(), "q.db")))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := app.W.Queries.Save(bridge.QueriesSaveArgs{ConnectionID: conn.ID, Path: "main/t", Name: "gone", Body: model.FilterBody{}})
	if err != nil {
		t.Fatalf("Queries.Save: %v", err)
	}
	if err := app.W.Queries.Delete(bridge.QueriesIDArgs{ID: saved.ID}); err != nil {
		t.Fatalf("Queries.Delete: %v", err)
	}
	pinned := true
	_, err = app.W.Queries.Update(bridge.QueriesUpdateArgs{ID: saved.ID, Pinned: &pinned})
	ie := wantCode(t, err, "E_NOT_FOUND")
	if want := i18n.T("backend.studio.query.notFound.body", nil); ie.Message != want {
		t.Fatalf("message = %q, want %q", ie.Message, want)
	}
}

func TestTabsSaveWithoutWindowKeyIsContract(t *testing.T) {
	app := studioflowharness.New(t)
	wantCode(t, app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: ""}), "E_CONTRACT")
	_, err := app.W.Tabs.List(bridge.TabsListArgs{WindowKey: ""})
	wantCode(t, err, "E_CONTRACT")
}

// Driver text is shown as the driver wrote it: no catalog wrapper, no rewording.
func TestAdapterFailureKeepsDriverText(t *testing.T) {
	app := studioflowharness.New(t)
	garbage := filepath.Join(t.TempDir(), "garbage.db")
	if err := os.WriteFile(garbage, []byte(strings.Repeat("not a sqlite file ", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	conn, err := app.W.Connections.Create(sqliteInput("garbage", garbage))
	if err != nil {
		t.Fatal(err)
	}
	st, err := app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: conn.ID})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if st.Status != "error" || st.Error == nil || !strings.Contains(*st.Error, "file is not a database") {
		t.Fatalf("state = %+v, want status error carrying the driver text %q", st, "file is not a database")
	}
}
