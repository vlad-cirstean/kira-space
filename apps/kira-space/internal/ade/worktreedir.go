package ade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
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

// ownWorktreePath is the folder reserved for the row, "" while it has none.
func (b *TaskBoard) ownWorktreePath(repoName string, sb model.AdeTaskBranch) string {
	if !ids.Is(ids.Worktree, sb.WorktreeDir) {
		return ""
	}
	return filepath.Join(WorktreesBase(b.deps.HomeDir), pathSegment(repoName), sb.WorktreeDir)
}

// worktreeOwners maps each own worktree path of the live rows of one repo to its row.
func (b *TaskBoard) worktreeOwners(repoName string, live []model.AdeTaskBranch, codeRepoID string) map[string]model.AdeTaskBranch {
	out := map[string]model.AdeTaskBranch{}
	for _, row := range live {
		if row.CodeRepoID != codeRepoID {
			continue
		}
		if p := b.ownWorktreePath(repoName, row); p != "" {
			out[filepath.Clean(p)] = row
		}
	}
	return out
}

// attachment is where a row's branch is checked out, seen from the row.
type attachment struct {
	// attached is the path the row works in now: its branch is checked out there and no other live
	// row owns that folder. "" = detached or not checked out.
	attached string
	// own is the row's reserved folder ("" = none yet).
	own string
	// holder is the path where git has the branch checked out, wherever that is.
	holder string
	// detached is true when the row's own folder exists but its branch is held elsewhere or nowhere.
	detached bool
	// owner is the other live row whose own folder holds the branch; nil when none.
	owner *model.AdeTaskBranch
}

// resolveAttachment derives the attach state of sb from git's holder path, the live rows' own
// folders and whether the row's own folder exists on disk. The row identity is its own folder, never
// the branch name: two rows can share a name.
func resolveAttachment(sb model.AdeTaskBranch, own, holder string, owners map[string]model.AdeTaskBranch, ownExists bool) attachment {
	a := attachment{own: own, holder: holder}
	cleanHolder := ""
	if holder != "" {
		cleanHolder = filepath.Clean(holder)
		if row, ok := owners[cleanHolder]; ok && row.ID != sb.ID {
			r := row
			a.owner = &r
		}
	}
	switch {
	case holder == "":
	case own != "" && cleanHolder == filepath.Clean(own):
		a.attached = holder
	case a.owner == nil:
		a.attached = holder
	}
	a.detached = a.attached == "" && own != "" && ownExists
	return a
}

// attachFacts is what one repo contributes to every row's attach state: git's inventory, the
// registered repo and the folders live rows own. Loaded once per repo per call.
type attachFacts struct {
	inv    []porcelain.InventoryRef
	rec    *model.CodeRepo
	owners map[string]model.AdeTaskBranch
}

// attachCache loads attachFacts once per repo. It is a snapshot for one call: build a new one after
// anything that can move a worktree.
type attachCache struct {
	b     *TaskBoard
	facts map[string]*attachFacts
}

func (b *TaskBoard) newAttachCache() *attachCache {
	return &attachCache{b: b, facts: map[string]*attachFacts{}}
}

func (c *attachCache) load(ctx context.Context, codeRepoID string) (*attachFacts, error) {
	if f, ok := c.facts[codeRepoID]; ok {
		return f, nil
	}
	entry, err := c.b.openRepo(ctx, codeRepoID)
	if err != nil {
		return nil, err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return nil, err
	}
	rec, err := c.b.deps.CodeRepos.Get(codeRepoID)
	if err != nil {
		return nil, err
	}
	f := &attachFacts{inv: inv, rec: rec}
	if rec != nil {
		live, err := c.b.deps.Tasks.BranchesLive()
		if err != nil {
			return nil, err
		}
		f.owners = c.b.worktreeOwners(rec.Name, live, codeRepoID)
	}
	c.facts[codeRepoID] = f
	return f, nil
}

// worktree reads the row's attach state from the cached repo facts.
func (c *attachCache) worktree(ctx context.Context, sb model.AdeTaskBranch) (attachment, error) {
	if sb.Name == "" {
		return attachment{}, nil
	}
	f, err := c.load(ctx, sb.CodeRepoID)
	if err != nil {
		return attachment{}, err
	}
	holder := ""
	if row, ok := localBranch(f.inv, sb.Name); ok {
		holder = row.WorktreePath
	}
	if f.rec == nil {
		return resolveAttachment(sb, "", holder, nil, false), nil
	}
	own := c.b.ownWorktreePath(f.rec.Name, sb)
	ownExists := false
	if own != "" {
		_, statErr := os.Lstat(own)
		ownExists = statErr == nil
	}
	return resolveAttachment(sb, own, holder, f.owners, ownExists), nil
}

// rowWorktree reads the row's attach state: its repo's inventory, registered name and live rows.
func (b *TaskBoard) rowWorktree(ctx context.Context, sb model.AdeTaskBranch) (attachment, error) {
	return b.newAttachCache().worktree(ctx, sb)
}
