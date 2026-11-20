package gitsession

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitops"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitprepare"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/fileservice"
)

// ---------------------------------------------------------------------------------------
// worktree.list (D1)
// ---------------------------------------------------------------------------------------

// WorktreeLockInfo/WorktreePrunableInfo mirror the wire's own WorktreeEntry.locked/.prunable —
// {reason: string} or null, never a bare boolean plus a separate string field (D1).
type WorktreeLockInfo struct {
	Reason string `json:"reason"`
}
type WorktreePrunableInfo struct {
	Reason string `json:"reason"`
}

// WorktreeEntry mirrors D1's own wire shape exactly — Head/Branch nil rather than "" for the
// bare/orphan/detached cases porcelain.WorktreeRecord already distinguishes.
type WorktreeEntry struct {
	Path          string                `json:"path"`
	Head          *string               `json:"head"`
	Branch        *string               `json:"branch"`
	IsBare        bool                  `json:"isBare"`
	IsDetached    bool                  `json:"isDetached"`
	IsMain        bool                  `json:"isMain"`
	IsCurrent     bool                  `json:"isCurrent"`
	Locked        *WorktreeLockInfo     `json:"locked"`
	Prunable      *WorktreePrunableInfo `json:"prunable"`
	OpenElsewhere bool                  `json:"openElsewhere"`
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// rawWorktreeList spawns WorktreeListArgs and parses it — the one spawn worktree.list, both
// pre-flights, and the remove op's own re-check all share (D1's own doc comment: "never cached").
func (e *RepoEntry) rawWorktreeList(ctx context.Context) ([]porcelain.WorktreeRecord, error) {
	raw, err := e.runOne(ctx, porcelain.WorktreeListArgs())
	if err != nil {
		return nil, err
	}
	return porcelain.ParseWorktreeList(raw)
}

func findWorktree(records []porcelain.WorktreeRecord, path string) (porcelain.WorktreeRecord, bool) {
	target := filepath.Clean(path)
	for _, r := range records {
		if filepath.Clean(r.Path) == target {
			return r, true
		}
	}
	return porcelain.WorktreeRecord{}, false
}

// Worktrees is worktree.list's own query — `worktree list`'s own first record is
// always the main worktree, so IsMain is positional, never re-derived from a separate spawn.
// OpenElsewhere is computed per entry via e.isOpen (Registry.IsOpen) for every entry other than
// this one's own current worktree — never for IsCurrent (a worktree cannot be "open elsewhere"
// relative to itself).
func (e *RepoEntry) Worktrees(ctx context.Context) ([]WorktreeEntry, error) {
	records, err := e.rawWorktreeList(ctx)
	if err != nil {
		return nil, err
	}
	ownRoot := filepath.Clean(e.Summary.Root)

	out := make([]WorktreeEntry, 0, len(records))
	for i, r := range records {
		entry := WorktreeEntry{
			Path: r.Path, Head: nonEmpty(r.Head), Branch: nonEmpty(r.Branch),
			IsBare: r.Bare, IsDetached: r.Detached,
			IsMain:    i == 0,
			IsCurrent: filepath.Clean(r.Path) == ownRoot,
		}
		if r.Locked {
			entry.Locked = &WorktreeLockInfo{Reason: r.LockedReason}
		}
		if r.Prunable {
			entry.Prunable = &WorktreePrunableInfo{Reason: r.PrunableReason}
		}
		if e.isOpen != nil && !entry.IsCurrent {
			entry.OpenElsewhere = e.isOpen(r.Path)
		}
		out = append(out, entry)
	}
	return out, nil
}

// ---------------------------------------------------------------------------------------
// preflight.worktreeAdd (D4)
// ---------------------------------------------------------------------------------------

// WorktreeAddParams is preflight.worktreeAdd's own request, minus repoId (the entry's own).
type WorktreeAddParams struct {
	Path       string
	Mode       string // "existingBranch" | "newBranch" | "detach"
	Branch     string
	StartPoint string
}

// isPathInsideRepo: Path resolves (Abs, never symlink-resolved — a legal nesting question, not a
// security boundary; D4's own note, never a blocker) inside root's own working tree.
func isPathInsideRepo(path, root string) bool {
	absPath, err1 := filepath.Abs(path)
	absRoot, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// WorktreeAddPreflight is preflight.worktreeAdd's own orchestration: no new spawn beyond
// the refs snapshot pre-flight already uses for branchCheckedOutElsewhere/branchExists, plus one
// resolveCommit re-check for the start point and two plain os.Stat calls for the path itself.
func (e *RepoEntry) WorktreeAddPreflight(ctx context.Context, p WorktreeAddParams) (gitpreflight.WorktreeAddPreflight, error) {
	snapshot, err := e.refsSnapshot(ctx)
	if err != nil {
		return gitpreflight.WorktreeAddPreflight{}, err
	}

	pathExists := false
	if _, statErr := fileservice.Host().Stat(p.Path); statErr == nil {
		pathExists = true
	}
	parentDirMissing := false
	if !pathExists {
		if _, statErr := fileservice.Host().Stat(filepath.Dir(p.Path)); statErr != nil {
			parentDirMissing = true
		}
	}

	var branchCheckedOutElsewhere *string
	branchExists := false
	if p.Mode == "existingBranch" {
		for _, r := range snapshot.Branches {
			if r.ShortName == p.Branch {
				branchCheckedOutElsewhere = r.CheckedOutIn
				break
			}
		}
		// refsSnapshot drops this session's own worktree; for an add it is another worktree.
		if branchCheckedOutElsewhere == nil && snapshot.Head.Kind == "branch" && snapshot.Head.Name == p.Branch {
			root := e.Summary.Root
			branchCheckedOutElsewhere = &root
		}
	}
	if p.Mode == "newBranch" {
		for _, r := range snapshot.Branches {
			if r.ShortName == p.Branch {
				branchExists = true
				break
			}
		}
	}

	startPointRef := p.StartPoint
	if p.Mode == "existingBranch" {
		startPointRef = p.Branch
	}
	startPointResolves := false
	if startPointRef != "" {
		resolved, rerr := e.resolveCommit(ctx, startPointRef)
		if rerr != nil {
			return gitpreflight.WorktreeAddPreflight{}, rerr
		}
		startPointResolves = resolved != nil
	}

	return gitpreflight.ClassifyWorktreeAdd(gitpreflight.ClassifyWorktreeAddInput{
		Path: p.Path, Mode: p.Mode, Branch: p.Branch, StartPoint: p.StartPoint,
		PathExists: pathExists, PathInsideRepo: isPathInsideRepo(p.Path, e.Summary.Root), ParentDirMissing: parentDirMissing,
		PathInsideGitDir:          isPathInsideRepo(p.Path, filepath.Join(e.Summary.Root, ".git")) || (e.Summary.GitDir != "" && isPathInsideRepo(p.Path, e.Summary.GitDir)) || (e.Summary.CommonDir != "" && isPathInsideRepo(p.Path, e.Summary.CommonDir)),
		BranchCheckedOutElsewhere: branchCheckedOutElsewhere, BranchExists: branchExists,
		StartPointResolves: startPointResolves,
	}), nil
}

// ---------------------------------------------------------------------------------------
// preflight.worktreeRemove (D8)
// ---------------------------------------------------------------------------------------

// runOneInDir runs a READ-ONLY spawn against an arbitrary directory — not necessarily this
// entry's own root — through this SAME entry's own read gate (e.Repo.Read): worktree remove's own
// dirty check targets a DIFFERENT worktree's own working tree, but every worktree under one
// `.git` shares the same object/ref store, so serializing it alongside this entry's other reads is
// the correct concurrency discipline, not merely a convenient one.
func (e *RepoEntry) runOneInDir(ctx context.Context, dir string, args []string) ([]byte, error) {
	var out []byte
	err := e.Repo.Read(ctx, func(ctx context.Context) error {
		res, rerr := gitclient.Run(ctx, e.Repo.Runner(), e.Repo.GitPath(), gitclient.Spec{
			Dir: dir, Args: args, ReadOnly: true,
		})
		if cerr := gitclient.Classify(ctx, args, res, rerr); cerr != nil {
			return cerr
		}
		out = res.Stdout
		return nil
	})
	return out, err
}

// DirtyToken fingerprints the uncommitted set of a worktree as the user is shown it: each non-ignored
// entry's kind, status and path (an untracked directory is one path). "" means nothing is uncommitted.
// Discard confirms this token, so it deletes only the paths the user saw. Contents are left out on
// purpose: saving a listed file again must not invalidate the token, and no file walk is needed.
func DirtyToken(entries []porcelain.StatusEntry) string {
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Kind == "ignored" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %c%c %s <- %s", e.Kind, e.Staged, e.Unstaged, e.Path, e.OriginalPath))
	}
	if len(lines) == 0 {
		return ""
	}
	slices.Sort(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:16])
}

