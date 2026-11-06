package dbflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"

	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const (
	fileSecret = "pw-connection-files-5d2"
	waitFor    = 5 * time.Second
)

// connectionNamed returns the listed connection with id.
func connectionNamed(t *testing.T, app *studioflowharness.App, id string) (model.ConnectionSummary, bool) {
	t.Helper()
	list, err := app.W.Connections.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.ID == id {
			return c, true
		}
	}
	return model.ConnectionSummary{}, false
}

func fileProblem(t *testing.T, app *studioflowharness.App, file string) *connfiles.ProblemView {
	t.Helper()
	v, err := app.W.ConnectionConfig.View()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range v.Files {
		if f.File == file {
			return f.Problem
		}
	}
	return nil
}

// Contract connection-files, read by tests/ui/connection-files.spec.ts: a connection is a file, and
// its color, name and left panel filters follow the file.
func TestConnectionConfigFile(t *testing.T) {
	app := studioflowharness.New(t)
	const sc = "connection-files"
	cfg := app.W.ConnectionConfig
	paths := studioflowharness.Replace(app.KiraHome, "<home>")
	dbPath := newSQLiteFile(t)
	times := studioflowharness.Mask("createdAt", "updatedAt")
	dbFile := studioflowharness.Replace(dbPath, "<db>")

	// 1. Creating a connection writes its file.
	conn := createConn(t, app, sqliteInput("Local", dbPath))
	if conn.File != "local.yaml" {
		t.Fatalf("file = %q, want local.yaml", conn.File)
	}
	file := filepath.Join(app.W.ConnectionFiles.Dir(), conn.File)
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("connection file missing: %v", err)
	}
	list, err := app.W.Connections.List()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "ConnectionsService.List", list, times, dbFile)
	view, err := cfg.View()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "ConnectionConfigService.View", view, paths)

	// 2. An external atomic write moves the color and the name with the file.
	text, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.NewReplacer("name: Local", "name: Local (edited)", "color: blue", "color: green").Replace(string(text))
	if edited == string(text) {
		t.Fatalf("the edit changed nothing in:\n%s", text)
	}
	if err := configfs.WriteAtomic(file, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		c, ok := connectionNamed(t, app, conn.ID)
		return ok && c.Name == "Local (edited)" && c.Color == "green"
	})
	list, err = app.W.Connections.List()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "ConnectionsService.List#edited", list, times, dbFile)

	// 3. A bad edit keeps the last good row and refuses app writes until fixed.
	bad := strings.Replace(edited, "kind: sqlite", "kind: nosuchkind", 1)
	if err := configfs.WriteAtomic(file, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return fileProblem(t, app, conn.File) != nil })
	if p := fileProblem(t, app, conn.File); p.Line == 0 || p.Message == "" {
		t.Errorf("problem = %+v, want a line and a message", p)
	}
	view, err = cfg.View()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "ConnectionConfigService.View#bad", view, paths)
	if c, _ := connectionNamed(t, app, conn.ID); c.Name != "Local (edited)" || c.Color != "green" {
		t.Errorf("row after the bad edit = %+v, want the last good name and color", c)
	}
	_, err = app.W.Filters.Replace(bridge.FiltersReplaceArgs{ConnectionID: conn.ID, Visibility: model.TreeVisibility{HiddenKinds: []string{"view"}}})
	if ie := testx.AsIpcErr(t, err); ie.Code != "E_BAD_REQUEST" {
		t.Errorf("Filters.Replace on a broken file = %v, want E_BAD_REQUEST", ie)
	}
	if err := configfs.WriteAtomic(file, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return fileProblem(t, app, conn.File) == nil })

	// 4. The left panel filters live in the file.
	vis := model.TreeVisibility{HiddenKinds: []string{"view"}, HiddenPaths: []string{"main"}}
	got, err := app.W.Filters.Replace(bridge.FiltersReplaceArgs{ConnectionID: conn.ID, Visibility: vis})
	if err != nil || len(got.HiddenKinds) != 1 || got.HiddenKinds[0] != "view" {
		t.Fatalf("Filters.Replace = %+v (%v)", got, err)
	}
	app.Contract(t, sc, "FiltersService.Replace", got)
	if b, _ := os.ReadFile(file); !strings.Contains(string(b), "hidden_kinds") || !strings.Contains(string(b), "(edited)") {
		t.Errorf("file after Filters.Replace lost the filters or the edit:\n%s", b)
	}

	// 5. Save through the file editor: a stale base hash is refused, a good one lands.
	detail, err := cfg.File(bridge.ConnectionFileArgs{File: conn.File})
	if err != nil || detail.ID != conn.ID || !detail.Editable {
		t.Fatalf("File = %+v (%v)", detail, err)
	}
	renamed := strings.Replace(detail.Text, "name: Local (edited)", "name: Local saved", 1)
	_, err = cfg.SaveFile(bridge.ConnectionSaveFileArgs{File: conn.File, Text: renamed, BaseHash: "stale"})
	if ie := testx.AsIpcErr(t, err); ie.Code != "E_CONFLICT" {
		t.Errorf("SaveFile with a stale hash = %v, want E_CONFLICT", ie)
	}
	saved, err := cfg.SaveFile(bridge.ConnectionSaveFileArgs{File: conn.File, Text: renamed, BaseHash: detail.Hash})
	if err != nil || saved.Problem != nil {
		t.Fatalf("SaveFile = %+v (%v)", saved, err)
	}
	if c, _ := connectionNamed(t, app, conn.ID); c.Name != "Local saved" {
		t.Errorf("name after SaveFile = %q, want Local saved", c.Name)
	}

	// 6. Duplicate copies the file; deleting the file removes the row.
	dup, err := app.W.Connections.Duplicate(bridge.ConnectionsIDArgs{ID: conn.ID})
	if err != nil || dup.File != "local-copy.yaml" {
		t.Fatalf("Duplicate = %+v (%v), want local-copy.yaml", dup, err)
	}
	if err := os.Remove(filepath.Join(app.W.ConnectionFiles.Dir(), dup.File)); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { _, ok := connectionNamed(t, app, dup.ID); return !ok })
	if err := app.W.Connections.Delete(bridge.ConnectionsIDArgs{ID: conn.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file after Delete: %v, want it removed", err)
	}
}

