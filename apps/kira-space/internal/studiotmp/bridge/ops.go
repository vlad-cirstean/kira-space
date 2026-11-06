package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/appcore"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Canceller is the one thing OpsService.Cancel needs (the per-consumer-interface discipline —
// the same shape as connections.Backend and tree.Backend). *adapterhost.Router satisfies this
// structurally: it asks its own in-process scheduler first (op ownership, not connection kind —
// Cancel's payload is only {opId}, so there is no kind to route on) and forwards to the
// engine child only for an op it never started.
type Canceller interface {
	Cancel(ctx context.Context, opID string) (bool, error)
}

// OpsService keeps Canceller as its own field rather than routing it through the shared
// appcore.Deps (the same shape bridge.FilesService's own Dialogs field already uses): appcore
// cannot import bridge (bridge already imports appcore), so a Canceller-typed field belongs here.
type OpsService struct {
	Deps      appcore.Deps
	Canceller Canceller
}

type OpsRecentArgs struct {
	Limit int `json:"limit"`
}

func (s *OpsService) Recent(args OpsRecentArgs) ([]model.OpRecord, error) {
	if args.Limit <= 0 {
		return nil, ipcerr.Contract("limit must be positive")
	}
	return ipcerr.InternalResult(s.Deps.Repos.Ops.Recent(args.Limit))
}

type OpsCancelArgs struct {
	OpID string `json:"opId"`
}

// Cancel is a bare passthrough to the Canceller (src/main/ipc/ops.ts:16-19) — there is nothing in
// internal/oplog to build on, which is why P55 §7 left it here.
func (s *OpsService) Cancel(args OpsCancelArgs) error {
	if args.OpID == "" {
		return ipcerr.Required("opId")
	}
	_, err := s.Canceller.Cancel(context.Background(), args.OpID)
	return err
}