// WorktreeRemovePreflight is preflight.worktreeRemove's own orchestration: notAWorktree
// is checked first and, when true, short-circuits every other read (the security gate — there is
// nothing left worth computing about a path that is not a real worktree). The dirty check is
// skipped whenever any of the four other blockers already applies — one fewer spawn, and
// irrelevant anyway since ClassifyWorktreeRemove's own verdict is "blocked" regardless of Dirty
// once any blocker is present.
func (e *RepoEntry) WorktreeRemovePreflight(ctx context.Context, path string) (gitpreflight.WorktreeRemovePreflight, error) {
	records, err := e.rawWorktreeList(ctx)
	if err != nil {
		return gitpreflight.WorktreeRemovePreflight{}, err
	}

	target, isWorktree := findWorktree(records, path)
	in := gitpreflight.ClassifyWorktreeRemoveInput{Path: path, IsWorktree: isWorktree}
	if !isWorktree {
		return gitpreflight.ClassifyWorktreeRemove(in), nil
	}

	if len(records) > 0 {
		in.IsMainWorktree = filepath.Clean(target.Path) == filepath.Clean(records[0].Path)
	}
	in.IsCurrentWorktree = filepath.Clean(target.Path) == filepath.Clean(e.Summary.Root)
	if e.isOpen != nil {
		in.OpenInAnotherWindow = e.isOpen(target.Path)
	}
	if target.Locked {
		reason := target.LockedReason
		in.LockedReason = &reason
	}

	if in.IsMainWorktree || in.IsCurrentWorktree || in.OpenInAnotherWindow || in.LockedReason != nil {
		return gitpreflight.ClassifyWorktreeRemove(in), nil
	}

	status, err := e.WorktreeStatus(ctx, target.Path)
	if err != nil {
		return gitpreflight.WorktreeRemovePreflight{}, err
	}
	in.DirtyToken = DirtyToken(status)
	return gitpreflight.ClassifyWorktreeRemove(in), nil
}