// Contract connection-secrets, read by tests/ui/connection-files.spec.ts: a password is a named
// secret the file refers to; no call returns its value but Reveal.
func TestConnectionSecrets(t *testing.T) {
	app := studioflowharness.New(t)
	const sc = "connection-secrets"
	cfg := app.W.ConnectionConfig
	in := sqliteInput("Pg", "")
	in.Kind, in.Database = "postgres", ptr("app")
	in.Host, in.Port, in.Username = ptr("127.0.0.1"), ptr(5432), ptr("kira")
	in.Password = ptr(fileSecret)
	conn := createConn(t, app, in)

	file := filepath.Join(app.W.ConnectionFiles.Dir(), conn.File)
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), fileSecret) || !strings.Contains(string(b), "${secret:pg.password}") {
		t.Fatalf("file must name the secret and not hold it:\n%s", b)
	}

	view, err := cfg.View()
	if err != nil {
		t.Fatal(err)
	}
	want := connfiles.SecretView{Kind: connfiles.KindPassword, Name: "pg.password", Set: true, Referenced: true}
	if len(view.Secrets) != 1 || view.Secrets[0] != want {
		t.Fatalf("secrets = %+v, want %+v", view.Secrets, want)
	}
	app.Contract(t, sc, "ConnectionConfigService.View", view, studioflowharness.Replace(app.KiraHome, "<home>"))
	if j, _ := json.Marshal(view); strings.Contains(string(j), fileSecret) {
		t.Fatal("View carries the secret value")
	}

	reveal := func() connections.RevealResult {
		return app.W.Connections.Reveal(bridge.ConnectionsRevealArgs{ID: conn.ID, Confirmed: true})
	}
	if r := reveal(); r.Outcome != "revealed" || r.Password == nil || *r.Password != fileSecret {
		t.Fatalf("Reveal = %+v, want the password", r)
	}

	// Rotating the secret leaves the file alone.
	before, _ := os.ReadFile(file)
	if _, err := cfg.SetSecret(bridge.ConnectionSecretArgs{Kind: connfiles.KindPassword, Name: "pg.password", Value: "rotated"}); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(file); !bytes.Equal(after, before) {
		t.Errorf("SetSecret rewrote the file:\n%s", after)
	}
	if r := reveal(); r.Password == nil || *r.Password != "rotated" {
		t.Fatalf("Reveal after SetSecret = %+v, want rotated", r)
	}

	// A secret the file names but nothing holds fails the connect by name.
	if _, err := cfg.DeleteSecret(bridge.ConnectionSecretNameArgs{Kind: connfiles.KindPassword, Name: "pg.password"}); err != nil {
		t.Fatal(err)
	}
	view, err = cfg.View()
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Secrets) != 1 || view.Secrets[0].Set || !view.Secrets[0].Referenced {
		t.Fatalf("secrets after delete = %+v, want pg.password unset but referenced", view.Secrets)
	}
	app.Contract(t, sc, "ConnectionConfigService.View#unset", view, studioflowharness.Replace(app.KiraHome, "<home>"))
	st, err := app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: conn.ID})
	if err == nil && (st.Status != "error" || st.Error == nil || !strings.Contains(*st.Error, "pg.password")) {
		t.Fatalf("Connect with an unset secret = %+v, want an error state naming pg.password", st)
	}
	if err != nil && !strings.Contains(err.Error(), "pg.password") {
		t.Fatalf("Connect with an unset secret = %v, want it to name pg.password", err)
	}

	if _, err := cfg.SetSecret(bridge.ConnectionSecretArgs{Kind: "bogus", Name: "x", Value: "v"}); testx.AsIpcErr(t, err).Code != "E_CONTRACT" {
		t.Errorf("SetSecret with an unknown kind = %v, want a contract error", err)
	}
}

