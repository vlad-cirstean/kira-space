package gitrpc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// These tests exercise the real Router.ForConn dispatch — the same convention reset_test.go
// already established: JSON decode through wire.go's own param types, handlers.go's five new
// switch cases, and gitsession's real orchestration underneath — the one layer gitsession's own
// tests (which call RepoEntry methods directly) never touch.

func worktreeSmokeGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	smokeGit(t, dir, nil, args...)
}

// worktreeSmokeConn opens dir through a fresh Router.ForConn dispatch.
func worktreeSmokeConn(t *testing.T, dir string) (Handlers, string) {
	t.Helper()
	return smokeConn(t, gitsession.ConnID("worktree-rpc-test-conn"), dir, smokeConnOpts{})
}

func initWorktreeSmokeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	worktreeSmokeGit(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	worktreeSmokeGit(t, dir, "add", "f.txt")
	worktreeSmokeGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestWorktreeList_OverDispatch(t *testing.T) {
	dir := initWorktreeSmokeRepo(t)
	wtPath := filepath.Join(t.TempDir(), "linked")
	worktreeSmokeGit(t, dir, "worktree", "add", "-b", "feature", wtPath)

	handlers, repoID := worktreeSmokeConn(t, dir)
	res, err := handlers.Request(context.Background(), "worktree.list", mustJSON(t, WorktreeListParams{RepoID: repoID}))
	if err != nil {
		t.Fatalf("worktree.list: %v", err)
	}
	result, ok := res.(WorktreeListResult)
	if !ok {
		t.Fatalf("result = %T, want WorktreeListResult", res)
	}
	if len(result.Worktrees) != 2 {
		t.Fatalf("got %d worktrees, want 2: %+v", len(result.Worktrees), result.Worktrees)
	}
}

func TestPreflightWorktreeAdd_OverDispatch(t *testing.T) {
	dir := initWorktreeSmokeRepo(t)
	handlers, repoID := worktreeSmokeConn(t, dir)

	res, err := handlers.Request(context.Background(), "preflight.worktreeAdd", mustJSON(t, PreflightWorktreeAddParams{
		RepoID: repoID, Path: filepath.Join(t.TempDir(), "wt"), Mode: "newBranch", Branch: "topic", StartPoint: "main",
	}))
	if err != nil {
		t.Fatalf("preflight.worktreeAdd: %v", err)
	}
	pf, ok := res.(gitpreflight.WorktreeAddPreflight)
	if !ok {
		t.Fatalf("result = %T, want gitpreflight.WorktreeAddPreflight", res)
	}
	if pf.Verdict != "clean" {
		t.Fatalf("got %+v", pf)
	}
}

func TestPreflightWorktreeRemove_MainBlocked_OverDispatch(t *testing.T) {
	dir := initWorktreeSmokeRepo(t)
	handlers, repoID := worktreeSmokeConn(t, dir)

	res, err := handlers.Request(context.Background(), "preflight.worktreeRemove", mustJSON(t, PreflightWorktreeRemoveParams{RepoID: repoID, Path: dir}))
	if err != nil {
		t.Fatalf("preflight.worktreeRemove: %v", err)
	}
	pf, ok := res.(gitpreflight.WorktreeRemovePreflight)
	if !ok {
		t.Fatalf("result = %T, want gitpreflight.WorktreeRemovePreflight", res)
	}
	if pf.Verdict != "blocked" {
		t.Fatalf("got %+v, want blocked (main worktree)", pf)
	}
}

func TestOpRunWorktreeAddAndRemove_OverDispatch(t *testing.T) {
	dir := initWorktreeSmokeRepo(t)
	handlers, repoID := worktreeSmokeConn(t, dir)
	wtPath := filepath.Join(t.TempDir(), "op-wt")

	addRes, err := handlers.Request(context.Background(), "op.run", mustJSON(t, OpRunParams{
		RepoID: repoID,
		Op:     gitsession.OpRequest{Kind: "worktreeAdd", Path: wtPath, Mode: "newBranch", Branch: "topic", StartPoint: "main"},
	}))
	if err != nil {
		t.Fatalf("op.run (worktreeAdd): %v", err)
	}
	addResult, ok := addRes.(gitsession.OpResult)
	if !ok || !addResult.OK {
		t.Fatalf("op.run (worktreeAdd) result = %+v (ok=%v)", addRes, ok)
	}

	// Removing it clean (no dirty files) needs no confirmation.
	removeRes, err := handlers.Request(context.Background(), "op.run", mustJSON(t, OpRunParams{
		RepoID: repoID,
		Op:     gitsession.OpRequest{Kind: "worktreeRemove", Path: wtPath},
	}))
	if err != nil {
		t.Fatalf("op.run (worktreeRemove): %v", err)
	}
	removeResult, ok := removeRes.(gitsession.OpResult)
	if !ok || !removeResult.OK {
		t.Fatalf("op.run (worktreeRemove) result = %+v (ok=%v)", removeRes, ok)
	}
}
