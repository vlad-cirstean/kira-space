package adeflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/internal/configfs"
)

func mustDoc(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	return &doc
}

func TestRetargetLibrary(t *testing.T) {
	const wf = `id: wf
name: Wf
steps:
  - step: lint # keep me
    routes:
      failed: fix
  - id: fix
    name: Fix
    kind: agent
    runs_on: each repo
    timeout: 30m
    prompt: p
    results:
      - id: done
        ok: true
        next: lint
      - id: failed
        ok: false
`
	t.Run("implicit id follows with references", func(t *testing.T) {
		doc := mustDoc(t, wf)
		if !retargetLibrary(docRoot(doc), "lint", "lint-repo") {
			t.Fatal("not changed")
		}
		out, _ := encodeDoc(doc)
		s := string(out)
		for _, want := range []string{"step: lint-repo # keep me", "next: lint-repo"} {
			if !strings.Contains(s, want) {
				t.Errorf("missing %q in\n%s", want, s)
			}
		}
		if strings.Contains(s, "next: lint\n") {
			t.Errorf("stale route in\n%s", s)
		}
	})
	t.Run("explicit id untouched", func(t *testing.T) {
		doc := mustDoc(t, strings.Replace(wf, "  - step: lint # keep me", "  - step: lint # keep me\n    id: first", 1))
		retargetLibrary(docRoot(doc), "lint", "lint-repo")
		out, _ := encodeDoc(doc)
		if !strings.Contains(string(out), "next: lint\n") || !strings.Contains(string(out), "step: lint-repo") {
			t.Errorf("unexpected\n%s", out)
		}
	})
	t.Run("collision pins old id", func(t *testing.T) {
		doc := mustDoc(t, strings.Replace(wf, "id: fix", "id: lint-repo", 1))
		retargetLibrary(docRoot(doc), "lint", "lint-repo")
		out, _ := encodeDoc(doc)
		s := string(out)
		if !strings.Contains(s, "id: lint\n") || !strings.Contains(s, "next: lint\n") {
			t.Errorf("not pinned\n%s", s)
		}
	})
	t.Run("review implementer", func(t *testing.T) {
		doc := mustDoc(t, `id: wf
name: Wf
steps:
  - step: impl
  - id: rv
    name: Rv
    kind: review
    review:
      implementer: impl
`)
		retargetLibrary(docRoot(doc), "impl", "build")
		out, _ := encodeDoc(doc)
		if !strings.Contains(string(out), "implementer: build") {
			t.Errorf("implementer not rewritten\n%s", out)
		}
	})
}

func TestRenameStepID(t *testing.T) {
	doc := mustDoc(t, `id: wf
name: Wf
steps:
  - id: stp_8h3k2m9qz7ab
    name: Lint
    results:
      - id: done
        ok: true
        next: stp_8h3k2m9qz7ab
  - step: x
    routes:
      failed: stp_8h3k2m9qz7ab
`)
	if !renameStepID(docRoot(doc), "stp_8h3k2m9qz7ab", "lint") {
		t.Fatal("not changed")
	}
	out, _ := encodeDoc(doc)
	if strings.Contains(string(out), "stp_") || strings.Count(string(out), "lint") != 3 {
		t.Errorf("unexpected\n%s", out)
	}
}

func TestFreeStem(t *testing.T) {
	long := strings.Repeat("a", 60)
	taken := map[string]bool{long: true}
	for n := 2; n < 12; n++ {
		taken[stemWithSuffix(long, n)] = true
	}
	got := freeStem(long, func(s string) bool { return taken[s] })
	if len(got) > maxStem || strings.HasSuffix(strings.TrimSuffix(got, "-12"), "-") || !strings.HasSuffix(got, "-12") {
		t.Errorf("got %q (%d)", got, len(got))
	}
	if !stemMatches(got, long, "step") {
		t.Errorf("%q does not match its own base", got)
	}
	dashed := strings.Repeat("ab-", 20) + "abc"
	got = freeStem(slug(dashed, "x"), func(s string) bool { return s == slug(dashed, "x") })
	if strings.Contains(got, "--") || strings.HasSuffix(got, "--2") {
		t.Errorf("bad %q", got)
	}
}

func TestStemMatches(t *testing.T) {
	for stem, want := range map[string]bool{"lint-repo": true, "lint-repo-2": true, "lint-repo-12": true, "lint-repo-1": false, "lint-repo-x": false, "lint": false, "new-step": false} {
		if got := stemMatches(stem, "Lint repo", "step"); got != want {
			t.Errorf("%s: %v want %v", stem, got, want)
		}
	}
}

