package prompts_test

import (
	"sort"
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/internal/desknotify"
	"github.com/kirathecat/kira-studio/internal/prompts"
)

type fakeWindows struct {
	mu      sync.Mutex
	orders  map[string]int
	eph     map[string]bool
	focused string
	focus   []string
}

func newWindows() *fakeWindows {
	return &fakeWindows{orders: map[string]int{}, eph: map[string]bool{}}
}

func (w *fakeWindows) add(key string, order int) { w.mu.Lock(); w.orders[key] = order; w.mu.Unlock() }
func (w *fakeWindows) drop(key string)           { w.mu.Lock(); delete(w.orders, key); w.mu.Unlock() }

func (w *fakeWindows) MainKey() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	keys := make([]string, 0, len(w.orders))
	for k := range w.orders {
		if !w.eph[k] {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		return w.orders[a] < w.orders[b] || (w.orders[a] == w.orders[b] && a < b)
	})
	if len(keys) == 0 {
		return "", false
	}
	return keys[0], true
}

func (w *fakeWindows) Has(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.orders[key]
	return ok
}

func (w *fakeWindows) Focused(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return key != "" && w.focused == key
}

func (w *fakeWindows) Focus(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.focus = append(w.focus, key)
	return true
}

type recSink struct {
	mu      sync.Mutex
	sent    []desknotify.Note
	removed []string
}

func (s *recSink) Send(n desknotify.Note) error {
	s.mu.Lock()
	s.sent = append(s.sent, n)
	s.mu.Unlock()
	return nil
}

func (s *recSink) Remove(id string) error {
	s.mu.Lock()
	s.removed = append(s.removed, id)
	s.mu.Unlock()
	return nil
}

type rig struct {
	w       *fakeWindows
	r       *prompts.Router
	sink    *recSink
	enabled bool
	reveals []string
	reopens int
	last    []prompts.Routed
}

func newRig(t *testing.T) *rig {
	t.Helper()
	g := &rig{w: newWindows(), sink: &recSink{}, enabled: true}
	var mu sync.Mutex
	g.r = prompts.New(prompts.Deps{
		Windows: g.w, App: "Studio",
		Emit:    func(l []prompts.Routed) { mu.Lock(); g.last = l; mu.Unlock() },
		Reveal:  func(key, id string) { g.reveals = append(g.reveals, key+" "+id) },
		Reopen:  func() { g.reopens++; g.w.add("reopened", 0); g.r.Reroute() },
		Enabled: func() bool { return g.enabled },
	})
	g.r.SetSink(g.sink)
	return g
}

func (g *rig) open(kind prompts.Kind, ref, origin string) {
	g.r.Open(prompts.Prompt{ID: prompts.ID(kind, ref), Kind: kind, Ref: ref, Origin: origin, Title: "T " + ref})
}

func targetOf(g *rig, id string) string {
	for _, p := range g.r.List() {
		if p.ID == id {
			return p.Target
		}
	}
	return "?"
}

func TestTargetRouting(t *testing.T) {
	g := newRig(t)
	g.open(prompts.KindSchedule, "q", "")
	if got := targetOf(g, "schedule:q"); got != "" {
		t.Fatalf("no windows: target %q, want queued", got)
	}
	g.w.add("a", 1)
	g.w.add("z", 0)
	g.w.eph["eph"] = true
	g.w.add("eph", -5)
	g.r.Reroute()
	if got := targetOf(g, "schedule:q"); got != "z" {
		t.Fatalf("target %q, want lowest order z (not smallest key, never ephemeral)", got)
	}
	g.open(prompts.KindApproval, "o", "a")
	if got := targetOf(g, "approval:o"); got != "a" {
		t.Fatalf("origin alive: target %q, want a", got)
	}
	g.open(prompts.KindApproval, "e", "eph")
	if got := targetOf(g, "approval:e"); got != "eph" {
		t.Fatalf("ephemeral origin: target %q, want eph", got)
	}
	g.w.drop("a")
	g.r.Reroute()
	if got := targetOf(g, "approval:o"); got != "z" {
		t.Fatalf("origin gone: target %q, want main z", got)
	}
	if err := g.r.Claim("schedule:q", "a"); err != nil {
		t.Fatal(err)
	}
	if got := targetOf(g, "schedule:q"); got != "z" {
		t.Fatalf("claim of closed window: target %q, want main z", got)
	}
	g.w.add("a", 1)
	g.r.Reroute()
	if got := targetOf(g, "schedule:q"); got != "a" {
		t.Fatalf("claim beats main: target %q, want a", got)
	}
	g.w.drop("z")
	g.r.Reroute()
	g.r.Close("schedule:q")
	if got := targetOf(g, "approval:o"); got != "a" {
		t.Fatalf("main closed: target %q, want next order a", got)
	}
	if err := g.r.Claim("nope", "a"); err == nil {
		t.Fatal("claim of unknown id succeeded")
	}
}

