package ade

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// fileswindow.go: task Files windows (P704). One ephemeral window per task shows its worktree files.
// The registry is in memory: a files window pins nothing, and a relaunch never restores one.

// FilesWindowKeyPrefix starts the key of every task Files window.
const FilesWindowKeyPrefix = "adefiles-"

// IsFilesWindowKey reports whether key names a task Files window.
func IsFilesWindowKey(key string) bool { return strings.HasPrefix(key, FilesWindowKeyPrefix) }

// filesWindows maps window keys to tasks and back; the zero value is ready.
type filesWindows struct {
	mu     sync.Mutex
	byKey  map[string]string
	byTask map[string]string
}

func (f *filesWindows) add(key, taskID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.byKey == nil {
		f.byKey, f.byTask = map[string]string{}, map[string]string{}
	}
	f.byKey[key], f.byTask[taskID] = taskID, key
}

func (f *filesWindows) keyOf(taskID string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key, ok := f.byTask[taskID]
	return key, ok
}

func (f *filesWindows) taskOf(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	taskID, ok := f.byKey[key]
	return taskID, ok
}

// remove drops key and returns its task.
func (f *filesWindows) remove(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	taskID, ok := f.byKey[key]
	if ok {
		delete(f.byKey, key)
		delete(f.byTask, taskID)
	}
	return taskID, ok
}

// FilesWindowOpen is OpenFilesWindow's outcome: Existing means the caller only focuses Key;
// otherwise it opens a native window with Key and Order and sets Title.
type FilesWindowOpen struct {
	Key      string
	Title    string
	Order    int
	Existing bool
}

// OpenFilesWindow records the task's Files window, or reports the open one.
func (b *TaskBoard) OpenFilesWindow(_ context.Context, taskID string) (FilesWindowOpen, error) {
	if b.deps.Windows == nil {
		return FilesWindowOpen{}, invalid("files windows are not available")
	}
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return FilesWindowOpen{}, err
	}
	if key, ok := b.filesWin.keyOf(taskID); ok {
		return FilesWindowOpen{Key: key, Existing: true}, nil
	}
	records, err := b.deps.Windows.List()
	if err != nil {
		return FilesWindowOpen{}, err
	}
	order := 0
	for _, r := range records {
		if r.Order >= order {
			order = r.Order + 1
		}
	}
	key := FilesWindowKeyPrefix + ids.NewID()
	if err := b.deps.Windows.Create(model.WindowRecord{Key: key, Order: order}); err != nil {
		return FilesWindowOpen{}, err
	}
	b.filesWin.add(key, taskID)
	title := i18n.T("backend.space.ade.filesWindow.title", map[string]string{"task": taskTitle(tc.task, tc.branches)})
	return FilesWindowOpen{Key: key, Title: title, Order: order}, nil
}

// FilesWindowTarget returns the task a Files window shows, nil when key is not one.
func (b *TaskBoard) FilesWindowTarget(key string) *adewire.TaskFilesWindowTarget {
	taskID, ok := b.filesWin.taskOf(key)
	if !ok {
		return nil
	}
	return &adewire.TaskFilesWindowTarget{TaskID: taskID}
}

// ForgetFilesWindow drops key's registration and window row, and releases the file sessions of its
// task. A native close never runs the window's own unmount, so this is the release. Idempotent.
func (b *TaskBoard) ForgetFilesWindow(key string) {
	taskID, ok := b.filesWin.remove(key)
	if !ok {
		return
	}
	if err := b.deps.Windows.Delete(key); err != nil {
		slog.Warn("ade: drop files window row", "scope", "ade", "key", key, "err", err)
	}
	branches, err := b.deps.Tasks.BranchesOfTask(taskID)
	if err != nil {
		return
	}
	for _, br := range branches {
		b.TaskFilesClose(br.ID)
	}
}

// filesWindowKey is the open Files window of taskID.
func (b *TaskBoard) filesWindowKey(taskID string) (string, bool) { return b.filesWin.keyOf(taskID) }
