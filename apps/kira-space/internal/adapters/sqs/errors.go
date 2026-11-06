package sqs

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/awscfg"
)

// mapError wraps awscfg.MapError (P58d D2/D4) — exists only so a future divergence has a place to
// live, mirroring every other adapter's own one-line errors.go.
func mapError(err error) *adapters.Error {
	return awscfg.MapError(err)
}
