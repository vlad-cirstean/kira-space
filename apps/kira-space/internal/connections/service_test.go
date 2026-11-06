package connections_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/localauth"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/preconnect"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

// fakeBackend replaces the real adapterhost.Router (via a real Node engine fixture) these tests
// used before the Node sidecar was deleted — every kind has been Go-native, so there is no more
// Node-served path left to exercise here, only Backend's own contract.
// Connect blocks on release until it is closed, for TestInFlightConnectDedupe's slow-connect case.
type fakeBackend struct {
	mu          sync.Mutex
	lastConfig  model.ResolvedConnectionConfig
	connectN    atomic.Int64
	testN       atomic.Int64
	disconnectN atomic.Int64
	release     chan struct{}
	unwind      chan struct{}
	// disconnectGate, when non-nil, blocks Disconnect until closed: a real adapter's slow teardown.
	disconnectGate chan struct{}
	throttleCalls  []throttleCall
	connectCtxVal  any
}

type ctxKey struct{}

type throttleCall struct {
	connectionID string
	perSec       float64
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{release: make(chan struct{}), unwind: make(chan struct{})}
}

func (b *fakeBackend) Connect(ctx context.Context, cfg model.ResolvedConnectionConfig) (connections.ConnectResult, error) {
	b.mu.Lock()
	b.lastConfig = cfg
	b.connectCtxVal = ctx.Value(ctxKey{})
	b.mu.Unlock()
	n := b.connectN.Add(1)
	switch cfg.Name {
	case "slow-conn":
		select {
		case <-b.release:
		case <-ctx.Done():
			return connections.ConnectResult{}, ctx.Err()
		}
	case "blind-conn":
		<-b.release // ignores ctx
	case "stubborn-conn":
		// First attempt honours ctx only after unwind is closed: a driver slow to notice a cancel.
		if n == 1 {
			<-ctx.Done()
			<-b.unwind
			return connections.ConnectResult{}, ctx.Err()
		}
	}
	return connections.ConnectResult{ServerVersion: "1.0", Caps: map[string]any{}}, nil
}

// Test never itself rejects a bad Port the way a real adapter would (postgres/client.go's
// uint16(*cfg.Port) truncation being the concrete case) — it always answers OK, so
// TestTestValidatesInputBeforeProbing below can only pass by Service.Test's own Validate() call
// stopping a bad request before it ever reaches here.
func (b *fakeBackend) Test(ctx context.Context, cfg model.ResolvedConnectionConfig) (string, error) {
	b.mu.Lock()
	b.lastConfig = cfg
	b.mu.Unlock()
	b.testN.Add(1)
	return "1.0", nil
}

func (b *fakeBackend) Disconnect(ctx context.Context, connectionID string) error {
	if b.disconnectGate != nil {
		<-b.disconnectGate
	}
	b.disconnectN.Add(1)
	return nil
}

func (b *fakeBackend) RunPreconnect(ctx context.Context, _ *string, _ string, _ time.Duration, run func(context.Context) error) error {
	return run(ctx)
}

func (b *fakeBackend) SetThrottle(connectionID string, perSec float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.throttleCalls = append(b.throttleCalls, throttleCall{connectionID: connectionID, perSec: perSec})
}

func (b *fakeBackend) throttleCallsSnapshot() []throttleCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]throttleCall(nil), b.throttleCalls...)
}

func (b *fakeBackend) releaseSlow() { close(b.release) }

func (b *fakeBackend) connectCount() int { return int(b.connectN.Load()) }

func (b *fakeBackend) disconnectCount() int { return int(b.disconnectN.Load()) }

func (b *fakeBackend) lastConnectConfig() model.ResolvedConnectionConfig {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastConfig
}

// fakeAuthorizer replaces the real internal/localauth.Authorizer: a canned outcome/error per
// call, with the (reason, confirmed) arguments it was called with recorded, so a test can
// both drive Reveal's response and assert the gate saw what it should have. Defaults to Granted —
// harmless for every test that doesn't itself exercise Reveal.
type fakeAuthorizer struct {
	mu            sync.Mutex
	outcome       localauth.Outcome
	err           error
	calls         int
	lastConfirmed bool
}

