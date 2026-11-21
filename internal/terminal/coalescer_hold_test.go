package terminal

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"sync"
	"testing"
	"time"
)

type emitted struct {
	to string
	ev Event
}

type recEmitter struct {
	mu  sync.Mutex
	out []emitted
}

func (r *recEmitter) Emit(string, any)        {}
func (r *recEmitter) EmitFocused(string, any) {}
func (r *recEmitter) EmitTo(to, _ string, data any) {
	r.mu.Lock()
	r.out = append(r.out, emitted{to, data.(Event)})
	r.mu.Unlock()
}

func (r *recEmitter) events() []emitted {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]emitted(nil), r.out...)
}

func decode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A producer pushes numbered chunks while the test holds and adopts: emitted-before, held and
// emitted-after bytes concatenate to the pushed stream, and end tracks the running byte count.
func TestCoalescerHold(t *testing.T) {
	t.Run("stream is exact across hold and adopt", func(t *testing.T) {
		em := &recEmitter{}
		c := newOutputCoalescer(em, "A", "t1")
		var want bytes.Buffer
		var wantMu sync.Mutex
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				chunk := []byte(fmt.Sprintf("<%06d>", i))
				wantMu.Lock()
				want.Write(chunk)
				wantMu.Unlock()
				c.push(chunk)
				if i%7 == 0 {
					time.Sleep(time.Millisecond)
				}
			}
		}()
		time.Sleep(30 * time.Millisecond)
		end, err := c.hold()
		if err != nil {
			t.Fatal(err)
		}
		c.reown("B")
		time.Sleep(40 * time.Millisecond)
		res, err := c.adopt("B")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
		close(stop)
		<-done
		c.finish(0, "")

		var pre, post bytes.Buffer
		var last int64
		for _, e := range em.events() {
			if e.ev.End < last {
				t.Fatalf("end went backwards: %d after %d", e.ev.End, last)
			}
			last = e.ev.End
			if e.ev.End <= end && e.to != "A" {
				t.Fatalf("pre-hold event to %q", e.to)
			}
			if e.ev.End > res.End && e.to != "B" {
				t.Fatalf("post-adopt event to %q", e.to)
			}
			if e.ev.End > end && e.ev.End <= res.End {
				t.Fatalf("event end %d emitted inside the held span (%d, %d]", e.ev.End, end, res.End)
			}
			if e.ev.End <= end {
				pre.Write(decode(t, e.ev.Data))
			} else {
				post.Write(decode(t, e.ev.Data))
			}
		}
		var got bytes.Buffer
		got.Write(pre.Bytes())
		got.Write(decode(t, res.Data))
		got.Write(post.Bytes())
		wantMu.Lock()
		defer wantMu.Unlock()
		if !bytes.Equal(got.Bytes(), want.Bytes()) {
			t.Fatalf("stream differs: got %d bytes, want %d", got.Len(), want.Len())
		}
		if last != int64(want.Len()) {
			t.Fatalf("final end %d, want %d", last, want.Len())
		}
		if res.End-end != int64(len(decode(t, res.Data))) {
			t.Fatalf("adopt end %d - hold end %d != data %d", res.End, end, len(decode(t, res.Data)))
		}
	})

	t.Run("overflow drops oldest and flags truncation", func(t *testing.T) {
		c := newOutputCoalescer(&recEmitter{}, "A", "t1")
		if _, err := c.hold(); err != nil {
			t.Fatal(err)
		}
		c.reown("B")
		chunk := bytes.Repeat([]byte("x"), 300*1024)
		for i := 0; i < 5; i++ {
			c.mu.Lock()
			c.holdLocked(chunk)
			c.mu.Unlock()
		}
		res, err := c.adopt("B")
		if err != nil {
			t.Fatal(err)
		}
		if !res.Truncated || len(decode(t, res.Data)) > maxHeldBytes {
			t.Fatalf("truncated=%v len=%d", res.Truncated, len(decode(t, res.Data)))
		}
		if res.End != int64(5*len(chunk)) {
			t.Fatalf("end %d counts dropped bytes, want %d", res.End, 5*len(chunk))
		}
	})

	t.Run("release replays to the old window", func(t *testing.T) {
		em := &recEmitter{}
		c := newOutputCoalescer(em, "A", "t1")
		if _, err := c.hold(); err != nil {
			t.Fatal(err)
		}
		c.mu.Lock()
		c.holdLocked([]byte("held"))
		c.mu.Unlock()
		c.release()
		evs := em.events()
		if len(evs) != 1 || evs[0].to != "A" || string(decode(t, evs[0].ev.Data)) != "held" || evs[0].ev.End != 4 {
			t.Fatalf("got %+v", evs)
		}
	})

	t.Run("adopt by the wrong window is refused", func(t *testing.T) {
		c := newOutputCoalescer(&recEmitter{}, "A", "t1")
		if _, err := c.hold(); err != nil {
			t.Fatal(err)
		}
		c.reown("B")
		if _, err := c.adopt("A"); err != ErrNotOwner {
			t.Fatalf("got %v", err)
		}
		if _, err := c.hold(); err != ErrAlreadyMoving {
			t.Fatalf("got %v", err)
		}
	})
}
