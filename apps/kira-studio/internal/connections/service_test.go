package connections_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/localauth"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/preconnect"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/kiratime"
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

func (b *fakeBackend) testCount() int { return int(b.testN.Load()) }

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

// harness wires a real SQLite db (through the real migrations), a real available cipher (the
// Linux KIRA_INSECURE_SECRETS fallback), a fakeBackend, a fakeAuthorizer, and a real preconnect
// supervisor behind one connections.Service.
type harness struct {
	svc     *connections.Service
	repos   *repos.Repos
	secrets *repos.SecretsRepo
	backend *fakeBackend
	auth    *fakeAuthorizer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("KIRA_HOME", t.TempDir())
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
	secretsRepo := repos.NewSecrets(db.DB, cipher)
	pre := preconnect.New()
	backend := newFakeBackend()
	auth := newFakeAuthorizer()

	svc := connections.New(connections.Deps{
		Conns: r.Connections, Secrets: secretsRepo, Metadata: r.Metadata,
		Cipher: cipher, Auth: auth, Backend: backend, Preconnect: pre, MaskRules: r.MaskRules,
	})
	svc.Start()
	t.Cleanup(svc.Shutdown)

	return &harness{svc: svc, repos: r, secrets: secretsRepo, backend: backend, auth: auth}
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

// newUnavailableCipherHarness is newHarness without KIRA_INSECURE_SECRETS set — on this Linux
// sandbox that leaves the real probe naturally unavailable, exercising the actual refusal path
// rather than a fake one.
func newUnavailableCipherHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("KIRA_HOME", t.TempDir())

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
	if cipher.Status().Available {
		t.Fatalf("cipher unexpectedly available: %+v", cipher.Status())
	}
	secretsRepo := repos.NewSecrets(db.DB, cipher)
	pre := preconnect.New()
	backend := newFakeBackend()
	auth := newFakeAuthorizer()

	svc := connections.New(connections.Deps{
		Conns: r.Connections, Secrets: secretsRepo, Metadata: r.Metadata,
		Cipher: cipher, Auth: auth, Backend: backend, Preconnect: pre,
	})
	svc.Start()
	t.Cleanup(svc.Shutdown)

	return &harness{svc: svc, repos: r, secrets: secretsRepo, backend: backend, auth: auth}
}

// serviceWithUnavailableCipher builds a second Service over h's own db/repos, but with a cipher
// forced unavailable (by momentarily hiding KIRA_INSECURE_SECRETS for the one secrets.New() call
// that reads it — safe here since nothing else touches that env var concurrently within a test).
// It never touches Host/Preconnect, so nil deps for those are fine: this is only ever used to
// exercise Update, which does not call either.
func (h *harness) serviceWithUnavailableCipher(t *testing.T) *connections.Service {
	t.Helper()
	old, existed := os.LookupEnv("KIRA_INSECURE_SECRETS")
	_ = os.Unsetenv("KIRA_INSECURE_SECRETS")
	cipher := secrets.New()
	if existed {
		_ = os.Setenv("KIRA_INSECURE_SECRETS", old)
	}
	if cipher.Status().Available {
		t.Fatalf("cipher unexpectedly available: %+v", cipher.Status())
	}
	secretsRepo := repos.NewSecrets(h.repos.Connections.DB, cipher)
	return connections.New(connections.Deps{
		Conns: h.repos.Connections, Secrets: secretsRepo, Metadata: h.repos.Metadata, Cipher: cipher,
	})
}

func mustCreate(t *testing.T, svc *connections.Service, in connections.Input) model.ConnectionSummary {
	t.Helper()
	created, err := svc.Create(in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return created
}

// asIpcErr is testx.AsIpcErr (P107 I2-28).
var asIpcErr = testx.AsIpcErr

// TestPasswordThreeStateConvention pins Update's three-state password contract — nil leaves the
// stored secret alone, "" clears it, non-empty replaces it — which a naive `if password != nil`
// collapses into two states and silently wipes credentials on every unrelated edit.
func TestPasswordThreeStateConvention(t *testing.T) {
	h := newHarness(t)
	in := fieldsInput("pw-conn")
	in.Password = strPtr("secret1")
	created := mustCreate(t, h.svc, in)

	get := func() *string {
		v, err := h.secrets.Get(created.ID)
		if err != nil {
			t.Fatalf("Secrets.Get: %v", err)
		}
		return v
	}
	if v := get(); v == nil || *v != "secret1" {
		t.Fatalf("initial secret = %v, want secret1", v)
	}

	// nil = unchanged.
	unchanged := fieldsInput("pw-conn")
	if _, err := h.svc.Update(created.ID, unchanged); err != nil {
		t.Fatalf("Update(nil password): %v", err)
	}
	if v := get(); v == nil || *v != "secret1" {
		t.Errorf("after nil-password update, secret = %v, want unchanged secret1", v)
	}

	// "" = clear.
	cleared := fieldsInput("pw-conn")
	cleared.Password = strPtr("")
	if _, err := h.svc.Update(created.ID, cleared); err != nil {
		t.Fatalf("Update(clear password): %v", err)
	}
	if v := get(); v != nil {
		t.Errorf("after clearing, secret = %v, want nil", *v)
	}

	// non-empty = replace.
	replaced := fieldsInput("pw-conn")
	replaced.Password = strPtr("secret2")
	if _, err := h.svc.Update(created.ID, replaced); err != nil {
		t.Fatalf("Update(replace password): %v", err)
	}
	if v := get(); v == nil || *v != "secret2" {
		t.Errorf("after replacing, secret = %v, want secret2", v)
	}
}

// TestCreateValidatesSecretBeforeWriting covers half of the deliberately asymmetric write
// ordering: Create must prove the secret can be encrypted BEFORE inserting, so an unavailable
// cipher leaves no half-written row behind.
func TestCreateValidatesSecretBeforeWriting(t *testing.T) {
	h := newUnavailableCipherHarness(t)

	before, err := h.svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	withPassword := fieldsInput("should-not-exist")
	withPassword.Password = strPtr("x")
	_, err = h.svc.Create(withPassword)
	if err == nil {
		t.Fatalf("Create with an unavailable cipher: want an error")
	}
	if ie := asIpcErr(t, err); ie.Code != "E_SECRET_STORE" {
		t.Errorf("Code = %q, want E_SECRET_STORE", ie.Code)
	}
	after, err := h.svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("List() grew from %d to %d — a row was left behind despite the failed Create", len(before), len(after))
	}

	withoutPassword := fieldsInput("no-password-needed")
	if _, err := h.svc.Create(withoutPassword); err != nil {
		t.Errorf("Create with a nil password on an unavailable cipher: %v, want success", err)
	}
}