func newFakeAuthorizer() *fakeAuthorizer { return &fakeAuthorizer{outcome: localauth.Granted} }

func (f *fakeAuthorizer) Authorize(reason string, confirmed bool) (localauth.Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastConfirmed = confirmed
	return f.outcome, f.err
}

// harness wires a real SQLite db (through the real migrations), a real connections folder with its
// secret store, a fakeBackend, a fakeAuthorizer, and a real preconnect supervisor behind one
// connections.Service. A file edit reaches the service the way a loaded change does: through
// ConfigChanged.
type harness struct {
	svc     *connections.Service
	repos   *repos.Repos
	files   *connfiles.Loader
	backend *fakeBackend
	auth    *fakeAuthorizer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("KIRA_SPACE_HOME", t.TempDir())
	t.Setenv("KIRA_INSECURE_SECRETS", "1")

	db, err := storage.Open()
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	r, err := repos.New(db.DB)
	if err != nil {
		t.Fatalf("repos.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	cipher := secrets.New()
	if !cipher.Status().Available {
		t.Fatalf("cipher unavailable: %+v", cipher.Status())
	}
	pre := preconnect.New()
	backend := newFakeBackend()
	auth := newFakeAuthorizer()

	var svc *connections.Service
	files, err := connfiles.New(connfiles.Options{
		DB: db.DB, Dir: filepath.Join(t.TempDir(), "connections"),
		Secrets:  &connfiles.Secrets{Store: secrets.NewStore(db.DB, cipher), DB: db.DB},
		NewID:    uuid.NewString,
		OnChange: func(ch connfiles.Changes) { svc.ConfigChanged(ch) },
		OnForget: func(id string) { svc.Removed(id) },
	})
	if err != nil {
		t.Fatalf("connfiles.New: %v", err)
	}
	t.Cleanup(files.Close)

	svc = connections.New(connections.Deps{
		Conns: r.Connections, Files: files, Metadata: r.Metadata, Auth: auth, Backend: backend, Preconnect: pre,
	})
	svc.Start()
	t.Cleanup(svc.Shutdown)

	return &harness{svc: svc, repos: r, files: files, backend: backend, auth: auth}
}

// edit applies leaf edits to connection id's file, as an editor save would.
func (h *harness) edit(t *testing.T, id string, sets ...configfs.NodeSet) {
	t.Helper()
	if err := h.files.Patch(id, sets...); err != nil {
		t.Fatalf("edit file: %v", err)
	}
}

// set names one dotted key of the file.
func set(key string, value any) configfs.NodeSet {
	return configfs.NodeSet{Path: strings.Split(key, "."), Value: value}
}

// reconnected waits out the asynchronous reconnect a file change starts: one more disconnect and
// connect than before, and the connection connected again.
func (h *harness) reconnected(t *testing.T, id string, connects, disconnects int) {
	t.Helper()
	testx.WaitUntil(t, 5*time.Second, func() bool {
		return h.backend.connectCount() == connects+1 && h.backend.disconnectCount() == disconnects+1 &&
			h.svc.StateOf(id).Status == "connected"
	})
}

// fieldsInput returns a valid, connectable fields-mode Input for name. Kind is "kafka", any real,
// valid connection kind (model.ValidConnectionKind requires one) — fakeBackend does not
// distinguish between kinds.
func fieldsInput(name string) connections.Input {
	return connections.Input{
		ConnectionFields: model.ConnectionFields{
			Name: name, Kind: "kafka", Color: "blue", Mode: "fields",
			Host: strPtr("localhost"), Port: intPtr(5432), Options: map[string]any{},
			McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny",
		},
	}
}

func mustCreate(t *testing.T, svc *connections.Service, in connections.Input) model.ConnectionSummary {
	t.Helper()
	created, err := svc.Create(in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return created
}

// TestInFlightConnectDedupe covers the in-flight attempt map: eight concurrent Connect calls for
// one id must share a single attempt — identical results for every caller, and exactly one
// adapter:connect on the wire.
func TestInFlightConnectDedupe(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("slow-conn"))

	const n = 8
	results := make([]model.ConnectionState, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = h.svc.Connect(created.ID)
		}(i)
	}

	// Give every goroutine time to reach the shared in-flight attempt before releasing it.
	time.Sleep(200 * time.Millisecond)
	h.backend.releaseSlow()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Connect[%d]: %v", i, err)
		}
		if results[i].Status != "connected" {
			t.Fatalf("Connect[%d].Status = %q, want connected", i, results[i].Status)
		}
		if diff := cmp.Diff(results[0], results[i]); diff != "" {
			t.Errorf("Connect[%d] mismatch vs Connect[0] (-want +got):\n%s", i, diff)
		}
	}

	if n := h.backend.connectCount(); n != 1 {
		t.Errorf("Backend.Connect was called %d times, want exactly 1", n)
	}
}

