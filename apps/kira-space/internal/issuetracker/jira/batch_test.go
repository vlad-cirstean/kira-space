package jira

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

// batchRunner answers views with the dev-field fixture and searches through search.
type batchRunner struct {
	views  atomic.Int32
	mu     sync.Mutex
	limits []string
	search func(args []string) ([]byte, error)
}

func (r *batchRunner) Run(_ context.Context, _ string, args []string) ([]byte, error) {
	switch {
	case args[0] == "--version":
		return []byte("acli version 1.3.4-stable\n"), nil
	case len(args) > 2 && args[2] == "status":
		return []byte("Authenticated\n  Site: acme.atlassian.net\n  Email: me@acme.com\n"), nil
	case args[2] == "search":
		r.mu.Lock()
		r.limits = append(r.limits, args[len(args)-1])
		r.mu.Unlock()
		return r.search(args)
	}
	r.views.Add(1)
	b, err := os.ReadFile(filepath.Join("testdata", "issue-string-dev.json"))
	return b, err
}

func (r *batchRunner) searches() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.limits) }

var jqlKeyRe = regexp.MustCompile(`[A-Z]+-\d+`)

// echoSearch returns one minimal issue per key in the JQL.
func echoSearch(args []string) ([]byte, error) {
	var items []string
	for _, k := range jqlKeyRe.FindAllString(args[4], -1) {
		items = append(items, fmt.Sprintf(`{"key":%q,"fields":{"summary":"T %s","status":{"name":"Open","statusCategory":{"key":"new"}},"updated":"2026-10-01T10:00:00.000+0000"}}`, k, k))
	}
	return []byte("[" + strings.Join(items, ",") + "]"), nil
}

func refsOf(keys ...string) []issuetracker.Ref {
	out := make([]issuetracker.Ref, len(keys))
	for i, k := range keys {
		out[i] = issuetracker.Ref{Provider: ProviderID, Key: k}
	}
	return out
}

func TestIssuesBatch(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "search.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	t.Run("chunks of 50 feed the Issue cache", func(t *testing.T) {
		r := &batchRunner{search: echoSearch}
		p, _ := newTestProvider(r)
		keys := make([]string, 120)
		for i := range keys {
			keys[i] = fmt.Sprintf("KIRA-%d", i+1)
		}
		out, err := p.Issues(ctx, refsOf(keys...))
		if err != nil || len(out) != 120 {
			t.Fatalf("%d issues, err %v", len(out), err)
		}
		if got := strings.Join(r.limits, ","); got != "50,50,20" {
			t.Errorf("limits %s", got)
		}
		before := r.views.Load()
		iss, err := p.Issue(ctx, refsOf("KIRA-77")[0], false)
		if err != nil || iss.Title != "T KIRA-77" || iss.Stale || r.views.Load() != before {
			t.Fatalf("%+v %v views %d->%d", iss, err, before, r.views.Load())
		}
	})

	t.Run("array and object shapes parse", func(t *testing.T) {
		for name, body := range map[string]string{"array": string(fixture), "object": `{"issues":` + string(fixture) + `}`} {
			r := &batchRunner{search: func([]string) ([]byte, error) { return []byte(body), nil }}
			p, _ := newTestProvider(r)
			out, err := p.Issues(ctx, refsOf("KIRA-12", "KIRA-14", "KIRA-99"))
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			a := out["KIRA-12"]
			if a.Summary.Count != 3 || a.Priority != "High" || a.Assignee != "acc-42" || a.UpdatedAt != time.Date(2026, 10, 2, 9, 30, 0, 0, time.UTC).UnixMilli() {
				t.Errorf("%s: %+v", name, a)
			}
			if b := out["KIRA-14"]; b.Priority != "" || b.Assignee != "" || b.Summary.Known {
				t.Errorf("%s: %+v", name, b)
			}
			if _, ok := out["KIRA-99"]; ok {
				t.Errorf("%s: key the output lacks is present", name)
			}
		}
	})

	t.Run("not found falls back per key", func(t *testing.T) {
		r := &batchRunner{search: func([]string) ([]byte, error) {
			return nil, &toolexec.ExecError{ExitCode: 1, Stderr: "An issue with key 'KIRA-99' does not exist for field 'key'."}
		}}
		p, _ := newTestProvider(r)
		out, err := p.Issues(ctx, refsOf("KIRA-12", "KIRA-13"))
		if err != nil || len(out) != 2 {
			t.Fatalf("%v %v", out, err)
		}
		// discovery + one view per key
		if r.views.Load() != 3 {
			t.Errorf("views %d, want 3", r.views.Load())
		}
		// a not-found search is not a refused search: no backoff
		if _, err := p.Issues(ctx, refsOf("KIRA-12")); err != nil || r.searches() != 2 {
			t.Errorf("searches %d, err %v", r.searches(), err)
		}
	})

	t.Run("other failure backs off", func(t *testing.T) {
		r := &batchRunner{search: func([]string) ([]byte, error) {
			return nil, &toolexec.ExecError{ExitCode: 1, Stderr: "unknown flag: --jql"}
		}}
		p, clk := newTestProvider(r)
		out, err := p.Issues(ctx, refsOf("KIRA-12", "KIRA-13"))
		if err != nil || len(out) != 2 {
			t.Fatalf("%v %v", out, err)
		}
		if _, err := p.Issues(ctx, refsOf("KIRA-12")); err != nil || r.searches() != 1 {
			t.Fatalf("inside backoff: searches %d, err %v", r.searches(), err)
		}
		clk.advance(noDevTTL + time.Second)
		if _, err := p.Issues(ctx, refsOf("KIRA-12")); err != nil || r.searches() != 2 {
			t.Fatalf("after backoff: searches %d, err %v", r.searches(), err)
		}
	})

	t.Run("unauthenticated is returned", func(t *testing.T) {
		r := &batchRunner{search: func([]string) ([]byte, error) {
			return nil, &toolexec.ExecError{ExitCode: 1, Stderr: "401 unauthorized"}
		}}
		p, _ := newTestProvider(r)
		if _, err := p.Issues(ctx, refsOf("KIRA-12")); !errors.Is(err, issuetracker.ErrUnauthenticated) {
			t.Fatalf("err %v", err)
		}
	})
}