// ---------------------------------------------------------------------------------------
// opTable's two new kinds (D2) — worktreeAdd/worktreeRemove
// ---------------------------------------------------------------------------------------

// worktreeAddBlockedError maps ClassifyWorktreeAdd's own five blocker kinds onto EXISTING
// OpErrorKind values (exactly one new kind is added to the whole contract, WorktreeLocked,
// spent entirely on the remove side) — a defensive re-check server-side; the dialog's own Create
// button is disabled by the SAME preflight before this is ever reachable in ordinary use.
func worktreeAddBlockedError(pf gitpreflight.WorktreeAddPreflight) *OpError {
	if len(pf.Blockers) == 0 {
		return &OpError{Kind: "Unknown", Message: "This worktree cannot be created."}
	}
	b := pf.Blockers[0]
	switch b.Kind {
	case "pathExists":
		return &OpError{Kind: "AlreadyExists", Message: fmt.Sprintf("%s already exists.", b.Path)}
	case "branchExists":
		return &OpError{Kind: "AlreadyExists", Message: fmt.Sprintf("A branch named %q already exists.", b.Branch)}
	case "branchCheckedOutElsewhere":
		return &OpError{Kind: "WorktreeConflict", Message: fmt.Sprintf("%q is already checked out at %s.", b.Branch, b.WorktreePath)}
	case "unknownStartPoint":
		return &OpError{Kind: "NotFound", Message: fmt.Sprintf("%s does not resolve to a commit.", b.StartPoint)}
	default: // invalidPath
		return &OpError{Kind: "Unknown", Message: "The worktree path is invalid."}
	}
}

// prepareWorktreeAdd is worktreeAdd's own opSpec.Prepare: re-runs the SAME preflight
// classification host-side immediately before the write (never trusting a client-side preflight
// that may have gone stale), then builds exactly one of the three explicit-commit-ish argv
// variants — bare DWIM is never reachable from this path.
func prepareWorktreeAdd(ctx context.Context, e *RepoEntry, _ ConnID, _ string, op OpRequest) (prepared, error) {
	pf, err := e.WorktreeAddPreflight(ctx, WorktreeAddParams{
		Path: op.Path, Mode: op.Mode, Branch: op.Branch, StartPoint: op.StartPoint,
	})
	if err != nil {
		return prepared{}, err
	}
	if pf.Verdict == "blocked" {
		return prepared{earlyError: worktreeAddBlockedError(pf)}, nil
	}

	var argv []string
	switch op.Mode {
	case "existingBranch":
		argv = gitops.WorktreeAddExistingBranchArgs(op.Path, op.Branch)
	case "newBranch":
		argv = gitops.WorktreeAddNewBranchArgs(op.Path, op.Branch, op.StartPoint)
	case "detach":
		argv = gitops.WorktreeAddDetachArgs(op.Path, op.StartPoint)
	default:
		return prepared{earlyError: &OpError{Kind: "Unknown", Message: "unrecognised worktree creation mode: " + op.Mode}}, nil
	}
	return prepared{argvList: [][]string{argv}}, nil
}

