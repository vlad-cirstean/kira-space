package appflow_test

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/homefold"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

var foldSteps = []string{
	"move", "db", "dirs", "tokens", "prompts", "files-done",
	"space-backup", "map", "rows", "run-files", "automations", "studio-delete", "done",
}

type foldFixture struct {
	space, legacy string
	spaceDB       *storage.DB
	opts          homefold.Options
}

func mustWrite(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// newFoldFixture builds a Space home with one script and a legacy Studio home covering every folded
// piece: a colliding script id, a shared and a Studio-only collection, runs with logs, sealed secrets,
// run files, automation folders, tokens and prompt overrides.
func newFoldFixture(t *testing.T) *foldFixture {
	t.Helper()
	root := t.TempDir()
	f := &foldFixture{space: filepath.Join(root, "space"), legacy: filepath.Join(root, "legacy")}

	scratch, err := studiostorage.OpenAt(filepath.Join(root, "scratch"))
	if err != nil {
		t.Fatal(err)
	}
	execAll(t, scratch.DB,
		`INSERT INTO custom_script_collections (name, sort_order) VALUES ('shared', 0), ('only-studio', 1)`,
		`INSERT INTO custom_scripts (id, name, command, sort_order, created_at, updated_at, collection, config_json) VALUES
			('build', 'Build', 'make', 0, 1, 1, 'shared', '{"id":"build"}'),
			('deploy', 'Deploy', 'ship', 1, 1, 1, 'only-studio', '{"id":"deploy"}'),
			('legacy-cfg', 'Legacy', 'old', 2, 1, 1, '', '')`,
		`INSERT INTO script_runs (id, script_id, script_name, trigger_kind, state, cwd, created_at) VALUES
			('r1', 'build', 'Build', 'manual', 'done', '`+f.legacy+`/automations/build/work', 10),
			('r2', 'deploy', 'Deploy', 'manual', 'done', '/elsewhere', 11),
			('run-dup', 'deploy', 'Deploy', 'manual', 'done', '', 12)`,
		`INSERT INTO script_run_logs (run_id, seq, stream, text) VALUES ('r1', 0, 'stdout', 'hi'), ('run-dup', 0, 'stdout', 'dup')`,
		`INSERT INTO secret_values (scope, owner, name, value, updated_ms) VALUES
			('script-param', 'build', 'TOKEN', 'kira:v3:script-param:aaa', 1),
			('script-param', 'deploy', 'KEY', 'kira:v3:script-param:bbb', 1),
			('connection', 'c1', 'password', 'kira:v3:connection:ccc', 1)`,
	)
	if err := scratch.Close(); err != nil {
		t.Fatal(err)
	}
	dbs, _ := filepath.Glob(filepath.Join(root, "scratch", "*.db"))
	if len(dbs) != 1 {
		t.Fatalf("scratch databases = %v", dbs)
	}
	b, err := os.ReadFile(dbs[0])
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(f.legacy, "kira.db"), string(b), 0o600)
	mustWrite(t, filepath.Join(f.legacy, "runs", "build.yaml"), "id: build\n", 0o644)
	mustWrite(t, filepath.Join(f.legacy, "runs", "deploy.yaml"), "id: deploy\n", 0o600)
	mustWrite(t, filepath.Join(f.legacy, "runs", "legacy-cfg.yaml"), "id: legacy-cfg\n", 0o644)
	mustWrite(t, filepath.Join(f.legacy, "automations", "build", "work", "secret.txt"), "s", 0o600)
	if err := os.Symlink("work/secret.txt", filepath.Join(f.legacy, "automations", "build", "link")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(f.legacy, "connections", "a.yaml"), "a: 1\n", 0o600)
	mustWrite(t, filepath.Join(f.legacy, "api", "c", "x.yaml"), "x: 1\n", 0o600)
	mustWrite(t, filepath.Join(f.legacy, "mcp-db-token.json"), `{"t":1}`, 0o600)
	mustWrite(t, filepath.Join(f.legacy, "mcp-db-header.token"), "tok", 0o600)
	mustWrite(t, filepath.Join(f.legacy, "llm-prompts.json"),
		`{"version":1,"revision":3,"overrides":{"a.one":{"text":"studio one"},"a.two":{"text":"studio two"}}}`, 0o600)
	mustWrite(t, filepath.Join(f.legacy, "logs", "x.log"), "log", 0o600)
	mustWrite(t, filepath.Join(f.legacy, "app.lock"), "", 0o600)
	mustWrite(t, filepath.Join(f.legacy, "mcp-header-helper.sh"), "#!/bin/sh\n", 0o700)
	mustWrite(t, filepath.Join(f.legacy, "unknown.txt"), "?", 0o644)

	f.spaceDB, err = storage.OpenAt(f.space)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.spaceDB.Close() })
	execAll(t, f.spaceDB.DB,
		`INSERT INTO custom_script_collections (name, sort_order) VALUES ('shared', 0)`,
		`INSERT INTO custom_scripts (id, name, command, sort_order, created_at, updated_at, collection, config_json) VALUES
			('build', 'Space build', 'cargo', 0, 1, 1, 'shared', '{"id":"build"}')`,
		`INSERT INTO script_runs (id, script_id, script_name, trigger_kind, state, created_at) VALUES ('run-dup', 'build', 'Space build', 'manual', 'done', 1)`,
		`INSERT INTO settings (key, value) VALUES ('config.migrated.runs', '1')`,
	)
	mustWrite(t, filepath.Join(f.space, "runs", "build.yaml"), "id: build\nspace: true\n", 0o644)
	mustWrite(t, filepath.Join(f.space, "llm-prompts.json"),
		`{"version":1,"revision":7,"overrides":{"a.one":{"text":"space one"}}}`, 0o600)

	f.opts = homefold.Options{
		Space: f.spaceDB.DB, SpaceHome: f.space, Legacy: f.legacy,
		Now: func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	}
	return f
}

