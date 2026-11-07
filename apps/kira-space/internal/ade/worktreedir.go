package ade

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
)

const maxWorktreeDirTry = 5

// WorktreesDir is the visible folder under the user home that holds every task worktree.
const WorktreesDir = "kira-space-wts"

// WorktreesBase returns <home>/kira-space-wts.
func WorktreesBase(home string) string { return filepath.Join(home, WorktreesDir) }

// worktreeDirFor returns <home>/kira-space-wts/<repo>/<wt_id> for the row, reserving the folder name on first use.
// owned is the path where the row's branch is already checked out ("" = none): any other existing
// path counts as taken and the name is redrawn.
func (b *TaskBoard) worktreeDirFor(sb model.AdeTaskBranch, repoName, owned string) (string, error) {
	parent := filepath.Join(WorktreesBase(b.deps.HomeDir), pathSegment(repoName))
	dir := sb.WorktreeDir
	for range maxWorktreeDirTry {
		if !ids.Is(ids.Worktree, dir) {
			name := ids.New(ids.Worktree)
			var err error
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
		name := ids.New(ids.Worktree)
		if err := b.deps.Tasks.ReplaceWorktreeDir(sb.ID, dir, name); err != nil {
			return "", err
		}
		dir = name
	}
	return "", fmt.Errorf("ade: no free worktree folder after %d tries", maxWorktreeDirTry)
}
