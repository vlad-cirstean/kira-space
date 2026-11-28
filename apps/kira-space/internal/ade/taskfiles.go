package ade

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/codeworkspace"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/fileservice"
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

// ErrNoMainWorktree marks a file call on a row without a worktree whose repo's main worktree is
// missing; the bridge maps it to E_NOT_FOUND.
var ErrNoMainWorktree = errors.New("ade: the repository's main worktree is missing")

// ErrBadPath marks a path outside the task worktree; the bridge maps it to E_BAD_REQUEST.
var ErrBadPath = errors.New("ade: path is outside the worktree")

// ErrTargetMissing marks a reveal or open target that is gone from the worktree; the bridge maps it
// to E_NOT_FOUND.
var ErrTargetMissing = errors.New("ade: the file no longer exists in the worktree")

// mainWorktree is the repo's main worktree: Summary.Root when the repo was imported at it, else
// git's first worktree record. A bare or missing checkout is ErrNoMainWorktree.
func mainWorktree(ctx context.Context, entry *gitsession.RepoEntry) (string, error) {
	root := ""
	if !entry.Summary.IsBare && !entry.Summary.IsLinkedWorktree {
		root = entry.Summary.Root
	} else {
		list, err := entry.Worktrees(ctx)
		if err != nil {
			return "", err
		}
		for _, w := range list {
			if w.IsMain && !w.IsBare {
				root = w.Path
			}
		}
	}
	if root == "" {
		return "", ErrNoMainWorktree
	}
	info, err := fileservice.Host().Lstat(root)
	if err != nil || !info.Dir {
		return "", ErrNoMainWorktree
	}
	return root, nil
}

// taskSession opens (or reuses) the file session of the branch's worktree; a row without one opens
// its repo's main worktree and main is true.
func (b *TaskBoard) taskSession(ctx context.Context, branchID string) (sess *codeworkspace.Session, main bool, err error) {
	entry, sb, worktree, err := b.taskWorktree(ctx, branchID, nil)
	if err != nil {
		return nil, false, err
	}
	if worktree == "" {
		main = true
		if worktree, err = mainWorktree(ctx, entry); err != nil {
			return nil, false, err
		}
	}
	return b.taskFiles.Open(sb.ID, worktree, entry.Repo.Runner(), entry.Repo.GitPath()), main, nil
}

// badPath maps a path escape to ErrBadPath; callers map fs.ErrNotExist first, other I/O errors stay internal.
func badPath(err error) error {
	if errors.Is(err, fileservice.ErrEscapes) || errors.Is(err, fileservice.ErrInvalidPath) {
		return fmt.Errorf("%w: %v", ErrBadPath, err)
	}
	return err
}

// TaskFileListing is a file listing plus the root it lists; Main is true when that root is the
// repo's main worktree because the row has none.
type TaskFileListing struct {
	codeworkspace.FileListing
	Root string
	Main bool
}

// TaskFiles lists the tracked and untracked files of the branch's worktree.
func (b *TaskBoard) TaskFiles(ctx context.Context, branchID string) (TaskFileListing, error) {
	sess, main, err := b.taskSession(ctx, branchID)
	if err != nil {
		return TaskFileListing{}, err
	}
	l, err := codeworkspace.ListFiles(ctx, sess)
	if err != nil {
		return TaskFileListing{}, err
	}
	return TaskFileListing{FileListing: l, Root: sess.Root, Main: main}, nil
}

// TaskFileRead reads one worktree file; ifStat lets an unchanged file answer without content.
func (b *TaskBoard) TaskFileRead(ctx context.Context, branchID, rel string, ifStat *codeworkspace.FileStat) (codeworkspace.FileContent, error) {
	sess, _, err := b.taskSession(ctx, branchID)
	if err != nil {
		return codeworkspace.FileContent{}, err
	}
	files, err := sess.Files()
	if err != nil {
		return codeworkspace.FileContent{}, err
	}
	if _, err := files.Path(rel); err != nil {
		return codeworkspace.FileContent{}, badPath(err)
	}
	return codeworkspace.ReadFileIf(files, rel, ifStat)
}

// TaskFilesSession is the branch's file session, for the bridge's search.
func (b *TaskBoard) TaskFilesSession(ctx context.Context, branchID string) (*codeworkspace.Session, error) {
	sess, _, err := b.taskSession(ctx, branchID)
	return sess, err
}

// TaskFilesPeek is the branch's live file session, or nil; it never opens one.
func (b *TaskBoard) TaskFilesPeek(branchID string) *codeworkspace.Session {
	return b.taskFiles.Peek(branchID)
}

// TaskFilesClose drops the branch's file session (and stops its search).
func (b *TaskBoard) TaskFilesClose(branchID string) { b.taskFiles.Close(branchID) }

// TaskFileTarget resolves rel under the branch's worktree for reveal and open. rel "" is the root.
func (b *TaskBoard) TaskFileTarget(ctx context.Context, branchID, rel string) (root, abs string, err error) {
	sess, _, err := b.taskSession(ctx, branchID)
	if err != nil {
		return "", "", err
	}
	if rel == "" {
		return sess.Root, sess.Root, nil
	}
	files, err := sess.Files()
	if err != nil {
		return "", "", err
	}
	abs, err = files.Path(rel)
	if err != nil {
		return "", "", badPath(err)
	}
	if _, err := files.Stat(rel); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", ErrTargetMissing
		}
		return "", "", err
	}
	return sess.Root, abs, nil
}