// fold runs both stages like main does: Files, then Runs against the Studio database it landed.
func (f *foldFixture) fold(t *testing.T) error {
	t.Helper()
	if err := homefold.Files(f.opts); err != nil {
		return err
	}
	studio, err := sqlitex.Open(config.ModuleDbPathAt(f.space))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = studio.Close() }()
	o := f.opts
	o.Studio = studio
	return homefold.Runs(o)
}

var uuidRE = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// treeSnapshot lists every path under root with type, mode and content, skipping databases.
func treeSnapshot(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if strings.Contains(rel, ".db") {
			return nil
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			l, _ := os.Readlink(p)
			lines = append(lines, fmt.Sprintf("%s -> %s", rel, l))
		case info.IsDir():
			lines = append(lines, fmt.Sprintf("%s/ %o", rel, info.Mode().Perm()))
		default:
			b, _ := os.ReadFile(p)
			lines = append(lines, fmt.Sprintf("%s %o %q", rel, info.Mode().Perm(), b))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func dump(t *testing.T, db *sql.DB, queries ...string) string {
	t.Helper()
	var out []string
	for _, q := range queries {
		out = append(out, q+"\n"+scalar(t, db, `SELECT group_concat(r, char(10)) FROM (`+q+` ORDER BY 1)`))
	}
	return uuidRE.ReplaceAllString(strings.Join(out, "\n"), "<uuid>")
}

const spaceRows = `SELECT id || '|' || name || '|' || collection || '|' || sort_order || '|' || config_json AS r FROM custom_scripts`

var spaceQueries = []string{
	`SELECT name || '|' || sort_order AS r FROM custom_script_collections`,
	spaceRows,
	`SELECT id || '|' || script_id || '|' || cwd AS r FROM script_runs`,
	`SELECT run_id || '|' || seq || '|' || text AS r FROM script_run_logs`,
	`SELECT scope || '|' || owner || '|' || name || '|' || value AS r FROM secret_values`,
	`SELECT key || '=' || value AS r FROM settings WHERE key <> 'homefold.p539'`,
}

// stateSnapshot is the Space side of a finished fold: tree plus database rows plus marker.
func (f *foldFixture) stateSnapshot(t *testing.T) string {
	t.Helper()
	marker := scalar(t, f.spaceDB.DB, `SELECT json_remove(value, '$.runs') FROM settings WHERE key = 'homefold.p539'`)
	out := treeSnapshot(t, f.space) + "\n" + dump(t, f.spaceDB.DB, spaceQueries...) + "\n" + uuidRE.ReplaceAllString(marker, "<uuid>")
	return strings.NewReplacer(f.space, "<space>", f.legacy, "<legacy>").Replace(out)
}

// lineDiff lists lines present in only one of a and b.
func lineDiff(a, b string) string {
	in := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, l := range strings.Split(s, "\n") {
			m[l] = true
		}
		return m
	}
	ma, mb := in(a), in(b)
	var out []string
	for _, l := range strings.Split(a, "\n") {
		if !mb[l] {
			out = append(out, "- "+l)
		}
	}
	for _, l := range strings.Split(b, "\n") {
		if !ma[l] {
			out = append(out, "+ "+l)
		}
	}
	return strings.Join(out, "\n")
}

func TestHomeFold(t *testing.T) {
	var want string
	t.Run("full", func(t *testing.T) {
		f := newFoldFixture(t)
		before := treeSnapshot(t, f.legacy)
		if err := f.fold(t); err != nil {
			t.Fatal(err)
		}
		checkFolded(t, f, before)
		want = f.stateSnapshot(t)

		again := f.stateSnapshot(t)
		if err := f.fold(t); err != nil {
			t.Fatal(err)
		}
		if got := f.stateSnapshot(t); got != again {
			t.Errorf("rerun changed state:\n%s", lineDiff(again, got))
		}
	})

	for _, step := range foldSteps {
		t.Run("interrupt/"+step, func(t *testing.T) {
			f := newFoldFixture(t)
			boom := errors.New("interrupted")
			f.opts.Step = func(name string) error {
				if name == step {
					return boom
				}
				return nil
			}
			if err := f.fold(t); !errors.Is(err, boom) {
				t.Fatalf("fold = %v, want the interruption at %s", err, step)
			}
			f.opts.Step = nil
			if err := f.fold(t); err != nil {
				t.Fatal(err)
			}
			if got := f.stateSnapshot(t); got != want {
				t.Errorf("state after resume differs:\n%s", lineDiff(want, got))
			}
		})
	}

	exdev := func(string, string) error { return &os.LinkError{Op: "rename", Err: syscall.EXDEV} }
	t.Run("cross-device", func(t *testing.T) {
		f := newFoldFixture(t)
		f.opts.Rename = exdev
		before := treeSnapshot(t, f.legacy)
		if err := f.fold(t); err != nil {
			t.Fatal(err)
		}
		checkFolded(t, f, before)
		if got := f.stateSnapshot(t); got != want {
			t.Errorf("cross-device state differs:\n%s", lineDiff(want, got))
		}
	})
	t.Run("cross-device/interrupt/move", func(t *testing.T) {
		f := newFoldFixture(t)
		f.opts.Rename = exdev
		boom := errors.New("interrupted")
		f.opts.Step = func(name string) error {
			if name == "db" {
				return boom
			}
			return nil
		}
		if err := f.fold(t); !errors.Is(err, boom) {
			t.Fatalf("fold = %v", err)
		}
		f.opts.Step = nil
		if err := f.fold(t); err != nil {
			t.Fatal(err)
		}
		if got := f.stateSnapshot(t); got != want {
			t.Errorf("state after resume differs:\n%s", lineDiff(want, got))
		}
	})

	for name, setup := range map[string]func(*foldFixture){
		"absent":       func(f *foldFixture) { f.opts.Legacy = filepath.Join(f.space, "..", "no-such") },
		"legacy-empty": func(f *foldFixture) { _ = os.RemoveAll(f.legacy); _ = os.MkdirAll(f.legacy, 0o700) },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFoldFixture(t)
			setup(f)
			before := treeSnapshot(t, f.space)
			if err := f.fold(t); err != nil {
				t.Fatal(err)
			}
			if got := scalar(t, f.spaceDB.DB, `SELECT value FROM settings WHERE key = 'homefold.p539'`); got != `{"state":"done","fresh":true}` {
				t.Errorf("marker = %s", got)
			}
			if got := treeSnapshot(t, f.space); got != before {
				t.Errorf("Space home changed:\n%s", got)
			}
		})
	}

	t.Run("studio-running", func(t *testing.T) {
		f := newFoldFixture(t)
		lock, ok, err := config.AcquireLock(filepath.Join(f.legacy, "app.lock"))
		if err != nil || !ok {
			t.Fatalf("lock: ok=%v err=%v", ok, err)
		}
		defer func() { _ = lock.Close() }()
		before := treeSnapshot(t, f.legacy)
		if err := f.fold(t); !errors.Is(err, homefold.ErrStudioRunning) {
			t.Fatalf("fold = %v, want ErrStudioRunning", err)
		}
		if got := treeSnapshot(t, f.legacy); got != before {
			t.Error("legacy folder changed")
		}
		if got := scalar(t, f.spaceDB.DB, `SELECT COUNT(*) FROM settings WHERE key = 'homefold.p539'`); got != "0" {
			t.Errorf("marker rows = %s", got)
		}
	})

	t.Run("legacy-home", func(t *testing.T) {
		t.Setenv("KIRA_SPACE_HOME", filepath.Join(t.TempDir(), "space"))
		t.Setenv("KIRA_HOME", "")
		if got := homefold.LegacyHome(); got != "" {
			t.Errorf("only KIRA_SPACE_HOME set: %q, want none", got)
		}
		legacy := filepath.Join(t.TempDir(), "old")
		t.Setenv("KIRA_HOME", legacy)
		if got := homefold.LegacyHome(); got != legacy {
			t.Errorf("KIRA_HOME set: %q, want %q", got, legacy)
		}
		t.Setenv("KIRA_HOME", filepath.Join(os.Getenv("KIRA_SPACE_HOME"), "inside"))
		if got := homefold.LegacyHome(); got != "" {
			t.Errorf("legacy inside the Space home: %q, want none", got)
		}
	})
}

