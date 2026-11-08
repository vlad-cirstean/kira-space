package appwire

import (
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appshell"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
)

// MenuActions are the Go-side handlers of the native menu's ItemAction items.
func (w *Wired) MenuActions() map[string]func() {
	return map[string]func(){
		// An empty session id raises the live proof window or opens one with no target.
		appshell.ActionOpenProofs: func() {
			if _, err := w.Proof.OpenWindow(bridge.ProofOpenArgs{}); err != nil {
				slog.Warn("menu: open proof window", "scope", "proof", "err", err)
			}
		},
	}
}
