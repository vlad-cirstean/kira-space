package mobileterm

import (
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

type fakeReg struct {
	mu      sync.Mutex
	writes  []string
	resizes [][2]uint16
}

func (f *fakeReg) Write(_ string, b []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, string(b))
	return nil
}

func (f *fakeReg) Resize(_ string, c, r uint16) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resizes = append(f.resizes, [2]uint16{c, r})
	return nil
}

func (f *fakeReg) lastResize() [2]uint16 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resizes[len(f.resizes)-1]
}

type fakeTimer struct {
	at      time.Time
	f       func()
	stopped bool
}

func (t *fakeTimer) Stop() bool { t.stopped = true; return true }

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }

func (c *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{at: c.now.Add(d), f: f}
	c.timers = append(c.timers, t)
	return t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var due []*fakeTimer
	for _, t := range c.timers {
		if !t.stopped && !t.at.After(c.now) {
			t.stopped = true
			due = append(due, t)
		}
	}
	c.mu.Unlock()
	for _, t := range due {
		t.f()
	}
}

func newTestBroker(t *testing.T) (*Broker, *fakeReg, *fakeClock) {
	t.Helper()
	reg, clk := &fakeReg{}, &fakeClock{now: time.Unix(1000, 0)}
	b := New(Deps{
		Registry: reg, Now: clk.Now, AfterFunc: clk.AfterFunc,
		Sessions: func(s string) (string, bool) { return "t-" + s, s != "gone" },
	})
	b.SetRecording(true)
	b.Opened("t-s1", 100, 30)
	return b, reg, clk
}

var phoneA = repos.MobileDeviceRow{ID: "a", Label: "Phone A"}

func TestBroker_TakeoverStateMachine(t *testing.T) {
	b, reg, clk := newTestBroker(t)
	if !b.AllowWrite("t-s1") {
		t.Fatal("desktop owns a fresh terminal")
	}
	c, err := b.Attach("s1", phoneA, 50, 20)
	if err != nil {
		t.Fatal(err)
	}
	if b.AllowWrite("t-s1") || b.AllowResize("t-s1", 120, 40) {
		t.Fatal("window input must be dropped while a phone holds")
	}
	if reg.lastResize() != [2]uint16{50, 20} {
		t.Fatalf("phone size not applied: %v", reg.lastResize())
	}
	if _, err := b.Attach("s1", repos.MobileDeviceRow{ID: "b"}, 50, 20); err == nil {
		t.Fatal("second device must be refused")
	}
	if !b.Input(c, []byte("x")) {
		t.Fatal("holder input refused")
	}

	b.Disconnected(c)
	if h := b.Holds(); len(h) != 1 || h[0].Connected || h[0].ReturnsAt == 0 {
		t.Fatalf("offline hold = %+v", h)
	}
	clk.advance(30 * time.Second)
	c2, err := b.Attach("s1", phoneA, 50, 20)
	if err != nil {
		t.Fatalf("reattach inside grace: %v", err)
	}
	clk.advance(40 * time.Second) // the first grace timer must not fire
	if len(b.Holds()) != 1 {
		t.Fatal("stale grace timer released the hold")
	}

	b.Disconnected(c2)
	clk.advance(61 * time.Second)
	if len(b.Holds()) != 0 || !b.AllowWrite("t-s1") {
		t.Fatal("grace expiry must return to desktop")
	}
	if reg.lastResize() != [2]uint16{120, 40} {
		t.Fatalf("desktop size (recorded while held) not restored: %v", reg.lastResize())
	}
}

func TestBroker_ReclaimReplaceAndIdle(t *testing.T) {
	b, reg, clk := newTestBroker(t)
	c1, _ := b.Attach("s1", phoneA, 50, 20)
	c2, _ := b.Attach("s1", phoneA, 50, 20)
	select {
	case <-c1.Done():
		if c1.Code() != CloseReplaced {
			t.Fatalf("code = %d", c1.Code())
		}
	default:
		t.Fatal("old socket not replaced")
	}
	if b.Input(c1, []byte("x")) {
		t.Fatal("replaced conn must not write")
	}
	b.Reclaim("t-s1", 90, 25)
	if c2.Code() != CloseReclaimed || reg.lastResize() != [2]uint16{90, 25} || len(b.Holds()) != 0 {
		t.Fatalf("reclaim: code %d resize %v", c2.Code(), reg.lastResize())
	}

	c3, _ := b.Attach("s1", phoneA, 50, 20)
	clk.advance(10 * time.Minute)
	b.Input(c3, []byte("k"))
	clk.advance(10 * time.Minute)
	if len(b.Holds()) != 1 {
		t.Fatal("input must reset the idle timer")
	}
	clk.advance(6 * time.Minute)
	if len(b.Holds()) != 0 || c3.Code() != CloseTimeout {
		t.Fatalf("idle timeout: code %d", c3.Code())
	}
}

func TestBroker_ReleaseDeviceAndExit(t *testing.T) {
	b, _, _ := newTestBroker(t)
	c, _ := b.Attach("s1", phoneA, 50, 20)
	b.ReleaseDevice("other")
	if len(b.Holds()) != 1 {
		t.Fatal("another device's revoke released the hold")
	}
	b.ReleaseDevice("a")
	if c.Code() != CloseRevoked || len(b.Holds()) != 0 {
		t.Fatalf("revoke: code %d", c.Code())
	}
	c, _ = b.Attach("s1", phoneA, 50, 20)
	b.Exited("t-s1")
	if c.Code() != CloseExit {
		t.Fatalf("exit code = %d", c.Code())
	}
	if _, err := b.Attach("s1", phoneA, 50, 20); err == nil {
		t.Fatal("attach after exit must fail")
	}
}

type blockReg struct {
	fakeReg
	entered chan struct{}
	release chan struct{}
}

func (f *blockReg) Write(id string, b []byte) error {
	close(f.entered)
	<-f.release
	return f.fakeReg.Write(id, b)
}

// A parked PTY write must not hold the entry lock: the PTY reader's Output would stall, which
// stalls the write itself.
func TestBroker_InputDoesNotBlockEntry(t *testing.T) {
	reg := &blockReg{entered: make(chan struct{}), release: make(chan struct{})}
	b := New(Deps{Registry: reg, Sessions: func(s string) (string, bool) { return "t-" + s, true }})
	b.SetRecording(true)
	b.Opened("t-s1", 100, 30)
	c, err := b.Attach("s1", phoneA, 50, 20)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan bool)
	go func() { done <- b.Input(c, []byte("x")) }()
	<-reg.entered

	finished := make(chan struct{})
	go func() {
		b.Output("t-s1", []byte("out"))
		b.AllowWrite("t-s1")
		b.Holds()
		b.Reclaim("t-s1", 0, 0)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("entry calls blocked behind a parked write")
	}
	close(reg.release)
	<-done
}

// A pump still holding a released conn must read an empty ring, not panic, after recording stops.
func TestBroker_ReleasedConnReadsAfterRecordingOff(t *testing.T) {
	b, _, _ := newTestBroker(t)
	c, err := b.Attach("s1", phoneA, 50, 20)
	if err != nil {
		t.Fatal(err)
	}
	b.Release(c)
	b.SetRecording(false)
	if d, end := c.Snapshot(); len(d) != 0 || end != 0 {
		t.Fatalf("snapshot = %q, %d", d, end)
	}
	if d, next, over := c.ReadFrom(0, 10); len(d) != 0 || next != 0 || over {
		t.Fatalf("read = %q, %d, %v", d, next, over)
	}
	if c.End() != 0 {
		t.Fatal("end must be 0")
	}
}
