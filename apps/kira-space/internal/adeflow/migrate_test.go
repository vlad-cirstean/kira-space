package adeflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var migrateA = head + step("implement", "    status: in-progress\n    results:\n      - id: done\n        ok: true\n      - id: redo\n        ok: false\n        next: implement\n") + `  - id: gate
    name: Gate
    kind: manual
    skip: true
`

// migrateB shares the id implement with a different body.
var migrateB = "id: second\nname: Second\nsteps:\n" + step("implement", "    allowed_tools:\n      - Bash(git *)\n") + reviewStep("review", "    review:\n      implementer: implement\n")

// migrateC shares the id and the body with migrateA's first step, minus its workflow-only route.
var migrateC = "id: third\nname: Third\nsteps:\n" + step("implement", "    status: in-progress\n    results:\n      - id: done\n        ok: true\n      - id: redo\n        ok: false\n")

func TestMigrateInlineSteps(t *testing.T) {
	r := libReader(t, nil)
	files := map[string]string{"w.yaml": migrateA, "second.yaml": migrateB, "third.yaml": migrateC, "broken.yaml": "id: broken\n  oops: [\n"}
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(r.Dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	before := map[string]string{}
	for _, id := range []string{"w", "second", "third"} {
		wf, werr := Parse([]byte(files[id+".yaml"]))
		if werr != nil {
			t.Fatalf("%s: %+v", id, werr)
		}
		js, _ := resolvedJSON(wf)
		before[id] = js
	}
	if err := r.MigrateInlineSteps(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"implement.yaml", "implement-2.yaml", "gate.yaml", "review.yaml"} {
		if _, err := os.Stat(filepath.Join(r.Steps.Dir, f)); err != nil {
			t.Errorf("library file %s: %v", f, err)
		}
	}
	// Files migrate in name order: second claims implement; third has another body, so implement-2;
	// w holds the same body as third once its route is stripped, so it reuses implement-2.
	second := read(t, r, "second.yaml")
	if !strings.Contains(second, "step: implement\n") || !strings.Contains(second, "implementer: implement") {
		t.Errorf("second.yaml:\n%s", second)
	}
	for _, name := range []string{"third.yaml", "w.yaml"} {
		got := read(t, r, name)
		if !strings.Contains(got, "step: implement-2\n") || !strings.Contains(got, "id: implement\n") {
			t.Errorf("%s:\n%s", name, got)
		}
	}
	if w := read(t, r, "w.yaml"); !strings.Contains(w, "routes:") || !strings.Contains(w, "redo: implement") || !strings.Contains(w, "skip: true") {
		t.Errorf("w.yaml keeps its workflow-only keys:\n%s", w)
	}
	if got, _ := os.ReadFile(filepath.Join(r.Dir, "broken.yaml")); string(got) != files["broken.yaml"] {
		t.Errorf("broken file changed: %q", got)
	}
	for name, src := range files {
		if bak, err := os.ReadFile(filepath.Join(r.Dir, backupDir, name)); err != nil || string(bak) != src {
			t.Errorf("backup of %s: %v", name, err)
		}
	}
	for _, id := range []string{"w", "third", "second"} {
		wf, ok := r.Get(id)
		if !ok {
			t.Fatalf("%s not available", id)
		}
		if js, _ := resolvedJSON(wf); js != before[id] {
			t.Errorf("%s resolves differently after the move:\n%s\n%s", id, js, before[id])
		}
	}
	// a second run is a no-op: edits after the move are not touched again
	if err := os.WriteFile(filepath.Join(r.Dir, "third.yaml"), []byte(migrateC), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.MigrateInlineSteps(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, r, "third.yaml"); got != migrateC {
		t.Errorf("migration ran twice:\n%s", got)
	}
}
