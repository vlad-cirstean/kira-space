package dbmcp

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type stubConns struct{ ConnectionsReader }
type stubTree struct{ MetadataReader }
type stubQuery struct{ QueryRunner }
type stubMasks struct{ MaskRules }
type stubReview struct{ ColumnReview }

func newTestPool(t *testing.T) *SessionPool {
	home := t.TempDir()
	b := NewApprovalBroker(time.Now, nil)
	return NewSessionPool(b, func(*SessionTokens) Config {
		return Config{
			Home: home, Conns: stubConns{}, Tree: stubTree{}, Query: stubQuery{}, Approvals: b,
			MaskRules: stubMasks{}, ColumnReview: stubReview{},
			ExplainThreshold: func() int { return 1 }, PreviewResponses: func() bool { return false },
		}
	})
}

func status(t *testing.T, url, bearer string) int {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	_ = res.Body.Close()
	return res.StatusCode
}

func TestSessionPoolLifecycle(t *testing.T) {
	p := newTestPool(t)
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				entry, release, err := p.Grant(SessionIdentity{SessionID: "s" + string(rune('a'+i%26))})
				if err != nil {
					t.Errorf("grant: %v", err)
					return
				}
				if entry.URL == "" || entry.Headers["Authorization"] == "" {
					t.Errorf("empty entry: %+v", entry)
				}
				release()
				release()
			}
		}()
	}
	wg.Wait()
	if p.Running() || p.tokens.Len() != 0 {
		t.Fatalf("after all releases: running=%v tokens=%d", p.Running(), p.tokens.Len())
	}

	first, releaseFirst, err := p.Grant(SessionIdentity{SessionID: "one"})
	if err != nil {
		t.Fatal(err)
	}
	bearer := strings.TrimPrefix(first.Headers["Authorization"], "Bearer ")
	if got := status(t, first.URL, bearer); got == http.StatusUnauthorized {
		t.Fatalf("live token refused: %d", got)
	}
	if got := status(t, first.URL, "wrong"); got != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d, want 401", got)
	}
	second, releaseSecond, err := p.Grant(SessionIdentity{SessionID: "two"})
	if err != nil {
		t.Fatal(err)
	}
	releaseFirst()
	if !p.Running() {
		t.Fatal("server closed while a grant is left")
	}
	if got := status(t, second.URL, bearer); got != http.StatusUnauthorized {
		t.Fatalf("released token: %d, want 401", got)
	}
	releaseSecond()
	if p.Running() {
		t.Fatal("server still running after the last release")
	}
	third, releaseThird, err := p.Grant(SessionIdentity{SessionID: "three"})
	if err != nil {
		t.Fatal(err)
	}
	defer releaseThird()
	if got := status(t, third.URL, bearer); got != http.StatusUnauthorized {
		t.Fatalf("old token on fresh server: %d, want 401", got)
	}
	p.Close()
	if _, _, err := p.Grant(SessionIdentity{SessionID: "late"}); err == nil {
		t.Fatal("grant after Close succeeded")
	}
}

func TestNewRefusesBadSessionConfig(t *testing.T) {
	p := newTestPool(t)
	cfg := p.build(&p.tokens)
	cfg.Sessions = &p.tokens
	cfg.Port = DefaultPort
	if _, err := New(cfg); err == nil {
		t.Fatal("session server on the default port accepted")
	}
}
