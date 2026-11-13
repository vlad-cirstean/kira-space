package ade

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
)

const appName = "Space"

var restartOutcome = model.AdeRunOutcome{Outcome: runoutcome.ForProcess(runoutcome.Process{End: runoutcome.EndRestart, App: appName})}

// Recover settles what the previous process life left behind: running runs become stuck,
// running setups failed, session rows stopped, orphaned MCP configs go. Held (pending) runs keep
// their launch spec and launch when their gate opens. Nothing resumes by itself.
// Call it once at boot, before Start and before any window exists.
func (b *TaskBoard) Recover() error {
	now := b.deps.Now().UnixMilli()
	runs, err := b.deps.Tasks.RecoverRunning(now, restartOutcome)
	if err != nil {
		return fmt.Errorf("ade: recover: %w", err)
	}
	failed, err := b.deps.Tasks.FailRunningSetups(now, restartOutcome.Reason)
	if err != nil {
		return fmt.Errorf("ade: recover: %w", err)
	}
	for _, sb := range failed {
		line := []repos.AdeLogChunk{{At: now, Stream: logEvent, Text: restartOutcome.Reason}}
		if _, err := b.deps.Logs.Append(repos.AdeLogSetup, sb.ID, line); err != nil {
			slog.Warn("ade: recover: setup log", "scope", "ade", "branch", sb.ID, "err", err)
		}
		b.setPendingNote(sb.ID, noteSetupFailed)
	}
	if err := b.deps.Sessions.StopAllTaskRunning(now); err != nil {
		return fmt.Errorf("ade: recover: %w", err)
	}
	if b.deps.AgentDir != "" {
		stale, _ := filepath.Glob(filepath.Join(b.deps.AgentDir, "*.mcp.json"))
		for _, f := range stale {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				slog.Warn("ade: recover: remove mcp config", "scope", "ade", "file", f, "err", err)
			}
		}
	}
	for _, r := range runs {
		if r.Purpose == model.AdeRunPurposeRebase {
			b.recoveredRebases = append(b.recoveredRebases, r)
		}
	}
	b.syncLiveStatuses()
	b.emitRuns(runs...)
	b.notifySessions()
	b.notifyBoard()
	return nil
}

// syncLiveStatuses applies the status of every live task's position once; it corrects the coarse
// backfill of migration 0051.
func (b *TaskBoard) syncLiveStatuses() {
	tasks, err := b.deps.Tasks.ListLive()
	if err != nil {
		slog.Warn("ade: recover: list tasks", "scope", "ade", "err", err)
		return
	}
	cfg, err := b.loadStatusConfig()
	if err != nil {
		slog.Warn("ade: recover: sync statuses", "scope", "ade", "err", err)
		return
	}
	for _, t := range tasks {
		mu := b.taskMu(t.ID)
		mu.Lock()
		b.syncStatusWith(t.ID, &cfg)
		mu.Unlock()
	}
}
