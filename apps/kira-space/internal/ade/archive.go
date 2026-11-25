package ade

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// archive.go archives a whole task (SPEC2 section 10, R18): its live work stops, its worktrees go
// and the task moves to history. The caller shows ArchiveRisk first; ArchiveTask is the confirmed
// delete, uncommitted changes included only as far as the tokens it carries (BranchRisk.DirtyToken).

// archiveSnapshotTimeout bounds the host lookups of the archive-time change request snapshot.
const archiveSnapshotTimeout = 5 * time.Second

// linkedBranch is a task branch with a linked worktree (not the repo root).
type linkedBranch struct {
	sb    model.AdeTaskBranch
	path  string
	entry *gitsession.RepoEntry
}

// linkedBranches lists the task's created branches that own a linked worktree.
func (b *TaskBoard) linkedBranches(ctx context.Context, tc *taskCtx) ([]linkedBranch, error) {
	var out []linkedBranch
	cache := b.newAttachCache()
	for _, sb := range tc.branches {
		lb, ok, err := b.linkedBranchIn(ctx, cache, sb)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, lb)
		}
	}
	return out, nil
}

// linkedBranchOf reports the branch's linked worktree; false for a not-created branch or one checked
// out at the repo root. A row's linked worktree is its own folder, attached or detached; it is never a
// folder another live row owns.
func (b *TaskBoard) linkedBranchOf(ctx context.Context, sb model.AdeTaskBranch) (linkedBranch, bool, error) {
	return b.linkedBranchIn(ctx, b.newAttachCache(), sb)
}

// linkedBranchIn is linkedBranchOf over a shared attach cache, for loops over many rows.
func (b *TaskBoard) linkedBranchIn(ctx context.Context, cache *attachCache, sb model.AdeTaskBranch) (linkedBranch, bool, error) {
	if sb.Name == "" {
		return linkedBranch{}, false, nil
	}
	at, err := cache.worktree(ctx, sb)
	if err != nil {
		return linkedBranch{}, false, err
	}
	path := at.attached
	if at.detached {
		path = at.own
	}
	if path == "" {
		return linkedBranch{}, false, nil
	}
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return linkedBranch{}, false, err
	}
	if filepath.Clean(path) == filepath.Clean(entry.Summary.Root) {
		return linkedBranch{}, false, nil
	}
	return linkedBranch{sb: sb, path: path, entry: entry}, true, nil
}

// ArchiveRisk reports what archiving the task would discard.
func (b *TaskBoard) ArchiveRisk(ctx context.Context, taskID string) (adewire.ArchiveRisk, error) {
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return adewire.ArchiveRisk{}, err
	}
	linked, err := b.linkedBranches(ctx, tc)
	if err != nil {
		return adewire.ArchiveRisk{}, err
	}
	out := adewire.ArchiveRisk{TaskID: taskID, Branches: []adewire.BranchRisk{}}
	for _, lb := range linked {
		risk, err := b.branchRisk(ctx, lb)
		if err != nil {
			return adewire.ArchiveRisk{}, err
		}
		out.Branches = append(out.Branches, risk)
	}
	return out, nil
}

func (b *TaskBoard) branchRisk(ctx context.Context, lb linkedBranch) (adewire.BranchRisk, error) {
	status, err := lb.entry.WorktreeStatus(ctx, lb.path)
	if err != nil {
		return adewire.BranchRisk{}, err
	}
	dirty := toDirtyEntries(status)
	tip, mainTip, hasMain, err := b.archiveTips(ctx, lb)
	if err != nil {
		return adewire.BranchRisk{}, err
	}
	// A merge record holds only at its tip: a stale one must not hide unmerged work.
	merged := lb.sb.MergedAt != nil && tip != "" && lb.sb.MergedTip == tip
	unmerged := 0
	if !merged && hasMain && tip != "" {
		if unmerged, _, err = lb.entry.AheadBehind(ctx, tip, mainTip); err != nil {
			return adewire.BranchRisk{}, err
		}
	}
	res := atRisk(atRiskInput{
		Kind: lb.sb.Kind, Merged: merged, HasLinkedWorktree: true,
		DirtyCount: len(dirty), UnmergedAheadOfMain: unmerged,
	})
	pf, err := lb.entry.WorktreeRemovePreflight(ctx, lb.path)
	if err != nil {
		return adewire.BranchRisk{}, err
	}
	risk := adewire.BranchRisk{
		BranchID: lb.sb.ID, Worktree: lb.path, Dirty: []adewire.DirtyEntry{}, Unmerged: res.Unmerged,
		DirtyToken: pf.ConfirmToken,
	}
	for _, d := range dirty {
		risk.Dirty = append(risk.Dirty, adewire.DirtyEntry{Code: d.Code, Path: d.Path})
	}
	if pf.Verdict == "blocked" && len(pf.Blockers) > 0 {
		risk.Blocked = pf.Blockers[0].Kind
	}
	return risk, nil
}

