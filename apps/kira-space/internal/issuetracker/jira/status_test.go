package jira

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

// probeRunner counts `--version` probes and can fail the auth check.
type probeRunner struct {
	probes   atomic.Int32
	loggedIn atomic.Bool
}

func (r *probeRunner) Run(_ context.Context, _ string, args []string) ([]byte, error) {
	if args[0] == "--version" {
		r.probes.Add(1)
		return []byte("acli version 1.3.4-stable\n"), nil
	}
	if !r.loggedIn.Load() {
		return nil, &toolexec.ExecError{ExitCode: 1, Stderr: "not logged in"}
	}
	return []byte("Authenticated\n  Site: acme.atlassian.net\n  Email: me@acme.com\n"), nil
}

func waitProbes(t *testing.T, r *probeRunner, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for r.probes.Load() != want {
		if time.Now().After(deadline) {
			t.Fatalf("probes %d, want %d", r.probes.Load(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStatusStaleWhileRevalidate(t *testing.T) {
	r := &probeRunner{}
	r.loggedIn.Store(true)
	clk := &testClock{t: time.Unix(1_700_000_000, 0)}
	d := newDiscovery(fixedLocator{}, r, clk)
	ctx := context.Background()

	if !d.status(ctx, false).OK() || r.probes.Load() != 1 {
		t.Fatal("first probe")
	}
	// OK past okTTL: served at once, one background probe however many callers
	clk.advance(okTTL + time.Second)
	for range 3 {
		if !d.status(ctx, false).OK() {
			t.Fatal("stale ok not served")
		}
	}
	waitProbes(t, r, 2)
	// the background probe replaced the entry
	if !d.status(ctx, false).OK() || r.probes.Load() != 2 {
		t.Fatal("entry not refreshed")
	}
	// refresh blocks
	d.status(ctx, true)
	if r.probes.Load() != 3 {
		t.Fatalf("refresh probes %d", r.probes.Load())
	}
	// past okStaleFor blocks and sees the logout inline
	r.loggedIn.Store(false)
	clk.advance(okStaleFor + time.Second)
	if s := d.status(ctx, false); s.Kind != issuetracker.KindUnauthenticated || r.probes.Load() != 4 {
		t.Fatalf("old entry %+v probes %d", s, r.probes.Load())
	}
	// a not-OK entry blocks once past notOKTTL
	r.loggedIn.Store(true)
	clk.advance(notOKTTL + time.Second)
	if !d.status(ctx, false).OK() || r.probes.Load() != 5 {
		t.Fatal("not-ok entry served stale")
	}
	// forget forces an inline probe
	d.forget()
	d.status(ctx, false)
	if r.probes.Load() != 6 {
		t.Fatalf("forget probes %d", r.probes.Load())
	}
}

func TestStatusCancelledProbeNotCached(t *testing.T) {
	r := &probeRunner{}
	r.loggedIn.Store(true)
	d := newDiscovery(fixedLocator{}, r, &testClock{t: time.Unix(1_700_000_000, 0)})
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	d.status(cctx, false)
	d.mu.Lock()
	cached := d.cached
	d.mu.Unlock()
	if cached != nil {
		t.Fatal("cancelled probe cached")
	}
}
