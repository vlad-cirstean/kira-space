package appflow_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/configfold"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

var configModules = []string{"connections", "api", "runs", "workflows", "steps", "repos", "agent"}

type configFixture struct {
	home string
	db   *storage.DB
	opts configfold.Options
}

func newConfigFixture(t *testing.T, legacy bool) *configFixture {
	t.Helper()
	home := filepath.Join(t.TempDir(), "space")
	db, err := storage.OpenAt(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &configFixture{home: home, db: db}
	f.opts = configfold.Options{
		Space: db.DB, Home: home,
		Now: func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	}
	if !legacy {
		return f
	}
	at := func(name string) string { return filepath.Join(home, name) }
	w := func(rel, body string, mode os.FileMode) { mustWrite(t, filepath.Join(home, rel), body, mode) }
	w("connections/a.yaml", "conn", 0o644)
	w("api/c.collection.yaml", "col", 0o644)
	w("runs/r.yaml", "run", 0o644)
	w("runs/AGENTS.md", "doc", 0o644)
	w("workflows/w.yaml", "wf", 0o644)
	w("workflows/.p526-backup/w.yaml", "old", 0o644)
	target := filepath.Join(filepath.Dir(home), "dotfiles", "steps")
	mustWrite(t, filepath.Join(target, "s.yaml"), "step", 0o644)
	if err := os.Symlink(target, at("steps")); err != nil {
		t.Fatal(err)
	}
	w("repos/x.yaml", "repo", 0o644)
	w("agent/settings.yaml", "set", 0o644)
	w("agent/context.md", "ctx", 0o600)
	w("llm-prompts.json", `{"version":1}`, 0o644)
	w("agent-secrets.db", "sealed", 0o600)
	w("mcp-db-token.json", "tok", 0o600)
	w("logs/x.log", "log", 0o644)
	w("automations/r/f", "auto", 0o644)
	w("agent-context/s.md", "snap", 0o644)
	w("ade/runs/x.mcp.json", "{}", 0o644)
	w("review.db", "rev", 0o644)
	return f
}

func (f *configFixture) marker(t *testing.T) string {
	t.Helper()
	var raw string
	if err := f.db.DB.QueryRow(`SELECT value FROM settings WHERE key = 'configfold.p540'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

// homeSnapshot is the home tree with the per-test temp root masked, so fixtures compare equal.
func (f *configFixture) homeSnapshot(t *testing.T) string {
	t.Helper()
	return strings.ReplaceAll(treeSnapshot(t, f.home), filepath.Dir(f.home), "ROOT")
}

func (f *configFixture) configDir(module string) string { return configfs.ModuleDir(f.home, module) }

func (f *configFixture) legacyName(module string) string { return filepath.Join(f.home, module) }

func (f *configFixture) legacySnapshots(t *testing.T) map[string]string {
	t.Helper()
	snaps := map[string]string{"llm-prompts.json": treeSnapshot(t, f.legacyName("llm-prompts.json"))}
	for _, m := range configModules {
		snaps[m] = treeSnapshot(t, f.legacyName(m))
	}
	return snaps
}

func (f *configFixture) checkFolded(t *testing.T, before map[string]string) {
	t.Helper()
	cfg := filepath.Join(f.home, "config")
	backups, err := filepath.Glob(filepath.Join(f.home, "backups", "p540-*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v, %v; want exactly one", backups, err)
	}
	for _, m := range configModules {
		if got := treeSnapshot(t, filepath.Join(cfg, m)); got != before[m] {
			t.Errorf("config/%s differs:\n%s", m, lineDiff(before[m], got))
		}
		if got := treeSnapshot(t, filepath.Join(backups[0], m)); got != before[m] {
			t.Errorf("backup %s differs:\n%s", m, lineDiff(before[m], got))
		}
		if exists(f.legacyName(m)) {
			t.Errorf("legacy %s still present", m)
		}
	}
	prompts := filepath.Join(cfg, "prompts", "llm-prompts.json")
	if got := treeSnapshot(t, prompts); got != before["llm-prompts.json"] {
		t.Errorf("prompts differ:\n%s", lineDiff(before["llm-prompts.json"], got))
	}
	if exists(f.legacyName("llm-prompts.json")) {
		t.Error("legacy llm-prompts.json still present")
	}
	if link, err := os.Readlink(filepath.Join(cfg, "steps")); err != nil || !strings.HasSuffix(link, filepath.Join("dotfiles", "steps")) {
		t.Errorf("steps link = %q, %v", link, err)
	}
	for _, p := range []string{"agent-secrets.db", "mcp-db-token.json", "logs/x.log", "automations/r/f", "agent-context/s.md", "ade/runs/x.mcp.json", "review.db"} {
		if !exists(filepath.Join(f.home, p)) {
			t.Errorf("non-config %s moved", p)
		}
	}
	err = filepath.WalkDir(cfg, func(p string, d fs.DirEntry, err error) error {
		name := d.Name()
		if err == nil && (strings.Contains(name, ".db") || strings.Contains(name, "token") || name == "logs" || strings.HasSuffix(name, ".tmp")) {
			t.Errorf("config/ holds %s", p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(cfg, "agent", "context.md")); string(b) != "ctx" {
		t.Errorf("context.md = %q", b)
	}
	if info, err := os.Stat(filepath.Join(cfg, "agent", "context.md")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("context.md mode = %v, %v", info, err)
	}
	if got := f.marker(t); !strings.Contains(got, `"state":"done"`) {
		t.Errorf("marker = %s", got)
	}
}

func TestConfigFold(t *testing.T) {
	t.Run("fresh", func(t *testing.T) {
		f := newConfigFixture(t, false)
		if err := configfold.Run(f.opts); err != nil {
			t.Fatal(err)
		}
		if got := f.marker(t); !strings.Contains(got, `"fresh":true`) || !strings.Contains(got, `"state":"done"`) {
			t.Errorf("marker = %s", got)
		}
		if exists(filepath.Join(f.home, "backups")) {
			t.Error("fresh home got a backup")
		}
	})

	t.Run("fold", func(t *testing.T) {
		f := newConfigFixture(t, true)
		before := f.legacySnapshots(t)
		if err := configfold.Run(f.opts); err != nil {
			t.Fatal(err)
		}
		f.checkFolded(t, before)
	})

	t.Run("idempotent", func(t *testing.T) {
		f := newConfigFixture(t, true)
		if err := configfold.Run(f.opts); err != nil {
			t.Fatal(err)
		}
		first := f.homeSnapshot(t)
		f.opts.Now = func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) }
		if err := configfold.Run(f.opts); err != nil {
			t.Fatal(err)
		}
		if got := f.homeSnapshot(t); got != first {
			t.Errorf("second run changed the home:\n%s", lineDiff(first, got))
		}
	})

	t.Run("interrupted", func(t *testing.T) {
		var steps []string
		dry := newConfigFixture(t, true)
		dry.opts.Step = func(name string) error { steps = append(steps, name); return nil }
		if err := configfold.Run(dry.opts); err != nil {
			t.Fatal(err)
		}
		want := dry.homeSnapshot(t)
		if len(steps) < 10 {
			t.Fatalf("recorded steps = %v", steps)
		}
		for _, step := range steps {
			t.Run(step, func(t *testing.T) {
				f := newConfigFixture(t, true)
				before := f.legacySnapshots(t)
				boom := errors.New("interrupt " + step)
				f.opts.Step = func(name string) error {
					if name == step {
						return boom
					}
					return nil
				}
				if err := configfold.Run(f.opts); !errors.Is(err, boom) {
					t.Fatalf("err = %v, want interrupt", err)
				}
				f.opts.Step = nil
				if err := configfold.Run(f.opts); err != nil {
					t.Fatal(err)
				}
				f.checkFolded(t, before)
				if got := f.homeSnapshot(t); got != want {
					t.Errorf("end state differs from an uninterrupted run:\n%s", lineDiff(want, got))
				}
			})
		}
	})

	t.Run("cross-device", func(t *testing.T) {
		f := newConfigFixture(t, true)
		before := f.legacySnapshots(t)
		f.opts.Rename = func(from, to string) error {
			if filepath.Base(from) == "workflows" {
				return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.EXDEV}
			}
			return os.Rename(from, to)
		}
		if err := configfold.Run(f.opts); err != nil {
			t.Fatal(err)
		}
		f.checkFolded(t, before)
	})

	t.Run("collision", func(t *testing.T) {
		f := newConfigFixture(t, true)
		mustWrite(t, filepath.Join(f.configDir("workflows"), "w.yaml"), "kept", 0o644)
		mustWrite(t, filepath.Join(f.legacyName("workflows"), "only.yaml"), "moved", 0o644)
		if err := configfold.Run(f.opts); err != nil {
			t.Fatal(err)
		}
		read := func(p ...string) string {
			b, err := os.ReadFile(filepath.Join(p...))
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
		cfg := f.configDir("workflows")
		if got := read(cfg, "w.yaml"); got != "kept" {
			t.Errorf("w.yaml = %q, want kept", got)
		}
		if got := read(cfg, "only.yaml"); got != "moved" {
			t.Errorf("only.yaml = %q", got)
		}
		if exists(f.legacyName("workflows")) {
			t.Error("legacy workflows still present")
		}
		backups, _ := filepath.Glob(filepath.Join(f.home, "backups", "p540-*"))
		if len(backups) != 1 {
			t.Fatalf("backups = %v", backups)
		}
		if got := read(backups[0], "workflows", "w.yaml"); got != "wf" {
			t.Errorf("backup w.yaml = %q, want wf", got)
		}
	})
}