// Rows and secrets written before the connection files existed become files at the next start: ids and
// sealed secrets carry over, masks, filters and DDL move into the file, and the superseded columns and
// tables are gone.
func TestConnectionFilesMigration(t *testing.T) {
	app := studioflowharness.New(t)
	const sc = "connection-files-migration"
	const now = "2026-01-01T00:00:00Z"
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := app.DB().Exec(q, args...); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	seal := func(scope secrets.Scope, plain string) string {
		t.Helper()
		sealed, err := secrets.New().Encrypt(scope, plain)
		if err != nil {
			t.Fatal(err)
		}
		return sealed
	}

	// Put the schema back the way it was before the export dropped it.
	exec(`ALTER TABLE connections ADD COLUMN password TEXT`)
	exec(`ALTER TABLE connections ADD COLUMN uri TEXT`)
	exec(`CREATE TABLE connection_tree_filters (connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
		scope TEXT NOT NULL, value TEXT NOT NULL, PRIMARY KEY (connection_id, scope, value))`)
	exec(`CREATE TABLE connection_ddl (connection_id TEXT PRIMARY KEY REFERENCES connections(id) ON DELETE CASCADE,
		ddl TEXT NOT NULL, updated_at TEXT NOT NULL)`)
	exec(`DELETE FROM settings WHERE key = 'config.migrated.connections'`)

	exec(`INSERT INTO connections (id, name, kind, color, mode, host, port, database, username, password, created_at, updated_at)
		VALUES ('11111111-1111-4111-8111-111111111111', 'Legacy Pg', 'postgres', 'red', 'fields', '127.0.0.1', 5432, 'app', 'kira', ?, ?, ?)`,
		seal(secrets.ScopeConnection, fileSecret), now, now)
	exec(`INSERT INTO connections (id, name, kind, color, mode, uri, created_at, updated_at)
		VALUES ('22222222-2222-4222-8222-222222222222', 'Legacy Mongo', 'mongodb', 'none', 'uri', ?, ?, ?)`,
		seal(secrets.ScopeConnectionURI, "mongodb://u:p@127.0.0.1:27017/app"), now, now)
	exec(`INSERT INTO connection_tree_filters (connection_id, scope, value) VALUES ('11111111-1111-4111-8111-111111111111', 'kind', 'view')`)
	exec(`INSERT INTO connection_ddl (connection_id, ddl, updated_at) VALUES ('11111111-1111-4111-8111-111111111111', 'create table t (id int);', ?)`, now)
	exec(`INSERT INTO connection_mask_rules (id, connection_id, table_name, column_name, mask_kind, keep_hint, correlate, created_at, updated_at)
		VALUES ('m1', '11111111-1111-4111-8111-111111111111', '*', 'email', 'redact', 1, 1, ?, ?)`, now, now)

	app.Restart(t)

	dir := app.W.ConnectionFiles.Dir()
	text, err := os.ReadFile(filepath.Join(dir, "legacy-pg.yaml"))
	if err != nil {
		t.Fatalf("connection file: %v", err)
	}
	for _, want := range []string{"name: Legacy Pg", "color: red", "${secret:legacy-pg.password}", "hidden_kinds", "create table t", "email"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("legacy-pg.yaml lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(string(text), fileSecret) {
		t.Errorf("the secret value is in the file:\n%s", text)
	}
	if _, err := os.Stat(filepath.Join(dir, "legacy-mongo.yaml")); err != nil {
		t.Fatalf("uri connection file: %v", err)
	}

	pg, ok := connectionNamed(t, app, "11111111-1111-4111-8111-111111111111")
	if !ok || pg.File != "legacy-pg.yaml" || pg.Color != "red" {
		t.Fatalf("migrated row = %+v (%v), want the same id bound to legacy-pg.yaml", pg, ok)
	}
	if r := app.W.Connections.Reveal(bridge.ConnectionsRevealArgs{ID: pg.ID, Confirmed: true}); r.Password == nil || *r.Password != fileSecret {
		t.Fatalf("Reveal after migration = %+v, want the original password", r)
	}
	if vis, err := app.W.Filters.List(bridge.FiltersListArgs{ConnectionID: pg.ID}); err != nil || len(vis.HiddenKinds) != 1 || vis.HiddenKinds[0] != "view" {
		t.Fatalf("Filters.List = %+v (%v), want the migrated filter", vis, err)
	}
	if ddl, err := app.W.Schema.Get(bridge.SchemaGetArgs{ConnectionID: pg.ID}); err != nil || ddl.DDL != "create table t (id int);" {
		t.Fatalf("Schema.Get = %+v (%v), want the migrated DDL", ddl, err)
	}
	if rules, err := app.W.MaskRules.List(bridge.MaskRulesListArgs{ConnectionID: pg.ID}); err != nil || len(rules) != 1 || rules[0].ColumnName != "email" {
		t.Fatalf("MaskRules.List = %+v (%v), want the migrated rule", rules, err)
	}
	view, err := app.W.ConnectionConfig.View()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, sc, "ConnectionConfigService.View", view, studioflowharness.Replace(app.KiraHome, "<home>"))

	// The superseded columns and tables are gone, and a later start exports nothing again.
	var n int
	if err := app.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('connection_tree_filters', 'connection_ddl')`).Scan(&n); err != nil || n != 0 {
		t.Errorf("legacy tables left = %d (%v), want 0", n, err)
	}
	if err := app.DB().QueryRow(`SELECT COUNT(*) FROM pragma_table_info('connections') WHERE name IN ('password', 'uri')`).Scan(&n); err != nil || n != 0 {
		t.Errorf("legacy columns left = %d (%v), want 0", n, err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "legacy-pg.yaml"))
	if err := os.Remove(filepath.Join(dir, "legacy-pg.yaml")); err != nil {
		t.Fatal(err)
	}
	app.Restart(t)
	if _, err := os.Stat(filepath.Join(dir, "legacy-pg.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a later start rewrote the deleted file (%d bytes before): %v", len(before), err)
	}
}

// A failed file write during the export leaves every legacy row in place, and the next start finishes it.
func TestConnectionFilesMigrationFailedWrite(t *testing.T) {
	app := studioflowharness.New(t)
	const now = "2026-01-01T00:00:00Z"
	ids := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}
	for _, q := range []string{
		`ALTER TABLE connections ADD COLUMN password TEXT`,
		`ALTER TABLE connections ADD COLUMN uri TEXT`,
		`CREATE TABLE connection_tree_filters (connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
			scope TEXT NOT NULL, value TEXT NOT NULL, PRIMARY KEY (connection_id, scope, value))`,
		`CREATE TABLE connection_ddl (connection_id TEXT PRIMARY KEY REFERENCES connections(id) ON DELETE CASCADE,
			ddl TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`DELETE FROM settings WHERE key = 'config.migrated.connections'`,
	} {
		if _, err := app.DB().Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	for i, name := range []string{"Legacy One", "Legacy Two"} {
		if _, err := app.DB().Exec(`INSERT INTO connections (id, name, kind, color, mode, host, port, database, username, created_at, updated_at)
			VALUES (?, ?, 'postgres', 'none', 'fields', '127.0.0.1', 5432, 'app', 'kira', ?, ?)`, ids[i], name, now, now); err != nil {
			t.Fatal(err)
		}
	}

	// An immutable folder makes every file write fail, even for root.
	dir := app.W.ConnectionFiles.Dir()
	app.Quit(t)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("chattr", "+i", dir).Run(); err != nil {
		t.Skipf("cannot make the folder immutable: %v", err)
	}
	unlock := func() { _ = exec.Command("chattr", "-i", dir).Run() }
	t.Cleanup(unlock)
	app.Restart(t)
	for _, id := range ids {
		if _, ok := connectionNamed(t, app, id); !ok {
			t.Fatalf("connection %s is gone after a failed export", id)
		}
	}

	app.Quit(t)
	unlock()
	app.Restart(t)
	for i, file := range []string{"legacy-one.yaml", "legacy-two.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Errorf("rerun did not write %s: %v", file, err)
		}
		if c, ok := connectionNamed(t, app, ids[i]); !ok || c.File != file {
			t.Errorf("connection %s = %+v (%v), want bound to %s", ids[i], c, ok, file)
		}
	}
}

