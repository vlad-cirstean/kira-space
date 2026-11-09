package gitsession

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitops"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
)

// worktreeGitDir is a worktree's per-worktree git dir, read from its `.git` entry without a spawn:
// a directory (the main worktree) is the dir itself, a file holds `gitdir: <path>` (a linked one).
func worktreeGitDir(path string) string {
	dot := filepath.Join(path, ".git")
	info, err := os.Stat(dot)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return dot
	}
	raw, err := os.ReadFile(dot)
	if err != nil {
		return ""
	}
	dir, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "gitdir:")
	if !ok {
		return ""
	}
	dir = strings.TrimSpace(dir)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(path, dir)
	}
	return dir
}

// WorktreeInProgress classifies the operation (rebase, merge, cherry-pick...) left half-done in the
// worktree at path; status is that worktree's WorktreeStatus. Nil when none.
func (e *RepoEntry) WorktreeInProgress(path string, status []porcelain.StatusEntry) *gitpreflight.InProgressOperation {
	gitDir := worktreeGitDir(path)
	if gitDir == "" {
		return nil
	}
	files := gitops.ReadInProgressStateFiles(gitDir)
	return gitpreflight.ClassifyInProgress(files, gitpreflight.UnmergedPaths(porcelain.StatusResult{Entries: status}))
}

// WorktreeAbort aborts the operation in progress in the worktree at path. It errors when none is
// in progress or git has no abort for it.
func (e *RepoEntry) WorktreeAbort(ctx context.Context, path string) error {
	status, err := e.WorktreeStatus(ctx, path)
	if err != nil {
		return err
	}
	op := e.WorktreeInProgress(path, status)
	if op == nil {
		return fmt.Errorf("no operation is in progress in %s", path)
	}
	args, ok := gitops.AbortArgs(op.Kind)
	if !ok {
		return fmt.Errorf("git cannot abort a %s", op.Kind)
	}
	return e.Repo.Write(ctx, func(ctx context.Context) error {
		res, rerr := gitclient.Run(ctx, e.Repo.Runner(), e.Repo.GitPath(), gitclient.Spec{
			Dir: path, Args: args, ReadOnly: false, Setsid: true,
		})
		if ctx.Err() != nil || rerr != nil {
			return gitclient.Classify(ctx, args, res, rerr)
		}
		if res.ExitCode != 0 {
			_, msg := gitops.ClassifyOpError(string(res.Stderr), res.ExitCode)
			return fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
		}
		return nil
	})
}

// FastForwardWorktree fast-forwards the branch checked out in the worktree at dir to target
// (`git merge --ff-only`). git refuses when it is not a fast-forward or local changes would be
// overwritten; its message is the error.
func (e *RepoEntry) FastForwardWorktree(ctx context.Context, dir, target string) error {
	defer e.invalidateAfterWrite()
	return e.writeIn(ctx, dir, gitops.MergeFFOnlyArgs(target))
}

// FastForwardRef moves a branch checked out nowhere from oldSha to newSha; it fails when the ref
// no longer holds oldSha.
func (e *RepoEntry) FastForwardRef(ctx context.Context, ref, newSha, oldSha string) error {
	defer e.invalidateAfterWrite()
	return e.writeIn(ctx, repoWorkingDir(e.Summary), gitops.FastForwardRefArgs(ref, newSha, oldSha))
}

// writeIn runs one write spawn in dir through the repo's write gate.
func (e *RepoEntry) writeIn(ctx context.Context, dir string, args []string) error {
	return e.Repo.Write(ctx, func(ctx context.Context) error {
		res, rerr := gitclient.Run(ctx, e.Repo.Runner(), e.Repo.GitPath(), gitclient.Spec{
			Dir: dir, Args: args, ReadOnly: false, Setsid: true,
		})
		if ctx.Err() != nil || rerr != nil {
			return gitclient.Classify(ctx, args, res, rerr)
		}
		if res.ExitCode != 0 {
			_, msg := gitops.ClassifyOpError(string(res.Stderr), res.ExitCode)
			return fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
		}
		return nil
	})
}

// runInDirAllowing runs a read-only spawn in dir, tolerating the given exit codes.
func (e *RepoEntry) runInDirAllowing(ctx context.Context, dir string, args []string, ok ...int) (gitclient.Result, error) {
	var res gitclient.Result
	err := e.Repo.Read(ctx, func(ctx context.Context) error {
		r, rerr := gitclient.Run(ctx, e.Repo.Runner(), e.Repo.GitPath(), gitclient.Spec{Dir: dir, Args: args, ReadOnly: true, OKExits: ok})
		res = r
		if rerr != nil {
			return gitclient.Classify(ctx, args, r, rerr)
		}
		for _, code := range ok {
			if r.ExitCode == code {
				return nil
			}
		}
		return gitclient.Classify(ctx, args, r, nil)
	})
	return res, err
}

// WorktreeHead returns the full ref HEAD of the worktree at path points to ("" when detached) and
// the commit it resolves to.
func (e *RepoEntry) WorktreeHead(ctx context.Context, path string) (ref, tip string, err error) {
	res, err := e.runInDirAllowing(ctx, path, []string{"symbolic-ref", "-q", "HEAD"}, 0, 1)
	if err != nil {
		return "", "", err
	}
	if res.ExitCode == 0 {
		ref = strings.TrimSpace(string(res.Stdout))
	}
	res, err = e.runInDirAllowing(ctx, path, []string{"rev-parse", "--verify", "-q", "HEAD"}, 0, 1)
	if err != nil {
		return "", "", err
	}
	if res.ExitCode == 0 {
		tip = strings.TrimSpace(string(res.Stdout))
	}
	return ref, tip, nil
}

// WorktreeUpstreamTip returns the commit the worktree branch's upstream points to, "" when it has
// none.
func (e *RepoEntry) WorktreeUpstreamTip(ctx context.Context, path string) (string, error) {
	res, err := e.runInDirAllowing(ctx, path, []string{"rev-parse", "--verify", "-q", "@{upstream}"}, 0, 1, 128)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", nil
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}
