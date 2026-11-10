package jira

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

type fixedLocator struct{}

func (fixedLocator) Locate() (string, []string, bool) { return "/bin/acli", nil, true }

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// scriptRunner answers by subcommand and counts the workitem views. gate, when set, blocks views.
type scriptRunner struct {
	views atomic.Int32
	gate  chan struct{}
	view  func(args []string) ([]byte, error)
}

func (r *scriptRunner) Run(ctx context.Context, _ string, args []string) ([]byte, error) {
	switch {
	case args[0] == "--version":
		return []byte("acli version 1.3.4-stable\n"), nil
	case len(args) > 2 && args[2] == "status":
		return []byte("Authenticated\n  Site: acme.atlassian.net\n  Email: me@acme.com\n"), nil
	}
	r.views.Add(1)
	if r.gate != nil {
		select {
		case <-r.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return r.view(args)
}

func fixtureView(t *testing.T, file string) func([]string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	return func([]string) ([]byte, error) { return b, nil }
}

func newTestProvider(r Runner) (*Provider, *testClock) {
	clk := &testClock{t: time.Unix(1_700_000_000, 0)}
	return New(Options{Locator: fixedLocator{}, Runner: r, Clock: clk}), clk
}

var ref = issuetracker.Ref{Provider: ProviderID, Key: "KIRA-12"}

func TestIssueCoalescesConcurrentCallers(t *testing.T) {
	r := &scriptRunner{gate: make(chan struct{}), view: fixtureView(t, "issue-string-dev.json")}
	p, _ := newTestProvider(r)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Issue(context.Background(), ref, false); err != nil {
				t.Error(err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(r.gate)
	wg.Wait()
	if n := r.views.Load(); n != 1 {
		t.Fatalf("spawned %d views, want 1", n)
	}
}

func TestIssueDetachedFetchFillsCache(t *testing.T) {
	r := &scriptRunner{gate: make(chan struct{}), view: fixtureView(t, "issue-string-dev.json")}
	p, _ := newTestProvider(r)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := p.Issue(ctx, ref, false)
	if !errors.Is(err, issuetracker.ErrTimeout) {
		t.Fatalf("err %v, want ErrTimeout", err)
	}
	close(r.gate)
	deadline := time.Now().Add(2 * time.Second)
	for {
		iss, err := p.Issue(context.Background(), ref, false)
		if err == nil {
			if iss.Title != "Add the thing" || iss.Summary.Count != 3 {
				t.Fatalf("issue %+v", iss)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := r.views.Load(); n != 1 {
		t.Fatalf("views %d, want 1 (cache after detached fetch)", n)
	}
}

func TestIssueErrorTTL(t *testing.T) {
	var calls atomic.Int32
	r := &scriptRunner{view: func([]string) ([]byte, error) {
		calls.Add(1)
		return nil, &toolexec.ExecError{ExitCode: 1, Stderr: "issue does not exist or you do not have permission"}
	}}
	p, clk := newTestProvider(r)
	for range 2 {
		if _, err := p.Issue(context.Background(), ref, false); !errors.Is(err, issuetracker.ErrNotFound) {
			t.Fatalf("err %v, want ErrNotFound", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("error not cached: %d calls", calls.Load())
	}
	clk.advance(resultErrTTL + time.Second)
	_, _ = p.Issue(context.Background(), ref, false)
	if calls.Load() != 2 {
		t.Fatalf("error cached past its TTL: %d calls", calls.Load())
	}
}

func TestIssueDevFieldLearnedOnce(t *testing.T) {
	var argv [][]string
	var mu sync.Mutex
	b := fixtureView(t, "issue-string-dev.json")
	r := &scriptRunner{view: func(a []string) ([]byte, error) {
		mu.Lock()
		argv = append(argv, a)
		mu.Unlock()
		return b(a)
	}}
	p, clk := newTestProvider(r)
	if _, err := p.Issue(context.Background(), ref, false); err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * resultOKTTL)
	iss, err := p.Issue(context.Background(), ref, true)
	if err != nil || iss.Summary.Count != 3 {
		t.Fatalf("second read %+v %v", iss, err)
	}
	if got := argv[0][len(argv[0])-1]; got != firstFields {
		t.Errorf("first fields %q", got)
	}
	if got := argv[1][len(argv[1])-1]; got != "summary,status,customfield_10000" {
		t.Errorf("later fields %q", got)
	}
}

func TestIssueOversizeFirstReadFallsBack(t *testing.T) {
	b := fixtureView(t, "issue-no-dev.json")
	r := &scriptRunner{view: func(a []string) ([]byte, error) {
		if a[len(a)-1] == firstFields {
			return nil, toolexec.ErrOutputTooLarge
		}
		return b(a)
	}}
	p, _ := newTestProvider(r)
	iss, err := p.Issue(context.Background(), issuetracker.Ref{Provider: ProviderID, Key: "KIRA-14"}, false)
	if err != nil || iss.Title != "No development info" || iss.Summary.Known {
		t.Fatalf("%+v %v", iss, err)
	}
}

// waitViews polls until the runner spawned want views, the background refresh being asynchronous.
func waitViews(t *testing.T, r *scriptRunner, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for r.views.Load() != want {
		if time.Now().After(deadline) {
			t.Fatalf("views %d, want %d", r.views.Load(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestIssueCacheRules(t *testing.T) {
	good := fixtureView(t, "issue-string-dev.json")
	var failing atomic.Bool
	r := &scriptRunner{view: func(a []string) ([]byte, error) {
		if failing.Load() {
			return nil, &toolexec.ExecError{ExitCode: 1, Stderr: "something went wrong"}
		}
		return good(a)
	}}
	p, clk := newTestProvider(r)
	ctx := context.Background()
	read := func(fresh bool) (issuetracker.Issue, error) { return p.Issue(ctx, ref, fresh) }

	if iss, err := read(false); err != nil || iss.Stale {
		t.Fatalf("first read %+v %v", iss, err)
	}
	// fresh window: no spawn
	clk.advance(resultOKTTL - time.Second)
	if iss, _ := read(false); iss.Stale || r.views.Load() != 1 {
		t.Fatalf("inside window: stale %v views %d", iss.Stale, r.views.Load())
	}
	// past the window: stale copy at once, exactly one background view
	clk.advance(2 * time.Second)
	if iss, err := read(false); err != nil || !iss.Stale || iss.Title != "Add the thing" {
		t.Fatalf("stale serve %+v %v", iss, err)
	}
	waitViews(t, r, 2)
	if iss, _ := read(false); iss.Stale || r.views.Load() != 2 {
		t.Fatalf("after revalidation: stale %v views %d", iss.Stale, r.views.Load())
	}
	// fresh failure returns copy and error; the cooldown then keeps serving the copy without a spawn
	failing.Store(true)
	clk.advance(2 * resultOKTTL)
	iss, err := read(true)
	if err == nil || !iss.Stale || iss.Title != "Add the thing" {
		t.Fatalf("fresh failure %+v %v", iss, err)
	}
	if iss, err := read(false); err != nil || !iss.Stale || r.views.Load() != 3 {
		t.Fatalf("error cooldown %+v %v views %d", iss, err, r.views.Load())
	}
	// cooldown over: revalidates again
	clk.advance(resultErrTTL + time.Second)
	if _, err := read(false); err != nil {
		t.Fatal(err)
	}
	waitViews(t, r, 4)
	// keep expiry blocks: a failing read now returns the bare error
	clk.advance(issueKeepFor + time.Minute)
	if iss, err := read(false); err == nil || iss.Title != "" {
		t.Fatalf("past keep %+v %v", iss, err)
	}
	// a saved detail refreshes the copy
	failing.Store(false)
	clk.advance(resultErrTTL + time.Second)
	key := cacheKey(ref)
	p.client.saveDetail(key, issuetracker.IssueDetail{Ref: ref, Title: "From detail", Status: "Done"}, p.client.detailGeneration(key))
	n := r.views.Load()
	if iss, err := read(false); err != nil || iss.Stale || iss.Title != "From detail" || r.views.Load() != n {
		t.Fatalf("after saveDetail %+v %v", iss, err)
	}
}