// TestUpdateWritesSecretBeforeRow is the other half: the row already exists, so Update writes the
// secret first and a cipher failure must abort before any other field is touched — a half-applied
// edit is the failure mode this ordering exists to prevent.
func TestUpdateWritesSecretBeforeRow(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("original-name"))

	unavailable := h.serviceWithUnavailableCipher(t)
	withPassword := fieldsInput("changed-name")
	withPassword.Password = strPtr("x")
	_, err := unavailable.Update(created.ID, withPassword)
	if err == nil {
		t.Fatalf("Update with an unavailable cipher: want an error")
	}
	if ie := asIpcErr(t, err); ie.Code != "E_SECRET_STORE" {
		t.Errorf("Code = %q, want E_SECRET_STORE", ie.Code)
	}

	row, err := h.repos.Connections.Get(created.ID)
	if err != nil {
		t.Fatalf("Connections.Get: %v", err)
	}
	if row == nil || row.Name != "original-name" {
		t.Errorf("row = %+v, want Name unchanged at original-name", row)
	}
}

// TestUpdateDoesNotLeaveANewPasswordStoredAgainstOldFields:
// connections.Service.Update used to write the new password (Secrets.Set) *before* writing the
// rest of the row (Conns.Update) — reasoning only about the "cipher unavailable" failure, which
// that ordering does correctly abort before touching anything. But Conns.Update can fail for a
// reason that has nothing to do with the cipher: its json.Marshal(f.Options) can itself refuse a
// value (NaN, which Input.Validate never inspects Options for — Go callers, and a future
// programmatic caller, can construct one even though the IPC/JSON boundary cannot). Under the old
// ordering, that failure happened *after* the new password had already been committed, leaving it
// stored against the connection's old host/port/database — exactly the "old destination's password
// on a new destination" state destinationUnchanged elsewhere in this file exists to prevent,
// reached by a different path. UpdateWithSecret's single combined statement computes and validates
// everything (including the json.Marshal) before issuing any write, so this failure now aborts with
// nothing changed at all.
func TestUpdateDoesNotLeaveANewPasswordStoredAgainstOldFields(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("original-name"))
	if err := h.secrets.Set(created.ID, strPtr("original-secret")); err != nil {
		t.Fatalf("seed original secret: %v", err)
	}

	in := fieldsInput("changed-name")
	in.Password = strPtr("new-secret")
	in.Options = map[string]any{"bad": math.NaN()}
	if _, err := h.svc.Update(created.ID, in); err == nil {
		t.Fatalf("Update with an unmarshalable Options value: want an error")
	}

	row, err := h.repos.Connections.Get(created.ID)
	if err != nil {
		t.Fatalf("Connections.Get: %v", err)
	}
	if row == nil || row.Name != "original-name" {
		t.Errorf("row = %+v, want Name unchanged at \"original-name\"", row)
	}

	stored, err := h.secrets.Get(created.ID)
	if err != nil {
		t.Fatalf("Secrets.Get: %v", err)
	}
	if stored == nil || *stored != "original-secret" {
		t.Errorf("stored secret = %v, want unchanged \"original-secret\" — a failed update must not leave the new password stored against the old row", stored)
	}
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
// its config before an edit must not finish on the old ReadOnly/destination.
func TestUpdateWhileConnectingReconnectsOnTheNewConfig(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("slow-conn"))

	connectDone := make(chan struct{})
	go func() {
		_, _ = h.svc.Connect(created.ID)
		close(connectDone)
	}()
	time.Sleep(200 * time.Millisecond)

	draft := fieldsInput("slow-conn")
	draft.ReadOnly = true
	updateDone := make(chan error, 1)
	go func() {
		_, err := h.svc.Update(created.ID, draft)
		updateDone <- err
	}()
	time.Sleep(200 * time.Millisecond)
	h.backend.releaseSlow()

	if err := <-updateDone; err != nil {
		t.Fatalf("Update: %v", err)
	}
	<-connectDone
	if got := h.backend.connectCount(); got != 2 {
		t.Errorf("Backend.Connect calls = %d, want 2 (aborted attempt plus reconnect)", got)
	}
	if !h.backend.lastConnectConfig().ReadOnly {
		t.Error("reconnect did not carry ReadOnly=true")
	}
	if got := h.svc.StateOf(created.ID).Status; got != "connected" {
		t.Errorf("status = %q, want connected", got)
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

// TestTestValidatesInputBeforeProbing is a regression test for the finding where Test was
// the one Input-accepting entry point (unlike Create/Update) that never called Validate() —  a
// port outside 1-65535 reached the backend unchecked, and postgres/client.go's own
// `uint16(*cfg.Port)` would silently wrap it around to a different, in-range port rather than
// erroring. fakeBackend.Test never rejects a bad port itself, so this can only pass if
// Service.Test's own Validate() call stops the request before the backend is ever reached.
func TestTestValidatesInputBeforeProbing(t *testing.T) {
	h := newHarness(t)
	in := fieldsInput("bad-port")
	badPort := 70000 // out of range; uint16(70000) would silently become 4464
	in.Port = &badPort

	result := h.svc.Test(in, "")

	if result.OK {
		t.Fatal("Test(port: 70000).OK = true, want a validation failure")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "Port") {
		t.Fatalf("Test(port: 70000).Error = %v, want a message naming the port", result.Error)
	}
	if n := h.backend.testCount(); n != 0 {
		t.Errorf("Backend.Test was called %d times, want 0 — Validate() should have short-circuited first", n)
	}
}

// TestTestDoesNotLeakStoredPasswordToARetargetedDraft is a regression test for the
// credential-reveal bypass: editing an existing connection's Host (or Port/Database/
// URI) and pressing "Test connection" with no password retyped must not decrypt the stored
// secret and send it to the new, unverified destination — it should test with no password at
// all, exactly as a brand-new draft would.
func TestTestDoesNotLeakStoredPasswordToARetargetedDraft(t *testing.T) {
	h := newHarness(t)
	in := fieldsInput("has-secret")
	in.Password = strPtr("s3cret")
	created := mustCreate(t, h.svc, in)

	t.Run("unchanged destination still gets the stored secret", func(t *testing.T) {
		draft := fieldsInput("has-secret")
		draft.Password = nil // D1: edit dialogs always open with password: null

		result := h.svc.Test(draft, created.ID)

		if !result.OK {
			t.Fatalf("Test(unchanged destination).OK = false, want true (err: %v)", result.Error)
		}
		got := h.backend.lastConnectConfig().Password
		if got == nil || *got != "s3cret" {
			t.Fatalf("Backend.Test saw password %v, want the stored secret", got)
		}
	})

	t.Run("retargeted host does not get the stored secret", func(t *testing.T) {
		draft := fieldsInput("has-secret")
		draft.Password = nil
		draft.Host = strPtr("attacker.example.com")

		result := h.svc.Test(draft, created.ID)

		if !result.OK {
			t.Fatalf("Test(retargeted host).OK = false, want true (err: %v)", result.Error)
		}
		got := h.backend.lastConnectConfig().Password
		if got != nil {
			t.Fatalf("Backend.Test saw password %q for a retargeted host, want nil — the stored secret leaked", *got)
		}
		if gotHost := h.backend.lastConnectConfig().Host; gotHost == nil || *gotHost != "attacker.example.com" {
			t.Fatalf("Backend.Test saw host %v, want the retargeted host", gotHost)
		}
	})

	t.Run("retargeted port does not get the stored secret", func(t *testing.T) {
		draft := fieldsInput("has-secret")
		draft.Password = nil
		otherPort := 15432
		draft.Port = &otherPort

		result := h.svc.Test(draft, created.ID)

		if !result.OK {
			t.Fatalf("Test(retargeted port).OK = false, want true (err: %v)", result.Error)
		}
		if got := h.backend.lastConnectConfig().Password; got != nil {
			t.Fatalf("Backend.Test saw password %q for a retargeted port, want nil — the stored secret leaked", *got)
		}
	})
}

// TestTestGatesOnPreconnectAndOptionsOnlyEdits is a regression test for the P12 round 2 finding
// #1 bypass: round 1's fix only compared Host/Port/Database/URI, so a draft that changes only
// Preconnect (the command Test starts and routes the probe through before the destination is even
// reached) or only Options["sslmode"] still passed the "unchanged" check and leaked the stored
// secret down a route the user just edited.
func TestTestGatesOnPreconnectAndOptionsOnlyEdits(t *testing.T) {
	h := newHarness(t)
	in := fieldsInput("has-secret-2")
	in.Password = strPtr("s3cret")
	in.Options = map[string]any{"sslmode": "verify-full"}
	created := mustCreate(t, h.svc, in)

	t.Run("preconnect-only edit does not get the stored secret", func(t *testing.T) {
		draft := fieldsInput("has-secret-2")
		draft.Password = nil
		draft.Options = map[string]any{"sslmode": "verify-full"}
		draft.Preconnect = strPtr("true")

		result := h.svc.Test(draft, created.ID)

		if !result.OK {
			t.Fatalf("Test(preconnect-only edit).OK = false, want true (err: %v)", result.Error)
		}
		if got := h.backend.lastConnectConfig().Password; got != nil {
			t.Fatalf("Backend.Test saw password %q for a preconnect-only edit, want nil — the stored secret leaked", *got)
		}
	})

	t.Run("sslmode-only edit does not get the stored secret", func(t *testing.T) {
		draft := fieldsInput("has-secret-2")
		draft.Password = nil
		draft.Options = map[string]any{"sslmode": "disable"}

		result := h.svc.Test(draft, created.ID)

		if !result.OK {
			t.Fatalf("Test(sslmode-only edit).OK = false, want true (err: %v)", result.Error)
		}
		if got := h.backend.lastConnectConfig().Password; got != nil {
			t.Fatalf("Backend.Test saw password %q for an sslmode-only edit, want nil — the stored secret leaked", *got)
		}
	})

	t.Run("truly unchanged destination still gets the stored secret", func(t *testing.T) {
		draft := fieldsInput("has-secret-2")
		draft.Password = nil
		draft.Options = map[string]any{"sslmode": "verify-full"}

		result := h.svc.Test(draft, created.ID)

		if !result.OK {
			t.Fatalf("Test(unchanged destination).OK = false, want true (err: %v)", result.Error)
		}
		got := h.backend.lastConnectConfig().Password
		if got == nil || *got != "s3cret" {
			t.Fatalf("Backend.Test saw password %v, want the stored secret", got)
		}
	})
}

// TestUpdateAppliesThrottleLiveOnlyWhenConnected covers the "applies live" rule directly: an
// edit reaches Backend.SetThrottle while the connection is connected, but a throttle
// edit to a connection with nothing running yet gets no such call (attemptConnect installs the
// right value on the next connect regardless — no double-install to reconcile).
func TestUpdateAppliesThrottleLiveOnlyWhenConnected(t *testing.T) {
	h := newHarness(t)

	t.Run("connected: Update pushes the new rate live", func(t *testing.T) {
		created := mustCreate(t, h.svc, fieldsInput("throttle-connected"))
		if _, err := h.svc.Connect(created.ID); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		// attemptConnect itself calls SetThrottle(id, 0) on the way to Connect — the assertion
		// below only cares about the *last* call, so that first install is fine to ignore.

		draft := fieldsInput("throttle-connected")
		draft.ThrottlePerSec = 7.5
		if _, err := h.svc.Update(created.ID, draft); err != nil {
			t.Fatalf("Update: %v", err)
		}

		calls := h.backend.throttleCallsSnapshot()
		if len(calls) == 0 {
			t.Fatal("expected at least one SetThrottle call")
		}
		last := calls[len(calls)-1]
		if last.connectionID != created.ID || last.perSec != 7.5 {
			t.Errorf("last SetThrottle call = %+v, want {%s 7.5}", last, created.ID)
		}
	})

	t.Run("disconnected: Update installs nothing", func(t *testing.T) {
		created := mustCreate(t, h.svc, fieldsInput("throttle-disconnected"))

		draft := fieldsInput("throttle-disconnected")
		draft.ThrottlePerSec = 3
		if _, err := h.svc.Update(created.ID, draft); err != nil {
			t.Fatalf("Update: %v", err)
		}

		for _, c := range h.backend.throttleCallsSnapshot() {
			if c.connectionID == created.ID {
				t.Errorf("unexpected SetThrottle(%s, %v) for a never-connected connection", c.connectionID, c.perSec)
			}
		}
	})
}

// TestTestInjectsStoredPasswordAcrossThrottleOnlyEdit is the §5.5 "easily-missed correctness
// detail" regression test: destinationUnchanged must keep holding across a throttle-only edit, or
// "edit the throttle → Test connection" silently stops injecting the stored password.
func TestTestInjectsStoredPasswordAcrossThrottleOnlyEdit(t *testing.T) {
	h := newHarness(t)
	in := fieldsInput("has-secret-throttle")
	in.Password = strPtr("s3cret")
	created := mustCreate(t, h.svc, in)

	draft := fieldsInput("has-secret-throttle")
	draft.Password = nil
	draft.ThrottlePerSec = 42

	result := h.svc.Test(draft, created.ID)

	if !result.OK {
		t.Fatalf("Test(throttle-only edit).OK = false, want true (err: %v)", result.Error)
	}
	got := h.backend.lastConnectConfig().Password
	if got == nil || *got != "s3cret" {
		t.Fatalf("Backend.Test saw password %v, want the stored secret", got)
	}
}

// TestUpdateReconnectsALiveConnectionOnDestinationOrReadOnlyChange is the regression: a connected
// adapter and both cache layers are
// stamped with the config resolved at Connect time and never revisit it on their own. Before the
// fix, Update wrote the new row and, at most, pushed the throttle live — a destination change
// (host/port/database/etc.) left the old adapter connected to the old server, and toggling
// ReadOnly on a live connection changed nothing an app-level or server-level guard ever reads
// (both are captured once at Connect). The fix makes Update disconnect and reconnect whenever
// destinationUnchanged no longer holds, or ReadOnly itself changed — the same "so the engine
// picks up the new flag" reconnect the toolbar's own setConnectionReadOnly already performs.
func TestUpdateReconnectsALiveConnectionOnDestinationOrReadOnlyChange(t *testing.T) {
	t.Run("toggling ReadOnly on a live connection reconnects", func(t *testing.T) {
		h := newHarness(t)
		created := mustCreate(t, h.svc, fieldsInput("ro-toggle"))
		if _, err := h.svc.Connect(created.ID); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		connectsBefore := h.backend.connectCount()
		disconnectsBefore := h.backend.disconnectCount()

		draft := fieldsInput("ro-toggle")
		draft.ReadOnly = true
		if _, err := h.svc.Update(created.ID, draft); err != nil {
			t.Fatalf("Update: %v", err)
		}

		if got := h.backend.disconnectCount(); got != disconnectsBefore+1 {
			t.Errorf("Backend.Disconnect calls = %d, want %d (a reconnect)", got, disconnectsBefore+1)
		}
		if got := h.backend.connectCount(); got != connectsBefore+1 {
			t.Errorf("Backend.Connect calls = %d, want %d (a reconnect)", got, connectsBefore+1)
		}
		if !h.backend.lastConnectConfig().ReadOnly {
			t.Error("reconnect did not carry the new ReadOnly=true through to Backend.Connect")
		}
		if got := h.svc.StateOf(created.ID).Status; got != "connected" {
			t.Errorf("status after reconnect = %q, want connected", got)
		}
	})

	t.Run("changing the destination (Host) on a live connection reconnects", func(t *testing.T) {
		h := newHarness(t)
		created := mustCreate(t, h.svc, fieldsInput("dest-change"))
		if _, err := h.svc.Connect(created.ID); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		connectsBefore := h.backend.connectCount()
		disconnectsBefore := h.backend.disconnectCount()

		draft := fieldsInput("dest-change")
		draft.Host = strPtr("prod.example.internal")
		if _, err := h.svc.Update(created.ID, draft); err != nil {
			t.Fatalf("Update: %v", err)
		}

		if got := h.backend.disconnectCount(); got != disconnectsBefore+1 {
			t.Errorf("Backend.Disconnect calls = %d, want %d (a reconnect)", got, disconnectsBefore+1)
		}
		if got := h.backend.connectCount(); got != connectsBefore+1 {
			t.Errorf("Backend.Connect calls = %d, want %d (a reconnect)", got, connectsBefore+1)
		}
		gotHost := h.backend.lastConnectConfig().Host
		if gotHost == nil || *gotHost != "prod.example.internal" {
			t.Errorf("reconnect config Host = %v, want prod.example.internal", gotHost)
		}
	})

	t.Run("a cosmetic-only edit (Color) on a live connection does not reconnect", func(t *testing.T) {
		h := newHarness(t)
		created := mustCreate(t, h.svc, fieldsInput("cosmetic-only"))
		if _, err := h.svc.Connect(created.ID); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		connectsBefore := h.backend.connectCount()
		disconnectsBefore := h.backend.disconnectCount()

		draft := fieldsInput("cosmetic-only")
		draft.Color = "red"
		if _, err := h.svc.Update(created.ID, draft); err != nil {
			t.Fatalf("Update: %v", err)
		}

		if got := h.backend.connectCount(); got != connectsBefore {
			t.Errorf("Backend.Connect calls = %d, want %d (no reconnect for a cosmetic-only edit)", got, connectsBefore)
		}
		if got := h.backend.disconnectCount(); got != disconnectsBefore {
			t.Errorf("Backend.Disconnect calls = %d, want %d (no reconnect for a cosmetic-only edit)", got, disconnectsBefore)
		}
	})

	t.Run("changing ReadOnly on a disconnected connection does not attempt a reconnect", func(t *testing.T) {
		h := newHarness(t)
		created := mustCreate(t, h.svc, fieldsInput("ro-disconnected"))
		connectsBefore := h.backend.connectCount()

		draft := fieldsInput("ro-disconnected")
		draft.ReadOnly = true
		if _, err := h.svc.Update(created.ID, draft); err != nil {
			t.Fatalf("Update: %v", err)
		}

		if got := h.backend.connectCount(); got != connectsBefore {
			t.Errorf("Backend.Connect calls = %d, want %d (nothing live to reconnect)", got, connectsBefore)
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

// TestDuplicateCapsGeneratedNameAtMaxLength: Duplicate used to append " copy"
// with no length cap at all — a 118-character name plus " copy" (5 more) is 123 characters, over
// connectionInputSchema's own 120-character cap that a hand-typed name of that length would have
// been rejected for at Create time.
func TestDuplicateCapsGeneratedNameAtMaxLength(t *testing.T) {
	h := newHarness(t)
	longName := strings.Repeat("a", 118)
	created := mustCreate(t, h.svc, fieldsInput(longName))

	dup, err := h.svc.Duplicate(created.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if len(dup.Name) > 120 {
		t.Errorf("Duplicate name is %d bytes (%q), want at most 120", len(dup.Name), dup.Name)
	}
	if !strings.HasSuffix(dup.Name, " copy") {
		t.Errorf("Duplicate name = %q, want it to still end in \" copy\"", dup.Name)
	}
	if !utf8.ValidString(dup.Name) {
		t.Errorf("Duplicate name %q is not valid UTF-8", dup.Name)
	}

	// A short name is untouched apart from the suffix — the cap only bites when it would actually
	// be exceeded.
	short := mustCreate(t, h.svc, fieldsInput("short-name"))
	dupShort, err := h.svc.Duplicate(short.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dupShort.Name != "short-name copy" {
		t.Errorf("Duplicate name = %q, want %q", dupShort.Name, "short-name copy")
	}
}

// TestDuplicateCopiesThePasswordAtomically: Duplicate used to Conns.Insert the row and
// Secrets.Copy the password as two separate statements with no transaction, so a crash between
// them left a passwordless duplicate. InsertDuplicateWithSecret copies the password in the very
// same INSERT statement the row itself is written by — this confirms the happy path commits both
// together against a real database (a crash injected between two statements that no longer exist
// has nothing left to inject between).
func TestDuplicateCopiesThePasswordAtomically(t *testing.T) {
	h := newHarness(t)
	in := fieldsInput("with-a-password")
	in.Password = strPtr("s3cret")
	created := mustCreate(t, h.svc, in)

	dup, err := h.svc.Duplicate(created.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup.ID == created.ID {
		t.Fatalf("Duplicate returned the same ID as the original")
	}

	got, err := h.secrets.Get(dup.ID)
	if err != nil {
		t.Fatalf("Secrets.Get(duplicate): %v", err)
	}
	if got == nil || *got != "s3cret" {
		t.Errorf("duplicate's stored password = %v, want %q", got, "s3cret")
	}

	// The original's own password must be untouched by the copy.
	original, err := h.secrets.Get(created.ID)
	if err != nil {
		t.Fatalf("Secrets.Get(original): %v", err)
	}
	if original == nil || *original != "s3cret" {
		t.Errorf("original's stored password = %v, want unchanged %q", original, "s3cret")
	}
}

// TestDuplicateCopiesMaskRules: ConnectionFields carries MCP exposure over to a duplicate, but
// mask rules live in a separate table keyed by connection_id and were never copied
// — a duplicated connection that was masked-and-MCP-exposed became immediately MCP-exposed with
// zero mask rules, silently exposing whatever PII the original was protecting.
func TestDuplicateCopiesMaskRules(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("masked-source"))

	if _, err := h.repos.MaskRules.Upsert(uuid.NewString(), created.ID, model.MaskRuleFields{
		TableName: "customers", ColumnName: "email", Kind: model.MaskKindEmail, KeepHint: true, Correlate: true,
	}, kiratime.NowISO()); err != nil {
		t.Fatalf("MaskRules.Upsert: %v", err)
	}
	if _, err := h.repos.MaskRules.Upsert(uuid.NewString(), created.ID, model.MaskRuleFields{
		TableName: "customers", ColumnName: "ssn", Kind: model.MaskKindRedact,
	}, kiratime.NowISO()); err != nil {
		t.Fatalf("MaskRules.Upsert: %v", err)
	}

	dup, err := h.svc.Duplicate(created.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}

	dupRules, err := h.repos.MaskRules.ListForConnection(dup.ID)
	if err != nil {
		t.Fatalf("MaskRules.ListForConnection(duplicate): %v", err)
	}
	if len(dupRules) != 2 {
		t.Fatalf("duplicate has %d mask rules, want 2 (copied from the source)", len(dupRules))
	}
	if dupRules[0].ColumnName != "email" || dupRules[0].Kind != model.MaskKindEmail || !dupRules[0].Correlate {
		t.Errorf("duplicate rule[0] = %+v, want the source's email rule copied verbatim", dupRules[0])
	}
	if dupRules[1].ColumnName != "ssn" || dupRules[1].Kind != model.MaskKindRedact {
		t.Errorf("duplicate rule[1] = %+v, want the source's ssn rule copied verbatim", dupRules[1])
	}

	// Fresh ids and a connection_id pointing at the duplicate, not the source.
	for _, r := range dupRules {
		if r.ConnectionID != dup.ID {
			t.Errorf("copied rule ConnectionID = %q, want %q", r.ConnectionID, dup.ID)
		}
	}

	// The source's own rules must be untouched by the copy.
	sourceRules, err := h.repos.MaskRules.ListForConnection(created.ID)
	if err != nil {
		t.Fatalf("MaskRules.ListForConnection(source): %v", err)
	}
	if len(sourceRules) != 2 {
		t.Fatalf("source has %d mask rules after Duplicate, want unchanged 2", len(sourceRules))
	}

	// A source with no mask rules at all duplicates cleanly with none — not an error.
	plain := mustCreate(t, h.svc, fieldsInput("unmasked-source"))
	plainDup, err := h.svc.Duplicate(plain.ID)
	if err != nil {
		t.Fatalf("Duplicate(no rules): %v", err)
	}
	noRules, err := h.repos.MaskRules.ListForConnection(plainDup.ID)
	if err != nil {
		t.Fatalf("MaskRules.ListForConnection(no-rules duplicate): %v", err)
	}
	if len(noRules) != 0 {
		t.Fatalf("no-rules duplicate has %d mask rules, want 0", len(noRules))
	}
}

// TestDuplicateEnablesMcpOnlyAfterMaskRulesCopySucceeds: a source with MCP exposure and mask rules
// must still end up MCP-enabled on the happy path — InsertDuplicateWithSecret now always inserts
// the duplicate with mcp_enabled off, so this confirms Duplicate's own follow-up SetMcpEnabled
// call restores the source's real state once the mask rule copy succeeds,
// both in the returned summary and in storage.
func TestDuplicateEnablesMcpOnlyAfterMaskRulesCopySucceeds(t *testing.T) {
	h := newHarness(t)
	in := fieldsInput("masked-and-exposed")
	in.McpEnabled = true
	created := mustCreate(t, h.svc, in)

	if _, err := h.repos.MaskRules.Upsert(uuid.NewString(), created.ID, model.MaskRuleFields{
		TableName: "customers", ColumnName: "email", Kind: model.MaskKindEmail,
	}, kiratime.NowISO()); err != nil {
		t.Fatalf("MaskRules.Upsert: %v", err)
	}

	dup, err := h.svc.Duplicate(created.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if !dup.McpEnabled {
		t.Fatal("Duplicate result McpEnabled = false, want true (matching the source, once mask rules copied)")
	}

	stored, err := h.repos.Connections.Get(dup.ID)
	if err != nil {
		t.Fatalf("Conns.Get(duplicate): %v", err)
	}
	if stored == nil || !stored.McpEnabled {
		t.Fatalf("stored duplicate McpEnabled = %v, want true", stored)
	}

	dupRules, err := h.repos.MaskRules.ListForConnection(dup.ID)
	if err != nil {
		t.Fatalf("MaskRules.ListForConnection(duplicate): %v", err)
	}
	if len(dupRules) != 1 {
		t.Fatalf("duplicate has %d mask rules, want 1 (copied before MCP was enabled)", len(dupRules))
	}
}

// TestDuplicateLeavesMcpDisabledWhenMaskRuleCopyFails: before this fix, InsertDuplicateWithSecret
// copied McpEnabled verbatim in the same INSERT that committed the duplicate row, so an error from
// the mask-rule copy that ran afterward still surfaced to the caller — but the already-committed
// duplicate was left live and MCP-exposed with zero mask rules. This forces that copy to fail (the
// mask rules table is dropped after the source's own rule is written, so ListForConnection errors)
// and confirms the duplicate row that HAS been committed by then is not MCP-exposed, closing
// exactly that gap.
func TestDuplicateLeavesMcpDisabledWhenMaskRuleCopyFails(t *testing.T) {
	h := newHarness(t)
	in := fieldsInput("masked-and-exposed")
	in.McpEnabled = true
	created := mustCreate(t, h.svc, in)

	if _, err := h.repos.MaskRules.Upsert(uuid.NewString(), created.ID, model.MaskRuleFields{
		TableName: "customers", ColumnName: "email", Kind: model.MaskKindEmail,
	}, kiratime.NowISO()); err != nil {
		t.Fatalf("MaskRules.Upsert: %v", err)
	}

	// Break the mask rule copy deterministically: ListForConnection (copyMaskRules' own first
	// step) now errors on the missing table.
	if _, err := h.repos.MaskRules.DB.Exec(`DROP TABLE connection_mask_rules`); err != nil {
		t.Fatalf("drop connection_mask_rules: %v", err)
	}

	dup, err := h.svc.Duplicate(created.ID)
	if err == nil {
		t.Fatalf("Duplicate = %+v, nil error, want an error once the mask rule copy fails", dup)
	}

	// The duplicate connection row was already committed by InsertDuplicateWithSecret before the
	// failing copy step ran — it must not be MCP-exposed regardless.
	all, err := h.repos.Connections.List()
	if err != nil {
		t.Fatalf("Conns.List: %v", err)
	}
	for _, c := range all {
		if c.ID == created.ID {
			continue
		}
		if c.McpEnabled {
			t.Fatalf("duplicate connection %s committed with McpEnabled = true despite the failed mask rule copy, want false", c.ID)
		}
	}
}

// TestDuplicateEmitsListChangedEvenWhenMaskRuleCopyFails: the new duplicate row is already
// committed (InsertDuplicateWithSecret) by the time copyMaskRules can fail — emitListChanged must
// still run on this failure path, or the UI's own connection list never learns the new row exists
// until some unrelated later refresh.
func TestDuplicateEmitsListChangedEvenWhenMaskRuleCopyFails(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, fieldsInput("masked-list-changed"))

	// Same deterministic failure as TestDuplicateLeavesMcpDisabledWhenMaskRuleCopyFails.
	if _, err := h.repos.MaskRules.DB.Exec(`DROP TABLE connection_mask_rules`); err != nil {
		t.Fatalf("drop connection_mask_rules: %v", err)
	}

	var (
		fired    int
		lastList []model.ConnectionSummary
	)
	unsubscribe := h.svc.OnListChanged(func(list []model.ConnectionSummary) {
		fired++
		lastList = list
	})
	defer unsubscribe()

	dup, err := h.svc.Duplicate(created.ID)
	if err == nil {
		t.Fatalf("Duplicate = %+v, nil error, want an error once the mask rule copy fails", dup)
	}
	if fired == 0 {
		t.Fatal("OnListChanged never fired despite the new duplicate row already being committed")
	}

	found := false
	for _, c := range lastList {
		if c.ID != created.ID && strings.HasSuffix(c.Name, " copy") {
			found = true
		}
	}
	if !found {
		t.Fatalf("emitted list %+v does not carry the already-committed duplicate row", lastList)
	}
}

func uriInput(name string, uri *string) connections.Input {
	in := fieldsInput(name)
	in.Mode, in.URI, in.Host, in.Port = "uri", uri, nil, nil
	return in
}

func (h *harness) rawColumns(t *testing.T, id string) (uri, options string, password *string) {
	t.Helper()
	var pw sql.NullString
	if err := h.repos.Connections.DB.QueryRow(`SELECT uri, options_json, password FROM connections WHERE id = ?`, id).
		Scan(&uri, &options, &pw); err != nil {
		t.Fatalf("raw select: %v", err)
	}
	if pw.Valid {
		password = &pw.String
	}
	return uri, options, password
}

// TestListNeverCarriesTheURI pins P181's one invariant across every exit: whatever secrets a URI
// holds (userinfo and all four query secrets), no summary, List result or list-changed event
// carries it, the row stores only ciphertext, and Connect still hands the backend the exact URI.
func TestListNeverCarriesTheURI(t *testing.T) {
	h := newHarness(t)
	const full = "postgresql://u:pw1@h:5432/db?password=pw2&sslpassword=pw3&tlsCertificateKeyFilePassword=pw4&proxyPassword=pw5&sslmode=require"
	secretValues := []string{"pw1", "pw2", "pw3", "pw4", "pw5"}

	var emitted [][]model.ConnectionSummary
	unsubscribe := h.svc.OnListChanged(func(list []model.ConnectionSummary) { emitted = append(emitted, list) })
	defer unsubscribe()

	created := mustCreate(t, h.svc, uriInput("uri-conn", strPtr(full)))
	updated, err := h.svc.Update(created.ID, uriInput("uri-conn-renamed", strPtr(full)))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	dup, err := h.svc.Duplicate(created.ID)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	listed, err := h.svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	check := func(where string, c model.ConnectionSummary) {
		t.Helper()
		if c.URI != nil {
			t.Errorf("%s: URI = %q, want nil", where, *c.URI)
		}
		for k := range c.Options {
			if strings.EqualFold(k, "password") || strings.HasSuffix(strings.ToLower(k), "password") {
				t.Errorf("%s: options carry %q", where, k)
			}
		}
	}
	check("Create", created)
	check("Update", updated)
	check("Duplicate", dup)
	for _, c := range listed {
		check("List", c)
	}
	if len(emitted) == 0 {
		t.Fatal("no list-changed emission observed")
	}
	for _, list := range emitted {
		for _, c := range list {
			check("list-changed", c)
		}
	}

	for _, id := range []string{created.ID, dup.ID} {
		uri, options, password := h.rawColumns(t, id)
		if !strings.HasPrefix(uri, "kira:v3:") {
			t.Errorf("stored uri = %q, want a kira:v3: envelope", uri)
		}
		for _, v := range secretValues {
			if strings.Contains(uri, v) || strings.Contains(options, v) {
				t.Errorf("stored columns contain %q", v)
			}
		}
		if password != nil {
			t.Errorf("password column = %q, want NULL", *password)
		}
		if _, err := h.svc.Connect(id); err != nil {
			t.Fatalf("Connect %s: %v", id, err)
		}
		if got := h.backend.lastConnectConfig().URI; got == nil || *got != full {
			t.Errorf("backend-bound uri = %v, want the exact original", got)
		}
	}
}

// TestURIModeUpdateRules pins the three-state URI x three-state password x stored-mode interplay,
// where a wrong cell silently loses a stored password.
func TestURIModeUpdateRules(t *testing.T) {
	fieldsRow := func(h *harness) string {
		in := fieldsInput("row")
		in.Password = strPtr("stored")
		return mustCreate(t, h.svc, in).ID
	}
	uriRow := func(h *harness) string {
		return mustCreate(t, h.svc, uriInput("row", strPtr("postgresql://u:orig@h/db"))).ID
	}
	tests := []struct {
		name         string
		setup        func(h *harness) string
		uri          *string
		password     *string
		wantCode     string
		wantURI      string
		wantPassword *string
	}{
		{name: "nil URI keeps both columns", setup: uriRow, wantURI: "postgresql://u:orig@h/db"},
		{name: "fields row flipped folds the stored password", setup: fieldsRow, uri: strPtr("postgresql://u@h/db"), wantURI: "postgresql://u:stored@h/db"},
		{name: "explicit empty password does not fold", setup: fieldsRow, uri: strPtr("postgresql://u@h/db"), password: strPtr(""), wantURI: "postgresql://u@h/db"},
		{name: "URI own password beats the stored one", setup: fieldsRow, uri: strPtr("postgresql://u:own@h/db"), wantURI: "postgresql://u:own@h/db"},
		{name: "nil URI on a fields row is refused", setup: fieldsRow, wantCode: "E_BAD_REQUEST", wantPassword: strPtr("stored")},
		{name: "nil URI with a password is refused", setup: uriRow, password: strPtr("x"), wantCode: "E_BAD_REQUEST", wantURI: "postgresql://u:orig@h/db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			id := tt.setup(h)
			in := uriInput("row", tt.uri)
			in.Password = tt.password
			in.Options = map[string]any{"sslmode": "require"}
			_, err := h.svc.Update(id, in)
			if tt.wantCode != "" {
				if err == nil || asIpcErr(t, err).Code != tt.wantCode {
					t.Fatalf("Update err = %v, want %s", err, tt.wantCode)
				}
			} else if err != nil {
				t.Fatalf("Update: %v", err)
			}
			var opts string
			if err := h.repos.Connections.DB.QueryRow(`SELECT options_json FROM connections WHERE id = ?`, id).Scan(&opts); err != nil {
				t.Fatalf("options select: %v", err)
			}
			if opts != "{}" {
				t.Errorf("options_json = %s, want {} in URI mode", opts)
			}
			gotURI, err := h.secrets.GetURI(id)
			if err != nil {
				t.Fatalf("GetURI: %v", err)
			}
			if tt.wantURI == "" && gotURI != nil || tt.wantURI != "" && (gotURI == nil || *gotURI != tt.wantURI) {
				t.Errorf("URI = %v, want %q", gotURI, tt.wantURI)
			}
			gotPw, err := h.secrets.Get(id)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if (gotPw == nil) != (tt.wantPassword == nil) || gotPw != nil && *gotPw != *tt.wantPassword {
				t.Errorf("password = %v, want %v", gotPw, tt.wantPassword)
			}
		})
	}
}

// TestLegacyPlaintextURIFailsLoudly: a pre-P181 plaintext uri stays listed, fails every read with
// the envelope error (no silent success, no data deleted), and a typed replacement fixes it.
func TestLegacyPlaintextURIFailsLoudly(t *testing.T) {
	h := newHarness(t)
	created := mustCreate(t, h.svc, uriInput("legacy", strPtr("postgresql://u:p@h/db")))
	if _, err := h.repos.Connections.DB.Exec(`UPDATE connections SET uri = ? WHERE id = ?`,
		"postgresql://u@h/db", created.ID); err != nil {
		t.Fatalf("raw plaintext uri: %v", err)
	}

	list, err := h.svc.List()
	if err != nil || len(list) != 1 || list[0].URI != nil {
		t.Fatalf("List = %+v, %v, want the row listed with no URI", list, err)
	}
	state, _ := h.svc.Connect(created.ID)
	if state.Status != "error" || state.Error == nil || !strings.Contains(*state.Error, "envelope") {
		t.Fatalf("Connect state = %+v, want an error naming the envelope", state)
	}
	if res := h.svc.Reveal(created.ID, false); res.Outcome != "error" || res.Error == nil {
		t.Fatalf("Reveal = %+v, want the error outcome", res)
	}

	if _, err := h.svc.Update(created.ID, uriInput("legacy", strPtr("postgresql://u:new@h/db"))); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if state, err := h.svc.Connect(created.ID); err != nil || state.Status != "connected" {
		t.Fatalf("Connect after fix = %+v, %v, want connected", state, err)
	}
}