func journalReader(t *testing.T, files map[string]string, workflows map[string]string) *Reader {
	t.Helper()
	r := libReader(t, files)
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for n, s := range workflows {
		if err := os.WriteFile(filepath.Join(r.Dir, n), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r.Journal = filepath.Join(filepath.Dir(r.Dir), "rename.json")
	return r
}

const libOld = "id: new-step\nname: Lint repo\nkind: manual\n"

const wfUses = "id: wf\nname: Wf\nsteps:\n  - step: new-step\n"

func TestRecoverRename(t *testing.T) {
	jr := func(r *Reader, hash string) {
		j := renameJournal{Kind: "step", Old: "new-step.yaml", New: "lint-repo.yaml", OldHash: hash, Workflows: []string{"wf.yaml"}}
		if err := r.writeJournal(j); err != nil {
			t.Fatal(err)
		}
	}
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }

	t.Run("new file absent drops the journal", func(t *testing.T) {
		r := journalReader(t, map[string]string{"new-step.yaml": libOld}, map[string]string{"wf.yaml": wfUses})
		jr(r, configfs.TextHash([]byte(libOld)))
		r.RecoverRename(nil)
		if _, err := os.Stat(r.Journal); !os.IsNotExist(err) {
			t.Error("journal kept")
		}
		if read(filepath.Join(r.Steps.Dir, "new-step.yaml")) != libOld || read(filepath.Join(r.Dir, "wf.yaml")) != wfUses {
			t.Error("files changed")
		}
	})
	t.Run("half rewritten workflows are finished", func(t *testing.T) {
		r := journalReader(t, map[string]string{"new-step.yaml": libOld, "lint-repo.yaml": "id: lint-repo\nname: Lint repo\nkind: manual\n"}, map[string]string{"wf.yaml": wfUses})
		jr(r, configfs.TextHash([]byte(libOld)))
		r.RecoverRename(nil)
		if !strings.Contains(read(filepath.Join(r.Dir, "wf.yaml")), "step: lint-repo") {
			t.Errorf("workflow not retargeted:\n%s", read(filepath.Join(r.Dir, "wf.yaml")))
		}
		if _, err := os.Stat(filepath.Join(r.Steps.Dir, "new-step.yaml")); !os.IsNotExist(err) {
			t.Error("old file kept")
		}
		if _, err := os.Stat(r.Journal); !os.IsNotExist(err) {
			t.Error("journal kept")
		}
		r.RecoverRename(nil) // no journal: no-op
	})
	t.Run("old file edited after the journal is kept", func(t *testing.T) {
		r := journalReader(t, map[string]string{"new-step.yaml": libOld + "# edited\n", "lint-repo.yaml": "id: lint-repo\nname: Lint repo\nkind: manual\n"}, map[string]string{"wf.yaml": wfUses})
		jr(r, configfs.TextHash([]byte(libOld)))
		r.RecoverRename(nil)
		if _, err := os.Stat(filepath.Join(r.Steps.Dir, "new-step.yaml")); err != nil {
			t.Error("edited file removed")
		}
	})
	t.Run("broken workflow keeps old file and journal", func(t *testing.T) {
		r := journalReader(t, map[string]string{"new-step.yaml": libOld, "lint-repo.yaml": "id: lint-repo\nname: Lint repo\nkind: manual\n"}, map[string]string{"wf.yaml": "steps: [\n"})
		jr(r, configfs.TextHash([]byte(libOld)))
		r.RecoverRename(nil)
		if _, err := os.Stat(filepath.Join(r.Steps.Dir, "new-step.yaml")); err != nil {
			t.Error("old file removed under a broken workflow")
		}
		if _, err := os.Stat(r.Journal); err != nil {
			t.Error("journal dropped")
		}
	})
	t.Run("workflow kind retargets once per run and is idempotent", func(t *testing.T) {
		r := journalReader(t, nil, map[string]string{"old.yaml": "id: old\nname: Release\nsteps: []\n", "release.yaml": "id: release\nname: Release\nsteps: []\n"})
		j := renameJournal{Kind: "workflow", Old: "old.yaml", New: "release.yaml", OldHash: configfs.TextHash([]byte("id: old\nname: Release\nsteps: []\n"))}
		if err := r.writeJournal(j); err != nil {
			t.Fatal(err)
		}
		calls := 0
		retarget := func(o, n string) error {
			calls++
			if o != "old" || n != "release" {
				t.Errorf("retarget(%s,%s)", o, n)
			}
			return nil
		}
		r.RecoverRename(retarget)
		r.RecoverRename(retarget)
		if calls != 1 {
			t.Errorf("calls = %d", calls)
		}
		if _, err := os.Stat(filepath.Join(r.Dir, "old.yaml")); !os.IsNotExist(err) {
			t.Error("old workflow kept")
		}
	})
}
