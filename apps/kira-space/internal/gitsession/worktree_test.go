package gitsession

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// --- test fixtures ---------------------------------------------------------------------------

// newWorktreeTestEntry opens repoDir exactly like newQueriesTestEntry.
func newWorktreeTestEntry(t *testing.T, repoDir string) *RepoEntry {
	t.Helper()
	_, entry := newTestEntry(t, ConnID("worktree-test-conn"), repoDir, testEntryOpts{})
	return entry
}

// initWorktreeTestRepo builds a bare-minimum one-commit repo on branch "main".
func initWorktreeTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitQ(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write f.txt: %v", err)
	}
	runGitQ(t, dir, "add", "f.txt")
	runGitQ(t, dir, "commit", "-q", "-m", "base")
	return dir
}

// --- Worktrees (worktree.list, D1) ------------------------------------------------------------

func TestWorktrees_MainAndLinked(t *testing.T) {
	t.Parallel()
	skipWithoutGitQueries(t)
	dir := initWorktreeTestRepo(t)
	wtPath := filepath.Join(t.TempDir(), "linked")
	runGitQ(t, dir, "worktree", "add", "-b", "feature", wtPath)

	entry := newWorktreeTestEntry(t, dir)
	list, err := entry.Worktrees(context.Background())
	if err != nil {
		t.Fatalf("Worktrees: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d worktrees, want 2: %+v", len(list), list)
	}
	if !list[0].IsMain || !list[0].IsCurrent {
		t.Fatalf("first entry = %+v, want IsMain and IsCurrent both true (this entry's own root)", list[0])
	}
	if list[1].IsMain || list[1].IsCurrent {
		t.Fatalf("second entry = %+v, want IsMain and IsCurrent both false", list[1])
	}
	if list[1].Branch == nil || *list[1].Branch != "refs/heads/feature" {
		t.Fatalf("second entry branch = %v, want refs/heads/feature", list[1].Branch)
	}
}

// --- WorktreeAddPreflight (D4) -----------------------------------------------------------------

func TestWorktreeAddPreflight_Clean(t *testing.T) {
	t.Parallel()
	skipWithoutGitQueries(t)
	dir := initWorktreeTestRepo(t)
	entry := newWorktreeTestEntry(t, dir)
	pf, err := entry.WorktreeAddPreflight(context.Background(), WorktreeAddParams{
		Path: filepath.Join(t.TempDir(), "wt"), Mode: "newBranch", Branch: "topic", StartPoint: "main",
	})
	if err != nil {
		t.Fatalf("WorktreeAddPreflight: %v", err)
	}
	if pf.Verdict != "clean" {
		t.Fatalf("got %+v", pf)
	}
}

func TestWorktreeAddPreflight_BranchCheckedOutElsewhere(t *testing.T) {
	t.Parallel()
	skipWithoutGitQueries(t)
	dir := initWorktreeTestRepo(t)
	wtPath := filepath.Join(t.TempDir(), "linked")
	runGitQ(t, dir, "worktree", "add", "-b", "feature", wtPath)

	entry := newWorktreeTestEntry(t, dir)
	pf, err := entry.WorktreeAddPreflight(context.Background(), WorktreeAddParams{
		Path: filepath.Join(t.TempDir(), "wt2"), Mode: "existingBranch", Branch: "feature",
	})
	if err != nil {
		t.Fatalf("WorktreeAddPreflight: %v", err)
	}
	if pf.Verdict != "blocked" || len(pf.Blockers) != 1 || pf.Blockers[0].Kind != "branchCheckedOutElsewhere" {
		t.Fatalf("got %+v", pf)
	}
}

// --- WorktreeRemovePreflight / RunOp worktreeRemove (D8/F7/F8) ----------------------------------

func TestWorktreeRemovePreflight_MainAndCurrentBlocked(t *testing.T) {
	t.Parallel()
	skipWithoutGitQueries(t)
	dir := initWorktreeTestRepo(t)
	entry := newWorktreeTestEntry(t, dir)
	pf, err := entry.WorktreeRemovePreflight(context.Background(), dir)
	if err != nil {
		t.Fatalf("WorktreeRemovePreflight: %v", err)
	}
	if pf.Verdict != "blocked" {
		t.Fatalf("got %+v", pf)
	}
	kinds := map[string]bool{}
	for _, b := range pf.Blockers {
		kinds[b.Kind] = true
	}
	if !kinds["mainWorktree"] || !kinds["currentWorktree"] {
		t.Fatalf("blockers = %+v, want both mainWorktree and currentWorktree", pf.Blockers)
	}
}

