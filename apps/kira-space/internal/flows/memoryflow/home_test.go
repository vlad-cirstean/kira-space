package memoryflow_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/memory"
)

// Memory lives under the Space home and is created on first use only: neither app boot nor a
// memory-mcp start (initialize, tools/list) creates it. No UI half: nothing user-visible changes.
func TestMemoryHomeLazy(t *testing.T) {
	app := flowharness.New(t)
	home := filepath.Join(app.SpaceHome, "memory")
	oldHome := filepath.Join(app.Home, ".kira-memory")
	absent := func(step string) {
		t.Helper()
		for _, p := range []string{home, oldHome} {
			if _, err := os.Stat(p); !os.IsNotExist(err) {
				t.Fatalf("%s: %s exists (%v)", step, p, err)
			}
		}
	}
	absent("after boot")

	sess, _ := opsSession(t, app, t.TempDir())
	tools, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ingest bool
	for _, tool := range tools.Tools {
		ingest = ingest || tool.Name == "ingest_files"
	}
	if !ingest {
		t.Fatal("ingest tools are not listed")
	}
	absent("after memory-mcp start")

	callRaw(t, sess, "search_memories", map[string]any{"query": "anything"})
	db := filepath.Join(home, "memory.db")
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("first tool call left no memory.db: %v", err)
	}
	if _, err := os.Stat(oldHome); !os.IsNotExist(err) {
		t.Fatalf("%s exists (%v)", oldHome, err)
	}

	if got := memory.DefaultPath(); got != db {
		t.Fatalf("DefaultPath = %s, want %s", got, db)
	}
	if _, err := app.W.Memory.Recent(ctx); err != nil {
		t.Fatal(err)
	}
}