// A duplicate owns its password: changing it leaves the original's alone.
func TestConnectionDuplicateOwnsSecret(t *testing.T) {
	app := studioflowharness.New(t)
	in := sqliteInput("Pg", "")
	in.Kind, in.Database = "postgres", ptr("app")
	in.Host, in.Port, in.Username = ptr("127.0.0.1"), ptr(5432), ptr("kira")
	in.Password = ptr(fileSecret)
	conn := createConn(t, app, in)
	dup, err := app.W.Connections.Duplicate(bridge.ConnectionsIDArgs{ID: conn.ID})
	if err != nil {
		t.Fatal(err)
	}
	password := func(id string) string {
		r := app.W.Connections.Reveal(bridge.ConnectionsRevealArgs{ID: id, Confirmed: true})
		if r.Password == nil {
			t.Fatalf("Reveal %s = %+v, want a password", id, r)
		}
		return *r.Password
	}
	if got := password(dup.ID); got != fileSecret {
		t.Fatalf("duplicate password = %q, want the original's", got)
	}
	if err := app.W.Connections.UpdateCredentials(bridge.ConnectionsCredentialsArgs{ID: dup.ID, Password: ptr("staging")}); err != nil {
		t.Fatal(err)
	}
	if got := password(conn.ID); got != fileSecret {
		t.Errorf("original password = %q after the copy changed, want unchanged", got)
	}
	if got := password(dup.ID); got != "staging" {
		t.Errorf("duplicate password = %q, want staging", got)
	}
}