// worktreeRemoveBlockedError maps ClassifyWorktreeRemove's own five blocker kinds — "locked" is the
// one row that reuses the new WorktreeLocked kind (the exact remedy git itself would name);
// every other blocker is a structural refusal the UI should never let a user reach in the first
// place (the Remove button/menu item is disabled for all four), so "Unknown" plus a plain-English
// reason is what a defensive server-side re-check answers for them.
func worktreeRemoveBlockedError(pf gitpreflight.WorktreeRemovePreflight) *OpError {
	if len(pf.Blockers) == 0 {
		return &OpError{Kind: "Unknown", Message: "This worktree cannot be removed."}
	}
	b := pf.Blockers[0]
	switch b.Kind {
	case "locked":
		return &OpError{Kind: "WorktreeLocked", Message: fmt.Sprintf("This worktree is locked (%s) — unlock it first.", b.Reason)}
	case "mainWorktree":
		return &OpError{Kind: "Unknown", Message: "The main worktree cannot be removed."}
	case "currentWorktree":
		return &OpError{Kind: "Unknown", Message: "This worktree is open in this window and cannot remove itself."}
	case "openInAnotherWindow":
		return &OpError{Kind: "Unknown", Message: "This worktree is open in another window — close it there first."}
	default: // notAWorktree
		return &OpError{Kind: "Unknown", Message: "This path is not one of this repository's worktrees."}
	}
}

// WorktreeRemoveBlockedMessage is the plain-English reason the preflight blocks a removal, for callers
// that refuse before running the op.
func WorktreeRemoveBlockedMessage(pf gitpreflight.WorktreeRemovePreflight) string {
	return worktreeRemoveBlockedError(pf).Message
}

// prepareWorktreeRemove is worktreeRemove's own opSpec.Prepare: re-runs the FULL
// preflight classification immediately before the write (never trusting the client's own possibly
// stale copy), and — for the dirty route — re-derives the confirmation token FRESH from that same
// re-check and compares it byte for byte against what the client sent; force is decided
// server-side from the fresh verdict, never taken as the client's own claim (fail-safe over
// fail-open, the stated principle for this destructive path). The token hashes the uncommitted set, so
// a worktree that changed since the user looked is refused.
func prepareWorktreeRemove(ctx context.Context, e *RepoEntry, _ ConnID, _ string, op OpRequest) (prepared, error) {
	pf, err := e.WorktreeRemovePreflight(ctx, op.Path)
	if err != nil {
		return prepared{}, err
	}
	if pf.Verdict == "blocked" {
		return prepared{earlyError: worktreeRemoveBlockedError(pf)}, nil
	}

	force := false
	if pf.Verdict == "dirty" {
		if op.ConfirmToken == nil {
			return prepared{earlyError: &OpError{
				Kind:    "ConfirmationRequired",
				Message: "This worktree has uncommitted changes. Discard them to remove it.",
			}}, nil
		}
		if *op.ConfirmToken != pf.ConfirmToken {
			return prepared{earlyError: &OpError{
				Kind:    "ConfirmationRequired",
				Message: "The uncommitted changes in this worktree changed since you reviewed them. Review them again before discarding.",
			}}, nil
		}
		force = true
	}
	return prepared{argvList: [][]string{gitops.WorktreeRemoveArgs(op.Path, force)}}, nil
}

// ParsePrepareTimeout parses the per-repo worktreePrepareTimeout leaf. A stored value that no longer
// validates falls back to the default and is logged, never blocks the run.
func ParsePrepareTimeout(stored string) (time.Duration, string) {
	d, err := model.ParsePrepareTimeout(stored)
	if err == nil {
		return d, stored
	}
	slog.Warn("worktree prepare: invalid timeout, using default", "scope", "git", "value", stored, "err", err)
	return gitprepare.DefaultPrepareTimeout, model.DefaultPrepareTimeout
}
