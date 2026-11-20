package ade

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/codeworkspace"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/pathsafe"
)

// taskWorktree loads a task branch, its repo entry and worktree path ("" when it has none). The
// path comes from the branch row, never from the caller. gate, when set, runs on the loaded task
// before the repo opens.
func (b *TaskBoard) taskWorktree(ctx context.Context, branchID string, gate func(tc *taskCtx, sb model.AdeTaskBranch) error) (*gitsession.RepoEntry, model.AdeTaskBranch, string, error) {
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return nil, sb, "", err
	}
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return nil, sb, "", err
	}
	if gate != nil {
		if err := gate(tc, sb); err != nil {
			return nil, sb, "", err
		}
	}
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return nil, sb, "", err
	}
	worktree, err := b.worktreeOf(ctx, sb)
	return entry, sb, worktree, err
}

// ErrNoWorktree marks a file call on a branch that has no worktree; the bridge maps it to E_NOT_FOUND.
var ErrNoWorktree = errors.New("ade: the branch has no worktree")

// ErrBadPath marks a path outside the task worktree; the bridge maps it to E_BAD_REQUEST.
var ErrBadPath = errors.New("ade: path is outside the worktree")

// ErrTargetMissing marks a reveal or open target that is gone from the worktree; the bridge maps it
// to E_NOT_FOUND.
var ErrTargetMissing = errors.New("ade: the file no longer exists in the worktree")

// taskSession opens (or reuses) the file session of the branch's worktree.
func (b *TaskBoard) taskSession(ctx context.Context, branchID string) (*codeworkspace.Session, error) {
	entry, sb, worktree, err := b.taskWorktree(ctx, branchID, nil)
	if err != nil {
		return nil, err
	}
	if worktree == "" {
		return nil, ErrNoWorktree
	}
	return b.taskFiles.Open(sb.ID, worktree, entry.Repo.Runner(), entry.Repo.GitPath()), nil
}

// badPath maps a path escape to ErrBadPath; callers map fs.ErrNotExist first, other I/O errors stay internal.
func badPath(err error) error {
	if errors.Is(err, pathsafe.ErrPathEscapesRoot) {
		return fmt.Errorf("%w: %v", ErrBadPath, err)
	}
	return err
}

// TaskFiles lists the tracked and untracked files of the branch's worktree.
func (b *TaskBoard) TaskFiles(ctx context.Context, branchID string) (codeworkspace.FileListing, error) {
	sess, err := b.taskSession(ctx, branchID)
	if err != nil {
		return codeworkspace.FileListing{}, err
	}
	return codeworkspace.ListFiles(ctx, sess)
}

// TaskFileRead reads one worktree file; ifStat lets an unchanged file answer without content.
func (b *TaskBoard) TaskFileRead(ctx context.Context, branchID, rel string, ifStat *codeworkspace.FileStat) (codeworkspace.FileContent, error) {
	sess, err := b.taskSession(ctx, branchID)
	if err != nil {
		return codeworkspace.FileContent{}, err
	}
	abs, err := pathsafe.ValidateRelPath(sess.Root, rel)
	if errors.Is(err, fs.ErrNotExist) {
		// A parent directory is gone: same state as a deleted file.
		return codeworkspace.FileContent{Kind: "missing", LimitBytes: codeworkspace.MaxReadBytes}, nil
	}
	if err != nil {
		return codeworkspace.FileContent{}, badPath(err)
	}
	return codeworkspace.ReadFileIf(abs, rel, ifStat)
}

// TaskFilesSession is the branch's file session, for the bridge's search.
func (b *TaskBoard) TaskFilesSession(ctx context.Context, branchID string) (*codeworkspace.Session, error) {
	return b.taskSession(ctx, branchID)
}

// TaskFilesPeek is the branch's live file session, or nil; it never opens one.
func (b *TaskBoard) TaskFilesPeek(branchID string) *codeworkspace.Session {
	return b.taskFiles.Peek(branchID)
}

// TaskFilesClose drops the branch's file session (and stops its search).
func (b *TaskBoard) TaskFilesClose(branchID string) { b.taskFiles.Close(branchID) }

// TaskFileTarget resolves rel under the branch's worktree for reveal and open. rel "" is the root.
func (b *TaskBoard) TaskFileTarget(ctx context.Context, branchID, rel string) (root, abs string, err error) {
	sess, err := b.taskSession(ctx, branchID)
	if err != nil {
		return "", "", err
	}
	if rel == "" {
		return sess.Root, sess.Root, nil
	}
	abs, err = pathsafe.ValidateRelPath(sess.Root, rel)
	if errors.Is(err, fs.ErrNotExist) {
		return "", "", ErrTargetMissing
	}
	if err != nil {
		return "", "", badPath(err)
	}
	if _, err := os.Stat(abs); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", ErrTargetMissing
		}
		return "", "", err
	}
	return sess.Root, abs, nil
}
