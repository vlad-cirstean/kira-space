package ipcfixture

import (
	"context"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// fakeTrackedAdapter is just enough of adapters.Adapter to prove App's own cleanup calls
// Disconnect — every other method panics via the embedded nil interface if reached.
type fakeTrackedAdapter struct {
	adapters.Adapter
	disconnected bool
}

func (a *fakeTrackedAdapter) Disconnect(context.Context) error {
	a.disconnected = true
	return nil
}

// All six *_test.go files in this package call Recorder.ConnectionsConnect and
// never the matching disconnect — real adapters accumulate in the process-global registry
// (adapters.live) across test runs, well after the test DB and everything else NewApp's own
// t.Cleanup calls have already torn down. This drives trackConnected/disconnectTracked directly
// (the two halves NewApp's t.Cleanup composes) rather than through a real ConnectionsConnect,
// which every Docker-gated fixture test in this package needs a real container to reach at all.
func TestApp_DisconnectTracked_DisconnectsEveryTrackedConnection(t *testing.T) {
	app := NewApp(t)
	const connID = "conn-f9-fake"
	fake := &fakeTrackedAdapter{}
	adapters.SetLiveAdapter(connID, fake)
	defer adapters.DeleteLiveAdapter(connID)

	app.trackConnected(connID)
	app.disconnectTracked()

	if !fake.disconnected {
		t.Error("disconnectTracked must call Disconnect on every tracked connection")
	}
	if _, ok := adapters.GetLiveAdapter(connID); ok {
		t.Error("disconnectTracked must remove the adapter from the live registry")
	}
}

// A test that never connects anything must not panic or error when its own cleanup runs with an
// empty tracked-connections list — the common case for every non-fixture test that happens to use
// this harness (most of them).
func TestApp_DisconnectTracked_NoOpWhenNothingWasConnected(t *testing.T) {
	app := NewApp(t)
	app.disconnectTracked() // must not panic
}
