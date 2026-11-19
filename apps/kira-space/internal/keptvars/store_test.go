package keptvars

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newClockStore(t *testing.T, onChange func(uint64)) (*Store, *clock) {
	t.Helper()
	c := &clock{t: time.Unix(1_700_000_000, 0)}
	s := New(Options{Now: c.now, OnChange: onChange})
	t.Cleanup(s.Close)
	return s, c
}

func put(t *testing.T, s *Store, scope, name, value string, kind SourceKind) Meta {
	t.Helper()
	m, err := s.Put(Put{Scope: scope, Name: name, Value: json.RawMessage(value), Source: Source{Kind: kind, MCP: "api"}})
	if err != nil {
		t.Fatalf("Put %s: %v", name, err)
	}
	return m
}

func TestGetRefusesAtExactExpiry(t *testing.T) {
	s, c := newClockStore(t, nil)
	m := put(t, s, "external", "a", `[1]`, SourceScript)
	c.advance(DefaultTTL - time.Nanosecond)
	if _, ok := s.Get("external", "a"); !ok {
		t.Fatal("entry gone before its expiry")
	}
	c.advance(time.Nanosecond)
	if _, ok := s.Get("external", "a"); ok {
		t.Fatal("Get returned an entry at now == ExpiresAt")
	}
	if _, ok := s.ByID(m.ID); ok {
		t.Fatal("ByID returned an expired entry")
	}
	if len(s.List("")) != 0 {
		t.Fatal("List returned an expired entry")
	}
}

func TestSweepBatchOneRevisionWatchersOnce(t *testing.T) {
	var revs []uint64
	var mu sync.Mutex
	s, c := newClockStore(t, func(r uint64) { mu.Lock(); revs = append(revs, r); mu.Unlock() })
	a := put(t, s, "external", "a", `1`, SourceScript)
	b := put(t, s, "external", "b", `2`, SourceScript)
	var fired atomic.Int32
	s.OnRemove([]string{a.ID, b.ID}, func(r Removal) {
		if r.Reason != ReasonExpired {
			t.Errorf("reason = %s", r.Reason)
		}
		fired.Add(1)
	})
	mu.Lock()
	before := len(revs)
	mu.Unlock()
	c.advance(DefaultTTL)
	s.sweep()
	mu.Lock()
	got := len(revs) - before
	mu.Unlock()
	if got != 1 || fired.Load() != 1 {
		t.Fatalf("sweep gave %d revisions and %d watcher fires, want 1 and 1", got, fired.Load())
	}
}

