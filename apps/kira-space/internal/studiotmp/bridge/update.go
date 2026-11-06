package bridge

import (
	"context"
	"errors"
	"log/slog"

	"github.com/kirathecat/kira-studio/internal/appupdate"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// UpdateStatus is the whole wire shape. It deliberately carries no URL and no error string: the
// renderer never supplies or receives a URL (main.go's own security note), and a failed poll is
// silence, not a surface (Check reports one). InstallLogPath is a display path only, read by the
// in-app dialog on a failed install — never sent back as, or accepted as, a URL.
type UpdateStatus struct {
	UpdateAvailable bool   `json:"updateAvailable"`
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	InstallLogPath  string `json:"installLogPath"`
}

// UpdateService is P66/P119's whole surface: Status (polled by the renderer), Check (the menu's
// forced check), InstallUpdate (the dialog's Update button) and CancelInstall (its Cancel button
// while installing).
type UpdateService struct {
	Checker   *appupdate.Checker
	Installer *appupdate.Installer
	// Quit is shell.Quitter.RequestQuit, assigned once that exists (main.go's own ordering knot).
	Quit func()
}

func toWireUpdateStatus(r appupdate.Result, logPath string) UpdateStatus {
	return UpdateStatus{
		UpdateAvailable: r.UpdateAvailable,
		CurrentVersion:  r.CurrentVersion,
		LatestVersion:   r.LatestVersion,
		InstallLogPath:  logPath,
	}
}

// Status blocks on the network only when a check is actually due (appupdate's own cache interval)
// — never on the boot critical path, since the renderer calls this only after the app has mounted.
func (s *UpdateService) Status(ctx context.Context) (UpdateStatus, error) {
	return toWireUpdateStatus(s.Checker.Status(ctx), s.Installer.DisplayLogPath()), nil
}

// Check fetches now, bypassing the cache, and reports a failed check, which Status keeps silent.
// A dev build has nothing to compare against, so it answers the not-a-release-build error.
func (s *UpdateService) Check(ctx context.Context) (UpdateStatus, error) {
	res, err := s.Checker.Check(ctx)
	switch {
	case errors.Is(err, appupdate.ErrNotReleaseBuild):
		return UpdateStatus{}, ipcerr.T(ipcerr.CodeConflict, "backend.core.appUpdate.notRelease.body", nil)
	case err != nil:
		slog.Warn("appupdate: manual check failed", "scope", "appupdate", "err", err)
		return UpdateStatus{}, ipcerr.T(ipcerr.CodeUnavailable, "backend.core.appUpdate.checkFailed.body", map[string]string{"cause": appupdate.FailureCause(err)})
	}
	return toWireUpdateStatus(res, s.Installer.DisplayLogPath()), nil
}

// InstallUpdate re-checks availability (cached; Check is the forced fetch), stages the update through the
// detached installer, then quits so the installer can swap the bundle into place. A dev build's
// Status is never UpdateAvailable, so a dev build can never reach Stage.
func (s *UpdateService) InstallUpdate(ctx context.Context) error {
	if !s.Checker.Status(ctx).UpdateAvailable {
		return ipcerr.T(ipcerr.CodeConflict, "backend.core.appUpdate.noNewerRelease.body", nil)
	}
	if err := s.Installer.Stage(ctx); err != nil {
		return err
	}
	go s.Quit() // after this call's own reply has gone out, not racing it
	return nil
}

// CancelInstall aborts an install in progress. A no-op once the installer has already handed off
// (the script owns the bundle swap from that point on).
func (s *UpdateService) CancelInstall() error {
	s.Installer.Cancel()
	return nil
}
