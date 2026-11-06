package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// StudioCanceller is the one thing StudioOpsService.Cancel needs (the per-consumer-interface discipline —
// the same shape as connections.Backend and tree.Backend). *adapterhost.Router satisfies this
// structurally: it asks its own in-process scheduler first (op ownership, not connection kind —
// Cancel's payload is only {opId}, so there is no kind to route on) and forwards to the
// engine child only for an op it never started.
type StudioCanceller interface {
	Cancel(ctx context.Context, opID string) (bool, error)
}

// StudioOpsService keeps StudioCanceller as its own field rather than routing it through the shared
// appcore.StudioDeps (the same shape bridge.FilesService's own Dialogs field already uses): appcore
// cannot import bridge (bridge already imports appcore), so a StudioCanceller-typed field belongs here.
type StudioOpsService struct {
	Deps      appcore.StudioDeps
	Canceller StudioCanceller
}

type StudioOpsRecentArgs struct {
	Limit int `json:"limit"`
}

func (s *StudioOpsService) Recent(args StudioOpsRecentArgs) ([]model.OpRecord, error) {
	if args.Limit <= 0 {
		return nil, ipcerr.Contract("limit must be positive")
	}
	return ipcerr.InternalResult(s.Deps.Repos.Ops.Recent(args.Limit))
}

type StudioOpsCancelArgs struct {
	OpID string `json:"opId"`
}

// Cancel is a bare passthrough to the StudioCanceller (src/main/ipc/ops.ts:16-19) — there is nothing in
// internal/oplog to build on, which is why P55 §7 left it here.
func (s *StudioOpsService) Cancel(args StudioOpsCancelArgs) error {
	if args.OpID == "" {
		return ipcerr.Required("opId")
	}
	_, err := s.Canceller.Cancel(context.Background(), args.OpID)
	return err
}
