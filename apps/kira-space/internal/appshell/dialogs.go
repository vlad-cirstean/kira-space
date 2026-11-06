package appshell

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// Dialogs adapts repo-root internal/shell.Dialogs' plain-parameter OpenDirectory to this app's
// own bound-service bridge.Dialogs interface (trimmed to the one method this app
// calls) — bridge.OpenDirectoryRequest is a Wails-bound service parameter type, so it stays here,
// destructured into shell.Dialogs' plain-parameter call, rather than moving into the shared
// package.
type Dialogs struct {
	d *shell.Dialogs
}

// NewDialogs wraps a repo-root internal/shell.Dialogs so it satisfies this app's own
// bridge.Dialogs — the drop-in replacement for shell.NewDeferredDialogs' old direct return.
func NewDialogs(d *shell.Dialogs) *Dialogs {
	return &Dialogs{d: d}
}

func (a *Dialogs) SaveFile(req bridge.SaveFileRequest) (string, error) {
	return a.d.SaveFile(req.Directory, req.Filename)
}

func (a *Dialogs) OpenDirectory(req bridge.OpenDirectoryRequest) (string, error) {
	return a.d.OpenDirectory(req.Title)
}

func (a *Dialogs) OpenFile(req bridge.OpenFileRequest) (string, error) {
	return a.d.OpenFile(req.Title, req.FilterName, req.FilterPattern)
}

func (a *Dialogs) OpenFiles(req bridge.OpenFilesRequest) ([]string, error) {
	return a.d.OpenMultipleFiles(req.Title, req.FilterName, req.FilterPattern)
}