// TestDisconnectWhileConnectingAbortsTheInFlightAttempt: a Disconnect that
// arrives while a Connect for the same id is still in flight must not be silently undone once
// that Connect finishes — without cancelling the attempt, Backend.Connect (fakeBackend, gated on
// the "slow-conn" name) finishes after Disconnect already reported "disconnected", finalizes as
// "connected", and resurrects exactly what the caller just asked to go away.
func TestDisconnectWhileConnectingAbortsTheInFlightAttempt(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("slow-conn"))

	type connectResult struct {
		state model.ConnectionState
		err   error
	}
	connectDone := make(chan connectResult, 1)
	go func() {
		state, err := h.svc.Connect(created.ID)
		connectDone <- connectResult{state, err}
	}()

	// TestInFlightConnectDedupe's own precedent: give Connect time to reach the blocked
	// Backend.Connect call before racing Disconnect against it.
	time.Sleep(200 * time.Millisecond)

	disconnectState, err := h.svc.Disconnect(created.ID)
	if err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if disconnectState.Status != "disconnected" {
		t.Fatalf("Disconnect status = %q, want disconnected", disconnectState.Status)
	}

	h.backend.releaseSlow()
	result := <-connectDone
	if result.err != nil {
		t.Fatalf("Connect: %v", result.err)
	}
	if result.state.Status == "connected" {
		t.Fatalf("Connect's own result = %q after a racing Disconnect, want anything but connected", result.state.Status)
	}

	// The critical assertion: StateOf must not have flipped back to "connected" once the
	// cancelled attempt actually finished running well after Disconnect already returned.
	if got := h.svc.StateOf(created.ID).Status; got == "connected" {
		t.Fatalf("StateOf after the aborted attempt finished = %q, want it to stay disconnected", got)
	}
}

// TestRemoveWhileConnectingAbortsTheInFlightAttemptAndLeavesNoStateEntry is F4's own Remove-side
// case: a Remove that arrives while Connect is still in flight must not let that Connect leave a
// live adapter/states entry behind for an id whose row no longer exists.
func TestRemoveWhileConnectingAbortsTheInFlightAttemptAndLeavesNoStateEntry(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("slow-conn"))

	connectDone := make(chan struct{})
	go func() {
		_, _ = h.svc.Connect(created.ID)
		close(connectDone)
	}()

	time.Sleep(200 * time.Millisecond)

	if err := h.svc.Remove(created.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	h.backend.releaseSlow()
	<-connectDone

	for _, st := range h.svc.States() {
		if st.ConnectionID == created.ID {
			t.Fatalf("States() still carries an entry for the removed id: %+v", st)
		}
	}
}

// TestConnectAfterDisconnectStartsFreshAttempt is P168 Part 2 F4: a Connect arriving while a
// cancelled attempt is still unwinding must not join it and return its stale "disconnected"; and
// Disconnect must not return before that attempt finished.
func TestConnectContextCarriesValuesButNotCancellation(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("ctx-conn"))

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "mcp"))
	cancel()
	state, err := h.svc.ConnectContext(ctx, created.ID)
	if err != nil {
		t.Fatalf("ConnectContext: %v", err)
	}
	if state.Status != "connected" {
		t.Fatalf("status = %q, want connected despite a cancelled caller ctx", state.Status)
	}
	h.backend.mu.Lock()
	defer h.backend.mu.Unlock()
	if h.backend.connectCtxVal != "mcp" {
		t.Fatalf("backend ctx value = %v, want mcp", h.backend.connectCtxVal)
	}
}

