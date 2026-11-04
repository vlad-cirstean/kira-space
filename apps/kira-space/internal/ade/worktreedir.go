package ade

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/shortid"
)

const (
	worktreeDirPrefix = "wt_"
	worktreeDirIDLen  = 10
	maxWorktreeDirTry = 5
)

var validWorktreeDir = regexp.MustCompile(`^wt_[0-9a-hjkmnp-tv-z]{10}$`)

// newWorktreeDirName is the one generator of worktree folder names (P523 folds it into the shared id helper).
func newWorktreeDirName() (string, error) {
	id, err := shortid.New(worktreeDirIDLen)
	if err != nil {
		return "", err
	}
	return worktreeDirPrefix + id, nil
}

// worktreeDirFor returns <home>/wt/<repo>/<wt_id> for the row, reserving the folder name on first use.
// owned is the path where the row's branch is already checked out ("" = none): any other existing
// path counts as taken and the name is redrawn.
func (b *TaskBoard) worktreeDirFor(sb model.AdeTaskBranch, repoName, owned string) (string, error) {
	parent := filepath.Join(b.deps.HomeDir, "wt", safeSegment(repoName))
	dir := sb.WorktreeDir
	for range maxWorktreeDirTry {
		if !validWorktreeDir.MatchString(dir) {
			name, err := newWorktreeDirName()
			if err != nil {
				return "", err
			}
			if dir == "" {
				if dir, err = b.deps.Tasks.ReserveWorktreeDir(sb.ID, name); err != nil {
					return "", err
				}
			} else {
				if err := b.deps.Tasks.ReplaceWorktreeDir(sb.ID, dir, name); err != nil {
					return "", err
				}
				dir = name
			}
			continue
		}
		path := filepath.Join(parent, dir)
		if _, err := os.Lstat(path); os.IsNotExist(err) || path == owned {
			return path, nil
		}
		name, err := newWorktreeDirName()
		if err != nil {
			return "", err
		}
		if err := b.deps.Tasks.ReplaceWorktreeDir(sb.ID, dir, name); err != nil {
			return "", err
		}
		dir = name
	}
	return "", fmt.Errorf("ade: no free worktree folder after %d tries", maxWorktreeDirTry)
}