func checkFolded(t *testing.T, f *foldFixture, legacyBefore string) {
	t.Helper()
	if exists(f.legacy) {
		t.Error("legacy folder still present")
	}
	backups, _ := filepath.Glob(filepath.Join(f.space, "backups", "p539-*"))
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	b := backups[0]
	if got := treeSnapshot(t, filepath.Join(b, "kira-studio")); got != legacyBefore {
		t.Errorf("backup differs from the legacy folder:\n%s", lineDiff(legacyBefore, got))
	}
	for _, name := range []string{"space.db", "space-llm-prompts.json"} {
		if !exists(filepath.Join(b, name)) {
			t.Errorf("backup lacks %s", name)
		}
	}

	studio, err := sqlitex.Open(config.ModuleDbPathAt(f.space))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = studio.Close() }()
	if got := scalar(t, studio, `SELECT group_concat(scope || owner, ',') FROM secret_values`); got != "connectionc1" {
		t.Errorf("Studio secrets = %q, want only the connection secret", got)
	}
	for _, tbl := range []string{"custom_scripts", "custom_script_collections", "script_runs", "script_run_logs"} {
		if got := scalar(t, studio, `SELECT COUNT(*) FROM `+tbl); got != "0" {
			t.Errorf("Studio %s rows = %s", tbl, got)
		}
	}

	for _, name := range []string{"connections/a.yaml", "api/c/x.yaml"} {
		if !exists(filepath.Join(f.space, name)) {
			t.Errorf("%s not landed", name)
		}
	}
	for _, name := range []string{"mcp-db-token.json", "mcp-db-header.token"} {
		fi, err := os.Stat(filepath.Join(f.space, name))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v %v, want 0600", name, fi, err)
		}
	}
	for _, name := range []string{"mcp-header-helper.sh", "unknown.txt", "logs/x.log"} {
		if exists(filepath.Join(f.space, name)) {
			t.Errorf("%s leaked into the Space home", name)
		}
	}

	db := f.spaceDB.DB
	if got := scalar(t, db, `SELECT group_concat(id, ',') FROM (SELECT id FROM custom_scripts ORDER BY id)`); got != "build,build-studio,deploy,legacy-cfg" {
		t.Errorf("scripts = %q", got)
	}
	if got := scalar(t, db, `SELECT name FROM custom_scripts WHERE id = 'build'`); got != "Space build" {
		t.Errorf("Space script overwritten: %q", got)
	}
	if got := scalar(t, db, `SELECT group_concat(name || ':' || sort_order, ',') FROM (SELECT * FROM custom_script_collections ORDER BY sort_order)`); got != "shared:0,only-studio:2" {
		t.Errorf("collections = %q", got)
	}
	if got := scalar(t, db, `SELECT group_concat(id, ',') FROM (SELECT id FROM custom_scripts WHERE sort_order > 0 ORDER BY sort_order)`); got != "build-studio,deploy,legacy-cfg" {
		t.Errorf("studio scripts not after Space's: %q", got)
	}
	if got := scalar(t, db, `SELECT script_id || '|' || cwd FROM script_runs WHERE id = 'r1'`); got != "build-studio|"+f.space+"/automations/build-studio/work" {
		t.Errorf("r1 = %q", got)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM script_runs`); got != "4" {
		t.Errorf("runs = %s, want 1 Space + 3 Studio", got)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM script_run_logs WHERE run_id IN (SELECT id FROM script_runs)`); got != "2" {
		t.Errorf("logs attached to runs = %s", got)
	}
	if got := scalar(t, db, `SELECT group_concat(owner || ':' || name || ':' || value, ',') FROM (SELECT * FROM secret_values ORDER BY owner)`); got != "build-studio:TOKEN:kira:v3:script-param:aaa,deploy:KEY:kira:v3:script-param:bbb" {
		t.Errorf("secrets = %q", got)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM settings WHERE key = 'config.migrated.runs'`); got != "0" {
		t.Error("config.migrated.runs not cleared")
	}

	for name, mode := range map[string]os.FileMode{"build-studio.yaml": 0o644, "deploy.yaml": 0o600, "legacy-cfg.yaml": 0o644, "build.yaml": 0o644} {
		fi, err := os.Stat(filepath.Join(f.space, "runs", name))
		if err != nil || fi.Mode().Perm() != mode {
			t.Errorf("runs/%s: %v %v, want mode %o", name, fi, err, mode)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(f.space, "runs", "build.yaml")); !strings.Contains(string(b), "space: true") {
		t.Error("Space run file overwritten")
	}
	if got, err := os.Readlink(filepath.Join(f.space, "automations", "build-studio", "link")); err != nil || got != "work/secret.txt" {
		t.Errorf("automation symlink = %q %v", got, err)
	}
	if fi, err := os.Stat(filepath.Join(f.space, "automations", "build-studio", "work", "secret.txt")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("automation file mode: %v %v", fi, err)
	}

	raw, _ := os.ReadFile(filepath.Join(f.space, "llm-prompts.json"))
	if s := string(raw); !strings.Contains(s, "space one") || strings.Contains(s, "studio one") || !strings.Contains(s, "studio two") {
		t.Errorf("prompts = %s", s)
	}
	if got := scalar(t, db, `SELECT json_extract(value, '$.state') || json_extract(value, '$.reregister') FROM settings WHERE key = 'homefold.p539'`); got != "done1" {
		t.Errorf("marker = %q", got)
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