// TestRunOp_WorktreeRemove_DirtyRequiresConfirmation is the "confirmation-refusal test" ("a dirty
// forced removal requires a typed token re-checked against a freshly-read status"): no token ->
// refused; wrong token -> refused; stale token (set changed since) -> refused; the fresh token succeeds.
func TestRunOp_WorktreeRemove_DirtyRequiresConfirmation(t *testing.T) {
	t.Parallel()
	skipWithoutGitQueries(t)
	dir := initWorktreeTestRepo(t)
	wtPath := filepath.Join(t.TempDir(), "feature-wt")
	runGitQ(t, dir, "worktree", "add", "-b", "feature", wtPath)
	if err := os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("uncommitted"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}

	entry := newWorktreeTestEntry(t, dir)
	ctx := context.Background()

	// No token at all.
	result, err := entry.RunOp(ctx, ConnID("c"), "label", OpRequest{Kind: "worktreeRemove", Path: wtPath})
	if err != nil {
		t.Fatalf("RunOp (no token): %v", err)
	}
	if result.OK || result.Error == nil || result.Error.Kind != "ConfirmationRequired" {
		t.Fatalf("got %+v, want ConfirmationRequired", result)
	}

	// Wrong token.
	wrong := "not-the-basename"
	result, err = entry.RunOp(ctx, ConnID("c"), "label", OpRequest{Kind: "worktreeRemove", Path: wtPath, ConfirmToken: &wrong})
	if err != nil {
		t.Fatalf("RunOp (wrong token): %v", err)
	}
	if result.OK || result.Error == nil || result.Error.Kind != "ConfirmationRequired" {
		t.Fatalf("got %+v, want ConfirmationRequired", result)
	}

	// The worktree must still exist after both refusals.
	records, err := entry.rawWorktreeList(ctx)
	if err != nil {
		t.Fatalf("rawWorktreeList: %v", err)
	}
	if _, ok := findWorktree(records, wtPath); !ok {
		t.Fatal("worktree was removed despite both confirmations being refused")
	}

	// A token fingerprinting an earlier dirty set is refused after the set changes.
	pf, err := entry.WorktreeRemovePreflight(ctx, wtPath)
	if err != nil || pf.ConfirmToken == "" {
		t.Fatalf("preflight = %+v, %v", pf, err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "more.txt"), []byte("later"), 0o644); err != nil {
		t.Fatalf("write more.txt: %v", err)
	}
	stale := pf.ConfirmToken
	result, err = entry.RunOp(ctx, ConnID("c"), "label", OpRequest{Kind: "worktreeRemove", Path: wtPath, ConfirmToken: &stale})
	if err != nil || result.OK || result.Error == nil || result.Error.Kind != "ConfirmationRequired" {
		t.Fatalf("stale token: %+v, %v, want ConfirmationRequired", result, err)
	}

	// The fresh token succeeds.
	fresh, err := entry.WorktreeRemovePreflight(ctx, wtPath)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	correct := fresh.ConfirmToken
	result, err = entry.RunOp(ctx, ConnID("c"), "label", OpRequest{Kind: "worktreeRemove", Path: wtPath, ConfirmToken: &correct})
	if err != nil {
		t.Fatalf("RunOp (correct token): %v", err)
	}
	if !result.OK {
		t.Fatalf("got %+v, want OK", result)
	}
	records, err = entry.rawWorktreeList(ctx)
	if err != nil {
		t.Fatalf("rawWorktreeList: %v", err)
	}
	if _, ok := findWorktree(records, wtPath); ok {
		t.Fatal("worktree still present after a correctly-confirmed removal")
	}
}

func TestRunOp_WorktreeRemove_MainWorktreeBlocked(t *testing.T) {
	t.Parallel()
	skipWithoutGitQueries(t)
	dir := initWorktreeTestRepo(t)
	entry := newWorktreeTestEntry(t, dir)
	result, err := entry.RunOp(context.Background(), ConnID("c"), "label", OpRequest{Kind: "worktreeRemove", Path: dir})
	if err != nil {
		t.Fatalf("RunOp: %v", err)
	}
	if result.OK || result.Error == nil {
		t.Fatalf("got %+v, want a refusal", result)
	}
}

func TestRunOp_WorktreeAdd_NewBranch(t *testing.T) {
	t.Parallel()
	skipWithoutGitQueries(t)
	dir := initWorktreeTestRepo(t)
	entry := newWorktreeTestEntry(t, dir)
	wtPath := filepath.Join(t.TempDir(), "new-wt")

	result, err := entry.RunOp(context.Background(), ConnID("c"), "label", OpRequest{
		Kind: "worktreeAdd", Path: wtPath, Mode: "newBranch", Branch: "topic", StartPoint: "main",
	})
	if err != nil {
		t.Fatalf("RunOp: %v", err)
	}
	if !result.OK {
		t.Fatalf("got %+v", result)
	}
	records, err := entry.rawWorktreeList(context.Background())
	if err != nil {
		t.Fatalf("rawWorktreeList: %v", err)
	}
	rec, ok := findWorktree(records, wtPath)
	if !ok || rec.Branch != "refs/heads/topic" {
		t.Fatalf("got records=%+v", records)
	}
}

// TestRunOp_WorktreeAdd_BlockedNeverSpawns proves a blocked preflight never reaches git at all —
// an unresolvable start point must leave no worktree behind.
func TestRunOp_WorktreeAdd_BlockedNeverSpawns(t *testing.T) {
	t.Parallel()
	skipWithoutGitQueries(t)
	dir := initWorktreeTestRepo(t)
	entry := newWorktreeTestEntry(t, dir)
	wtPath := filepath.Join(t.TempDir(), "bad-wt")

	result, err := entry.RunOp(context.Background(), ConnID("c"), "label", OpRequest{
		Kind: "worktreeAdd", Path: wtPath, Mode: "newBranch", Branch: "topic", StartPoint: "no-such-ref",
	})
	if err != nil {
		t.Fatalf("RunOp: %v", err)
	}
	if result.OK || result.Error == nil || result.Error.Kind != "NotFound" {
		t.Fatalf("got %+v, want NotFound", result)
	}
	if _, statErr := os.Stat(wtPath); statErr == nil {
		t.Fatal("the target path was created despite the blocked preflight")
	}
}
