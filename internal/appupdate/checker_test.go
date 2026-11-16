package appupdate

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/releaseline"
)

const (
	testAsset   = "app.dmg"
	testVersion = "1.1.0"
)

func testReleases() []releaseline.Release {
	return []releaseline.Release{{
		Tag:    "v" + testVersion,
		Assets: []releaseline.Asset{{Name: testAsset, State: "uploaded", Size: 1}},
	}}
}

// TestChecker_Status_SingleFlight: N concurrent Status calls on a cold cache produce exactly one
// fetch — concurrent callers (two windows, or a renderer poll racing another window's boot) join
// the one in-flight request instead of issuing a second.
func TestChecker_Status_SingleFlight(t *testing.T) {
	var calls int32
	c := &Checker{
		running: "1.0.0",
		asset:   testAsset,
		now:     time.Now,
		fetch: func(ctx context.Context, userAgent string) ([]releaseline.Release, error) {
			atomic.AddInt32(&calls, 1)
			time.Sleep(20 * time.Millisecond) // widen the window so concurrent callers actually overlap
			return testReleases(), nil
		},
	}

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			c.Status(context.Background())
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("fetch called %d times across %d concurrent Status calls, want 1", got, n)
	}
}

// TestChecker_Status_CacheIntervals: a second call inside okInterval produces no fetch, a call
// after it produces one, and a call after a failed check produces one at failureInterval rather
// than okInterval.
func TestChecker_Status_CacheIntervals(t *testing.T) {
	var calls int32
	failing := false
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	c := &Checker{
		running: "1.0.0",
		asset:   testAsset,
		now:     func() time.Time { return clock },
		fetch: func(ctx context.Context, userAgent string) ([]releaseline.Release, error) {
			atomic.AddInt32(&calls, 1)
			if failing {
				return nil, errors.New("boom")
			}
			return testReleases(), nil
		},
	}

	want := func(t *testing.T, n int32) {
		t.Helper()
		if got := atomic.LoadInt32(&calls); got != n {
			t.Fatalf("fetch called %d times, want %d", got, n)
		}
	}

	c.Status(context.Background()) // cold cache: fetch #1, succeeds
	want(t, 1)

	clock = clock.Add(okInterval / 2)
	c.Status(context.Background()) // still within okInterval: cached
	want(t, 1)

	clock = clock.Add(okInterval)
	failing = true
	c.Status(context.Background()) // past okInterval: fetch #2, fails
	want(t, 2)

	clock = clock.Add(failureInterval / 2)
	c.Status(context.Background()) // within failureInterval of the failure: cached
	want(t, 2)

	clock = clock.Add(failureInterval)
	failing = false
	result := c.Status(context.Background()) // past failureInterval: fetch #3, succeeds
	want(t, 3)

	if !result.UpdateAvailable || result.TargetVersion != testVersion {
		t.Fatalf("Status() = %+v, want UpdateAvailable with TargetVersion %q", result, testVersion)
	}
}

// TestChecker_Check_BypassesCache: Check fetches inside okInterval, surfaces the fetch error Status
// hides (Status keeps the last good result), and a dev build never fetches.
func TestChecker_Check_BypassesCache(t *testing.T) {
	var calls int32
	failing := false
	c := &Checker{
		running: "1.0.0",
		asset:   testAsset,
		now:     time.Now,
		fetch: func(ctx context.Context, userAgent string) ([]releaseline.Release, error) {
			atomic.AddInt32(&calls, 1)
			if failing {
				return nil, errors.New("boom")
			}
			return testReleases(), nil
		},
	}

	c.Status(context.Background())
	if _, err := c.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("fetch called %d times, want 2 (Check must bypass the cache)", got)
	}

	failing = true
	if _, err := c.Check(context.Background()); err == nil {
		t.Fatal("Check() error = nil, want the fetch error")
	}
	if res := c.Status(context.Background()); !res.UpdateAvailable || res.TargetVersion != testVersion {
		t.Fatalf("Status() after a failed Check = %+v, want the last good result", res)
	}

	var devCalls int32
	dev := &Checker{running: "dev", asset: testAsset, now: time.Now, fetch: func(context.Context, string) ([]releaseline.Release, error) {
		atomic.AddInt32(&devCalls, 1)
		return testReleases(), nil
	}}
	if _, err := dev.Check(context.Background()); !errors.Is(err, ErrNotReleaseBuild) {
		t.Fatalf("dev Check() error = %v, want ErrNotReleaseBuild", err)
	}
	if devCalls != 0 {
		t.Fatalf("dev build fetched %d times, want 0", devCalls)
	}
}
