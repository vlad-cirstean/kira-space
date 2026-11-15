package ade

import (
	"context"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

// awaitSetups waits, until deadline, for the running worktree setup of every branch a call made or
// found. A branch whose setup is still running at the deadline answers waiting.
func (b *TaskBoard) awaitSetups(ctx context.Context, taskID string, branches []agentmcp.RequestedBranch, deadline time.Time) {
	for i := range branches {
		rb := &branches[i]
		if rb.Setup.State != model.AdeSetupRunning || !branchMade(rb) {
			continue
		}
		b.awaitSetup(ctx, taskID, rb, deadline)
	}
}

// awaitSetup refreshes one branch once its setup ended; at the deadline it stays running and the
// outcome becomes waiting. A caller that is gone leaves the branch as it was.
func (b *TaskBoard) awaitSetup(ctx context.Context, taskID string, rb *agentmcp.RequestedBranch, deadline time.Time) {
	br, nick, err := b.taskBranch(taskID, rb.Repo, rb.Branch)
	if err != nil {
		return
	}
	lastNote, reported := "", false
	for time.Now().Before(deadline) {
		// Take the channel before reading the row: a setup that ends between the two still closes
		// the channel taken here, so the wait wakes at once instead of at the fallback tick.
		done := b.setupDone(br.ID)
		row, err := b.deps.Tasks.GetSetup(br.ID)
		if err != nil || row == nil || row.State != model.AdeSetupRunning {
			break
		}
		if !reported || row.Note != lastNote {
			reported, lastNote = true, row.Note
			agentmcp.ReportProgress(ctx, "preparing "+br.Name+": "+row.Note)
		}
		select {
		case <-ctx.Done():
			return
		case <-done:
		case <-time.After(min(5*time.Second, time.Until(deadline))):
		}
	}
	at, _ := b.rowWorktree(ctx, br)
	info := b.branchInfo(br, nick, at.attached)
	if next := switchedNext(br, at); next != "" && info.Setup.State != model.AdeSetupFailed {
		info.Next = next
	}
	rb.BranchInfo = info
	if info.Setup.State == model.AdeSetupRunning {
		rb.Outcome = agentmcp.BranchWaiting
	}
}