func TestConnectAfterDisconnectStartsFreshAttempt(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("stubborn-conn"))

	firstDone := make(chan struct{})
	go func() {
		_, _ = h.svc.Connect(created.ID)
		close(firstDone)
	}()
	time.Sleep(200 * time.Millisecond)

	disconnectDone := make(chan struct{})
	go func() {
		_, _ = h.svc.Disconnect(created.ID)
		close(disconnectDone)
	}()
	time.Sleep(100 * time.Millisecond)

	secondDone := make(chan model.ConnectionState, 1)
	go func() {
		state, _ := h.svc.Connect(created.ID)
		secondDone <- state
	}()
	time.Sleep(100 * time.Millisecond)

	select {
	case <-disconnectDone:
		t.Fatal("Disconnect returned while the cancelled attempt was still unwinding")
	default:
	}

	close(h.backend.unwind)
	<-firstDone
	<-disconnectDone
	if state := <-secondDone; state.Status != "connected" {
		t.Fatalf("second Connect status = %q, want connected (a fresh attempt)", state.Status)
	}
	if got := h.backend.connectCount(); got != 2 {
		t.Fatalf("Backend.Connect calls = %d, want 2", got)
	}
}

// TestConnectWaiterDoesNotRestartDuringTeardown is P170 S4: a Connect already waiting on an
// attempt that Disconnect/Remove aborts must not start a fresh attempt while that teardown is
// still running — it would reconnect what the user just closed, or leak an adapter for a deleted
// id.
func TestConnectWaiterDoesNotRestartDuringTeardown(t *testing.T) {
	teardowns := map[string]func(svc *connections.Service, id string){
		"Disconnect": func(svc *connections.Service, id string) { _, _ = svc.Disconnect(id) },
		"Remove":     func(svc *connections.Service, id string) { _ = svc.Remove(id) },
	}
	for name, teardown := range teardowns {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			created := mustCreate(t, h.svc, fieldsInput("slow-conn"))
			h.backend.disconnectGate = make(chan struct{})

			var wg sync.WaitGroup
			for range 2 { // A runs the attempt, B waits on it
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, _ = h.svc.Connect(created.ID)
				}()
				time.Sleep(100 * time.Millisecond)
			}

			teardownDone := make(chan struct{})
			go func() {
				teardown(h.svc, created.ID)
				close(teardownDone)
			}()
			time.Sleep(200 * time.Millisecond)

			if got := h.backend.connectCount(); got != 1 {
				t.Errorf("Backend.Connect calls during teardown = %d, want 1 (no waiter restart)", got)
			}
			close(h.backend.disconnectGate)
			h.backend.releaseSlow() // keeps a regressed restart from hanging wg.Wait
			<-teardownDone
			wg.Wait()

			if got := h.backend.connectCount(); got != 1 {
				t.Errorf("Backend.Connect calls = %d, want 1", got)
			}
			if got := h.svc.StateOf(created.ID).Status; got == "connected" {
				t.Errorf("status after teardown = %q, want not connected", got)
			}
		})
	}
}

// TestUpdateWhileConnectingReconnectsOnTheNewConfig is P168 Part 2 F3: an attempt that resolved
// its config before a file edit must not finish on the old ReadOnly/destination.
func TestUpdateWhileConnectingReconnectsOnTheNewConfig(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("slow-conn"))

	connectDone := make(chan struct{})
	go func() {
		_, _ = h.svc.Connect(created.ID)
		close(connectDone)
	}()
	time.Sleep(200 * time.Millisecond)

	h.edit(t, created.ID, set("read_only", true))
	time.Sleep(200 * time.Millisecond)
	h.backend.releaseSlow()

	<-connectDone
	testx.WaitUntil(t, 5*time.Second, func() bool {
		return h.backend.connectCount() == 2 && h.svc.StateOf(created.ID).Status == "connected"
	})
	if !h.backend.lastConnectConfig().ReadOnly {
		t.Error("reconnect did not carry ReadOnly=true")
	}
}

