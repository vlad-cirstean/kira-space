package testsupport

import (
	"strings"
	"sync"
	"testing"
)

// fixture memoizes one expensive resource (typically a real container) per test binary: started
// lazily on the first get() call and reused by every later call in the same process. Stop is
// called exactly once, from the package's own TestMain after m.Run() returns — never from an
// individual test's t.Cleanup, which Go's testing package runs the instant the registering test
// function itself returns, long before the rest of the package's tests run. That is the bug the
// findings record (CLAUDE.md): the first StartPostgres implementation wired termination to
// t.Cleanup and silently restarted a fresh container for every single test instead of reusing one,
// turning an ~8s suite into ~50s. This type exists so a fourth fixture cannot re-discover it.
type fixture[T any] struct {
	mu  sync.Mutex
	val *T
	err error
	// reset returns the shared resource to its seeded state. start calls it when a top-level test
	// runs again (`-count>1`), so an iteration never sees the previous one's writes. Nil: the
	// resource needs no reset.
	reset func(*T) error
	seen  map[string]*testing.T
}

// rerun reports whether t is a top-level test that already ran in this process under another
// *testing.T, meaning a later `-count` iteration. The same *testing.T asking twice is not one.
func (f *fixture[T]) rerun(t *testing.T) bool {
	if f.reset == nil || strings.Contains(t.Name(), "/") {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seen == nil {
		f.seen = map[string]*testing.T{}
	}
	prev, ok := f.seen[t.Name()]
	f.seen[t.Name()] = t
	return ok && prev != t
}

// get returns the memoized value, calling start() at most once. A prior failure from start() is
// remembered and returned again rather than retried, matching bun:test's own beforeAll semantics
// (a failed beforeAll fails every test in the file, not just the first).
func (f *fixture[T]) get(start func() (*T, error)) (*T, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.val != nil {
		return f.val, nil
	}
	if f.err != nil {
		return nil, f.err
	}
	val, err := start()
	if err != nil {
		f.err = err
		return nil, err
	}
	f.val = val
	return val, nil
}

// start is every package-level Start<X>'s own body (P107 T2-5): skip the test when Docker isn't
// available, memoize via get, fail the test on error. label names the failure in the t.Fatalf
// message ("postgres container").
func (f *fixture[T]) start(t *testing.T, label string, startFn func() (*T, error)) *T {
	t.Helper()
	if !IsDockerAvailable() {
		t.Skip(DockerUnavailableMessage)
	}
	val, err := f.get(startFn)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	if f.rerun(t) {
		if err := f.reset(val); err != nil {
			t.Fatalf("%s reset: %v", label, err)
		}
	}
	return val
}

// stop calls terminate on the memoized value, if one was ever started, and clears the fixture so a
// later get() (a later, independent test binary run in the same process — not expected in
// practice, but cheap to make correct) starts fresh.
func (f *fixture[T]) stop(terminate func(*T)) {
	f.mu.Lock()
	val := f.val
	f.val = nil
	f.err = nil
	f.seen = nil
	f.mu.Unlock()
	if val != nil {
		terminate(val)
	}
}
