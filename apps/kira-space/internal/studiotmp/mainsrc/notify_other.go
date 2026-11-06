//go:build !darwin || server

package main

import (
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/appwire"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// notifyServices is a no-op off macOS: the D-Bus and toast backends are not wired, and a service
// that fails to start would stop the app.
func notifyServices(*appwire.Wired) []application.Service {
	slog.Debug("desktop notifications: macOS only", "scope", "notify")
	return nil
}
