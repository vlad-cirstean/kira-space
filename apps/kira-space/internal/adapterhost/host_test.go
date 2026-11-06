package adapterhost

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiooplog"
	"github.com/kirathecat/kira-studio/internal/ids"
)

// fakeAdapter overrides only the methods a test needs; every other adapters.Adapter method panics
// via the embedded nil interface if a test accidentally reaches it.
type fakeAdapter struct {
	adapters.Adapter
	cancelFn func(ctx context.Context, opID string) (bool, error)
}

func (f *fakeAdapter) Cancel(ctx context.Context, opID string) (bool, error) {
	return f.cancelFn(ctx, opID)
}

// RunOp must refuse a duplicate op id outright — scheduler/ops.ts's own reasoning is that a
// duplicate id would corrupt the op log's primary key and let the stop button cancel the wrong
// query, so this is checked before op:start is even emitted, not caught downstream.
func TestRunOp_RefusesDuplicateOpID(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	release := make(chan struct{})
	started := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, _ = h.RunOp(context.Background(), OpSpec{OpID: "35aa4fd1-8a08-76b1-8044-5e6cf39f36fa", Kind: "read"},
			func(ctx context.Context, op *adapters.OpCtx) (any, error) {
				close(started)
				<-release
				return nil, nil
			})
	}()
	<-started

	_, _, err := h.RunOp(context.Background(), OpSpec{OpID: "35aa4fd1-8a08-76b1-8044-5e6cf39f36fa", Kind: "read"},
		func(ctx context.Context, op *adapters.OpCtx) (any, error) {
			t.Fatal("the duplicate call's fn must never run")
			return nil, nil
		})
	if err == nil {
		t.Fatal("expected an error for a duplicate op id")
	}
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeQuery {
		t.Fatalf("got %v, want an E_QUERY *adapters.Error", err)
	}
	if ae.Message != "duplicate operation id: 35aa4fd1-8a08-76b1-8044-5e6cf39f36fa" {
		t.Errorf("message = %q", ae.Message)
	}

	close(release)
	wg.Wait()
}

// quoteIdent (postgres/mysqlfamily/sqlite/clickhouse read.go) panics with an *adapters.Error
// carrying E_QUERY on a NUL byte in an identifier — a user-input problem, not an internal fault.
// safeRun's recover() used to always fold a panic into a fresh E_INTERNAL, losing that code and
// presenting the query error as an internal one.
func TestRunOp_PreservesAnAdaptersErrorCodeAcrossAPanic(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	_, _, err := h.RunOp(context.Background(), OpSpec{OpID: "ba787b76-f835-7399-85c0-9b175a38cffe", Kind: "read"},
		func(ctx context.Context, op *adapters.OpCtx) (any, error) {
			panic(adapters.New(adapters.CodeQuery, "identifier contains a NUL byte", nil))
		})
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeQuery {
		t.Fatalf("got %v, want the panicked *adapters.Error's own E_QUERY code preserved", err)
	}
	if ae.Message != "identifier contains a NUL byte" {
		t.Errorf("message = %q, want the original message preserved", ae.Message)
	}
}

// An untyped panic (a real internal fault, not adapters' own error type) must still fold into
// E_INTERNAL — the fix above only special-cases a panicked *adapters.Error.
func TestRunOp_UntypedPanicStillBecomesInternalError(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	_, _, err := h.RunOp(context.Background(), OpSpec{OpID: "60bb84ae-9935-7f6a-8d19-95976ae61a7f", Kind: "read"},
		func(ctx context.Context, op *adapters.OpCtx) (any, error) {
			panic("boom")
		})
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.ErrorCode("E_INTERNAL") {
		t.Fatalf("got %v, want an E_INTERNAL *adapters.Error", err)
	}
}

