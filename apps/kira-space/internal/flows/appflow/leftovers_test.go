package appflow_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/leftovers"
	"github.com/kirathecat/kira-studio/internal/filelock"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// The boot cleanup removes every allowlisted leftover of a populated v0.3.1 home and nothing else: the
// live files, this chapter's backup, a near-miss name and a symlink out of the home all survive.
func TestLeftoversCleanupRemovesOnlyTheAllowlist(t *testing.T) {
	var home, outside string
	dead := []string{
		"studio.db", "studio.db-wal", "studio.db-shm", "git.sock", "llm-prompts.json",
		"mobile/ca.key", "mobile/ca.crt", "mobile/stale.tmp",
		"connections/a.yaml", "api/b.yaml", "runs/c.yaml", "workflows/d.yaml", "repos/e.yaml", "agent/f.yaml",
		"config/connections.p539tmp/g.yaml",
		"backups/p539-20260101T000000Z/kira-studio/kira.db", "backups/p544-20260102T000000Z/studio.db",
		"backups/p552-20260103T000000Z/kira.db",
	}
	live := []string{
		"mcp-db-token.json", "mcp-header-helper.sh", "agent-usage.json", "studio.db.bak",
		"mobile/server.pem", "config/connections/keep.yaml",
		"backups/p557-20260301T000000Z/agent-secrets.db", "backups/p558-20260401T000000Z/x",
		"backups/p539-notes.txt",
	}
	app := flowharness.New(t, flowharness.WithSeed(func(a *flowharness.App) {
		home = a.SpaceHome
		outside = filepath.Join(a.Root, "outside")
		for _, rel := range append(append([]string{}, dead...), live...) {
			touch(t, filepath.Join(home, filepath.FromSlash(rel)))
		}
		touch(t, filepath.Join(outside, "precious.txt"))
		if err := os.Symlink(outside, filepath.Join(home, "steps")); err != nil {
			t.Fatal(err)
		}
	}))
	check := func(step string) {
		t.Helper()
		for _, rel := range dead {
			if exists(filepath.Join(home, filepath.FromSlash(rel))) {
				t.Errorf("%s: %s survived", step, rel)
			}
		}
		for _, rel := range live {
			if !exists(filepath.Join(home, filepath.FromSlash(rel))) {
				t.Errorf("%s: decoy %s was removed", step, rel)
			}
		}
		if !exists(filepath.Join(outside, "precious.txt")) {
			t.Errorf("%s: a symlinked name deleted its target", step)
		}
		if !exists(filepath.Join(home, "steps")) {
			t.Errorf("%s: the symlink itself was removed", step)
		}
		if !exists(filepath.Join(home, "kira.db")) {
			t.Errorf("%s: kira.db was removed", step)
		}
	}
	check("first boot")
	app.Restart()
	check("second boot")
}

// The retired Studio home goes only when no running Studio holds its lock; a boot under a home override
// never touches it.
func TestLeftoversStudioHome(t *testing.T) {
	var studio, notes string
	app := flowharness.New(t, flowharness.WithSeed(func(a *flowharness.App) {
		studio = filepath.Join(a.Home, ".kira-studio")
		notes = filepath.Join(a.Home, ".kira-studio-notes")
		touch(t, filepath.Join(studio, "kira.db"))
		touch(t, filepath.Join(studio, "connections", "a.json"))
		touch(t, notes)
	}))
	if !exists(studio) {
		t.Fatal("a boot under KIRA_SPACE_HOME removed the Studio home")
	}

	held, ok, err := filelock.TryLock(filepath.Join(studio, "app.lock"))
	if err != nil || !ok {
		t.Fatalf("lock = %v, %v", ok, err)
	}
	leftovers.Run(leftovers.Options{SpaceHome: app.SpaceHome, StudioHome: studio})
	if !exists(filepath.Join(studio, "kira.db")) {
		t.Fatal("the Studio home was removed while its lock was held")
	}
	_ = held.Close()

	leftovers.Run(leftovers.Options{SpaceHome: app.SpaceHome, StudioHome: studio})
	if exists(studio) {
		t.Fatal("the Studio home survived with no lock held")
	}
	if !exists(notes) {
		t.Fatal("a sibling with a similar name was removed")
	}
	leftovers.Run(leftovers.Options{SpaceHome: app.SpaceHome, StudioHome: studio})
}