// TestConnectRefusesAfterShutdown is F13 (P108 Part 7): once Shutdown has run, every later Connect
// must refuse outright rather than starting a new attempt behind a supervisor that has already been
// told to stop everything.
func TestConnectRefusesAfterShutdown(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("conn"))

	h.svc.Shutdown()

	if _, err := h.svc.Connect(created.ID); err == nil {
		t.Fatal("Connect succeeded after Shutdown, want a refusal")
	}
}

// TestShutdownWaitsOutInFlightConnectBeforeStoppingPreconnect is F13 (P108 Part 7): Preconnect.
// StopAll only kills what is already tracked in its own entries map — an attempt whose
// Preconnect.Start call is still inside the settle window is not tracked yet at all
// (preconnect/supervisor.go's own doc comment on entry). Shutdown must cancel every in-flight
// attempt and wait for it to actually unwind before calling StopAll, not race ahead of it — a
// StopAll that ran first could miss an attempt that only gets tracked moments later, orphaning it.
func TestShutdownWaitsOutInFlightConnectBeforeStoppingPreconnect(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("blind-conn"))

	connectDone := make(chan struct{})
	go func() {
		_, _ = h.svc.Connect(created.ID)
		close(connectDone)
	}()

	// TestInFlightConnectDedupe's own precedent: give Connect time to reach the blocked
	// Backend.Connect call before racing Shutdown against it.
	time.Sleep(200 * time.Millisecond)

	shutdownDone := make(chan struct{})
	go func() {
		h.svc.Shutdown()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
		t.Fatal("Shutdown returned while the in-flight Connect was still blocked on Backend.Connect — it must wait for the attempt to unwind before calling Preconnect.StopAll")
	case <-time.After(200 * time.Millisecond):
	}

	h.backend.releaseSlow()
	<-connectDone
	<-shutdownDone

	if got := h.svc.StateOf(created.ID).Status; got == "connected" {
		t.Fatalf("StateOf after Shutdown = %q, want the cancelled attempt to never finalize as connected", got)
	}
	if _, err := h.svc.Connect(created.ID); err == nil {
		t.Fatal("Connect succeeded after Shutdown, want a refusal")
	}
}

// TestUpdateAppliesThrottleLiveOnlyWhenConnected covers the "applies live" rule directly: a
// file edit reaches Backend.SetThrottle while the connection is connected, but a throttle
// edit to a connection with nothing running yet gets no such call (attemptConnect installs the
// right value on the next connect regardless — no double-install to reconcile).
func TestUpdateAppliesThrottleLiveOnlyWhenConnected(t *testing.T) {
	h := newHarness(t)

	t.Run("connected: a file edit pushes the new rate live", func(t *testing.T) {
		created := mustCreate(t, h.svc, fieldsInput("throttle-connected"))
		if _, err := h.svc.Connect(created.ID); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		// attemptConnect itself calls SetThrottle(id, 0) on the way to Connect — the assertion
		// below only cares about the *last* call, so that first install is fine to ignore.

		h.edit(t, created.ID, set("throttle_per_sec", 7.5))

		testx.WaitUntil(t, 5*time.Second, func() bool {
			calls := h.backend.throttleCallsSnapshot()
			if len(calls) == 0 {
				return false
			}
			last := calls[len(calls)-1]
			return last.connectionID == created.ID && last.perSec == 7.5
		})
	})

	t.Run("disconnected: a file edit installs nothing", func(t *testing.T) {
		created := mustCreate(t, h.svc, fieldsInput("throttle-disconnected"))

		h.edit(t, created.ID, set("throttle_per_sec", 3))

		for _, c := range h.backend.throttleCallsSnapshot() {
			if c.connectionID == created.ID {
				t.Errorf("unexpected SetThrottle(%s, %v) for a never-connected connection", c.connectionID, c.perSec)
			}
		}
	})
}

