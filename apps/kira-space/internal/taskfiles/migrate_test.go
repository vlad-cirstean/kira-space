package taskfiles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
)

type fakeExporter struct {
	exports    int
	failExport error
	failVerify error
	recorded   int
}

func (f *fakeExporter) Part() string { return "notes" }

func (f *fakeExporter) Export(ctx context.Context, _ string, st store.Store) (Manifest, error) {
	f.exports++
	if f.failExport != nil {
		return Manifest{}, f.failExport
	}
	for _, rel := range []string{"notes/20261101T090000-a1.md", "notes/20261101T090001-b2.md"} {
		if _, err := st.WriteAtomic(ctx, rel, []byte("exported\n"), nil); err != nil {
			return Manifest{}, err
		}
	}
	return Manifest{Paths: []string{"notes/20261101T090000-a1.md", "notes/20261101T090001-b2.md"}}, nil
}

func (f *fakeExporter) Verify(context.Context, string, store.Store, Manifest) error { return f.failVerify }

func (f *fakeExporter) Record(context.Context, Tx, string, Manifest) error {
	f.recorded++
	return nil
}

func TestMigrator(t *testing.T) {
	e := newEnv(t)
	root := t.TempDir()
	ex := &fakeExporter{}
	m := &Migrator{Index: RepoIndex(e.repo), Marks: e.repo, Root: root, Tasks: func() ([]string, error) { return []string{e.taskID}, nil }, Exporters: []Exporter{ex}}
	ctx := context.Background()
	mark := func() string {
		got, err := e.repo.Migration(e.taskID, "notes")
		if err != nil || got == nil {
			t.Fatalf("mark %v %v", got, err)
		}
		return got.State
	}

	// Export failure: marked failed, nothing in place, no staging left.
	ex.failExport = errors.New("boom")
	if err := m.Run(ctx); err == nil || mark() != model.TaskFilesMigrationFailed {
		t.Fatalf("run err %v state %s", err, mark())
	}
	if _, err := os.Stat(filepath.Join(root, e.taskID+".migrating")); !os.IsNotExist(err) {
		t.Fatal("staging left behind")
	}

	// Verify failure: nothing moved into the task folder.
	ex.failExport, ex.failVerify = nil, errors.New("hash mismatch")
	if err := m.Run(ctx); err == nil {
		t.Fatal("verify failure not reported")
	}
	if _, err := os.Stat(filepath.Join(root, e.taskID, "notes")); !os.IsNotExist(err) {
		t.Fatal("unverified files moved into place")
	}

	// A stale staging folder from a crash and a newer file already in place: resume merges only absent files.
	ex.failVerify = nil
	if err := os.MkdirAll(filepath.Join(root, e.taskID+".migrating", "junk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, e.taskID, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, e.taskID, "notes", "20261101T090000-a1.md")
	if err := os.WriteFile(keep, []byte("newer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Run(ctx); err != nil || mark() != model.TaskFilesMigrationDone {
		t.Fatalf("resume: %v %s", err, mark())
	}
	if b, _ := os.ReadFile(keep); string(b) != "newer\n" {
		t.Fatalf("existing file overwritten: %q", b)
	}
	if b, err := os.ReadFile(filepath.Join(root, e.taskID, "notes", "20261101T090001-b2.md")); err != nil || string(b) != "exported\n" {
		t.Fatalf("absent file not moved: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(root, e.taskID+".migrating")); !os.IsNotExist(err) {
		t.Fatal("staging left behind")
	}

	// Done parts are skipped.
	before := ex.exports
	if err := m.Run(ctx); err != nil || ex.exports != before || ex.recorded != 1 {
		t.Fatalf("done part ran again: exports %d->%d recorded %d err %v", before, ex.exports, ex.recorded, err)
	}
}
