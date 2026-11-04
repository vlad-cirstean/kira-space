package agentsandbox

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// NotEnforcedKey is the catalog key shown when the OS cannot enforce a profile.
const NotEnforcedKey = "backend.space.ade.sandbox.notEnforced.body"

// NotEnforcedText is the not-enforced notice in the user's language.
func NotEnforcedText(a Namer) string {
	return i18n.T("backend.space.ade.sandbox.notEnforced.body", map[string]string{"agent": a.Label()})
}

// NotEnforcedScriptKey is the catalog key shown when the OS cannot enforce a script's profile.
const NotEnforcedScriptKey = "backend.space.ade.sandbox.notEnforcedScript.body"

// NotEnforcedScriptText is the not-enforced notice of a script step in the user's language.
func NotEnforcedScriptText() string {
	return i18n.T(NotEnforcedScriptKey, nil)
}

// Namer names the agent in sandbox texts; a harness provider satisfies it.
type Namer interface {
	Label() string
}

// Support says whether profiles are enforced on this OS.
type Support struct {
	Enforced bool
	// Reason is a catalog key, set when not enforced.
	Reason string
}

// Prepared is a run's sandbox, ready to apply to a harness.HeadlessSpec.
type Prepared struct {
	Profile, ProfilePath string
	// Exec is nil when the OS does not enforce the profile.
	Exec *harness.SandboxExec
	// Env are overrides for the child environment, on every OS.
	Env []string
	// AddDirs and Disallowed are the agent permission flags (second layer).
	AddDirs, Disallowed []string
}

// Enforcer applies a policy to a launch. Production uses System; flow tests use sandboxtest.Fake.
type Enforcer interface {
	Support() Support
	// Prepare renders the profile, writes it and, where enforced, checks it compiles.
	Prepare(p Policy, rt Runtime) (Prepared, error)
	// Watch reports denials for the process tree rooted at pid until stop is called.
	Watch(pid int, runID string, on func(Denial)) (stop func())
	// Release drops what Prepare registered for runID. It is idempotent and covers a run that
	// never reached Watch.
	Release(runID string)
}

// System is the real enforcer: Seatbelt on darwin, declared-only elsewhere.
func System() Enforcer { return newSystem(runtime.GOOS) }

type system struct {
	goos  string
	hub   *hub
	proxy proxyOnce
	watch *watcher
}

type proxyOnce struct {
	once sync.Once
	p    atomic.Pointer[Proxy]
	err  error
}

func newSystem(goos string) *system {
	h := newHub()
	s := &system{goos: goos, hub: h}
	s.watch = newWatcher(h)
	return s
}

func (s *system) Support() Support {
	if s.goos == "darwin" {
		return Support{Enforced: true}
	}
	return Support{Reason: NotEnforcedKey}
}

func (s *system) Watch(pid int, runID string, on func(Denial)) func() {
	stopHub := s.hub.add(runID, on)
	if s.goos != "darwin" {
		return stopHub
	}
	stopWatch := s.watch.watch(pid, runID)
	return func() {
		stopWatch()
		stopHub()
		s.proxy.release(runID)
	}
}

func (s *system) Release(runID string) { s.proxy.release(runID) }

func (s *system) proxyURL(runID string, agent harness.Sandboxed) (string, error) {
	s.proxy.once.Do(func() {
		p, err := StartProxy(s.hub)
		s.proxy.p.Store(p)
		s.proxy.err = err
	})
	if s.proxy.err != nil {
		return "", fmt.Errorf("agentsandbox: proxy: %w", s.proxy.err)
	}
	var allow Allowlist = func(string) bool { return false }
	if agent != nil {
		allow = HostAllowlist(agent.APIHosts(osGetenv))
	}
	return s.proxy.p.Load().Register(runID, allow), nil
}

// hub routes denials to the run that owns them.
type hub struct {
	mu   sync.Mutex
	runs map[string]*runDenials
}

type runDenials struct {
	on   func(Denial)
	seen map[[2]string]int
}

// MaxDenials bounds the distinct denials a run keeps; the consumer applies it.
const MaxDenials = 50

func newHub() *hub { return &hub{runs: map[string]*runDenials{}} }

func (h *hub) add(runID string, on func(Denial)) func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.runs[runID] = &runDenials{on: on, seen: map[[2]string]int{}}
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.runs, runID)
	}
}

// emit delivers every denial to its run. Denial.Count is how often (op, target) occurred so far,
// so the consumer keeps the first of each and still counts all. Once a run has MaxDenials distinct
// pairs, new ones are not remembered and arrive as repeats.
func (h *hub) emit(runID string, d Denial) bool {
	h.mu.Lock()
	r := h.runs[runID]
	if r == nil {
		h.mu.Unlock()
		return false
	}
	key := [2]string{d.Op, d.Target}
	if _, known := r.seen[key]; known || len(r.seen) < MaxDenials {
		r.seen[key]++
		d.Count = r.seen[key]
	} else {
		d.Count = 2 // past the cap: counts as a repeat, never kept
	}
	on := r.on
	h.mu.Unlock()
	on(d)
	return true
}

func (p *proxyOnce) release(runID string) {
	if pr := p.p.Load(); pr != nil {
		pr.release(runID)
	}
}