// TestFileEditReconnectsALiveConnectionOnDestinationOrReadOnlyChange: a connected adapter and both
// cache layers are stamped with the config resolved at Connect time and never revisit it on their
// own. A destination change (host, port, database) or a read_only flip in the file must disconnect and
// reconnect a live connection; a cosmetic edit must not.
func TestFileEditReconnectsALiveConnectionOnDestinationOrReadOnlyChange(t *testing.T) {
	connected := func(t *testing.T, name string) (*harness, model.ConnectionSummary, int, int) {
		t.Helper()
		h := newHarness(t)
		created := mustCreate(t, h.svc, fieldsInput(name))
		if _, err := h.svc.Connect(created.ID); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		return h, created, h.backend.connectCount(), h.backend.disconnectCount()
	}

	t.Run("toggling read_only on a live connection reconnects", func(t *testing.T) {
		h, created, connects, disconnects := connected(t, "ro-toggle")
		h.edit(t, created.ID, set("read_only", true))
		h.reconnected(t, created.ID, connects, disconnects)
		if !h.backend.lastConnectConfig().ReadOnly {
			t.Error("reconnect did not carry the new ReadOnly=true through to Backend.Connect")
		}
	})

	t.Run("changing the destination (host) on a live connection reconnects", func(t *testing.T) {
		h, created, connects, disconnects := connected(t, "dest-change")
		h.edit(t, created.ID, set("host", "prod.example.internal"))
		h.reconnected(t, created.ID, connects, disconnects)
		gotHost := h.backend.lastConnectConfig().Host
		if gotHost == nil || *gotHost != "prod.example.internal" {
			t.Errorf("reconnect config Host = %v, want prod.example.internal", gotHost)
		}
	})

	t.Run("a cosmetic-only edit (color) on a live connection does not reconnect", func(t *testing.T) {
		h, created, connects, disconnects := connected(t, "cosmetic-only")
		h.edit(t, created.ID, set("color", "red"))
		time.Sleep(300 * time.Millisecond) // a regressed reconnect starts on its own goroutine
		if got := h.backend.connectCount(); got != connects {
			t.Errorf("Backend.Connect calls = %d, want %d (no reconnect for a cosmetic-only edit)", got, connects)
		}
		if got := h.backend.disconnectCount(); got != disconnects {
			t.Errorf("Backend.Disconnect calls = %d, want %d (no reconnect for a cosmetic-only edit)", got, disconnects)
		}
	})

	t.Run("changing read_only on a disconnected connection does not attempt a reconnect", func(t *testing.T) {
		h := newHarness(t)
		created := mustCreate(t, h.svc, fieldsInput("ro-disconnected"))
		connects := h.backend.connectCount()
		h.edit(t, created.ID, set("read_only", true))
		time.Sleep(300 * time.Millisecond)
		if got := h.backend.connectCount(); got != connects {
			t.Errorf("Backend.Connect calls = %d, want %d (nothing live to reconnect)", got, connects)
		}
	})
}

// TestConnectKeepsMetadataCacheAndEmitsInvalidation is the regression guard: a successful connect
// must no longer delete the connection's metadata_cache rows (the actual bug — a delete
// guarantees a miss on every path for the rest of the session, which is the "refetched all
// the time" the user reported) while still emitting metadataInvalidated, so the renderer's own
// in-memory copies (the tree's expanded set, schemaColumnsState) drop and re-read through
// internal/tree.Service's own freshness rule rather than serving something genuinely stale. Fails
// on main: attemptConnect's DropConnection call empties the table before this assertion could ever
// see a row.
func TestConnectKeepsMetadataCacheAndEmitsInvalidation(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("keeps-metadata"))

	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})
	if err := h.repos.Metadata.Put(created.ID, path, "children", json.RawMessage(`["x"]`)); err != nil {
		t.Fatalf("seed metadata_cache row: %v", err)
	}

	var invalidated []string
	unsubscribe := h.svc.OnMetadataInvalidated(func(connectionID string) {
		invalidated = append(invalidated, connectionID)
	})
	defer unsubscribe()

	if _, err := h.svc.Connect(created.ID); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	got, _, err := h.repos.Metadata.Get(created.ID, path, "children")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Error("metadata_cache row was deleted by Connect — want it kept (P24 D6)")
	}

	if len(invalidated) != 1 || invalidated[0] != created.ID {
		t.Errorf("metadataInvalidated emissions = %v, want exactly [%s]", invalidated, created.ID)
	}
}
