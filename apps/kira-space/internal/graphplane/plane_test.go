package graphplane

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type pipe struct {
	in, out chan []byte
	closed  chan struct{}
}

func newPipe() *pipe {
	return &pipe{in: make(chan []byte, 16), out: make(chan []byte, 64), closed: make(chan struct{})}
}

func (p *pipe) Send(b []byte) error {
	select {
	case p.out <- b:
		return nil
	case <-p.closed:
		return errors.New("closed")
	}
}

func (p *pipe) Receive() ([]byte, error) {
	select {
	case b := <-p.in:
		return b, nil
	case <-p.closed:
		return nil, errors.New("closed")
	}
}

func (p *pipe) send(t *testing.T, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	p.in <- b
}

// next reads one outbound frame and splits a blob frame into header and blob.
func (p *pipe) next(t *testing.T) (outHeader, []byte) {
	t.Helper()
	select {
	case raw := <-p.out:
		body, blob := raw, []byte(nil)
		if raw[0] == blobDiscrimin {
			n := int(binary.BigEndian.Uint32(raw[1:5]))
			body, blob = raw[5:5+n], raw[5+n:]
		}
		var h outHeader
		if err := json.Unmarshal(body, &h); err != nil {
			t.Fatalf("bad frame %q: %v", body, err)
		}
		return h, blob
	case <-time.After(5 * time.Second):
		t.Fatal("no frame within 5s")
		return outHeader{}, nil
	}
}

func (p *pipe) quiet(t *testing.T) {
	t.Helper()
	select {
	case raw := <-p.out:
		t.Fatalf("unexpected frame %q", raw)
	case <-time.After(100 * time.Millisecond):
	}
}

func serve(t *testing.T, p *pipe, open OpenFunc, maxFrame int) (stop func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		Serve(context.Background(), p, open, maxFrame)
		close(done)
	}()
	return func() {
		close(p.closed)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Serve did not return after close")
		}
	}
}

func chunksOf(n int) OpenFunc {
	return func(_ context.Context, _ string, _ json.RawMessage, emit func(any, []byte) error) error {
		for i := 0; i < n; i++ {
			if err := emit(map[string]int{"i": i}, []byte{1, 2, 3}); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestPlane_CreditGatesChunks(t *testing.T) {
	p := newPipe()
	stop := serve(t, p, chunksOf(3), 0)
	defer stop()

	p.send(t, map[string]any{"t": "open", "id": 1, "windowKey": "w", "credit": 1})
	h, blob := p.next(t)
	if h.T != "chunk" || h.ID != 1 || string(blob) != "\x01\x02\x03" {
		t.Fatalf("first frame = %+v blob %v", h, blob)
	}
	p.quiet(t)

	p.send(t, map[string]any{"t": "credit", "id": 1, "n": 2})
	for i := 0; i < 2; i++ {
		if h, _ := p.next(t); h.T != "chunk" {
			t.Fatalf("frame %d = %+v", i, h)
		}
	}
	if h, _ := p.next(t); h.T != "end" || h.Error != nil {
		t.Fatalf("last frame = %+v", h)
	}
}

func TestPlane_CancelUnblocksCreditWait(t *testing.T) {
	p := newPipe()
	var ended atomic.Bool
	open := func(ctx context.Context, w string, params json.RawMessage, emit func(any, []byte) error) error {
		defer ended.Store(true)
		return chunksOf(3)(ctx, w, params, emit)
	}
	stop := serve(t, p, open, 0)
	defer stop()

	p.send(t, map[string]any{"t": "open", "id": 1, "credit": 1})
	p.next(t)
	p.send(t, map[string]any{"t": "cancel", "id": 1})
	deadline := time.Now().Add(5 * time.Second)
	for !ended.Load() {
		if time.Now().After(deadline) {
			t.Fatal("stream still blocked after cancel")
		}
		time.Sleep(5 * time.Millisecond)
	}
	p.quiet(t) // a cancelled stream sends no end
}

func TestPlane_OpenErrorEndsStream(t *testing.T) {
	p := newPipe()
	open := func(context.Context, string, json.RawMessage, func(any, []byte) error) error {
		return errors.New("boom")
	}
	stop := serve(t, p, open, 0)
	defer stop()

	p.send(t, map[string]any{"t": "open", "id": 4})
	h, _ := p.next(t)
	if h.T != "end" || h.ID != 4 || h.Error == nil || h.Error.Message != "boom" {
		t.Fatalf("frame = %+v", h)
	}
}

func TestPlane_FrameCapEndsStream(t *testing.T) {
	p := newPipe()
	stop := serve(t, p, chunksOf(2), 8)
	defer stop()

	p.send(t, map[string]any{"t": "open", "id": 1, "credit": 2})
	h, _ := p.next(t)
	if h.T != "end" || h.Error == nil || h.Error.Code != "E_FRAME_TOO_LARGE" {
		t.Fatalf("frame = %+v", h)
	}
}

func TestPlane_DuplicateLiveIDKeepsFirst(t *testing.T) {
	p := newPipe()
	stop := serve(t, p, chunksOf(2), 0)
	defer stop()

	p.send(t, map[string]any{"t": "open", "id": 1, "credit": 1})
	p.next(t)
	p.send(t, map[string]any{"t": "open", "id": 1, "credit": 1})
	if h, _ := p.next(t); h.T != "end" || h.Error == nil {
		t.Fatalf("duplicate open = %+v, want end with error", h)
	}
	p.send(t, map[string]any{"t": "credit", "id": 1, "n": 1})
	if h, _ := p.next(t); h.T != "chunk" {
		t.Fatalf("first stream frame = %+v", h)
	}
}

func TestPlane_CloseCancelsStreams(t *testing.T) {
	p := newPipe()
	started, stopped := make(chan struct{}), make(chan struct{})
	open := func(ctx context.Context, _ string, _ json.RawMessage, _ func(any, []byte) error) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	}
	stop := serve(t, p, open, 0)
	p.send(t, map[string]any{"t": "open", "id": 1})
	<-started
	stop()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stream not cancelled by plane close")
	}
}
