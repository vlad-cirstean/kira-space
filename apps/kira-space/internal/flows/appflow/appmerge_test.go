package appflow_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appmerge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage"
)

func execAll(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func scalar(t *testing.T, db *sql.DB, q string) string {
	t.Helper()
	var v sql.NullString
	if err := db.QueryRow(q).Scan(&v); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return v.String
}

// First launch carries a Studio install over; a second run and a resumed run change nothing more.
func TestAppMergeFirstLaunch(t *testing.T) {
	root := t.TempDir()
	spaceHome, studioHome := filepath.Join(root, "space"), filepath.Join(root, "studio")
	sp, err := storage.OpenAt(spaceHome)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.Close() })
	st, err := studiostorage.OpenAt(studioHome)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	execAll(t, sp.DB,
		`INSERT INTO windows (key, "order") VALUES ('hostview-x', 0), ('w1', 1)`,
		`INSERT INTO tabs (id, path, kind, state_json, "order", active, window_key, workspace_id) VALUES ('s1', '/r', 'git-changes', '{}', 4, 1, 'w1', 'ws')`,
	)
	execAll(t, st.DB,
		`UPDATE windows SET "order" = 5 WHERE key = 'main'`,
		`INSERT INTO windows (key, "order") VALUES ('w1', 6)`,
		`INSERT INTO tabs (id, path, kind, state_json, "order", active, window_key) VALUES
			('a', 'q1', 'query', '{}', 0, 0, 'main'), ('b', 'q2', 'query', '{}', 1, 1, 'main'), ('c', 'q3', 'query', '{}', 0, 1, 'w1')`,
		`INSERT INTO ui_layout (key, value) VALUES ('modes.api.project.width', '321'), ('modes.git.project.width', '999'), ('modes.automations.project.width', '400')`,
		`INSERT INTO settings (key, value) VALUES ('appearance.fontSize', '17'), ('agent.provider', '"codex"'), ('advanced.logLevel', '"debug"'), ('advanced.notifyPrompts', 'false')`,
	)
	if err := os.WriteFile(filepath.Join(studioHome, "llm-prompts.json"),
		[]byte(`{"version":1,"revision":3,"overrides":{"a.one":{"text":"studio one"},"a.two":{"text":"studio two"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(spaceHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spaceHome, "llm-prompts.json"),
		[]byte(`{"version":1,"revision":7,"overrides":{"a.one":{"text":"space one"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	opts := appmerge.Options{
		Space: sp.DB, Studio: st.DB, SpaceHome: spaceHome, StudioHome: studioHome,
		Ephemeral: func(k string) bool { return k == "hostview-x" },
	}
	if err := appmerge.Run(opts); err != nil {
		t.Fatal(err)
	}

	if got := scalar(t, st.DB, `SELECT group_concat(id || ':' || window_key || ':' || "order" || ':' || active, ',') FROM (SELECT * FROM tabs ORDER BY id)`); got != "a:w1:5:0,b:w1:6:0,c:w1:0:1" {
		t.Errorf("tabs = %q", got)
	}
	if got := scalar(t, sp.DB, `SELECT group_concat(key || '=' || value, ',') FROM (SELECT * FROM ui_layout WHERE key LIKE 'modes.%' ORDER BY key)`); got != "modes.api.project.width=321" {
		t.Errorf("layout = %q", got)
	}
	if got := scalar(t, sp.DB, `SELECT group_concat(key || '=' || value, ',') FROM (SELECT * FROM settings WHERE key != 'appmerge.p537' ORDER BY key)`); got != "advanced.gitLogLevel=\"debug\",advanced.notifyPrompts=false,agent.provider=\"codex\",appearance.fontSize=17" {
		t.Errorf("settings = %q", got)
	}
	raw, err := os.ReadFile(filepath.Join(spaceHome, "llm-prompts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Revision  int
		Overrides map[string]struct{ Text string }
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Revision != 8 || doc.Overrides["a.one"].Text != "space one" || doc.Overrides["a.two"].Text != "studio two" {
		t.Errorf("prompts = %s", raw)
	}
	backups, _ := filepath.Glob(filepath.Join(spaceHome, "backups", "p537-*", "*.db"))
	if len(backups) != 2 {
		t.Errorf("backups = %v, want space.db and studio.db", backups)
	}

	// Resume: a half-done run (marker still running) repeats cleanly and keeps its backup.
	execAll(t, sp.DB, `UPDATE settings SET value = json_set(value, '$.state', 'running') WHERE key = 'appmerge.p537'`)
	execAll(t, st.DB, `INSERT INTO tabs (id, path, kind, state_json, "order", active, window_key) VALUES ('d', 'q4', 'query', '{}', 0, 0, 'w1')`)
	if err := appmerge.Run(opts); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, sp.DB, `SELECT json_extract(value, '$.state') FROM settings WHERE key = 'appmerge.p537'`); got != "done" {
		t.Errorf("marker state = %q", got)
	}
	if got := scalar(t, st.DB, `SELECT COUNT(*) FROM tabs WHERE window_key = 'w1'`); got != "4" {
		t.Errorf("tabs on w1 = %s", got)
	}
	again, _ := filepath.Glob(filepath.Join(spaceHome, "backups", "p537-*", "*.db"))
	if len(again) != 2 {
		t.Errorf("resume made another backup: %v", again)
	}
	raw2, _ := os.ReadFile(filepath.Join(spaceHome, "llm-prompts.json"))
	if string(raw2) != string(raw) {
		t.Errorf("prompts rewritten on resume: %s", raw2)
	}
}
