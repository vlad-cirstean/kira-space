package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/sessionrestore"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// SessionRestoreService is the binding of the one restore dialog at launch (P701): script runs,
// Docker containers and agents a restart cut, read and restored together.
type SessionRestoreService struct{ Svc *sessionrestore.Service }

// AdeAgentRestoreError maps an agent restore failure to its wire error, as the task service does.
func AdeAgentRestoreError(err error) error { return adeTaskError(err) }

// State reads the three offers. Open says the dialog may still open itself.
func (s *SessionRestoreService) State(_ context.Context) (sessionrestore.State, error) {
	st, err := s.Svc.State()
	if err != nil {
		return sessionrestore.State{}, ipcerr.Wrap(err)
	}
	return st, nil
}

// Claim answers true once per process: the window that wins shows the dialog.
func (s *SessionRestoreService) Claim(_ context.Context) (bool, error) { return s.Svc.Claim(), nil }

// Restore starts the chosen items; each one succeeds or fails alone.
func (s *SessionRestoreService) Restore(ctx context.Context, args sessionrestore.Args) (sessionrestore.Result, error) {
	res, err := s.Svc.Restore(ctx, args)
	if err != nil {
		return sessionrestore.Result{}, ipcerr.Wrap(err)
	}
	return res, nil
}

// Close ends the auto-open and drops the agent offer without restoring anything.
func (s *SessionRestoreService) Close(_ context.Context) error {
	s.Svc.Close()
	return nil
}
