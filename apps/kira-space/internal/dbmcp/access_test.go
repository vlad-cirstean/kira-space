package dbmcp

import (
	"context"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// fakeConnsReader is a minimal ConnectionsReader stub — just enough to drive connectForQuery
// and record whether Connect was actually called.
type fakeConnsReader struct {
	state        model.ConnectionState
	connectState model.ConnectionState
	connectErr   error
	connectCalls int
}

func (f *fakeConnsReader) List() ([]model.ConnectionSummary, error) { return nil, nil }

func (f *fakeConnsReader) StateOf(string) model.ConnectionState { return f.state }

func (f *fakeConnsReader) ConnectContext(context.Context, string) (model.ConnectionState, error) {
	f.connectCalls++
	return f.connectState, f.connectErr
}

// TestConnectForQuerySkipsReconnectWhenAlreadyConnected guards the bug: Connect
// tears a live connection down and reconnects it unconditionally, so run_query calling it on
// every already-open connection churns the adapter, drops cached metadata and re-runs the
// pre-connect script on every call. connectForQuery must check StateOf first and only dial
// Connect when the connection isn't already up.
func TestConnectForQuerySkipsReconnectWhenAlreadyConnected(t *testing.T) {
	conns := &fakeConnsReader{state: model.ConnectionState{ConnectionID: "c1", Status: "connected"}}
	s := &Server{cfg: Config{Conns: conns}}

	state, err := s.connectForQuery(context.Background(), "c1")
	if err != nil {
		t.Fatalf("connectForQuery returned err = %v, want nil", err)
	}
	if state.Status != "connected" {
		t.Fatalf("state.Status = %q, want connected", state.Status)
	}
	if conns.connectCalls != 0 {
		t.Fatalf("Connect called %d times, want 0 for an already-connected connection", conns.connectCalls)
	}
}

// TestConnectForQueryConnectsWhenNotConnected is the complement: a not-yet-connected connection
// still dials Connect exactly once, on demand (§6.3).
func TestConnectForQueryConnectsWhenNotConnected(t *testing.T) {
	want := model.ConnectionState{ConnectionID: "c1", Status: "connected"}
	conns := &fakeConnsReader{
		state:        model.ConnectionState{ConnectionID: "c1", Status: "disconnected"},
		connectState: want,
	}
	s := &Server{cfg: Config{Conns: conns}}

	state, err := s.connectForQuery(context.Background(), "c1")
	if err != nil {
		t.Fatalf("connectForQuery returned err = %v, want nil", err)
	}
	if state.Status != "connected" {
		t.Fatalf("state.Status = %q, want connected", state.Status)
	}
	if conns.connectCalls != 1 {
		t.Fatalf("Connect called %d times, want 1 for a not-yet-connected connection", conns.connectCalls)
	}
}

// TestConnectStateErrorSurfacesConnectionsOwnError guards the second review finding: doConnect
// encodes a failed connect as a state with a nil Go error, so the caller must read the state's
// own Error text rather than a generic dispatcher error.
func TestConnectStateErrorSurfacesConnectionsOwnError(t *testing.T) {
	msg := "connection refused"
	got := connectStateError(model.ConnectionState{Status: "error", Error: &msg})
	if got == "" {
		t.Fatal("connectStateError returned empty string")
	}
	if !strings.Contains(got, msg) {
		t.Fatalf("connectStateError = %q, want it to contain %q", got, msg)
	}
}