func TestEarlierExpiryResetsTimer(t *testing.T) {
	s := New(Options{})
	defer s.Close()
	if _, err := s.Put(Put{Scope: "external", Name: "long", Value: json.RawMessage(`1`), TTL: time.Hour}); err != nil {
		t.Fatal(err)
	}
	short, err := s.Put(Put{Scope: "external", Name: "short", Value: json.RawMessage(`1`), TTL: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan Removal, 1)
	s.OnRemove([]string{short.ID}, func(r Removal) { done <- r })
	select {
	case r := <-done:
		if r.Reason != ReasonExpired {
			t.Fatalf("reason = %s", r.Reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the sweeper did not wake for the earlier expiry")
	}
	if _, ok := s.Get("external", "long"); !ok {
		t.Fatal("the long entry went with the short one")
	}
}

func TestReplaceNewIDFiresReplacedFreesSize(t *testing.T) {
	s, _ := newClockStore(t, nil)
	old := put(t, s, "external", "a", `1`, SourceScript)
	reasons := make(chan Reason, 1)
	s.OnRemove([]string{old.ID}, func(r Removal) { reasons <- r.Reason })
	next := put(t, s, "external", "a", `22`, SourceScript)
	if next.ID == old.ID || <-reasons != ReasonReplaced {
		t.Fatal("replace must give a new id and fire replaced")
	}
	if s.bytes != 2 || len(s.List("")) != 1 {
		t.Fatalf("bytes = %d, entries = %d, want the old size freed", s.bytes, len(s.List("")))
	}
	// A full scope still accepts a replacement: the old entry's slot is freed first.
	for i := 0; i < MaxScopeEntries-1; i++ {
		put(t, s, "external", "n"+strconv.Itoa(i), `1`, SourceScript)
	}
	put(t, s, "external", "a", `3`, SourceScript)
}

func TestCaps(t *testing.T) {
	s, c := newClockStore(t, nil)
	big := `"` + strings.Repeat("x", MaxValueBytes) + `"`
	if _, err := s.Put(Put{Scope: "external", Name: "big", Value: json.RawMessage(big)}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("big value: %v", err)
	}
	deep := strings.Repeat("[", 65) + strings.Repeat("]", 65)
	if _, err := s.Put(Put{Scope: "external", Name: "deep", Value: json.RawMessage(deep)}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("deep value: %v", err)
	}
	for i := 0; i < MaxScopeEntries; i++ {
		put(t, s, "external", "n"+strconv.Itoa(i), `1`, SourceScript)
	}
	if _, err := s.Put(Put{Scope: "external", Name: "extra", Value: json.RawMessage(`1`)}); !errors.Is(err, ErrScopeFull) {
		t.Fatalf("scope cap: %v", err)
	}
	// Expired entries are swept before the cap check.
	c.advance(DefaultTTL)
	put(t, s, "external", "extra", `1`, SourceScript)

	for i := 0; i < MaxStoreEntries/MaxScopeEntries; i++ {
		for j := 0; j < MaxScopeEntries; j++ {
			if _, err := s.Put(Put{Scope: fmt.Sprintf("session:%d", i), Name: "n" + strconv.Itoa(j), Value: json.RawMessage(`1`)}); err != nil {
				break
			}
		}
	}
	if _, err := s.Put(Put{Scope: "session:zz", Name: "x", Value: json.RawMessage(`1`)}); !errors.Is(err, ErrStoreFull) {
		t.Fatalf("store entry cap: %v", err)
	}
	s.ClearAll()
	chunk := json.RawMessage(`"` + strings.Repeat("y", MaxValueBytes-10) + `"`)
	n := 0
	for ; n < 40; n++ {
		if _, err := s.Put(Put{Scope: fmt.Sprintf("session:%d", n/MaxScopeEntries), Name: "c" + strconv.Itoa(n), Value: chunk}); err != nil {
			if !errors.Is(err, ErrStoreFull) {
				t.Fatalf("store bytes cap: %v", err)
			}
			break
		}
	}
	if n < 28 || n >= 40 {
		t.Fatalf("stored %d large entries, want the 32 MiB cap to stop it near 32", n)
	}
}

func TestNameTakenAcrossSourceKinds(t *testing.T) {
	s, _ := newClockStore(t, nil)
	put(t, s, "external", "tok", `"x"`, SourceUser)
	if _, err := s.Put(Put{Scope: "external", Name: "tok", Value: json.RawMessage(`"y"`), Source: Source{Kind: SourceScript}}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("script over user value: %v", err)
	}
	if _, err := s.Put(Put{Scope: "session:1", Name: "tok", Value: json.RawMessage(`"y"`), Source: Source{Kind: SourceScript}}); err != nil {
		t.Fatalf("another scope: %v", err)
	}
}

func TestOnRemoveAbsentStopAndReentry(t *testing.T) {
	s, _ := newClockStore(t, nil)
	fired := make(chan Removal, 2)
	s.OnRemove([]string{"kv_missing"}, func(r Removal) { fired <- r })
	if r := <-fired; r.Reason != ReasonExpired {
		t.Fatalf("absent id reason = %s", r.Reason)
	}
	m := put(t, s, "external", "a", `1`, SourceScript)
	stop := s.OnRemove([]string{m.ID}, func(Removal) { t.Error("stopped watcher fired") })
	stop()
	// A callback may call back into the store without deadlock.
	m2 := put(t, s, "external", "b", `1`, SourceScript)
	s.OnRemove([]string{m2.ID}, func(Removal) { s.List(""); s.Clear("none"); fired <- Removal{} })
	s.Clear(m.ID, m2.ID)
	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("re-entrant callback deadlocked")
	}
}

func TestClearZeroesStoredBytesNotCopies(t *testing.T) {
	s, _ := newClockStore(t, nil)
	m := put(t, s, "external", "a", `"secret-value"`, SourceScript)
	s.mu.Lock()
	raw := s.byID[m.ID].value
	s.mu.Unlock()
	copyOf, _ := s.Get("external", "a")
	s.Clear(m.ID)
	if strings.Contains(string(raw), "secret") {
		t.Fatal("stored bytes survive a clear")
	}
	if string(copyOf.Value) != `"secret-value"` {
		t.Fatal("an earlier Get copy was zeroed")
	}
}

func TestCloseFiresQuitAndStopsSweeper(t *testing.T) {
	s := New(Options{})
	m, _ := s.Put(Put{Scope: "external", Name: "a", Value: json.RawMessage(`1`)})
	got := make(chan Reason, 1)
	s.OnRemove([]string{m.ID}, func(r Removal) { got <- r.Reason })
	s.Close()
	s.Close()
	if r := <-got; r != ReasonQuit {
		t.Fatalf("reason = %s", r)
	}
	select {
	case <-s.done:
	default:
		t.Fatal("sweeper still running")
	}
	if _, err := s.Put(Put{Scope: "external", Name: "b", Value: json.RawMessage(`1`)}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Put after Close: %v", err)
	}
}

func TestConcurrentInvariants(t *testing.T) {
	var changes atomic.Uint64
	s := New(Options{OnChange: func(uint64) { changes.Add(1) }})
	defer s.Close()
	var fires sync.Map
	var wg sync.WaitGroup
	for g := 0; g < 64; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scope := "session:" + strconv.Itoa(g%4)
			for i := 0; i < 50; i++ {
				name := "n" + strconv.Itoa(i%8)
				m, err := s.Put(Put{Scope: scope, Name: name, Value: json.RawMessage(strconv.Itoa(i)), TTL: 5 * time.Millisecond})
				if err != nil {
					continue
				}
				key := fmt.Sprintf("%d-%d", g, i)
				s.OnRemove([]string{m.ID}, func(Removal) {
					if _, dup := fires.LoadOrStore(key, true); dup {
						t.Errorf("watcher %s fired twice", key)
					}
				})
				s.Get(scope, name)
				s.ByID(m.ID)
				if i%7 == 0 {
					s.Clear(m.ID)
				}
			}
		}()
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for _, it := range s.byID {
		total += it.meta.Bytes
	}
	if total != s.bytes {
		t.Fatalf("bytes = %d, live entries sum to %d", s.bytes, total)
	}
	count := 0
	for _, m := range s.byScope {
		count += len(m)
	}
	if count != len(s.byID) || len(s.expiry) != len(s.byID) {
		t.Fatalf("index mismatch: scopes %d, ids %d, heap %d", count, len(s.byID), len(s.expiry))
	}
}
