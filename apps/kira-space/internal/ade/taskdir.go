package ade

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// taskDir is the folder a run or session works in while its task has no branch. It is created on first
// use, never at task create.
func (b *TaskBoard) taskDir(taskID string) (string, error) {
	if b.deps.TasksDir == "" {
		return "", fmt.Errorf("the task folder root is not configured")
	}
	dir := filepath.Join(b.deps.TasksDir, safeSegment(taskID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create task folder: %w", err)
	}
	return dir, nil
}

// removeTaskDir drops the task folder; best effort.
func (b *TaskBoard) removeTaskDir(taskID string) {
	if b.deps.TasksDir == "" {
		return
	}
	if err := os.RemoveAll(filepath.Join(b.deps.TasksDir, safeSegment(taskID))); err != nil {
		slog.Warn("ade: remove task folder", "scope", "ade", "task", taskID, "err", err)
	}
}
