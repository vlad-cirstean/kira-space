package adapterhost

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/oplog"
)

// opEndStatus drains events until the op:end for the single op under test.
func opEndStatus(t *testing.T, events <-chan oplog.Event) opEndPayload {
	t.Helper()
	for {
		select {
		case e := <-events:
			if e.Topic != oplog.EventOpEnd {
				continue
			}
			var p opEndPayload
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				t.Fatal(err)
			}
			return p
		case <-time.After(2 * time.Second):
			t.Fatal("no op:end")
		}
	}
}

func TestStatementTimeout_BackstopStopsOp(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	events, unsub := h.Subscribe()
	defer unsub()
	const connID = "conn-st-backstop"
	cancelled := make(chan string, 1)
	adapters.SetLiveAdapter(connID, &fakeAdapter{cancelFn: func(_ context.Context, opID string) (bool, error) {
		cancelled <- opID
		return true, nil
	}})
	defer adapters.DeleteLiveAdapter(connID)

	opID, _, err := h.RunOp(context.Background(), OpSpec{Kind: "execute", ConnectionID: strp(connID), Timeout: 80 * time.Millisecond},
		func(ctx context.Context, _ *adapters.OpCtx) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})
	var d *adapters.TimeoutDetail
	if !errors.As(err, &d) || !errors.Is(err, adapters.ErrStatementTimeout) {
		t.Fatalf("err = %v, want a statement timeout", err)
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeTimeout {
		t.Errorf("code = %s", code)
	}
	if d.Limit != 80*time.Millisecond || d.Elapsed < d.Limit {
		t.Errorf("limit %v elapsed %v", d.Limit, d.Elapsed)
	}
	select {
	case got := <-cancelled:
		if got != opID {
			t.Errorf("Cancel got %s, want %s", got, opID)
		}
	case <-time.After(time.Second):
		t.Fatal("adapter.Cancel not called")
	}
	if p := opEndStatus(t, events); p.Status != "error" {
		t.Errorf("status = %s, want error", p.Status)
	}
}

func TestStatementTimeout_GraceKeepsEngineError(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	const connID = "conn-st-grace"
	adapters.SetLiveAdapter(connID, &fakeAdapter{cancelFn: func(context.Context, string) (bool, error) {
		t.Error("Cancel must not run: the engine answered inside the grace")
		return true, nil
	}})
	defer adapters.DeleteLiveAdapter(connID)

	spec := OpSpec{Kind: "execute", ConnectionID: strp(connID), Timeout: 50 * time.Millisecond, TimeoutGrace: 2 * time.Second}
	_, _, err := h.RunOp(context.Background(), spec, func(ctx context.Context, _ *adapters.OpCtx) (any, error) {
		time.Sleep(60 * time.Millisecond)
		return nil, adapters.StatementTimeout(50*time.Millisecond, 0, errors.New("engine"))
	})
	var d *adapters.TimeoutDetail
	if !errors.As(err, &d) || d.Elapsed < 60*time.Millisecond {
		t.Fatalf("err = %v, detail %+v: Elapsed must be filled by the host", err, d)
	}
}

func TestStatementTimeout_QueueWaitNotCounted(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	h.SetThrottle("conn-st-queue", 1000)
	release, holderDone := holdOp(t, h, "conn-st-queue")

	done := make(chan error, 1)
	go func() {
		_, _, err := h.RunOp(context.Background(), OpSpec{Kind: "execute", ConnectionID: strp("conn-st-queue"), Timeout: 50 * time.Millisecond},
			func(context.Context, *adapters.OpCtx) (any, error) {
				time.Sleep(10 * time.Millisecond)
				return nil, nil
			})
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	close(release)
	<-holderDone
	if err := <-done; err != nil {
		t.Fatalf("queued op timed out on queue wait: %v", err)
	}
}

func TestStatementTimeout_ZeroArmsNoTimer(t *testing.T) {
	h := NewHost(adapters.Deps{}, nil)
	_, _, err := h.RunOp(context.Background(), OpSpec{Kind: "execute"},
		func(context.Context, *adapters.OpCtx) (any, error) {
			time.Sleep(120 * time.Millisecond)
			return nil, nil
		})
	if err != nil {
		t.Fatal(err)
	}
}