// CancelOp must stay two-step and in order: the local abort unblocks the running RunOp call
// immediately (never mind whether the adapter ever answers), and only then is the adapter's own
// Cancel called — the one that kills the server-side work (cancellation is always
// forwarded, and the local abort alone is not a cancel).
func TestCancelOp_LocalAbortThenAdapterCancel(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	const connID = "conn-1"

	cancelCalled := make(chan string, 1)
	fake := &fakeAdapter{cancelFn: func(ctx context.Context, opID string) (bool, error) {
		cancelCalled <- opID
		return true, nil
	}}
	adapters.SetLiveAdapter(connID, fake)
	defer adapters.DeleteLiveAdapter(connID)

	opStarted := make(chan struct{})
	opDone := make(chan struct{})
	var derivedCtx context.Context
	go func() {
		_, _, _ = h.RunOp(context.Background(), OpSpec{OpID: "a0b1e93b-9743-700a-8bd9-36410af20fd8", Kind: "read", ConnectionID: strp(connID)},
			func(ctx context.Context, op *adapters.OpCtx) (any, error) {
				derivedCtx = ctx
				close(opStarted)
				<-ctx.Done()
				return nil, ctx.Err()
			})
		close(opDone)
	}()
	<-opStarted

	ok, err := h.CancelOp(context.Background(), "a0b1e93b-9743-700a-8bd9-36410af20fd8")
	if err != nil || !ok {
		t.Fatalf("CancelOp = %v, %v, want true, nil", ok, err)
	}
	// By the time CancelOp returns, the local abort must already have unblocked RunOp's fn —
	// CancelOp calls op.cancel() before it ever touches the adapter, so this holds regardless of
	// how long the fake adapter's own Cancel takes to answer.
	if derivedCtx.Err() == nil {
		t.Error("the local context must already be cancelled once CancelOp returns")
	}

	select {
	case opID := <-cancelCalled:
		if opID != "a0b1e93b-9743-700a-8bd9-36410af20fd8" {
			t.Errorf("adapter.Cancel called with opID %q, want op-2", opID)
		}
	case <-time.After(time.Second):
		t.Fatal("adapter.Cancel was never called")
	}
	<-opDone

	// A second cancel of the same, now-finished op is a no-op — cancel() "never throws for
	// already finished".
	ok2, err2 := h.CancelOp(context.Background(), "a0b1e93b-9743-700a-8bd9-36410af20fd8")
	if err2 != nil || ok2 {
		t.Fatalf("second CancelOp = %v, %v, want false, nil", ok2, err2)
	}
}

// Delivery used to be a non-blocking send into a fixed 32-slot buffer, silently
// dropping an event outright once it filled — a dropped op:end leaves an op_log row stuck
// "running" forever, and a dropped op:start produces a phantom "test"-kind record.
// This emits far more than that old buffer's own capacity before the
// subscriber ever reads a single one (the exact shape a burst of concurrent ops produces — up to
// 64 concurrent data ops, per session.go's own inflight cap, plus tree/dbmcp/http/grpc ops, easily
// outpacing oplog's own synchronous SQLite write plus Wails emit per event) and confirms every one
// is still eventually delivered, none dropped.
func TestSubscribe_DeliversEveryEventEvenWhenFarPastTheOldFixedBufferSize(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	events, unsubscribe := h.Subscribe()
	defer unsubscribe()

	const n = 500 // well past the old fixed 32-slot buffer
	for i := 0; i < n; i++ {
		h.emitJSON(oplog.EventOpStart, opStartPayload{OpID: strconv.Itoa(i)})
	}

	received := make(map[string]bool, n)
	for len(received) < n {
		select {
		case e := <-events:
			var payload struct {
				OpID string `json:"opId"`
			}
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			received[payload.OpID] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("received only %d/%d events before timing out — some were dropped", len(received), n)
		}
	}
}

// Unsubscribe must never panic even when it races a concurrent Emit trying to deliver into the
// same channel — the exact bug eventSub's own mutex exists to prevent (host.go's doc comment on
// eventSub explains why notify.Emitter's own contract makes this possible). Run with -race.
func TestSubscribe_UnsubscribeRacesEmitWithoutPanicking(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		_, unsubscribe := h.Subscribe()
		wg.Add(2)
		go func() {
			defer wg.Done()
			h.emitJSON(oplog.EventOpStart, opStartPayload{OpID: "x"})
		}()
		go func() {
			defer wg.Done()
			unsubscribe()
		}()
	}
	wg.Wait()
}

func strp(s string) *string { return &s }

func TestCancelOp_BeforeRunOpCancelsTheLaterRun(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	if ok, err := h.CancelOp(context.Background(), "818743b9-67a1-7dbf-852b-beef8fb9d62f"); ok || err != nil {
		t.Fatalf("CancelOp = %v, %v", ok, err)
	}
	_, _, err := h.RunOp(context.Background(), OpSpec{OpID: "818743b9-67a1-7dbf-852b-beef8fb9d62f", Kind: "read"},
		func(context.Context, *adapters.OpCtx) (any, error) {
			t.Fatal("a pre-cancelled op must not run")
			return nil, nil
		})
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeCancelled {
		t.Fatalf("got %v, want E_CANCELLED", err)
	}

	h.earlyTTL = time.Millisecond
	_, _ = h.CancelOp(context.Background(), "a4e976a3-d3fc-7d0b-8045-3de216fbbe2e")
	time.Sleep(5 * time.Millisecond)
	ran := false
	_, _, err = h.RunOp(context.Background(), OpSpec{OpID: "a4e976a3-d3fc-7d0b-8045-3de216fbbe2e", Kind: "read"},
		func(context.Context, *adapters.OpCtx) (any, error) { ran = true; return nil, nil })
	if err != nil || !ran {
		t.Fatalf("expired early cancel blocked a later run: ran=%v err=%v", ran, err)
	}

	for range maxEarlyCancels + 10 {
		_, _ = h.CancelOp(context.Background(), ids.NewID())
	}
	if n := len(h.earlyCancels); n > maxEarlyCancels {
		t.Fatalf("earlyCancels grew to %d", n)
	}
}
