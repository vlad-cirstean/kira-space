package dbflow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Contract connection-lifecycle, read by tests/ui/connection-lifecycle.spec.ts: Duplicate copies the
// file with its comments and gives the copy its own password reference, Delete removes file and row,
// and DeleteFile removes a file that never held a connection and refuses one that has a row.
func TestConnectionLifecycle(t *testing.T) {
	app := flowharness.New(t)
	const sc = "connection-lifecycle"
	cfg := app.W.ConnectionConfig
	mask := flowharness.Mask("hash", "baseHash", "createdAt", "updatedAt")
	paths := flowharness.Replace(app.KiraHome, "<home>")

	in := sqliteInput("Pg", "")
	in.Kind, in.Database = "postgres", ptr("app")
	in.Host, in.Port, in.Username = ptr("127.0.0.1"), ptr(5432), ptr("kira")
	in.Password = ptr(fileSecret)
	conn := createConn(t, app, in)
	d, err := cfg.File(bridge.ConnectionFileArgs{File: conn.File})
	if err != nil {
		t.Fatal(err)
	}
	seed := "# Staging database.\n" + strings.Replace(d.Text, "name: Pg", "name: Pg # shown in the sidebar", 1)
	if _, err := cfg.SaveFile(bridge.ConnectionSaveFileArgs{File: conn.File, Text: seed, BaseHash: d.Hash}); err != nil {
		t.Fatal(err)
	}

	argsID := bridge.ConnectionsIDArgs{ID: conn.ID}
	app.Contract(t, sc, "args:ConnectionsService.Duplicate", argsID, mask, paths)
	dup, err := app.W.Connections.Duplicate(argsID)
	if err != nil || dup.File != "pg-copy.yaml" {
		t.Fatalf("Duplicate = %+v, %v, want pg-copy.yaml", dup, err)
	}
	app.Contract(t, sc, "duplicate", dup, mask, paths)
	copyFile, err := cfg.File(bridge.ConnectionFileArgs{File: dup.File})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Staging database.", "# shown in the sidebar", "${secret:pg-copy.password}"} {
		if !strings.Contains(copyFile.Text, want) {
			t.Errorf("copy lacks %q:\n%s", want, copyFile.Text)
		}
	}
	if strings.Contains(copyFile.Text, "${secret:pg.password}") || strings.Contains(copyFile.Text, fileSecret) {
		t.Errorf("copy shares the source's password reference or holds a value:\n%s", copyFile.Text)
	}
	app.Contract(t, sc, "file#copy", copyFile, mask, paths)

	if err := app.W.Connections.Delete(bridge.ConnectionsIDArgs{ID: dup.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(app.W.ConnectionFiles.Dir(), dup.File)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("copy file after Delete: %v, want it removed", err)
	}
	testx.WaitUntil(t, waitFor, func() bool { _, ok := connectionNamed(t, app, dup.ID); return !ok })
	if _, ok := connectionNamed(t, app, conn.ID); !ok {
		t.Fatal("Delete of the copy removed the source row")
	}

	broken := filepath.Join(app.W.ConnectionFiles.Dir(), "broken.yaml")
	if err := os.WriteFile(broken, []byte("kind: nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bf := bridge.ConnectionFileArgs{File: "broken.yaml"}
	testx.WaitUntil(t, waitFor, func() bool {
		v, err := cfg.File(bf)
		return err == nil && v.Problem != nil && v.ID == ""
	})
	view, _ := cfg.File(bf)
	app.Contract(t, sc, "file#broken", view, mask, paths)
	app.Contract(t, sc, "args:ConnectionConfigService.DeleteFile", bf, mask, paths)
	if err := cfg.DeleteFile(bridge.ConnectionFileArgs{File: conn.File}); err == nil {
		t.Fatal("DeleteFile of a file with a connection succeeded")
	}
	if _, err := os.Stat(filepath.Join(app.W.ConnectionFiles.Dir(), conn.File)); err != nil {
		t.Fatalf("DeleteFile refusal removed the file: %v", err)
	}
	if err := cfg.DeleteFile(bf); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(broken); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("broken file after DeleteFile: %v, want it removed", err)
	}
	if _, err := cfg.File(bf); err == nil {
		t.Fatal("File of a deleted file succeeded")
	}
}
