package mobileweb

import (
	"context"
	"net/http"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

// LaunchResult is what a phone learns from a launch or take over: the ADE session id. The
// terminal id never crosses to the phone; it addresses sessions, not terminals.
type LaunchResult struct {
	SessionID string `json:"sessionId"`
}

// Writer is the write slice of the ADE task service the phone reaches. *bridge.MobileWriter
// forwards each method to the AdeTaskService method desktop calls, so validation, engine rules and
// error mapping are the same. Each method is one row in routes().
type Writer interface {
	CaptureTask(ctx context.Context, args adewire.CaptureTaskArgs) (adewire.Task, error)
	ShelveTask(ctx context.Context, args adewire.ShelveTaskArgs) error
	SetTaskStep(ctx context.Context, args adewire.SetTaskStepArgs) (adewire.Task, error)
	StartRun(ctx context.Context, args adewire.StartRunArgs) (adewire.StartRunResult, error)
	LaunchStep(ctx context.Context, args adewire.LaunchStepArgs) (LaunchResult, error)
	Send(ctx context.Context, args adewire.SendArgs) error
	TakeOver(ctx context.Context, args adewire.TakeOverArgs) (LaunchResult, error)
	// MarkSessionSeen clears the finished-not-viewed marker; attaching a terminal is the phone's view.
	MarkSessionSeen(ctx context.Context, args adewire.MarkSessionSeenArgs) (bool, error)
}

// TerminalBroker owns phone-attached agent terminals (internal/mobileterm). Serve upgrades the
// request to a WebSocket after resolving the ADE session id itself; the other two end holds.
type TerminalBroker interface {
	Serve(w http.ResponseWriter, r *http.Request, dev repos.MobileDeviceRow, sessionID string, authorized func() bool)
	ReleaseDevice(deviceID string)
	ReleaseAll(reason string)
}