// A secret name outlives a file rename: duplicating again never overwrites the renamed file's password.
func TestConnectionDuplicateKeepsRenamedFileSecret(t *testing.T) {
	app := studioflowharness.New(t)
	in := sqliteInput("Pg", "")
	in.Kind, in.Database = "postgres", ptr("app")
	in.Host, in.Port, in.Username = ptr("127.0.0.1"), ptr(5432), ptr("kira")
	in.Password = ptr(fileSecret)
	conn := createConn(t, app, in)
	dup, err := app.W.Connections.Duplicate(bridge.ConnectionsIDArgs{ID: conn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.W.Connections.UpdateCredentials(bridge.ConnectionsCredentialsArgs{ID: dup.ID, Password: ptr("staging")}); err != nil {
		t.Fatal(err)
	}
	dir := app.W.ConnectionFiles.Dir()
	if err := os.Rename(filepath.Join(dir, dup.File), filepath.Join(dir, "staging.yaml")); err != nil {
		t.Fatal(err)
	}
	var staging model.ConnectionSummary
	testx.WaitUntil(t, waitFor, func() bool {
		list, _ := app.W.Connections.List()
		for _, c := range list {
			if c.File == "staging.yaml" {
				staging = c
				return true
			}
		}
		return false
	})
	again, err := app.W.Connections.Duplicate(bridge.ConnectionsIDArgs{ID: conn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if again.File == dup.File {
		t.Errorf("second duplicate reused %s, whose secret the renamed file still names", dup.File)
	}
	r := app.W.Connections.Reveal(bridge.ConnectionsRevealArgs{ID: staging.ID, Confirmed: true})
	if r.Password == nil || *r.Password != "staging" {
		t.Errorf("renamed file password = %+v, want staging", r)
	}
}
