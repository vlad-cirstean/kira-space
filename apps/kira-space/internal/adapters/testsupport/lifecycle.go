package testsupport

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// ConnectionLifecycleScenarios runs the pinned-connection scenarios shared by the engines that keep
// one connection per database (postgres, mysqlfamily) against a live server reached through a
// PausableProxy: cancel-then-reuse, cancelling an op queued behind a long one and
// teardown while an op hangs. newAdapter returns an unconnected adapter; cfg is the fixture's
// config. Every scenario holds the primary connection with a catalog query whose response the
// proxy withholds.
func ConnectionLifecycleScenarios(t *testing.T, cfg model.ResolvedConnectionConfig, newAdapter func(t *testing.T) adapters.Adapter) {
	root := NodePath(cfg.ID)
	setup := func(t *testing.T) (adapters.Adapter, *PausableProxy) {
		t.Helper()
		proxy := StartPausableProxy(t, cfg)
		a := newAdapter(t)
		if _, err := a.Connect(context.Background(), proxy.Config(), adapters.NewOpCtx("lc-connect")); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		t.Cleanup(func() { proxy.Resume(); _ = a.Disconnect(context.Background()) })
		if _, err := a.Children(context.Background(), root, adapters.NewOpCtx("lc-prime")); err != nil {
			t.Fatalf("prime Children: %v", err)
		}
		return a, proxy
	}
	children := func(ctx context.Context, a adapters.Adapter, opID string) <-chan error {
		errc := make(chan error, 1)
		go func() {
			_, err := a.Children(ctx, root, adapters.NewOpCtx(opID))
			errc <- err
		}()
		return errc
	}
	wantCancelled := func(t *testing.T, errc <-chan error, what string) {
		t.Helper()
		select {
		case err := <-errc:
			if code, _ := adapters.CodeOf(err); code != adapters.CodeCancelled {
				t.Fatalf("%s: err = %v, want E_CANCELLED", what, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%s: still blocked after 3s", what)
		}
	}

	t.Run("a cancelled catalog query leaves the connection usable", func(t *testing.T) {
		a, proxy := setup(t)
		proxy.Pause()
		ctx, cancel := context.WithCancel(context.Background())
		errc := children(ctx, a, "lc-cancelled")
		time.Sleep(300 * time.Millisecond)
		cancel()
		wantCancelled(t, errc, "cancelled Children")
		proxy.Resume()
		if _, err := a.Children(context.Background(), root, adapters.NewOpCtx("lc-reuse")); err != nil {
			t.Fatalf("Children after a cancelled one: %v", err)
		}
	})

	t.Run("an op queued behind a long one is cancellable", func(t *testing.T) {
		a, proxy := setup(t)
		proxy.Pause()
		holder := children(context.Background(), a, "lc-holder")
		time.Sleep(300 * time.Millisecond)
		ctx, cancel := context.WithCancel(context.Background())
		queued := children(ctx, a, "lc-queued")
		time.Sleep(200 * time.Millisecond)
		cancel()
		wantCancelled(t, queued, "queued Children")
		proxy.Resume()
		if err := <-holder; err != nil {
			t.Fatalf("holder Children: %v", err)
		}
	})

	t.Run("disconnect is not held up by a hung op", func(t *testing.T) {
		a, proxy := setup(t)
		proxy.Pause()
		holder := children(context.Background(), a, "lc-hung")
		time.Sleep(300 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- a.Disconnect(ctx) }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Disconnect: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Disconnect still blocked 5s after its 1s deadline")
		}
		proxy.Resume()
		var ae *adapters.Error
		if err := <-holder; err != nil && !errors.As(err, &ae) {
			t.Fatalf("hung Children: unexpected error type %T", err)
		}
	})
}

// ConnectCancelScenario asserts that Connect honours its ctx: with the server's responses withheld
// by proxy, a cancelled ctx must end Connect promptly with an error, never block (Part 2's
// abortInFlight waits on it unbounded). cfg is the config that dials through proxy.
func ConnectCancelScenario(t *testing.T, a adapters.Adapter, cfg model.ResolvedConnectionConfig, proxy *PausableProxy) {
	t.Helper()
	proxy.Pause()
	t.Cleanup(func() { proxy.Resume(); _ = a.Disconnect(context.Background()) })
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := a.Connect(ctx, cfg, adapters.NewOpCtx("lc-connect-cancel"))
		errc <- err
	}()
	time.Sleep(500 * time.Millisecond)
	cancel()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("Connect on a cancelled ctx returned nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Connect still blocked 2s after its ctx was cancelled")
	}
}
