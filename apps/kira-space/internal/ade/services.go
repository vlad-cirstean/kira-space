package ade

import (
	"context"
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
)

// RequiredServices reads the repo services the task needs and whether a Kira run of each is live. A
// service that runs in its own folder is shared by every task, so any running run of it counts. A
// service that runs in the ADE worktree is per branch: one entry per mine branch of the task in that
// repo. It only reads: no op row, no task lock, no script folder is prepared.
func (b *TaskBoard) RequiredServices(ctx context.Context, taskID string) (adewire.RequiredServices, error) {
	out := adewire.RequiredServices{Items: []adewire.RequiredService{}}
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return out, err
	}
	if b.deps.RepoServices == nil || b.deps.CodeRepos == nil {
		return out, nil
	}
	var repoIDs []string
	for _, br := range tc.branches {
		if !slices.Contains(repoIDs, br.CodeRepoID) {
			repoIDs = append(repoIDs, br.CodeRepoID)
		}
	}
	type repoRefs struct {
		id   string
		refs []model.ScriptRef
	}
	var todo []repoRefs
	for _, id := range repoIDs {
		rec, err := b.deps.CodeRepos.Get(id)
		if err != nil {
			return out, err
		}
		if rec == nil {
			continue
		}
		if refs := b.deps.RepoServices(rec.RepoID); len(refs) > 0 {
			todo = append(todo, repoRefs{id, refs})
		}
	}
	if len(todo) == 0 {
		return out, nil
	}
	all, err := b.listScripts()
	if err != nil {
		return out, err
	}
	cache := b.newAttachCache()
	seen := map[string]bool{}
	for _, r := range todo {
		for _, ref := range r.refs {
			item := adewire.RequiredService{CodeRepoID: r.id, Script: ref.Script, Params: ref.Values()}
			script, err := findRefScript(all, repoFileUse, ref.Script)
			if err != nil {
				item.State, item.Problem = adewire.ServiceInvalid, err.Error()
				out.Items = append(out.Items, item)
				continue
			}
			item.ScriptID = script.ID
			if !script.UseAdeDir {
				if seen[script.ID] {
					continue
				}
				seen[script.ID] = true
				run, err := b.runningScript(script.ID)
				if err != nil {
					return out, err
				}
				item.State, item.RunID = serviceState(run)
				out.Items = append(out.Items, item)
				continue
			}
			for _, sb := range tc.branches {
				if sb.CodeRepoID != r.id || sb.Kind != model.AdeBranchKindMine || seen[script.ID+"/"+sb.ID] {
					continue
				}
				seen[script.ID+"/"+sb.ID] = true
				per := item
				per.BranchID = sb.ID
				at, err := cache.worktree(ctx, sb)
				if err != nil {
					return out, err
				}
				if at.attached == "" {
					per.State = adewire.ServiceNoWorktree
				} else {
					var run *scriptruns.Run
					if b.deps.ScriptRuns != nil {
						if run, err = b.deps.ScriptRuns.Running(script.ID, taskID, sb.ID); err != nil {
							return out, err
						}
					}
					per.State, per.RunID = serviceState(run)
				}
				out.Items = append(out.Items, per)
			}
		}
	}
	return out, nil
}

func (b *TaskBoard) runningScript(scriptID string) (*scriptruns.Run, error) {
	if b.deps.ScriptRuns == nil {
		return nil, nil
	}
	return b.deps.ScriptRuns.RunningScript(scriptID)
}

func serviceState(run *scriptruns.Run) (state, runID string) {
	if run == nil {
		return adewire.ServiceStopped, ""
	}
	return adewire.ServiceRunning, run.ID
}
