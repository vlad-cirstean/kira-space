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
			if _, err := p.Issue(context.Background(), ref); err != nil {
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
	_, err := p.Issue(ctx, ref)
	if !errors.Is(err, issuetracker.ErrTimeout) {
		t.Fatalf("err %v, want ErrTimeout", err)
	}
	close(r.gate)
	deadline := time.Now().Add(2 * time.Second)
	for {
		iss, err := p.Issue(context.Background(), ref)
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
		if _, err := p.Issue(context.Background(), ref); !errors.Is(err, issuetracker.ErrNotFound) {
			t.Fatalf("err %v, want ErrNotFound", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("error not cached: %d calls", calls.Load())
	}
	clk.advance(resultErrTTL + time.Second)
	_, _ = p.Issue(context.Background(), ref)
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
	if _, err := p.Issue(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * resultOKTTL)
	iss, err := p.Issue(context.Background(), ref)
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
	iss, err := p.Issue(context.Background(), issuetracker.Ref{Provider: ProviderID, Key: "KIRA-14"})
	if err != nil || iss.Title != "No development info" || iss.Summary.Known {
		t.Fatalf("%+v %v", iss, err)
	}
}