// archiveTips resolves the branch's current tip and the repo's main tip. tip is "" when the branch
// does not resolve.
func (b *TaskBoard) archiveTips(ctx context.Context, lb linkedBranch) (tip, mainTip string, hasMain bool, err error) {
	_, mainTip, hasMain, err = lb.entry.MainRef(ctx)
	if err != nil {
		return "", "", false, err
	}
	inv, err := lb.entry.BranchInventory(ctx)
	if err != nil {
		return "", "", false, err
	}
	remote, _ := lb.entry.DefaultRemote(ctx)
	if row, ok := resolveQueuedRef(inv, lb.sb.Name, remote); ok {
		tip = row.Tip
	}
	return tip, mainTip, hasMain, nil
}

// ArchiveTask stops the task's runs, setups and terminals, removes its worktrees and archives it.
func (b *TaskBoard) ArchiveTask(ctx context.Context, taskID string, discardTokens map[string]string) error {
	// Block new launches first, then wait out any launch already past its check: the task mutex is
	// held for a launch's whole critical section, so one lock/unlock drains them.
	defer b.beginArchive(taskID)()
	barrier := b.taskMu(taskID)
	barrier.Lock()
	barrier.Unlock() //nolint:staticcheck // empty critical section drains in-flight launches
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return err
	}
	linked, err := b.linkedBranches(ctx, tc)
	if err != nil {
		return err
	}
	var blocked []string
	var risks []adewire.BranchRisk
	for _, lb := range linked {
		risk, err := b.branchRisk(ctx, lb)
		if err != nil {
			return err
		}
		if risk.Blocked != "" {
			blocked = append(blocked, fmt.Sprintf("%s (%s)", lb.sb.Name, risk.Blocked))
		}
		risks = append(risks, risk)
	}
	if len(blocked) > 0 {
		return invalid("cannot archive: worktree of %s cannot be removed", strings.Join(blocked, ", "))
	}
	b.abandonBranchRequests(taskID)
	b.abandonTaskRequests(taskID)
	if err := b.stopTaskWork(tc); err != nil {
		return err
	}

	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	if err := b.closeTaskTerminals(taskID); err != nil {
		return err
	}
	// Work is stopped, so the dirty sets no longer move: compare every token once, before any removal,
	// so a refusal never leaves some worktrees gone. A refused archive leaves the task live and stopped.
	for _, lb := range linked {
		if err := b.checkDiscard(ctx, lb, discardTokens[lb.sb.ID]); err != nil {
			return err
		}
	}
	for _, lb := range linked {
		if err := b.removeWorktree(ctx, lb, discardTokens[lb.sb.ID]); err != nil {
			return err
		}
	}
	b.closeTaskWindows(tc)
	b.snapshotChangeRequests(ctx, taskID, tc.branches)
	err = b.deps.Tasks.ArchiveTask(taskID, b.deps.Now().UnixMilli())
	if err == nil {
		b.writeArchivedState(taskID)
	}
	b.touchHistory()
	if err != nil {
		return err
	}
	if err := b.deps.Sessions.ClearUnseenOfTask(taskID); err != nil {
		slog.Warn("ade: archive: clear unseen", "scope", "ade", "task", taskID, "err", err)
	}
	b.closeHandoverPrompt(taskID)
	b.removeTaskDir(taskID)
	b.sweepBlobs()
	b.snapshotReviewComments(ctx, tc.branches)
	b.notifyBoard()
	b.notifySessions()
	return nil
}

// checkDiscard refuses when lb's worktree holds uncommitted changes that token does not confirm.
func (b *TaskBoard) checkDiscard(ctx context.Context, lb linkedBranch, token string) error {
	pf, err := lb.entry.WorktreeRemovePreflight(ctx, lb.path)
	if err != nil {
		return err
	}
	if pf.Verdict != "dirty" {
		return nil
	}
	status, err := lb.entry.WorktreeStatus(ctx, lb.path)
	if err != nil {
		return err
	}
	return b.refuseDiscard(lb.sb.Name, len(toDirtyEntries(status)), pf.ConfirmToken, token)
}