func TestNotificationsCoalescePerKind(t *testing.T) {
	g := newRig(t)
	g.w.add("a", 0)
	g.open(prompts.KindApproval, "1", "")
	g.open(prompts.KindApproval, "1", "")
	g.open(prompts.KindApproval, "2", "")
	g.open(prompts.KindSchedule, "s", "")
	if len(g.sink.sent) != 3 {
		t.Fatalf("sent %d notes, want 3 (one dbmcp, its count update, one schedule)", len(g.sink.sent))
	}
	if g.sink.sent[0].ID != "prompt:approval" || g.sink.sent[1].ID != "prompt:approval" || g.sink.sent[1].Title != "2 things await your approval" {
		t.Fatalf("notes %+v", g.sink.sent)
	}
	g.r.Close("approval:1")
	if n := g.sink.sent[len(g.sink.sent)-1]; n.Title != "T 2" {
		t.Fatalf("one left: title %q, want the single title", n.Title)
	}
	g.r.Close("approval:2")
	if len(g.sink.removed) != 1 || g.sink.removed[0] != "prompt:approval" {
		t.Fatalf("removed %v, want prompt:approval", g.sink.removed)
	}
}

func TestNotificationSuppression(t *testing.T) {
	g := newRig(t)
	g.w.add("a", 0)
	g.w.focused = "a"
	g.open(prompts.KindApproval, "1", "")
	if len(g.sink.sent) != 0 {
		t.Fatal("note posted for a focused target")
	}
	g.r.Close("approval:1")
	if len(g.sink.removed) != 0 {
		t.Fatal("removed a note that was never shown")
	}
	g.w.focused = ""
	g.enabled = false
	g.open(prompts.KindApproval, "2", "")
	if len(g.sink.sent) != 0 {
		t.Fatal("note posted with notifyPrompts off")
	}
	g.r.SendTest()
	if len(g.sink.sent) != 0 {
		t.Fatal("test note posted with notifyPrompts off")
	}
	g.enabled = true
	g.w.focused = "a"
	g.r.SendTest()
	if len(g.sink.sent) != 1 {
		t.Fatal("test note must ignore focus")
	}
}

func TestRevealKind(t *testing.T) {
	g := newRig(t)
	g.open(prompts.KindSchedule, "old", "")
	g.r.Open(prompts.Prompt{ID: "schedule:new", Kind: prompts.KindSchedule, Ref: "new", CreatedAt: 1 << 60})
	g.r.RevealKind(prompts.KindSchedule)
	if g.reopens != 1 || len(g.reveals) != 1 || g.reveals[0] != "reopened schedule:old" {
		t.Fatalf("reopens %d reveals %v, want reopen then reveal of the oldest", g.reopens, g.reveals)
	}
	if len(g.w.focus) != 1 || g.w.focus[0] != "reopened" {
		t.Fatalf("focus %v", g.w.focus)
	}
	g.r.RevealKind(prompts.KindApproval)
	if len(g.reveals) != 1 {
		t.Fatal("revealed a kind with no prompt")
	}
}

func TestConcurrentOpenCloseReroute(t *testing.T) {
	g := newRig(t)
	g.w.add("a", 0)
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ref := string(rune('A' + i%8))
			g.open(prompts.KindApproval, ref, "")
			g.r.Reroute()
			if i%2 == 0 {
				g.r.Close(prompts.ID(prompts.KindApproval, ref))
			}
		}()
	}
	wg.Wait()
	g.r.Reroute()
	want := g.r.List()
	if len(g.last) != len(want) {
		t.Fatalf("last emit has %d entries, list has %d", len(g.last), len(want))
	}
}
