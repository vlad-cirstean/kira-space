package gitpreflight_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
)

func TestClassifyWorktreeAdd_Clean(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeAdd(gitpreflight.ClassifyWorktreeAddInput{
		Path: "/repos/wt", Mode: "newBranch", Branch: "topic", StartPoint: "main",
		StartPointResolves: true,
	})
	if got.Verdict != "clean" || len(got.Blockers) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestClassifyWorktreeAdd_InvalidPath(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeAdd(gitpreflight.ClassifyWorktreeAddInput{
		Path: "", Mode: "detach", StartPoint: "abc123", StartPointResolves: true,
	})
	if got.Verdict != "blocked" || len(got.Blockers) != 1 || got.Blockers[0].Kind != "invalidPath" {
		t.Fatalf("got %+v", got)
	}
}

func TestClassifyWorktreeAdd_PathExists(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeAdd(gitpreflight.ClassifyWorktreeAddInput{
		Path: "/repos/wt", Mode: "detach", StartPoint: "abc123",
		PathExists: true, StartPointResolves: true,
	})
	if got.Verdict != "blocked" || len(got.Blockers) != 1 || got.Blockers[0].Kind != "pathExists" {
		t.Fatalf("got %+v", got)
	}
}

// TestClassifyWorktreeAdd_BranchCheckedOutElsewhere is widened at G28 D7: when
// branchCheckedOutElsewhere is the SOLE blocker, Routes offers "detachHere" — closing G25 §9's own
// named hand-forward.
func TestClassifyWorktreeAdd_BranchCheckedOutElsewhere(t *testing.T) {
	t.Parallel()
	elsewhere := "/repos/other-wt"
	got := gitpreflight.ClassifyWorktreeAdd(gitpreflight.ClassifyWorktreeAddInput{
		Path: "/repos/wt", Mode: "existingBranch", Branch: "feature",
		BranchCheckedOutElsewhere: &elsewhere, StartPointResolves: true,
	})
	if got.Verdict != "blocked" || len(got.Blockers) != 1 {
		t.Fatalf("got %+v", got)
	}
	b := got.Blockers[0]
	if b.Kind != "branchCheckedOutElsewhere" || b.Branch != "feature" || b.WorktreePath != elsewhere {
		t.Fatalf("blocker = %+v", b)
	}
	if len(got.Routes) != 1 || got.Routes[0] != "detachHere" {
		t.Fatalf("Routes = %v, want exactly [detachHere] (D7's sole-blocker case)", got.Routes)
	}
}

// TestClassifyWorktreeAdd_DetachHere_SuppressedAlongsideAnotherBlocker is D7's own negative case:
// branchCheckedOutElsewhere alongside ANY other blocker withholds the route — detaching cannot fix
// an invalid path, an existing path, an existing branch name, or an unresolved start point.
func TestClassifyWorktreeAdd_DetachHere_SuppressedAlongsideAnotherBlocker(t *testing.T) {
	t.Parallel()
	elsewhere := "/repos/other-wt"
	got := gitpreflight.ClassifyWorktreeAdd(gitpreflight.ClassifyWorktreeAddInput{
		Path: "/repos/wt", Mode: "existingBranch", Branch: "feature",
		BranchCheckedOutElsewhere: &elsewhere, StartPointResolves: true,
		PathExists: true,
	})
	if len(got.Blockers) != 2 {
		t.Fatalf("got %+v, want 2 blockers (pathExists, branchCheckedOutElsewhere)", got)
	}
	if len(got.Routes) != 0 {
		t.Fatalf("Routes = %v, want none when branchCheckedOutElsewhere is not the sole blocker", got.Routes)
	}
}

// TestClassifyWorktreeAdd_DetachHere_NeverForDetachMode proves detachHere is meaningless (and
// never offered) for a request already in detach mode — ClassifyWorktreeAddInput's own
// BranchCheckedOutElsewhere is defined only for existingBranch mode anyway, so this proves the
// route stays empty rather than depending on that convention alone.
func TestClassifyWorktreeAdd_DetachHere_NeverForDetachMode(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeAdd(gitpreflight.ClassifyWorktreeAddInput{
		Path: "/repos/wt", Mode: "detach", StartPoint: "abc123", StartPointResolves: true,
	})
	if len(got.Routes) != 0 {
		t.Fatalf("Routes = %v, want none for a clean detach-mode request", got.Routes)
	}
}

// TestClassifyWorktreeAdd_RoutesNeverNil proves the empty-slice-never-nil convention this package
// follows elsewhere (Blockers/Notes) also holds for Routes.
func TestClassifyWorktreeAdd_RoutesNeverNil(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeAdd(gitpreflight.ClassifyWorktreeAddInput{
		Path: "/repos/wt", Mode: "newBranch", Branch: "topic", StartPoint: "main",
		StartPointResolves: true,
	})
	if got.Routes == nil {
		t.Fatal("Routes is nil, want a non-nil empty slice")
	}
}

func TestClassifyWorktreeAdd_BranchExists(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeAdd(gitpreflight.ClassifyWorktreeAddInput{
		Path: "/repos/wt", Mode: "newBranch", Branch: "main", StartPoint: "HEAD",
		BranchExists: true, StartPointResolves: true,
	})
	if got.Verdict != "blocked" || len(got.Blockers) != 1 || got.Blockers[0].Kind != "branchExists" {
		t.Fatalf("got %+v", got)
	}
}

func TestClassifyWorktreeAdd_UnknownStartPoint(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeAdd(gitpreflight.ClassifyWorktreeAddInput{
		Path: "/repos/wt", Mode: "newBranch", Branch: "topic", StartPoint: "no-such-ref",
		StartPointResolves: false,
	})
	if got.Verdict != "blocked" || len(got.Blockers) != 1 || got.Blockers[0].Kind != "unknownStartPoint" || got.Blockers[0].StartPoint != "no-such-ref" {
		t.Fatalf("got %+v", got)
	}
}

// TestClassifyWorktreeAdd_BlockerOrderAndAccumulation proves D4's own blocker order
// (invalidPath, pathExists, branchCheckedOutElsewhere, branchExists, unknownStartPoint) AND that
// earlier blockers never suppress later ones — every applicable blocker is reported.
func TestClassifyWorktreeAdd_BlockerOrderAndAccumulation(t *testing.T) {
	t.Parallel()
	elsewhere := "/repos/other-wt"
	got := gitpreflight.ClassifyWorktreeAdd(gitpreflight.ClassifyWorktreeAddInput{
		Path: "", Mode: "newBranch", Branch: "main", StartPoint: "no-such-ref",
		PathExists: true, BranchExists: true, StartPointResolves: false,
		BranchCheckedOutElsewhere: &elsewhere, // ignored for newBranch mode
	})
	if got.Verdict != "blocked" {
		t.Fatalf("got %+v", got)
	}
	wantKinds := []string{"invalidPath", "pathExists", "branchExists", "unknownStartPoint"}
	if len(got.Blockers) != len(wantKinds) {
		t.Fatalf("got %d blockers, want %d: %+v", len(got.Blockers), len(wantKinds), got.Blockers)
	}
	for i, k := range wantKinds {
		if got.Blockers[i].Kind != k {
			t.Fatalf("blocker %d = %q, want %q (full: %+v)", i, got.Blockers[i].Kind, k, got.Blockers)
		}
	}
}

func TestClassifyWorktreeAdd_Notes(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeAdd(gitpreflight.ClassifyWorktreeAddInput{
		Path: "/repos/main/nested-wt", Mode: "detach", StartPoint: "abc123",
		StartPointResolves: true, PathInsideRepo: true, ParentDirMissing: true,
	})
	if got.Verdict != "clean" {
		t.Fatalf("got %+v", got)
	}
	if len(got.Notes) != 3 {
		t.Fatalf("got %d notes, want 3: %+v", len(got.Notes), got.Notes)
	}
	wantKinds := map[string]bool{"pathInsideRepo": true, "parentDirectoryMissing": true, "detachedHead": true}
	for _, n := range got.Notes {
		if !wantKinds[n.Kind] {
			t.Fatalf("unexpected note kind %q", n.Kind)
		}
		delete(wantKinds, n.Kind)
	}
	if len(wantKinds) != 0 {
		t.Fatalf("missing notes: %v", wantKinds)
	}
}

func TestClassifyWorktreeRemove_Clean(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeRemove(gitpreflight.ClassifyWorktreeRemoveInput{
		Path: "/repos/wt", IsWorktree: true,
	})
	if got.Verdict != "clean" || len(got.Blockers) != 0 || len(got.Routes) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestClassifyWorktreeRemove_NotAWorktree(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeRemove(gitpreflight.ClassifyWorktreeRemoveInput{
		Path: "/etc/passwd", IsWorktree: false,
	})
	if got.Verdict != "blocked" || len(got.Blockers) != 1 || got.Blockers[0].Kind != "notAWorktree" {
		t.Fatalf("got %+v", got)
	}
}

// TestClassifyWorktreeRemove_MainAndCurrentUnconditional proves F8/D8: mainWorktree and
// currentWorktree block even when every other input claims "dirty" — no force route unlocks them.
func TestClassifyWorktreeRemove_MainAndCurrentUnconditional(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeRemove(gitpreflight.ClassifyWorktreeRemoveInput{
		Path: "/repos/main", IsWorktree: true, IsMainWorktree: true, IsCurrentWorktree: true, DirtyToken: "tok",
	})
	if got.Verdict != "blocked" || len(got.Routes) != 0 {
		t.Fatalf("got %+v", got)
	}
	if len(got.Blockers) != 2 || got.Blockers[0].Kind != "mainWorktree" || got.Blockers[1].Kind != "currentWorktree" {
		t.Fatalf("blockers = %+v", got.Blockers)
	}
}

func TestClassifyWorktreeRemove_OpenInAnotherWindow(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeRemove(gitpreflight.ClassifyWorktreeRemoveInput{
		Path: "/repos/wt", IsWorktree: true, OpenInAnotherWindow: true, DirtyToken: "tok",
	})
	if got.Verdict != "blocked" || len(got.Routes) != 0 || len(got.Blockers) != 1 || got.Blockers[0].Kind != "openInAnotherWindow" {
		t.Fatalf("got %+v", got)
	}
}

func TestClassifyWorktreeRemove_LockedUnconditional(t *testing.T) {
	t.Parallel()
	reason := "testing lock reason"
	got := gitpreflight.ClassifyWorktreeRemove(gitpreflight.ClassifyWorktreeRemoveInput{
		Path: "/repos/wt", IsWorktree: true, LockedReason: &reason, DirtyToken: "tok",
	})
	if got.Verdict != "blocked" || len(got.Routes) != 0 || len(got.Blockers) != 1 {
		t.Fatalf("got %+v", got)
	}
	if got.Blockers[0].Kind != "locked" || got.Blockers[0].Reason != reason {
		t.Fatalf("blocker = %+v", got.Blockers[0])
	}
}

// TestClassifyWorktreeRemove_DirtyRequiresTypedConfirmation proves D8/F8: a dirty, otherwise-clear
// worktree gets verdict "dirty", a "force" route, and a confirmation token equal to the
// fingerprint the host computed — never a caller-supplied value.
func TestClassifyWorktreeRemove_DirtyRequiresTypedConfirmation(t *testing.T) {
	t.Parallel()
	got := gitpreflight.ClassifyWorktreeRemove(gitpreflight.ClassifyWorktreeRemoveInput{
		Path: "/repos/feature-wt", IsWorktree: true, DirtyToken: "tok",
	})
	if got.Verdict != "dirty" || len(got.Blockers) != 0 {
		t.Fatalf("got %+v", got)
	}
	if !got.RequiresTypedConfirmation || got.ConfirmToken != "tok" {
		t.Fatalf("got %+v", got)
	}
	if len(got.Routes) != 1 || got.Routes[0] != "force" {
		t.Fatalf("routes = %v", got.Routes)
	}
}

// TestClassifyWorktreeRemove_NotAWorktreeBeatsEveryOtherBlocker proves the security-check ordering:
// notAWorktree is reported even when every other field also claims a blocker, and it is always
// first.
func TestClassifyWorktreeRemove_NotAWorktreeBeatsEveryOtherBlocker(t *testing.T) {
	t.Parallel()
	reason := "x"
	got := gitpreflight.ClassifyWorktreeRemove(gitpreflight.ClassifyWorktreeRemoveInput{
		Path: "/etc/passwd", IsWorktree: false, IsMainWorktree: true, IsCurrentWorktree: true,
		OpenInAnotherWindow: true, LockedReason: &reason, DirtyToken: "tok",
	})
	if got.Verdict != "blocked" || len(got.Blockers) != 5 {
		t.Fatalf("got %+v", got)
	}
	if got.Blockers[0].Kind != "notAWorktree" {
		t.Fatalf("first blocker = %+v, want notAWorktree first", got.Blockers[0])
	}
}