// snapshotChangeRequests stores each row's change request, so the archived view shows it when the host
// is later unreachable or the ref is gone. A row the host no longer answers for loses its old snapshot.
// Best effort: failures log and the caller goes on.
func (b *TaskBoard) snapshotChangeRequests(ctx context.Context, taskID string, rows []model.AdeTaskBranch) {
	ctx, cancel := context.WithTimeout(ctx, archiveSnapshotTimeout)
	defer cancel()
	repoIDs, byRepo := branchesByRepo(rows)
	crs := map[string]model.AdeChangeRequest{}
	for _, id := range repoIDs {
		entry, err := b.openRepo(ctx, id)
		if err != nil {
			slog.Warn("ade: archive snapshot: open repo", "scope", "ade", "repo", id, "err", err)
			continue
		}
		_, _, found, _ := resolveRepoHost(ctx, entry, byRepo[id])
		for branchID, cr := range found {
			crs[branchID] = model.AdeChangeRequest{Number: cr.Number, Title: cr.Title, State: cr.State, URL: cr.URL}
		}
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	if err := b.deps.Tasks.SetBranchChangeRequests(ids, crs); err != nil {
		slog.Warn("ade: archive snapshot: store", "scope", "ade", "task", taskID, "err", err)
	}
}

// stopTaskWork cancels the task's running processes and prepare scripts and waits for them to end.
func (b *TaskBoard) stopTaskWork(tc *taskCtx) error {
	runs, err := b.deps.Tasks.RunsOfTask(tc.task.ID)
	if err != nil {
		return err
	}
	var waits []<-chan struct{}
	for _, r := range runs {
		if r.State == model.AdeRunRunning {
			if done := b.cancelLive(b.live, r.ID, errArchived); done != nil {
				waits = append(waits, done)
			}
		}
	}
	for _, sb := range tc.branches {
		if done := b.cancelLive(b.setupLive, sb.ID, errArchived); done != nil {
			waits = append(waits, done)
		}
	}
	for _, done := range waits {
		if err := b.awaitEnd(done); err != nil {
			return err
		}
	}
	return nil
}

func (b *TaskBoard) closeTaskTerminals(taskID string) error {
	rows, err := b.deps.Sessions.ListRunningTUI()
	if err != nil {
		return err
	}
	for _, s := range rows {
		if s.TaskID != taskID || s.TerminalID == "" {
			continue
		}
		if b.deps.CloseTerminal == nil {
			continue
		}
		if err := b.deps.CloseTerminal(s.TerminalID); err != nil {
			slog.Warn("ade: close task terminal", "scope", "ade", "task", taskID, "terminal", s.TerminalID, "err", err)
		}
	}
	return nil
}

// removeWorktree removes a linked worktree under the repo lock.
func (b *TaskBoard) removeWorktree(ctx context.Context, lb linkedBranch, discardToken string) error {
	b.taskFiles.Close(lb.sb.ID)
	mu := b.repoMutex(lb.sb.CodeRepoID)
	mu.Lock()
	defer mu.Unlock()
	return b.removeWorktreeLocked(ctx, lb, discardToken)
}

// removeWorktreeLocked is removeWorktree for a caller holding the repo lock. Uncommitted changes go only
// with discardToken, the fingerprint of the set the user saw (archive confirm or a discard action). The
// op's own preflight reads the dirty set again and compares, so a worktree that changed since the user looked stays.
func (b *TaskBoard) removeWorktreeLocked(ctx context.Context, lb linkedBranch, discardToken string) error {
	req := gitsession.OpRequest{Kind: "worktreeRemove", Path: lb.path}
	if discardToken != "" {
		req.ConfirmToken = &discardToken
	}
	res, err := lb.entry.RunOp(ctx, b.conn.ID, boardConnLabel, req)
	if err != nil {
		return err
	}
	if !res.OK {
		if res.Error != nil && res.Error.Kind == "ConfirmationRequired" {
			// The op's own preflight found uncommitted changes the token does not confirm.
			return b.refuseDiscard(lb.sb.Name, 0, "", discardToken)
		}
		msg := "worktree remove failed"
		if res.Error != nil {
			msg = res.Error.Message
		}
		return invalid("worktree of %s: %s", lb.sb.Name, msg)
	}
	return nil
}
